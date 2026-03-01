# Quickstart: devcontainer オプショナル化

**Branch**: `003-optional-devcontainer` | **Date**: 2026-03-01

## 最速の動作確認

### 1. devcontainer なしのリポジトリでワークツリー作成

```bash
# devcontainer.json のないリポジトリに移動
cd /path/to/repo-without-devcontainer

# ワークツリーを作成（Docker 不要）
worktree-container create feature-test

# 期待される出力:
# Created worktree environment "feature-test"
#   Branch:    feature-test
#   Path:      /path/to/repo-without-devcontainer-feature-test
```

### 2. 一覧で確認

```bash
worktree-container list

# 期待される出力:
# NAME           BRANCH          STATUS         SERVICES  PORTS
# feature-test   feature-test    no-container   0         -
```

### 3. クリーンアップ

```bash
worktree-container remove feature-test

# 期待される出力:
# Remove worktree environment "feature-test"? (y/N): y
# Removed worktree environment "feature-test"
#   Worktree removed: /path/to/repo-without-devcontainer-feature-test
```

## devcontainer あり・なし混在の確認

```bash
# devcontainer.json のあるリポジトリに移動
cd /path/to/repo-with-devcontainer

# devcontainer 付きワークツリーを作成
worktree-container create feature-with-dc

# devcontainer なしリポジトリに移動して作成
cd /path/to/repo-without-devcontainer
worktree-container create feature-no-dc

# どちらのリポジトリからでも list で全環境を確認
worktree-container list

# 期待される出力:
# NAME              BRANCH             STATUS         SERVICES  PORTS
# feature-with-dc   feature-with-dc    running        1         13000
# feature-no-dc     feature-no-dc      no-container   0         -
```

## ビルド・テスト

```bash
# ビルド
go build -o worktree-container ./cmd/worktree-container

# ユニットテスト
go test ./internal/... -timeout 120s -count=1 -race

# リント
golangci-lint run
```
