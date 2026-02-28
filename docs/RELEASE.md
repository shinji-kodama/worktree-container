# リリース手順チェックリスト

本ドキュメントは `worktree-container` のリリースプロセスを定義する。

## 前提条件

- [ ] Go >= 1.25 がインストールされている
- [ ] GoReleaser がインストールされている（`brew install goreleaser`）
- [ ] golangci-lint がインストールされている（`brew install golangci-lint`）
- [ ] GitHub CLI（`gh`）がインストールされ、認証済みである
- [ ] Docker Desktop が稼働中である
- [ ] `main` ブランチが最新の状態である

---

## 初回リリース固有の手順

初回リリース（v0.1.0）でのみ必要な手順。2回目以降のリリースではスキップ可能。

### Homebrew Tap リポジトリの作成

- [ ] `shinji-kodama/homebrew-tap` リポジトリを作成

  ```bash
  gh repo create shinji-kodama/homebrew-tap --public \
    --description "Homebrew tap for shinji-kodama packages"
  ```

- [ ] `Formula/` ディレクトリと README.md を作成してコミット・push

  ```bash
  gh repo clone shinji-kodama/homebrew-tap
  cd homebrew-tap
  mkdir Formula
  echo "# Homebrew Tap" > README.md
  git add .
  git commit -m "chore: 初期化"
  git push
  ```

### GitHub Personal Access Token（PAT）の作成

- [ ] GitHub Settings > Developer settings > Personal access tokens > Fine-grained tokens で新しい PAT を作成
  - トークン名: `HOMEBREW_TAP_TOKEN`
  - リポジトリアクセス: `shinji-kodama/homebrew-tap` のみ
  - 権限: Contents（Read and write）

### GitHub Secrets の設定

- [ ] `worktree-container` リポジトリの Secrets に `HOMEBREW_TAP_TOKEN` を設定

  ```bash
  gh secret set HOMEBREW_TAP_TOKEN --repo shinji-kodama/worktree-container
  ```

---

## 通常リリース手順

すべてのリリースで実行する手順。

### 1. リリース前の確認

- [ ] `main` ブランチで作業していることを確認

  ```bash
  git checkout main
  git pull origin main
  ```

- [ ] ユニットテストが全通過することを確認

  ```bash
  go test ./internal/... -race -count=1
  ```

- [ ] lint エラーがないことを確認

  ```bash
  golangci-lint run
  ```

- [ ] ビルドが成功することを確認

  ```bash
  go build ./cmd/worktree-container/
  ```

### 2. GoReleaser スナップショットビルドの検証

- [ ] スナップショットビルドを実行

  ```bash
  goreleaser release --snapshot --clean
  ```

- [ ] `dist/` ディレクトリに以下のアーティファクトが存在することを確認
  - `worktree-container_*_darwin_amd64.tar.gz`
  - `worktree-container_*_darwin_arm64.tar.gz`
  - `worktree-container_*_linux_amd64.tar.gz`
  - `worktree-container_*_linux_arm64.tar.gz`
  - `worktree-container_*_windows_amd64.zip`
  - `checksums.txt`

- [ ] ローカルプラットフォーム用バイナリで `--version` を確認

  ```bash
  ./dist/worktree-container_*/worktree-container --version
  ```

### 3. バージョンタグの作成と push

- [ ] バージョンタグを作成して push

  ```bash
  git tag v<VERSION>
  git push origin v<VERSION>
  ```

### 4. GitHub Actions リリースワークフローの確認

- [ ] Release ワークフローが自動実行されることを確認

  ```bash
  gh run watch
  ```

- [ ] GitHub Release ページにアーティファクトが正しくアップロードされていることを確認

  ```bash
  gh release view v<VERSION>
  ```

### 5. Homebrew Formula の確認

- [ ] `shinji-kodama/homebrew-tap` リポジトリに Formula が push されていることを確認

  ```bash
  gh api repos/shinji-kodama/homebrew-tap/contents/Formula/worktree-container.rb
  ```

- [ ] Homebrew でインストールできることを確認（オプション）

  ```bash
  brew install shinji-kodama/tap/worktree-container
  worktree-container --version
  ```

---

## WinGet マニフェスト提出手順

リリース後に手動で実施する。

### 1. SHA256 ハッシュの取得

- [ ] GitHub Release の `checksums.txt` から Windows 用バイナリの SHA256 を取得

  ```bash
  gh release download v<VERSION> --pattern checksums.txt
  grep windows checksums.txt
  ```

### 2. マニフェストテンプレートの更新

- [ ] テンプレートのプレースホルダーを実際の値に置換

  ```bash
  cd packaging/winget/
  # VERSION, SHA256_X64, SHA256_ARM64 を置換
  sed -i '' 's/{{VERSION}}/<VERSION>/g' *.yaml
  sed -i '' 's/{{SHA256_X64}}/<SHA256_X64>/g' *.yaml
  sed -i '' 's/{{SHA256_ARM64}}/<SHA256_ARM64>/g' *.yaml
  ```

### 3. PR の提出

- [ ] `microsoft/winget-pkgs` リポジトリをフォーク
- [ ] フォークしたリポジトリにマニフェストファイルをコピー

  ```bash
  # manifests/s/shinji-kodama/worktree-container/<VERSION>/ に配置
  mkdir -p manifests/s/shinji-kodama/worktree-container/<VERSION>/
  cp packaging/winget/*.yaml manifests/s/shinji-kodama/worktree-container/<VERSION>/
  ```

- [ ] PR を提出して WinGet チームのレビューを待つ

---

## トラブルシューティング

### GoReleaser が失敗する場合

```bash
# 設定の検証
goreleaser check

# 詳細ログでスナップショット実行
goreleaser release --snapshot --clean --verbose
```

### Homebrew Formula が push されない場合

- `HOMEBREW_TAP_TOKEN` の権限を確認
- PAT の有効期限を確認
- `shinji-kodama/homebrew-tap` リポジトリの存在を確認

### CI が Go バージョンエラーで失敗する場合

- `go.mod` の `go` ディレクティブと `.github/workflows/ci.yml` の `go-version` が一致していることを確認
