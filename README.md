# Worktree Container

[![Go](https://img.shields.io/badge/Go-%3E%3D%201.22-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Git ワークツリーごとに独立した Dev Container 環境を自動構築する CLI ツールです。

複数のブランチで同時に開発を行う際、ポート衝突やコンテナの競合を気にすることなく、
各ワークツリーに完全に分離された Dev Container 環境を1コマンドで作成できます。
コーディングエージェント（Claude Code 等）に別ブランチの作業を任せながら、
メインブランチの開発環境を中断せずに使い続けることができます。

## 特徴

- **ポート衝突ゼロ保証** -- 最大 10 環境の同時稼働でもポート衝突が発生しません。ポートシフトアルゴリズムにより、すべてのポート割り当てを自動処理します
- **4パターンの devcontainer.json 対応** -- image 指定、Dockerfile ビルド、Compose 単一サービス、Compose 複数サービスのすべてに対応します
- **Docker ラベルベース状態管理** -- 外部の状態ファイルは不要です。すべてのメタデータは Docker コンテナラベルから動的に検出します
- **複数ツール対応** -- VS Code Dev Container、Dev Container CLI、DevPod のいずれからでも接続可能です
- **クロスプラットフォーム** -- macOS、Linux、Windows をサポートします
- **元の設定を破壊しない** -- 元プロジェクトの devcontainer.json は読み取り専用。ワークツリー側にコピーを生成して改変します

## インストール

### Homebrew（macOS / Linux）

```bash
brew install shinji-kodama/tap/worktree-container
```

### go install

```bash
go install github.com/shinji-kodama/worktree-container/cmd/worktree-container@latest
```

### ソースからビルド

```bash
git clone https://github.com/shinji-kodama/worktree-container.git
cd worktree-container
go build -o worktree-container ./cmd/worktree-container
```

### WinGet（Windows）

```powershell
winget install shinji-kodama.worktree-container
```

## 前提条件

- Docker Engine または Docker Desktop が稼働中であること
- Git >= 2.15
- 対象プロジェクトに `.devcontainer/devcontainer.json` が存在すること

## クイックスタート

### 1. ワークツリー環境を作成する

devcontainer.json を持つプロジェクトのルートで実行します。

```bash
# 新しいブランチでワークツリー環境を作成
worktree-container create feature-auth

# 既存のブランチをベースにワークツリー環境を作成
worktree-container create --base main bugfix-login

# 作成先パスを指定
worktree-container create --path ~/dev/feature-auth feature-auth
```

### 2. ワークツリー環境の一覧を確認する

```bash
# テキスト表示
worktree-container list

# JSON 形式
worktree-container list --json

# 稼働中の環境のみ表示
worktree-container list --status running
```

### 3. ワークツリー環境を停止・再起動する

```bash
# 停止
worktree-container stop feature-auth

# 再起動
worktree-container start feature-auth
```

### 4. ワークツリー環境を削除する

```bash
# 対話的に確認して削除
worktree-container remove feature-auth

# 確認なしで削除
worktree-container remove --force feature-auth

# コンテナのみ削除し、Git ワークツリーは保持
worktree-container remove --keep-worktree feature-auth
```

## コマンドリファレンス

```
worktree-container <command> [flags]

Commands:
  create    新しいワークツリー環境を作成・起動する
  list      ワークツリー環境の一覧を表示する
  start     停止中のワークツリー環境を再起動する
  stop      稼働中のワークツリー環境を停止する
  remove    ワークツリー環境を削除する

Global Flags:
  --json            JSON 形式で出力する
  --verbose, -v     詳細なログを出力する
  --help, -h        ヘルプを表示する
  --version         バージョンを表示する
```

### `worktree-container create`

新しい Git ワークツリーを作成し、そのワークツリー専用の Dev Container 環境を起動します。

```
worktree-container create <branch-name> [flags]

Flags:
  --base <ref>       ワークツリーのベースとなるコミット/ブランチ（デフォルト: HEAD）
  --path <dir>       ワークツリーの作成先パス（デフォルト: ../<repo>-<branch-name>）
  --name <name>      ワークツリー環境の識別名（デフォルト: <branch-name>）
  --no-start         ワークツリー作成のみ行い、コンテナは起動しない
```

**出力例（テキスト）:**

```
Created worktree environment "feature-auth"
  Branch:    feature/auth
  Path:      /Users/user/myproject-feature-auth
  Pattern:   compose-multi (3 services)

  Services:
    app     http://localhost:13000  (container: 3000)
    db      localhost:15432         (container: 5432)
    redis   localhost:16379         (container: 6379)
```

**出力例（JSON）:**

```json
{
  "name": "feature-auth",
  "branch": "feature/auth",
  "worktreePath": "/Users/user/myproject-feature-auth",
  "status": "running",
  "configPattern": "compose-multi",
  "services": [
    { "name": "app", "containerPort": 3000, "hostPort": 13000, "protocol": "tcp" },
    { "name": "db", "containerPort": 5432, "hostPort": 15432, "protocol": "tcp" },
    { "name": "redis", "containerPort": 6379, "hostPort": 16379, "protocol": "tcp" }
  ]
}
```

### `worktree-container list`

全ワークツリー環境の一覧を表示します。

```
worktree-container list [flags]

Flags:
  --status <status>  フィルタ: running / stopped / orphaned / all（デフォルト: all）
```

**出力例:**

```
NAME           BRANCH          STATUS    SERVICES  PORTS
feature-auth   feature/auth    running   3         13000,15432,16379
bugfix-login   bugfix/login    stopped   1         -
old-branch     old/branch      orphaned  0         -
```

### `worktree-container stop`

稼働中のワークツリー環境のコンテナを停止します。

```
worktree-container stop <name>
```

### `worktree-container start`

停止中のワークツリー環境のコンテナを再起動します。

```
worktree-container start <name>
```

### `worktree-container remove`

ワークツリー環境を削除します。コンテナ、ネットワーク、ワークツリー専用ボリュームを削除し、
オプションで Git ワークツリーも削除します。

```
worktree-container remove <name> [flags]

Flags:
  --force, -f         確認なしで削除する
  --keep-worktree     Git ワークツリーは削除せず保持する
```

### 終了コード

| コード | 意味 |
|--------|------|
| 0 | 成功 |
| 1 | 一般エラー |
| 2 | devcontainer.json が見つからない |
| 3 | Docker が起動していない |
| 4 | ポート割り当て失敗 |
| 5 | Git 操作エラー |
| 6 | 指定された環境が見つからない |
| 7 | ユーザーがキャンセルした |

## ポート管理

Worktree Container はポートシフトアルゴリズムにより、各ワークツリー環境のホスト側ポートを自動的に割り当てます。

### ポートシフトアルゴリズム

```
shiftedPort = originalPort + (worktreeIndex * 10000)
```

| 環境 | ベースポート 3000 | ベースポート 5432 | ベースポート 6379 |
|------|-------------------|-------------------|-------------------|
| 元環境（index 0） | 3000 | 5432 | 6379 |
| ワークツリー 1 | 13000 | 15432 | 16379 |
| ワークツリー 2 | 23000 | 25432 | 26379 |
| ワークツリー 3 | 33000 | 35432 | 36379 |

### 衝突回避

1. シフト後のポートが 65535 を超える場合、空きポートを動的に探索します
2. 他のプロセスが使用中のポートは `net.Listen()` で検出し、自動的に回避します
3. 他のワークツリー環境が使用中のポートは Docker ラベルから検出します

ユーザーがポート番号を手動で指定する必要はありません。
`worktree-container list` コマンドで各環境のアクセス先を確認できます。

## サポートする devcontainer.json パターン

### パターン A: image 指定

`image` フィールドで Docker イメージを直接指定するパターンです。

```json
{
  "name": "My Project",
  "image": "mcr.microsoft.com/devcontainers/typescript-node:18",
  "forwardPorts": [3000]
}
```

### パターン B: Dockerfile ビルド

`build` フィールドで Dockerfile からビルドするパターンです。

```json
{
  "name": "My Project",
  "build": {
    "dockerfile": "Dockerfile",
    "context": ".."
  },
  "forwardPorts": [3000, 5000]
}
```

### パターン C: Docker Compose 単一サービス

`dockerComposeFile` フィールドで Docker Compose を使用し、サービスが1つのパターンです。

```json
{
  "name": "My Project",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace"
}
```

### パターン D: Docker Compose 複数サービス

`dockerComposeFile` フィールドで Docker Compose を使用し、サービスが2つ以上のパターンです。
アプリ + DB + キャッシュなどの構成に対応します。

```json
{
  "name": "My Project",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace"
}
```

```yaml
# docker-compose.yml
services:
  app:
    build: .
    ports:
      - "3000:3000"
  db:
    image: postgres:16
    ports:
      - "5432:5432"
  redis:
    image: redis:7
    ports:
      - "6379:6379"
```

## 対応ツール

ワークツリー環境を作成した後、以下のいずれの方法でもコンテナに接続できます。

### VS Code

1. ワークツリーフォルダを VS Code で開く
2. コマンドパレット → 「Reopen in Container」

### Dev Container CLI

```bash
devcontainer up --workspace-folder /path/to/worktree
devcontainer exec --workspace-folder /path/to/worktree bash
```

### DevPod

```bash
devpod up /path/to/worktree
```

## 開発

### 前提条件

- Go >= 1.22
- Docker Engine または Docker Desktop
- Git >= 2.15

### ビルド

```bash
go build -o worktree-container ./cmd/worktree-container
```

### テスト

```bash
# ユニットテスト
go test ./internal/...

# 統合テスト（Docker 必要）
go test -tags=integration ./tests/integration/...

# 全テスト
go test ./...
```

### リント

```bash
golangci-lint run
```

### リリース（GoReleaser）

```bash
goreleaser release --snapshot --clean
```

詳しい開発手順は [CONTRIBUTING.md](./CONTRIBUTING.md) を参照してください。

## ライセンス

[MIT License](./LICENSE)

Copyright (c) 2026 shinji-kodama
