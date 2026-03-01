# Tasks: devcontainer オプショナル化

**Input**: Design documents from `/specs/003-optional-devcontainer/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: テストファースト原則（憲法 VII）に基づき、テストタスクを含む。

**Organization**: タスクはユーザーストーリーごとにグループ化。各ストーリーは独立して実装・テスト可能。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 別ファイルへの変更で依存なし、並列実行可能
- **[Story]**: 対応するユーザーストーリー（US1, US2, US3, US4）

---

## Phase 1: Setup

**Purpose**: テストデータとデータモデルの準備

- [ ] T001 [P] devcontainer なしのテスト用ディレクトリを作成 `tests/testdata/no-devcontainer/`（Git リポジトリとして初期化、devcontainer.json なし）
- [ ] T002 [P] `PatternNone` 定数を `ConfigPattern` 型に追加 `internal/model/types.go`（`const PatternNone ConfigPattern = "none"`）
- [ ] T003 [P] `StatusNoContainer` 定数を `WorktreeStatus` 型に追加 `internal/model/types.go`（`const StatusNoContainer WorktreeStatus = "no-container"`）

---

## Phase 2: Foundational（全ストーリーの前提）

**Purpose**: マーカーファイルと FindDevContainerJSON のオプショナル化。全ユーザーストーリーが依存。

**⚠️ CRITICAL**: このフェーズが完了するまでユーザーストーリーの実装は開始できない

- [ ] T004 マーカーファイル構造体 `MarkerFile` とファイル名定数を定義 `internal/worktree/manager.go`（data-model.md のスキーマに準拠: managedBy, name, branch, sourceRepoPath, configPattern, createdAt）
- [ ] T005 マーカーファイル書き込み関数 `WriteMarkerFile(worktreePath string, marker MarkerFile) error` を実装 `internal/worktree/manager.go`（JSON でファイルパス `<worktreePath>/.worktree-container` に書き込み）
- [ ] T006 マーカーファイル読み取り関数 `ReadMarkerFile(worktreePath string) (*MarkerFile, error)` を実装 `internal/worktree/manager.go`（ファイルが存在しない場合は nil, nil を返す）
- [ ] T007 マーカーファイル読み書きのユニットテストを作成 `internal/worktree/manager_test.go`（書き込み→読み取り→検証、存在しないファイル読み取りで nil 返却）
- [ ] T008 `FindDevContainerJSON` の戻り値を変更 `internal/devcontainer/config.go`（見つからない場合に error ではなく `("", nil)` を返却。呼び出し元で空文字列チェックで分岐）
- [ ] T009 `FindDevContainerJSON` の変更に対するユニットテストを更新 `internal/devcontainer/config_test.go`（devcontainer.json が見つからない場合のテストを error 期待から nil 期待に変更）

**Checkpoint**: マーカーファイル読み書き可能、FindDevContainerJSON がオプショナル化完了

---

## Phase 3: User Story 1 - devcontainer なしのリポジトリでワークツリーを作成する (Priority: P1) 🎯 MVP

**Goal**: devcontainer.json のないリポジトリで `create` コマンドが正常にワークツリーを作成する

**Independent Test**: `tests/testdata/no-devcontainer/` で `create` を実行し、worktree 作成 + マーカーファイル配置を確認

### Tests for User Story 1

- [ ] T010 [P] [US1] devcontainer なしリポジトリでの create テストを作成 `internal/cli/create_test.go`（worktree 作成成功、マーカーファイル配置確認、Docker 呼び出しなし）

### Implementation for User Story 1

- [ ] T011 [US1] `create` コマンドの処理順序を変更 `internal/cli/create.go`（Docker 接続を devcontainer.json 検出の後に移動。改修後フロー: RepoRoot → EnvName → WorktreePath → Worktree作成 → マーカーファイル配置 → devcontainer検索 → 分岐）
- [ ] T012 [US1] devcontainer.json 検出結果による分岐を実装 `internal/cli/create.go`（`FindDevContainerJSON` が空文字列を返した場合: コンテナ関連処理をスキップ、成功メッセージで Branch と Path のみ表示。見つかった場合: 従来フローを継続しマーカーファイルの configPattern を更新）
- [ ] T013 [US1] devcontainer なし時の create 出力フォーマットを実装 `internal/cli/create.go`（テキスト: `Created worktree environment "<name>"\n  Branch:    <branch>\n  Path:      <path>`。JSON: `{"name":"...","branch":"...","worktreePath":"...","status":"no-container","configPattern":"none","services":[]}`)

**Checkpoint**: devcontainer なしリポジトリで `create` が正常動作。既存テストもパス（後方互換性）

---

## Phase 4: User Story 2 - devcontainer ありのリポジトリで従来通り動作する (Priority: P1)

**Goal**: devcontainer.json のあるリポジトリで既存の動作が完全に維持される

**Independent Test**: 既存の全テストケースがパスすること + マーカーファイルが追加配置されること

### Tests for User Story 2

- [ ] T014 [P] [US2] devcontainer ありリポジトリでの create テストにマーカーファイル検証を追加 `internal/cli/create_test.go`（既存テストの末尾にマーカーファイル存在確認と configPattern 値の検証を追加）

### Implementation for User Story 2

- [ ] T015 [US2] devcontainer あり時に create コマンドでマーカーファイルの configPattern を更新する処理を実装 `internal/cli/create.go`（Pattern 検出後にマーカーファイルの configPattern フィールドを実際のパターン値に上書き）
- [ ] T016 [US2] 既存の全テストを実行して後方互換性を検証（`go test ./internal/... -count=1 -race`）

**Checkpoint**: 既存テスト全パス + マーカーファイル追加

---

## Phase 5: User Story 3 - devcontainer なしのワークツリーを一覧・管理する (Priority: P2)

**Goal**: `list` でマーカーファイルベースの環境を表示、`remove` で削除、`start`/`stop` で適切なメッセージ

**Independent Test**: devcontainer なし環境を create → list で表示確認 → start で「コンテナなし」メッセージ → remove で削除

### Tests for User Story 3

- [ ] T017 [P] [US3] list コマンドのデュアルソース検出テストを作成 `internal/cli/list_test.go`（マーカーファイルのみの環境が一覧に表示、Docker ラベル環境とのマージ、status="no-container" / configPattern="none" 確認）
- [ ] T018 [P] [US3] start/stop コマンドの ConfigPattern=none ハンドリングテストを作成 `internal/cli/start_test.go`、`internal/cli/stop_test.go`（「コンテナがありません」メッセージ表示、終了コード 0）
- [ ] T019 [P] [US3] remove コマンドの devcontainer なし環境削除テストを作成 `internal/cli/remove_test.go`（Docker コンテナ削除スキップ、worktree 削除成功確認）

### Implementation for User Story 3

- [ ] T020 [US3] `git worktree list` の出力をパースしてワークツリーパス一覧を取得する関数を実装 `internal/worktree/manager.go`（`ListWorktrees(repoPath string) ([]string, error)`）
- [ ] T021 [US3] list コマンドにデュアルソース検出ロジックを実装 `internal/cli/list.go`（1. git worktree list + マーカーファイルで環境マップ作成 → 2. Docker ラベルで環境マップ作成 → 3. マージルール適用: Docker 優先、マーカーのみは no-container）
- [ ] T022 [US3] list コマンドで Docker 接続失敗時もマーカー環境を表示するフォールバックを実装 `internal/cli/list.go`（Docker 未起動でもマーカーファイルベースの環境一覧を表示）
- [ ] T023 [US3] `findEnvironment` 関数をマーカーファイル対応に拡張 `internal/cli/stop.go`（Docker ラベルで見つからない場合、マーカーファイルから検索。ConfigPattern=none の環境を返却可能にする）
- [ ] T024 [US3] start コマンドに ConfigPattern=none のハンドリングを追加 `internal/cli/start.go`（`env.ConfigPattern == PatternNone` の場合、「Environment "<name>" has no container configuration.」メッセージ表示、終了コード 0）
- [ ] T025 [US3] stop コマンドに ConfigPattern=none のハンドリングを追加 `internal/cli/stop.go`（`env.ConfigPattern == PatternNone` の場合、「Environment "<name>" has no container configuration. Nothing to stop.」メッセージ表示、終了コード 0）
- [ ] T026 [US3] remove コマンドで devcontainer なし環境の削除をサポート `internal/cli/remove.go`（ConfigPattern=none の場合: Docker コンテナ削除をスキップ、worktree 削除のみ実行）

**Checkpoint**: devcontainer なし環境が list で表示、start/stop で適切メッセージ、remove で削除可能

---

## Phase 6: User Story 4 - ワークツリー作成後に devcontainer を追加する (Priority: P3)

**Goal**: devcontainer なしのワークツリーに後から devcontainer.json を追加し、`start` で Dev Container が起動する

**Independent Test**: devcontainer なし環境を create → ワークツリー内に devcontainer.json を配置 → `start` でコンテナ起動

### Tests for User Story 4

- [ ] T027 [P] [US4] 後から devcontainer を追加した場合の start テストを作成 `internal/cli/start_test.go`（マーカーファイルで ConfigPattern=none → start 時に devcontainer.json を検出 → コンテナ起動 → マーカーファイルの configPattern 更新）

### Implementation for User Story 4

- [ ] T028 [US4] start コマンドで ConfigPattern=none の環境に対し devcontainer.json の再検出を実装 `internal/cli/start.go`（ConfigPattern=none の場合、ワークツリーパスで devcontainer.json を検索。見つかった場合は Pattern 検出 → ポート割当 → コンテナ起動 → マーカーファイル更新）

**Checkpoint**: 後から追加した devcontainer で start が動作

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 全ストーリーにまたがる品質向上

- [ ] T029 既存テスト全体のリグレッションテストを実行し結果を確認（`go test ./internal/... -count=1 -race -coverprofile=coverage.out`）
- [ ] T030 [P] golangci-lint を実行しすべての警告を修正（`golangci-lint run`）
- [ ] T031 [P] 新規コードに詳細な GoDoc コメントと Go イディオム解説コメントを付与（憲法 X 準拠の確認）
- [ ] T032 quickstart.md の手順を手動実行して動作を検証

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: 依存なし — 即座に開始可能
- **Phase 2 (Foundational)**: Phase 1 完了後 — 全ユーザーストーリーをブロック
- **Phase 3 (US1)**: Phase 2 完了後
- **Phase 4 (US2)**: Phase 2 完了後（US1 と並列可能だが、create.go を共有するため順次推奨）
- **Phase 5 (US3)**: Phase 3, 4 完了後（create で作成した環境を list/start/stop/remove する）
- **Phase 6 (US4)**: Phase 5 完了後（start の ConfigPattern=none ハンドリングに依存）
- **Phase 7 (Polish)**: 全ストーリー完了後

### User Story Dependencies

- **US1 (P1)**: Phase 2 完了後に開始可能。他ストーリーへの依存なし
- **US2 (P1)**: Phase 2 完了後に開始可能。US1 の create.go 変更に依存（順次実行推奨）
- **US3 (P2)**: US1, US2 完了後に開始可能（create で環境が作れることが前提）
- **US4 (P3)**: US3 完了後に開始可能（start の no-container ハンドリングに依存）

### Within Each User Story

- テストを先に書き、FAIL を確認してから実装
- モデル → サービス → コマンドの順
- コア実装 → 統合の順

### Parallel Opportunities

- Phase 1: T001, T002, T003 は全て並列実行可能
- Phase 2: T004 → T005, T006 は順次（型定義 → 読み書き）。T008, T009 は T004-T006 と並列可能
- Phase 5: T017, T018, T019 のテスト作成は並列可能

---

## Parallel Example: Phase 1

```bash
# 全 Setup タスクを並列で実行:
Task T001: "テスト用ディレクトリ tests/testdata/no-devcontainer/ 作成"
Task T002: "PatternNone 定数を model/types.go に追加"
Task T003: "StatusNoContainer 定数を model/types.go に追加"
```

## Parallel Example: User Story 3 Tests

```bash
# US3 のテストを並列で作成:
Task T017: "list コマンドのデュアルソース検出テスト"
Task T018: "start/stop の ConfigPattern=none テスト"
Task T019: "remove の devcontainer なし環境削除テスト"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup（T001-T003）
2. Phase 2: Foundational（T004-T009）
3. Phase 3: User Story 1（T010-T013）
4. **STOP and VALIDATE**: devcontainer なしリポジトリで create が動作。既存テスト全パス
5. この時点で最小限の価値を提供可能

### Incremental Delivery

1. Setup + Foundational → 基盤完成
2. US1 → devcontainer なし create が動作 → **MVP!**
3. US2 → 後方互換性の完全保証
4. US3 → list/start/stop/remove の対応 → フル機能
5. US4 → 後からの devcontainer 追加 → 完全版

---

## Notes

- T002, T003 は同じファイル `model/types.go` だが追加箇所が異なるため並列可能
- T011, T012, T013 は同じファイル `create.go` への変更のため順次実行
- 既存テストの後方互換性確認（T016）は US2 のチェックポイントとして重要
- マーカーファイル名 `.worktree-container` のリネームは本フィーチャーのスコープ外（別ブランチで対応予定）
