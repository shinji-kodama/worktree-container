# タスク: Worktree Container CLI

**入力**: `/specs/001-worktree-container-cli/` の設計ドキュメント
**前提**: plan.md, spec.md, research.md, data-model.md, contracts/cli-commands.md

**テスト**: 憲法原則 VII（テストファースト）に基づき、各フェーズにユニットテストを含む。

**構成**: タスクはユーザーストーリー単位で整理し、独立した実装・テストを可能にする。

## 書式: `[ID] [P?] [Story] 説明`

- **[P]**: 並列実行可能（異なるファイル、依存なし）
- **[Story]**: 所属するユーザーストーリー（例: US1, US2, US3）
- 各タスクには正確なファイルパスを含む

---

## Phase 1: セットアップ（プロジェクト初期化）

**目的**: Go プロジェクトの初期構造を構築し、依存ライブラリをインストールする

- [ ] T001 Go モジュールを初期化する（`go mod init github.com/<org>/worktree-container`）。go.mod をリポジトリルートに作成
- [ ] T002 [P] プロジェクトのディレクトリ構造を作成する。plan.md のソースコード構造に従い、`cmd/worktree-container/`、`internal/cli/`、`internal/devcontainer/`、`internal/port/`、`internal/worktree/`、`internal/docker/`、`internal/model/` を作成
- [ ] T003 [P] 主要依存ライブラリをインストールする。`go get github.com/spf13/cobra github.com/docker/docker/client github.com/compose-spec/compose-go/v2 github.com/tidwall/jsonc github.com/stretchr/testify`
- [ ] T004 [P] MIT ライセンスファイルを作成する。`LICENSE` をリポジトリルートに配置
- [ ] T005 [P] .gitignore を作成する。Go バイナリ、テストカバレッジ、IDE 設定を除外するルールを記述
- [ ] T006 [P] GoReleaser 設定ファイル `.goreleaser.yml` を作成する。macOS/Linux/Windows の3プラットフォーム対応、Homebrew Tap 設定を含む
- [ ] T007 [P] テストデータディレクトリに4パターンのサンプル devcontainer.json を作成する。`tests/testdata/image-simple/.devcontainer/devcontainer.json`、`tests/testdata/dockerfile-build/.devcontainer/devcontainer.json`（+ Dockerfile）、`tests/testdata/compose-single/.devcontainer/devcontainer.json`（+ docker-compose.yml）、`tests/testdata/compose-multi/.devcontainer/devcontainer.json`（+ docker-compose.yml with app/db/redis）

---

## Phase 2: 基盤（全ユーザーストーリーの前提）

**目的**: すべてのユーザーストーリーが依存するコアインフラを構築する

**⚠️ 重要**: このフェーズが完了するまでユーザーストーリーの実装は開始できない

- [ ] T008 ドメインモデルの型定義を実装する。`internal/model/types.go` に WorktreeEnv, PortAllocation, DevContainerConfig, ContainerInfo, WorktreeStatus, ConfigPattern を定義。data-model.md の全フィールド・列挙型・バリデーションルールを反映。すべての型・フィールドに GoDoc コメントを付与
- [ ] T009 ドメインモデルのユニットテストを作成する。`internal/model/types_test.go` に WorktreeStatus/ConfigPattern の文字列変換、PortAllocation のバリデーション（範囲チェック、重複チェック）のテストを記述
- [ ] T010 [P] Docker クライアント初期化を実装する。`internal/docker/client.go` に Docker API クライアントの初期化・接続確認・クローズ処理を実装。macOS/Linux/Windows のソケット自動検出。Docker 未起動時の明確なエラーメッセージ
- [ ] T011 [P] Docker ラベル操作・状態検出を実装する。`internal/docker/label.go` に、`worktree.managed-by` ラベルでのコンテナフィルタリング、ラベルからの WorktreeEnv 構築、ラベルスキーマ定数定義を実装。data-model.md の Docker ラベルスキーマに準拠
- [ ] T012 [P] Docker ラベル操作のユニットテストを作成する。`internal/docker/label_test.go` にラベルパース、WorktreeEnv 構築、不正ラベルのエラーハンドリングのテストを記述
- [ ] T013 [P] ポートスキャナーを実装する。`internal/port/scanner.go` に `net.Listen()` を使用したポート使用状況の確認機能を実装。Windows Firewall エラー時のメッセージ案内を含む
- [ ] T014 [P] ポートアロケーターを実装する。`internal/port/allocator.go` にオフセットベースのポートシフトアルゴリズム（`originalPort + worktreeIndex * 10000`）、65535 超過時の動的探索、Docker ラベルからの既存割り当て収集、衝突検証フローを実装
- [ ] T015 [P] ポート管理のユニットテストを作成する。`internal/port/allocator_test.go` と `internal/port/scanner_test.go` に、ポートシフト計算、オーバーフロー処理、衝突検出のテストを記述
- [ ] T016 [P] Git ワークツリーマネージャーを実装する。`internal/worktree/manager.go` に `os/exec` で `git worktree add/list/remove` を実行する機能を実装。`filepath.Join()` で Windows 対応パスを使用
- [ ] T017 [P] Git ワークツリーマネージャーのユニットテストを作成する。`internal/worktree/manager_test.go` に一時 Git リポジトリを使ったワークツリー操作のテストを記述
- [ ] T018 devcontainer.json パターン判別・読み込みを実装する。`internal/devcontainer/config.go` に JSONC パーサー（tidwall/jsonc）を使った devcontainer.json の読み込み、パターン A/B/C/D の自動判別ロジック、ポート定義（forwardPorts, appPort, portsAttributes）の抽出を実装。plan.md のパターン判別フローに準拠
- [ ] T019 devcontainer.json パターン判別のユニットテストを作成する。`internal/devcontainer/config_test.go` に4パターンのテストデータ（tests/testdata/）を使ったパターン判別とポート抽出のテストを記述
- [ ] T020 cobra ルートコマンドとグローバルフラグを実装する。`internal/cli/root.go` に `--json`、`--verbose`、`--version` フラグ、`cmd/worktree-container/main.go` にエントリポイントを実装。contracts/cli-commands.md のグローバルフラグ仕様に準拠

**チェックポイント**: 基盤完了 — ユーザーストーリーの実装を開始できる

---

## Phase 3: ユーザーストーリー 1 — ワークツリー環境の作成 (P1) 🎯 MVP

**ゴール**: `worktree-container create <branch-name>` で新しいワークツリーと Dev Container 環境を一括作成・起動する

**独立テスト**: devcontainer.json を持つ任意のプロジェクトでコマンドを実行し、新しいワークツリーとコンテナセットが起動し、既存環境と干渉しないことを確認する

### テスト（US1）

- [ ] T021 [P] [US1] devcontainer.json 書き換えのユニットテストを作成する。`internal/devcontainer/rewrite_test.go` にパターン A/B の書き換え（name, runArgs, appPort, portsAttributes, containerEnv の変更）を検証するテストを記述
- [ ] T022 [P] [US1] Compose override YAML 生成のユニットテストを作成する。`internal/devcontainer/compose_test.go` にパターン C/D の override YAML 生成（name, ports, labels, volumes の設定）を検証するテストを記述
- [ ] T023 [P] [US1] Docker コンテナライフサイクル操作のユニットテストを作成する。`internal/docker/container_test.go` にコンテナ起動・ラベル付与のモックテストを記述

### 実装（US1）

- [ ] T024 [US1] devcontainer.json 書き換え機能を実装する（パターン A/B）。`internal/devcontainer/rewrite.go` にワークツリー側へのコピー生成、name/runArgs/appPort/portsAttributes/containerEnv の書き換え、パターン B の相対パス検証を実装。FR-012（元ファイル読み取り専用）を遵守
- [ ] T025 [US1] Compose override YAML 生成機能を実装する（パターン C/D）。`internal/devcontainer/compose.go` に compose-go/v2 を使った元 YAML 解析、override YAML（docker-compose.worktree.yml）の生成、COMPOSE_PROJECT_NAME 設定、全サービスのポートシフト・ラベル追加・ボリュームマウント変更を実装。Compose の ports 置換戦略に注意（元定義を含む完全な ports リストを生成）
- [ ] T026 [US1] Docker コンテナライフサイクル操作を実装する。`internal/docker/container.go` に Docker SDK を使ったコンテナ起動（単一コンテナ: `docker run` 相当）と Docker Compose 起動（`docker compose up -d` を os/exec で実行）、ラベル付与を実装
- [ ] T027 [US1] create サブコマンドを実装する。`internal/cli/create.go` に cobra コマンド定義、--base/--path/--name/--no-start フラグ、以下のオーケストレーション処理を実装: (1) Git ワークツリー作成、(2) devcontainer.json パターン判別、(3) 設定コピー・書き換え、(4) ポート割り当て、(5) コンテナ起動、(6) 結果出力（テキスト/JSON）。contracts/cli-commands.md の create 仕様と終了コードに準拠
- [ ] T028 [US1] create コマンドのエラーハンドリングを実装する。`internal/cli/create.go` に devcontainer.json 未検出（終了コード 2）、Docker 未起動（終了コード 3）、ポート割り当て失敗（終了コード 4）、Git エラー（終了コード 5）のハンドリングと明確なエラーメッセージ出力を追加
- [ ] T029 [US1] create コマンドの統合テストを作成する。`tests/integration/create_test.go` にテストデータの image-simple パターンを使った create コマンドのエンドツーエンドテスト（ワークツリー作成 → コンテナ起動 → ラベル確認 → クリーンアップ）を記述。Docker 必須のため `//go:build integration` タグを使用

**チェックポイント**: `worktree-container create` が単独で動作し、4パターンすべてでワークツリー環境を作成できる

---

## Phase 4: ユーザーストーリー 2 — ポートの自動管理 (P2)

**ゴール**: 複数のワークツリー環境を同時に作成しても、ポート衝突が一切発生しない

**独立テスト**: 同一プロジェクトで3つのワークツリー環境を同時に作成し、すべてのサービスがポート衝突なくアクセス可能であることを確認する

### テスト（US2）

- [ ] T030 [P] [US2] 複数環境同時稼働のポート衝突検証テストを作成する。`internal/port/allocator_test.go` に3環境分のポート割り当てを実行し、すべてのホストポートが一意であることを検証するテストを追加
- [ ] T031 [P] [US2] 外部プロセスによるポート占有時の回避テストを作成する。`internal/port/allocator_test.go` に net.Listen() でポートを事前占有し、アロケーターが衝突を回避して別ポートを割り当てることを検証するテストを追加

### 実装（US2）

- [ ] T032 [US2] 外部プロセスのポート使用状況を考慮した割り当てロジックを強化する。`internal/port/allocator.go` に既存ワークツリー環境（Docker ラベル経由）と外部プロセスの両方をチェックする衝突検証フローを実装。FR-010（ツール外プロセスの検出）を遵守
- [ ] T033 [US2] list サブコマンドのポートアクセス情報表示を実装する（list コマンド本体は US3 で実装するが、ポート情報の出力フォーマットを先に定義）。`internal/model/types.go` に PortAllocation の String() メソッド（ホスト:ポート形式の表示）を追加
- [ ] T034 [US2] 複数環境同時稼働の統合テストを作成する。`tests/integration/multi_env_test.go` に2つのワークツリー環境を順次作成し、ポート衝突がないことを確認するテストを記述（Docker 必須、`//go:build integration`）

**チェックポイント**: 最大 10 環境を同時に作成してもポート衝突が発生しない

---

## Phase 5: ユーザーストーリー 3 — ライフサイクル管理 (P3)

**ゴール**: ワークツリー環境の一覧表示・停止・再起動・削除が1コマンドで完結する

**独立テスト**: ワークツリー環境を作成・一覧表示・停止・再起動・削除し、各操作後にリソースの状態が期待通りであることを確認する

### テスト（US3）

- [ ] T035 [P] [US3] list コマンドの出力フォーマットテストを作成する。`internal/cli/list_test.go` にテキスト出力と JSON 出力のフォーマット検証テストを記述
- [ ] T036 [P] [US3] stop/start コマンドのユニットテストを作成する。`internal/docker/container_test.go` にコンテナ停止・再起動のモックテストを追加
- [ ] T037 [P] [US3] remove コマンドのクリーンアップテストを作成する。`internal/docker/container_test.go` にコンテナ・ネットワーク・ボリューム削除のモックテストを追加

### 実装（US3）

- [ ] T038 [US3] list サブコマンドを実装する。`internal/cli/list.go` に cobra コマンド定義、--status フィルタフラグ、Docker ラベルからの環境一覧取得、テキスト/JSON 出力フォーマット（contracts/cli-commands.md の list 仕様に準拠）、孤立環境（orphaned）の検出を実装
- [ ] T039 [US3] stop サブコマンドを実装する。`internal/cli/stop.go` に cobra コマンド定義、Docker API を使った指定環境の全コンテナ停止、Compose 環境の場合は `docker compose stop` を実行、終了コード仕様に準拠
- [ ] T040 [US3] start サブコマンドを実装する。`internal/cli/start.go` に cobra コマンド定義、停止中コンテナの再起動、ポート再検証（以前のポートが占有されている場合のエラー処理）、Compose 環境の場合は `docker compose start` を実行
- [ ] T041 [US3] remove サブコマンドを実装する。`internal/cli/remove.go` に cobra コマンド定義、--force/--keep-worktree フラグ、対話的確認プロンプト、コンテナ・ネットワーク・ボリュームの削除、Git ワークツリーの削除（ユーザー確認後）、FR-006（リソースクリーンアップ）を遵守
- [ ] T042 [US3] ライフサイクル管理の統合テストを作成する。`tests/integration/lifecycle_test.go` に create → list → stop → start → remove のフルライフサイクルテストを記述（Docker 必須、`//go:build integration`）

**チェックポイント**: 全ライフサイクル操作（list/stop/start/remove）が単独で動作し、リソースリーク率 0%

---

## Phase 6: ユーザーストーリー 4 — 複数 Dev Container ツール対応 (P4)

**ゴール**: VS Code、Dev Container CLI、DevPod のいずれからでもワークツリー環境に接続できる

**独立テスト**: 同一ワークツリー環境に対して VS Code、Dev Container CLI、DevPod のそれぞれで接続できることを確認する

### 実装（US4）

- [ ] T043 [US4] devcontainer.json 書き換え時に Dev Container CLI / VS Code / DevPod 互換性を検証・確保する。`internal/devcontainer/rewrite.go` に、生成した設定が devcontainer 仕様に準拠していること（不要なカスタムフィールドを含まない、変数展開 `${localWorkspaceFolder}` が正しく機能すること）を確認するバリデーション関数を追加
- [ ] T044 [US4] DevPod 固有の対応を実装する。`internal/devcontainer/rewrite.go` に DevPod の `--devcontainer-path` オプション用のパス情報を出力に含める処理を追加。create コマンドの出力に DevPod 接続コマンド例を追加
- [ ] T045 [US4] ツール互換性の手動テスト手順書を作成する。`tests/manual/tool-compatibility.md` に VS Code、Dev Container CLI（`devcontainer up --workspace-folder`）、DevPod（`devpod up`）での接続確認手順を記述

**チェックポイント**: 3つのツールすべてから作成済みワークツリー環境に接続できる

---

## Phase 7: ポリッシュ & 横断的関心事

**目的**: 複数のユーザーストーリーにまたがる改善

- [ ] T046 [P] README.md を作成する。プロジェクト概要、インストール方法（`brew install` / `winget install`）、基本的な使い方（create/list/stop/start/remove の例）、対応 devcontainer.json パターンの説明、コントリビューション方法へのリンクを日本語で記述
- [ ] T047 [P] CONTRIBUTING.md を作成する。開発環境セットアップ手順（Go インストール、リポジトリクローン、ビルド・テスト方法）、コーディング規約（コメント規約、Go イディオムの解説コメント必須）、PR ワークフロー、Conventional Commits 規約を日本語で記述
- [ ] T048 [P] CI/CD パイプラインを設定する。`.github/workflows/ci.yml` に Go ビルド、ユニットテスト（`go test ./internal/...`）、golangci-lint、3プラットフォームのクロスコンパイル確認を設定
- [ ] T049 [P] リリースワークフローを設定する。`.github/workflows/release.yml` に GoReleaser によるタグベースリリース（バイナリ生成 + Homebrew Tap 更新 + GitHub Release）を設定
- [ ] T050 golangci-lint 設定を作成する。`.golangci.yml` にリンタールール（gofmt, govet, errcheck, staticcheck 等）を設定
- [ ] T051 全パッケージのコメントとドキュメントを最終レビューする。各パッケージの doc.go（パッケージレベルコメント）を追加し、GoDoc の出力品質を確認。憲法原則 X（詳細コードコメント）への準拠を検証
- [ ] T052 quickstart.md のシナリオに沿って手動検証を実施する。`specs/001-worktree-container-cli/quickstart.md` の全操作を実際に実行し、期待通りに動作することを確認

---

## 依存関係 & 実行順序

### フェーズ依存関係

- **セットアップ（Phase 1）**: 依存なし — 即時開始可能
- **基盤（Phase 2）**: セットアップ完了後 — **全ユーザーストーリーをブロック**
- **ユーザーストーリー（Phase 3-6）**: 基盤完了後に開始可能
  - US1（P1）と US2（P2）は順次実行（US2 は US1 の create コマンドに依存）
  - US3（P3）は US1 完了後に開始可能（list/stop/start/remove は create の存在が前提）
  - US4（P4）は US1 完了後に開始可能
- **ポリッシュ（Phase 7）**: 全ユーザーストーリー完了後

### ユーザーストーリー依存関係

- **US1 (P1)**: 基盤（Phase 2）完了後に開始可能。他ストーリーへの依存なし
- **US2 (P2)**: US1 完了後に開始。create コマンドのポート割り当てロジックを強化
- **US3 (P3)**: US1 完了後に開始。create で作成した環境の管理操作を追加
- **US4 (P4)**: US1 完了後に開始。create で生成した設定のツール互換性を検証

### 各ユーザーストーリー内の順序

- テスト → モデル → サービス → コマンド → 統合テスト
- テストは実装前に作成し、失敗することを確認
- ストーリー完了後に次の優先度へ移行

### 並列実行可能なタスク

- Phase 1: T002, T003, T004, T005, T006, T007 はすべて並列可能
- Phase 2: T010, T011, T013, T014, T016 は並列可能（T008 完了後）
- Phase 3: T021, T022, T023 は並列可能
- Phase 5: T035, T036, T037 は並列可能
- Phase 7: T046, T047, T048, T049 は並列可能

---

## 並列実行例: ユーザーストーリー 1

```bash
# US1 のテストを並列起動:
Task: "T021 devcontainer.json 書き換えのユニットテスト (internal/devcontainer/rewrite_test.go)"
Task: "T022 Compose override YAML 生成のユニットテスト (internal/devcontainer/compose_test.go)"
Task: "T023 Docker コンテナライフサイクル操作のユニットテスト (internal/docker/container_test.go)"

# US1 の実装を順次実行（T024 → T025 は並列可能、T027 は両方に依存）:
Task: "T024 devcontainer.json 書き換え (internal/devcontainer/rewrite.go)"
Task: "T025 Compose override YAML 生成 (internal/devcontainer/compose.go)"
# ↓ 上記2つ完了後
Task: "T027 create サブコマンド (internal/cli/create.go)"
```

---

## 実装戦略

### MVP ファースト（ユーザーストーリー 1 のみ）

1. Phase 1 完了: セットアップ
2. Phase 2 完了: 基盤（重要 — 全ストーリーをブロック）
3. Phase 3 完了: ユーザーストーリー 1
4. **停止して検証**: create コマンドを独立テスト
5. 動作確認できたらデモ / 初期リリース

### インクリメンタルデリバリー

1. セットアップ + 基盤 → 基盤完了
2. US1 追加 → 独立テスト → リリース v0.1.0（MVP！）
3. US2 追加 → 独立テスト → リリース v0.2.0
4. US3 追加 → 独立テスト → リリース v0.3.0
5. US4 追加 → 独立テスト → リリース v0.4.0
6. ポリッシュ → リリース v1.0.0

### AI エージェント戦略

Claude Code 等の AI エージェントによる実装:

1. Phase 1 + Phase 2 を順次完了
2. Phase 3（US1）を完了 → 手動検証
3. Phase 4-6 を順次完了
4. Phase 7 で品質仕上げ

---

## 注意事項

- [P] タスク = 異なるファイル、依存なし
- [Story] ラベルでタスクを対応するユーザーストーリーに紐づけ
- 各ユーザーストーリーは独立して完了・テスト可能であるべき
- テストは実装前に作成し、失敗を確認してから実装
- 各タスクまたは論理グループの完了後にコミット
- すべてのコードに詳細なコメントを付与（憲法原則 X）
- ファイルパスは `filepath.Join()` を使用し、`/` のハードコードを避ける（Windows 対応）
