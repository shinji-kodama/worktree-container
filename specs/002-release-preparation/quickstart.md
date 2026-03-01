# Quickstart: v0.1.0 リリース準備

**Branch**: `002-release-preparation` | **Date**: 2026-02-28

## 前提条件

- GoReleaser がインストールされていること（`brew install goreleaser`）
- golangci-lint がインストールされていること（`brew install golangci-lint`）
- GitHub CLI がインストールされていること（`gh`）
- Docker Desktop が稼働中であること

## ステップ 1: CI の修正と検証

```bash
# go.mod に合わせて CI の Go バージョンを修正
# .github/workflows/ci.yml → go-version: ["1.25"]
# .github/workflows/release.yml → go-version: "1.25"

# ローカルでテスト・ビルド確認
go test ./internal/... -race -count=1
go build ./cmd/worktree-container/
```

## ステップ 2: GoReleaser スナップショットビルド

```bash
goreleaser release --snapshot --clean

# 生成されたアーティファクトの確認
ls -la dist/
./dist/worktree-container_darwin_arm64_v8.0/worktree-container --version
```

## ステップ 3: Homebrew Tap 準備

```bash
# Tap リポジトリの作成（まだ存在しない場合）
gh repo create mmr-tortoise/homebrew-tap --public --description "Homebrew tap for mmr-tortoise packages"
gh repo clone mmr-tortoise/homebrew-tap
cd homebrew-tap && mkdir Formula && git add . && git commit -m "chore: 初期化" && git push

# HOMEBREW_TAP_TOKEN の設定
# GitHub Settings > Developer settings > Personal access tokens で PAT を作成
# worktree-container リポジトリの Settings > Secrets に HOMEBREW_TAP_TOKEN として登録
gh secret set HOMEBREW_TAP_TOKEN --repo mmr-tortoise/worktree-container
```

## ステップ 4: WinGet マニフェスト準備

```bash
# テンプレートファイルの確認
ls packaging/winget/
# → mmr-tortoise.worktree-container.yaml
# → mmr-tortoise.worktree-container.installer.yaml
# → mmr-tortoise.worktree-container.locale.en-US.yaml
```

## ステップ 5: リリース実行（タグの作成）

```bash
# バージョンタグの作成と push
git tag v0.1.0
git push origin v0.1.0

# GitHub Actions の release ワークフローが自動実行される
gh run watch
```

## ステップ 6: リリース後の WinGet 提出

```bash
# checksums.txt から SHA256 を取得
cat dist/checksums.txt | grep windows

# テンプレートのプレースホルダーを置換
cd packaging/winget/
sed -i '' 's/{{VERSION}}/0.1.0/g' *.yaml
sed -i '' 's/{{SHA256_X64}}/実際のハッシュ/g' *.yaml

# microsoft/winget-pkgs にフォーク → PR 提出
```
