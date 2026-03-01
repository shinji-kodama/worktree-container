# 実装計画: 003-optional-devcontainer（32タスク）

## Context

devcontainer.json が存在しないリポジトリでも `create` コマンドで Git worktree を作成可能にする。マーカーファイル（`.worktree-container`）による管理対象の識別を導入し、Docker ラベルとのデュアルソース方式で `list` コマンドを拡張する。既存の devcontainer あり環境の動作は完全に維持する。

タスクは `specs/003-optional-devcontainer/tasks.md` に定義された 32 タスク / 7 フェーズに従う。

---

## Phase 1: Setup（T001-T003）— 全並列

### T001: テストデータ作成
- `tests/testdata/no-devcontainer/` ディレクトリを作成
- 中に `README.md` のみ配置（`.devcontainer/` なし）
- FindDevContainerJSON テストのフィクスチャとして使用

### T002: PatternNone 定数追加
- **ファイル**: `internal/model/types.go`
- L94 の後に `PatternNone ConfigPattern = "none"` を追加
- `IsValid()` (L106) の switch に `PatternNone` を追加
- `ParseConfigPattern()` (L122-128) のエラーメッセージに `none` を追加
- `IsCompose()` は変更不要（none は Compose ではない）

### T003: StatusNoContainer 定数追加
- **ファイル**: `internal/model/types.go`
- L37 の後に `StatusNoContainer WorktreeStatus = "no-container"` を追加
- `IsValid()` (L51) の switch に `StatusNoContainer` を追加
- `ParseWorktreeStatus()` (L60-66) のエラーメッセージに `no-container` を追加

---

## Phase 2: Foundational（T004-T009）— 全ストーリーの前提

### T004: MarkerFile 構造体定義
- **ファイル**: `internal/worktree/manager.go`
- ファイル名定数: `const MarkerFileName = ".worktree-container"`
- 構造体:
  ```go
  type MarkerFile struct {
      ManagedBy      string `json:"managedBy"`
      Name           string `json:"name"`
      Branch         string `json:"branch"`
      SourceRepoPath string `json:"sourceRepoPath"`
      ConfigPattern  string `json:"configPattern"`
      CreatedAt      string `json:"createdAt"`
  }
  ```

### T005: WriteMarkerFile 実装
- **ファイル**: `internal/worktree/manager.go`
- シグネチャ: `func WriteMarkerFile(worktreePath string, marker MarkerFile) error`
- パス: `filepath.Join(worktreePath, MarkerFileName)`
- JSON エンコード + `os.WriteFile(path, data, 0644)`

### T006: ReadMarkerFile 実装
- **ファイル**: `internal/worktree/manager.go`
- シグネチャ: `func ReadMarkerFile(worktreePath string) (*MarkerFile, error)`
- ファイルが存在しない場合は `nil, nil` を返す
- JSON デコード

### T007: マーカーファイルのユニットテスト
- **ファイル**: `internal/worktree/manager_test.go`
- テストケース:
  1. Write → Read → フィールド検証
  2. 存在しないパスで Read → nil, nil
  3. 不正な JSON の Read → error

### T008: FindDevContainerJSON オプショナル化
- **ファイル**: `internal/devcontainer/config.go` L404-426
- **変更**: L422-425 の `return "", model.NewCLIError(...)` を `return "", nil` に変更
- 呼び出し元（create.go L139-142）は空文字列チェックで分岐する

### T009: FindDevContainerJSON テスト更新
- **ファイル**: `internal/devcontainer/config_test.go`
- 既存の "not found" テストケースを修正:
  - `assert.Error(t, err)` → `assert.NoError(t, err)`
  - `assert.Empty(t, path)` を追加
- T001 で作成した `no-devcontainer` フィクスチャも使用

---

## Phase 3: US1 — devcontainer なし create（T010-T013）MVP

### T010: devcontainer なし create テスト
- **ファイル**: `internal/cli/create_test.go`（新規作成）
- テスト内容: setupTestRepo() で git リポジトリ作成（devcontainer なし）→ runCreate 実行 → 以下を検証:
  1. ワークツリーが作成されている
  2. マーカーファイルが存在し configPattern="none"
  3. Docker 関連の処理が呼ばれていない（Docker 不要のためエラーにならない）
- **注意**: runCreate を直接テストするか、cobra コマンド経由でテストするかは既存テストパターンに合わせる。現状 create_test.go は存在しないため、list_test.go のパターン（純粋な関数テスト）に合わせる

### T011: create.go 処理順序変更
- **ファイル**: `internal/cli/create.go`
- **改修箇所**: runCreate() L88-287
- **変更内容**:
  1. Step 4（L129-134: worktree作成）の直後に、マーカーファイル初期配置を追加:
     ```go
     // Step 4.5: Place marker file with initial configPattern=none.
     marker := worktree.MarkerFile{
         ManagedBy:      "worktree-container",
         Name:           envName,
         Branch:         branchName,
         SourceRepoPath: repoRoot,
         ConfigPattern:  model.PatternNone.String(),
         CreatedAt:      time.Now().UTC().Format(time.RFC3339),
     }
     if err := worktree.WriteMarkerFile(worktreePath, marker); err != nil {
         return model.WrapCLIError(model.ExitGeneralError, "failed to write marker file", err)
     }
     ```
  2. Step 5（L136-157: FindDevContainerJSON）の err チェックを空文字列チェックに変更:
     ```go
     devcontainerPath, err := devcontainer.FindDevContainerJSON(repoRoot)
     if err != nil {
         return err
     }
     if devcontainerPath == "" {
         // No devcontainer.json found — output worktree-only result and return.
         env := &model.WorktreeEnv{...status: StatusNoContainer, configPattern: PatternNone...}
         printCreateResult(env)
         return nil
     }
     ```
  3. L158 以降の全コンテナ処理は `devcontainerPath != ""` の場合のみ実行される（既存コードをインデントなしでそのまま維持）

### T012: devcontainer 検出結果による分岐
- **ファイル**: `internal/cli/create.go`
- T011 で追加した `if devcontainerPath == ""` ブロック内に:
  - WorktreeEnv を作成（Status=StatusNoContainer, ConfigPattern=PatternNone, PortAllocations=nil）
  - printCreateResult() を呼び出して return nil

### T013: devcontainer なし時の出力フォーマット
- **ファイル**: `internal/cli/create.go`
- printCreateResultText() を修正（L475-497）:
  - `env.ConfigPattern == PatternNone` の場合、Pattern 行と Services セクションを表示しない
  - 出力: `Created worktree environment "<name>"\n  Branch:    <branch>\n  Path:      <path>`
- printCreateResultJSON() は変更不要（既に ConfigPattern と Services を動的に出力）
  - PatternNone の場合: `"configPattern": "none"`, `"services": []`

---

## Phase 4: US2 — 後方互換性（T014-T016）

### T014: 既存 create テストにマーカーファイル検証追加
- **ファイル**: `internal/cli/create_test.go`
- devcontainer ありの create テスト末尾に:
  - マーカーファイル存在確認
  - configPattern が実際のパターン値（image, compose-multi 等）であることを検証

### T015: devcontainer あり時のマーカーファイル更新
- **ファイル**: `internal/cli/create.go`
- Pattern 検出（L166）の後に、マーカーファイルの configPattern を更新:
  ```go
  marker.ConfigPattern = pattern.String()
  if err := worktree.WriteMarkerFile(worktreePath, marker); err != nil {
      return model.WrapCLIError(model.ExitGeneralError, "failed to update marker file", err)
  }
  ```

### T016: リグレッションテスト
- `go test ./internal/... -count=1 -race` を実行して全テストパス確認

---

## Phase 5: US3 — list/start/stop/remove 対応（T017-T026）

### T017: list デュアルソーステスト
- **ファイル**: `internal/cli/list_test.go`
- テスト: マーカーファイルのみの環境が一覧表示される、Docker 環境とのマージ検証

### T018: start/stop ConfigPattern=none テスト
- **ファイル**: `internal/cli/start_test.go`（新規）、`internal/cli/stop_test.go`（新規）
- テスト: PatternNone の環境に対する start/stop が適切なメッセージを出力し、exit 0

### T019: remove devcontainer なし環境テスト
- **ファイル**: `internal/cli/remove_test.go`（新規）
- テスト: Docker 操作スキップ、worktree 削除のみ

### T020: ListWorktrees 関数
- **ファイル**: `internal/worktree/manager.go`
- シグネチャ: `func ListWorktrees(repoPath string) ([]string, error)`
- 実装: `git worktree list --porcelain` の出力から各ワークツリーのパスを抽出
- **再利用**: 既存の `(m *Manager) List()` が `[]WorktreeInfo` を返すので、これを活用してパスだけ抽出する関数を作成

### T021: list デュアルソース検出
- **ファイル**: `internal/cli/list.go` runList() L71-135
- **変更**:
  1. まず `git worktree list` + マーカーファイルで環境マップ作成
  2. Docker 接続（失敗してもエラーにしない）
  3. Docker ラベルで環境マップ作成
  4. マージ: Docker 優先、マーカーのみは no-container
  5. ソート → フィルター → 出力

### T022: Docker 接続失敗時フォールバック
- **ファイル**: `internal/cli/list.go`
- Docker 未起動時もマーカー環境を表示
- `docker.NewClient()` のエラーを致命的エラーではなく警告として処理

### T023: findEnvironment マーカーファイル対応
- **ファイル**: `internal/cli/stop.go` findEnvironment() L129-159
- Docker で見つからない場合、repoRoot 特定 → git worktree list → マーカーファイル検索
- ConfigPattern=none の環境を返却可能にする

### T024: start ConfigPattern=none ハンドリング
- **ファイル**: `internal/cli/start.go` runStart() L57-120
- Step 2（findEnvironment）の後、ConfigPattern チェック追加:
  ```go
  if env.ConfigPattern == model.PatternNone {
      fmt.Printf("Environment %q has no container configuration.\n", envName)
      fmt.Println("To add containers, create a .devcontainer/devcontainer.json in the worktree.")
      return nil
  }
  ```

### T025: stop ConfigPattern=none ハンドリング
- **ファイル**: `internal/cli/stop.go` runStop() L54-100
- findEnvironment の後に:
  ```go
  if env.ConfigPattern == model.PatternNone {
      fmt.Printf("Environment %q has no container configuration. Nothing to stop.\n", envName)
      return nil
  }
  ```

### T026: remove devcontainer なし環境対応
- **ファイル**: `internal/cli/remove.go` runRemove() L80-160
- findEnvironment の後、Docker 操作（L109-131）を ConfigPattern != PatternNone の場合のみ実行
- worktree 削除（L133-155）は従来通り実行

---

## Phase 6: US4 — 後から devcontainer 追加（T027-T028）

### T027: 後から devcontainer 追加テスト
- **ファイル**: `internal/cli/start_test.go`
- テスト: マーカーファイル configPattern=none → ワークツリーに devcontainer.json 配置 → start → コンテナ起動 → マーカー更新

### T028: start での devcontainer 再検出
- **ファイル**: `internal/cli/start.go`
- T024 の `if env.ConfigPattern == model.PatternNone` ブロック内で:
  1. `devcontainer.FindDevContainerJSON(env.WorktreePath)` を実行
  2. 見つかった場合: Pattern 検出 → ポート割当 → コンテナ起動 → マーカーファイル更新
  3. 見つからない場合: 従来通り「no container configuration」メッセージ

---

## Phase 7: Polish（T029-T032）

### T029: リグレッションテスト
- `go test ./internal/... -count=1 -race -coverprofile=coverage.out`

### T030: golangci-lint
- `golangci-lint run` → 警告修正

### T031: GoDoc コメント
- 新規コード全体に詳細な GoDoc コメント付与（憲法 X 準拠）

### T032: quickstart.md 手動検証
- quickstart.md の各ステップの期待出力との一致を確認

---

## 重要な実装上の注意点

### create.go の改修フロー（T011-T013）
```
改修後:
  RepoRoot → EnvName → WorktreePath → Worktree作成
  → マーカーファイル配置（ConfigPattern=none で仮作成）
  → devcontainer検索（FindDevContainerJSON → 空文字 or パス）
  ├─ 空文字 → WorktreeEnv(StatusNoContainer) → 出力 → return
  └─ パス   → LoadConfig → DetectPattern → ポート割当 → コピー → 起動
              → マーカーファイル更新（ConfigPattern=実パターン）→ 出力
```

### list.go の改修フロー（T021-T022）
```
改修後:
  1. repoRoot 取得 → git worktree list → 各パスでマーカーファイル検索 → マーカー環境マップ
  2. Docker 接続（失敗OK）→ ListManagedContainers → GroupByEnv → Docker 環境マップ
  3. マージ: Docker 優先、マーカーのみ = no-container
  4. ソート → フィルター → 出力
```

### findEnvironment 拡張（T023）
```
改修後:
  1. Docker で検索（従来通り）
  2. Docker で見つからない場合:
     repoRoot 取得 → git worktree list → マーカーファイル検索
     → マーカーから WorktreeEnv を構築（containers = []）
  3. どちらでも見つからない → ExitEnvNotFound
```

---

## 検証方法

### 各フェーズのチェックポイント
1. **Phase 2 完了後**: `go test ./internal/model/... ./internal/worktree/... ./internal/devcontainer/... -count=1 -race`
2. **Phase 3 完了後**: devcontainer なしリポジトリで `create` が動作 + 既存テスト全パス
3. **Phase 4 完了後**: `go test ./internal/... -count=1 -race`（全テスト）
4. **Phase 5 完了後**: list/start/stop/remove が no-container 環境に対応
5. **Phase 7 完了後**: `go test ./internal/... -count=1 -race -coverprofile=coverage.out` + `golangci-lint run`

### E2E 検証（Phase 7: T032）
```bash
cd /path/to/repo-without-devcontainer
worktree-container create feature-test
# → Created worktree environment "feature-test" / Branch / Path のみ
worktree-container list
# → feature-test  no-container  0  -
worktree-container start feature-test
# → "has no container configuration" メッセージ
worktree-container remove --force feature-test
# → Removed worktree environment "feature-test"
```
