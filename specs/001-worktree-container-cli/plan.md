# 実装計画: Worktree Container CLI

**ブランチ**: `001-worktree-container-cli` | **日付**: 2026-02-28 | **仕様**: [spec.md](./spec.md)
**入力**: `/specs/001-worktree-container-cli/spec.md` の機能仕様

## サマリー

Git ワークツリーごとに独立した Dev Container 環境を自動構築し、ポート衝突を完全に
排除する CLI ツールを Go 言語で実装する。devcontainer.json の4つの設定パターン
（image 指定、Dockerfile ビルド、Compose 単一サービス、Compose 複数サービス）を
すべてサポートし、ポートシフト方式によりホスト側ポートの自動割り当てを行う。
環境の状態管理は Docker コンテナラベルに基づく動的検出で実現し、外部状態ファイルは
使用しない。Homebrew Tap + GoReleaser で配布する。
オープンソースとして公開し、プロジェクトオーナーが Go 言語に不慣れであるため、
すべてのソースコードに詳細なコメントを付与する。

## 技術コンテキスト

**言語/バージョン**: Go >= 1.22
**主要依存ライブラリ**:
- `github.com/spf13/cobra` — CLI フレームワーク
- `github.com/docker/docker/client` — Docker API クライアント
- `github.com/compose-spec/compose-go/v2` — Docker Compose 仕様パーサー
- `github.com/tidwall/jsonc` — JSONC パーサー（devcontainer.json のコメント対応）

**ストレージ**: なし（Docker コンテナラベルによる動的状態管理）
**テスト**: `go test` + `github.com/stretchr/testify`
**対象プラットフォーム**: macOS、Linux、Windows
**プロジェクト種別**: CLI ツール
**パフォーマンス目標**: ワークツリー環境作成は Docker イメージ取得/ビルド時間を除き 10 秒以内
**制約**: 最大 10 環境の同時稼働、ポート衝突ゼロの保証
**スケール**: 単一マシン上で最大 10 ワークツリー環境

## 憲法チェック

*GATE: Phase 0 リサーチ前に通過必須。Phase 1 設計後に再チェック。*

| 原則 | 状態 | 根拠 |
|------|------|------|
| I. CLI ファースト配布 | ✅ 合格 | cobra による CLI、`--json` フラグ、終了コード規約、Homebrew Tap + WinGet 配布 |
| II. ワークツリー-コンテナ分離 | ✅ 合格 | `git worktree add` + devcontainer.json コピー + 独立コンテナセット起動 |
| III. ポート衝突ゼロ | ✅ 合格 | ポートシフト方式 + `net.Listen()` 事前検証 + Docker ラベルによる追跡 |
| IV. 透過的ネットワーキング | ✅ 合格 | ポートシフト方式を採用。リバースプロキシは非 HTTP プロトコルの制約と macOS DNS 設定の煩雑さから不採用（research.md に根拠を文書化済み） |
| V. 言語非依存の実用主義 | ✅ 合格 | Go を選択。Docker が Go ネイティブ、3プラットフォームへのクロスコンパイル容易、GoReleaser + Homebrew/WinGet パイプライン確立（research.md に評価を文書化済み） |
| VI. 簡潔性・YAGNI | ✅ 合格 | 標準ライブラリ優先（os/exec, encoding/json）。不要な抽象化なし |
| VII. テストファースト | ✅ 合格 | testdata/ に4パターンのサンプル設定配置。ユニット + 統合テスト設計 |
| VIII. 日本語ドキュメント | ✅ 合格 | 全マークダウン成果物を日本語で記述。CLI ヘルプは英語（例外規定に準拠） |
| IX. オープンソース配布 | ✅ 合格 | MIT or Apache 2.0 ライセンス、README.md 整備、依存ライブラリのライセンス互換性確認 |
| X. 詳細コードコメント | ✅ 合格 | 全エクスポート関数に GoDoc コメント、非公開関数にも意図説明、Go イディオムの解説コメント |

**原則 IV に関する設計判断の記録**:
リバースプロキシ（Traefik/Caddy/nginx）とポートシフトを比較評価した結果、
以下の理由からポートシフトを採用した:
1. PostgreSQL（5432）、Redis（6379）等の非 HTTP プロトコルは
   ホスト名ベースルーティングに TLS（SNI）が必要で、開発環境では非実用的
2. macOS で `*.localhost` DNS 解決には dnsmasq + sudo 権限が必要で UX を損なう
3. ポートシフトは Docker Compose override YAML で容易に実現可能
4. 憲法原則 VI（簡潔性）に最も適合
将来的にリバースプロキシを追加する拡張パスは閉じていないが、MVP では不要。

## devcontainer.json パターン別対応方針

ユーザーの devcontainer.json 設定に応じて、本ツールは4つのパターンを
自動判別し、パターンごとに適切な書き換えを行う。

### パターン判別フロー

```
devcontainer.json を読み込み:
  ├─ "dockerComposeFile" フィールドが存在する？
  │   ├─ YES → Compose YAML を解析 → サービス数は？
  │   │   ├─ 2つ以上 → パターン D（Compose 複数サービス）
  │   │   └─ 1つ     → パターン C（Compose 単一サービス）
  │   └─ NO  → "build" フィールドが存在する？
  │       ├─ YES → パターン B（Dockerfile ビルド）
  │       └─ NO  → パターン A（image 指定）
```

### パターン A/B（単一コンテナ: image / Dockerfile）

**元の devcontainer.json を変更せず**、ワークツリー側にコピーを生成し、以下を書き換え:
- `name`: ワークツリー名を含む表示名に変更
- `runArgs`: `--name`（コンテナ名衝突回避）、`--label`（メタデータ記録）を追加
- `appPort`: ホスト側ポートをシフト値に変更（`"hostPort:containerPort"` 形式）
- `portsAttributes`: キーをシフト後ポートに変更。`requireLocalPort: true` → `false`
- `containerEnv`: ワークツリー固有の環境変数を追加
- パターン B 追加: `build.dockerfile`/`build.context` の相対パスが正しく解決されるか検証

**ポイント**: `forwardPorts` にはホスト側ポートを明示指定する構文がないため、
ポート衝突回避には `appPort`（Docker `-p` フラグ相当）を使用する。

### パターン C/D（Docker Compose: 単一/複数サービス）

**元の devcontainer.json と Compose YAML を変更せず**、ワークツリー側に以下を生成:

1. **devcontainer.json のコピー**: `dockerComposeFile` を配列に変換し、
   override YAML のパスを末尾に追加
2. **Override YAML の自動生成**: `docker-compose.worktree.yml`
   - `name`: `COMPOSE_PROJECT_NAME` をワークツリー固有の値に設定
     → コンテナ名・ネットワーク・名前付きボリュームが自動分離
   - 各サービスの `ports`: ホスト側ポートをシフト値に変更
   - 各サービスの `labels`: ワークツリーメタデータを追加
   - メインサービスの `volumes`: ソースコードのマウント先をワークツリーパスに変更

**重要な技術的注意点**:
- Compose の `ports` はシーケンス型のため、override ファイルで定義すると
  元の定義を**完全に置換**する。override には元のポートマッピング（ホスト側のみ変更）も
  すべて含める必要がある
- `COMPOSE_PROJECT_NAME` 変更後もサービス間 DNS 解決は機能する
  （サービス名は不変、ネットワークはプロジェクトごとに分離される）
- `forwardPorts` の `"service:port"` 形式（例: `"db:5432"`）はコンテナ内ポートを
  参照するため、書き換え不要

## ポート割り当てアルゴリズム

### 方式: オフセットベースのポートシフト

```
ワークツリーインデックス（1-10）に基づき、ホスト側ポートを算出:

  shiftedPort = originalPort + (worktreeIndex * 10000)

  例: ベースポート 3000 の場合
    環境 0（元環境）: 3000（変更なし）
    環境 1: 13000
    環境 2: 23000
    環境 3: 33000

  ポートが 65535 を超える場合:
    空きポートを動的に探索（net.Listen で確認）
```

### 衝突検証フロー

```
1. シフト後ポートを算出
2. Docker ラベルから既存ワークツリー環境の使用ポートを収集
3. net.Listen() でシステム上のポート使用状況を確認
4. 衝突する場合:
   a. 別のオフセットを試行
   b. それでも失敗する場合はランダムな空きポートを使用
5. すべてのポートが確定したら、コンテナラベルに記録
```

## プロジェクト構造

### ドキュメント（本機能）

```text
specs/001-worktree-container-cli/
├── plan.md              # 本ファイル
├── research.md          # Phase 0 リサーチ結果
├── data-model.md        # Phase 1 データモデル
├── quickstart.md        # Phase 1 クイックスタート
├── contracts/           # Phase 1 インターフェース契約
│   └── cli-commands.md  #   CLI コマンドスキーマ
└── tasks.md             # Phase 2 タスク（/speckit.tasks で生成）
```

### ソースコード（リポジトリルート）

```text
cmd/
└── worktree-container/
    └── main.go                  # エントリポイント

internal/
├── cli/                         # CLI コマンド定義（cobra）
│   ├── root.go                  #   ルートコマンド + グローバルフラグ
│   ├── create.go                #   create サブコマンド
│   ├── list.go                  #   list サブコマンド
│   ├── start.go                 #   start サブコマンド
│   ├── stop.go                  #   stop サブコマンド
│   └── remove.go                #   remove サブコマンド
├── devcontainer/                # devcontainer.json 解析・生成
│   ├── config.go                #   パターン判別・設定読み込み
│   ├── rewrite.go               #   ワークツリー用設定書き換え
│   └── compose.go               #   Compose override YAML 生成
├── port/                        # ポート管理
│   ├── allocator.go             #   ポート割り当てアルゴリズム
│   └── scanner.go               #   使用中ポートスキャン
├── worktree/                    # Git ワークツリー操作
│   └── manager.go               #   git worktree add/list/remove
├── docker/                      # Docker API ラッパー
│   ├── client.go                #   Docker クライアント初期化
│   ├── container.go             #   コンテナライフサイクル操作
│   └── label.go                 #   ラベル操作・状態検出
└── model/                       # ドメインモデル
    └── types.go                 #   WorktreeEnv, PortAllocation 等

tests/
├── unit/                        # ユニットテスト
├── integration/                 # 統合テスト（Docker 必要）
└── testdata/                    # テスト用 devcontainer.json サンプル
    ├── image-simple/            #   パターン A サンプル
    ├── dockerfile-build/        #   パターン B サンプル
    ├── compose-single/          #   パターン C サンプル
    └── compose-multi/           #   パターン D サンプル
```

**構造の決定**: Go 標準の `cmd/` + `internal/` レイアウトを採用。
`internal/` パッケージは関心事ごとに分離（cli, devcontainer, port, worktree, docker, model）。
テストデータは4つの devcontainer.json パターンそれぞれのサンプルを配置する。

## コードコメント規約

プロジェクトオーナーが Go 言語に不慣れであること、およびオープンソースとして
外部コントリビュータの参入を容易にすることから、以下のコメント規約を適用する。

### 必須コメント

1. **GoDoc コメント（全エクスポートシンボル）**:
   すべての公開（大文字始まり）の関数、型、定数、変数に GoDoc 形式のコメントを記述する。
   ```go
   // Allocator manages port allocation for worktree environments.
   // It ensures that no two environments share the same host port
   // by tracking allocations via Docker container labels.
   type Allocator struct { ... }
   ```

2. **非公開関数の意図説明**:
   非公開関数であっても「何をするか」「なぜそうするか」を説明する。
   ```go
   // shiftPort calculates the shifted host port for a given worktree index.
   // We use an offset-based approach (index * 10000) to provide predictable,
   // non-overlapping port ranges for up to 10 concurrent environments.
   func shiftPort(originalPort, worktreeIndex int) int { ... }
   ```

3. **Go イディオムの解説**:
   ゴルーチン、チャネル、インターフェース、defer、エラーハンドリングパターン等の
   Go 固有の概念を使用する箇所では、そのパターンの目的と動作を説明する。
   ```go
   // Use defer to ensure the Docker client connection is always closed
   // when this function returns, even if an error occurs midway.
   // This is a standard Go pattern for resource cleanup.
   defer client.Close()
   ```

4. **複雑なロジックのステップ説明**:
   複雑なアルゴリズムやビジネスロジックにはステップごとのコメントを付与する。

### コメント言語

ソースコード内のコメントは**英語**で記述する（憲法原則 VIII の例外規定に準拠）。

## オープンソース配布要件

- **ライセンス**: MIT License を採用（Go エコシステムで最も一般的。
  依存ライブラリ（cobra: Apache 2.0, docker/client: Apache 2.0,
  compose-go: Apache 2.0, tidwall/jsonc: MIT）とすべて互換）
- **LICENSE ファイル**: リポジトリルートに配置
- **README.md**: プロジェクト概要、インストール方法（`brew install` / `winget install`）、
  基本的な使い方、コントリビューション方法を日本語で記載
- **CONTRIBUTING.md**: コントリビューション手順、開発環境セットアップ、
  コーディング規約（コメント規約を含む）を記載

## Windows 対応の注意事項

Go のクロスコンパイル（`GOOS=windows`）と GoReleaser により、Windows バイナリ（.exe）を
生成し WinGet マニフェストで配布する。実装時に以下の点に注意が必要:

- **ファイルパス**: `filepath.Join()` を使用し、`/` のハードコードを避ける。
  `path/filepath` パッケージが OS に応じた区切り文字を自動処理する
- **Docker ソケット**: Windows では Named Pipe（`npipe:////./pipe/docker_engine`）
  経由で接続する。Go の Docker SDK は自動検出するが、テストで確認が必要
- **ポートスキャン**: `net.Listen()` は Windows でも動作する。
  ただし Windows Firewall がポートへのアクセスをブロックする可能性があるため、
  エラーメッセージで対処方法を案内する
- **Git worktree**: Windows の Git でもワークツリー機能は完全サポートされている
- **GoReleaser 設定**: `goos: [darwin, linux, windows]` で3プラットフォーム対応。
  WinGet マニフェスト生成には `goreleaserextras/winget` または
  GitHub Release への `.exe` + マニフェストの手動/自動登録で対応

## 複雑性トラッキング

> 憲法チェックに違反がないため、本セクションは該当なし。

すべての設計判断が憲法原則に適合しており、正当化を要する違反は存在しない。
