# クイックスタート: Worktree Container CLI

**ブランチ**: `001-worktree-container-cli` | **日付**: 2026-02-28

## 前提条件

- macOS または Linux
- Docker Engine / Docker Desktop が稼働中
- Git >= 2.15
- Go >= 1.22（開発時のみ）

## インストール

```bash
# Homebrew（リリース後）
brew install <tap>/worktree-container

# ソースからビルド（開発中）
git clone <repo-url>
cd worktree-container
go build -o worktree-container ./cmd/worktree-container
```

## 基本的な使い方

### 1. ワークツリー環境を作成する

devcontainer.json を持つプロジェクトのルートで実行:

```bash
# 新しいブランチでワークツリー環境を作成
worktree-container create feature-auth

# 既存のブランチからワークツリー環境を作成
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

## Dev Container ツールからの接続

ワークツリー環境を作成した後、以下のいずれの方法でもコンテナに接続できる:

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

## 開発ワークフロー

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
