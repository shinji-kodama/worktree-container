# コントリビューションガイド

Worktree Container へのコントリビューションを歓迎します。
本ドキュメントでは、開発環境のセットアップからプルリクエストの作成までの手順を説明します。

## 開発環境のセットアップ

### 必要なツール

| ツール | バージョン | 用途 |
|--------|-----------|------|
| Go | >= 1.22 | ビルド・テスト |
| Docker Engine / Docker Desktop | 最新推奨 | コンテナ操作・統合テスト |
| Git | >= 2.15 | ワークツリー機能 |
| golangci-lint | 最新推奨 | 静的解析 |
| GoReleaser | 最新推奨 | リリースビルド（任意） |

### リポジトリのクローンとビルド

```bash
git clone https://github.com/mmr-tortoise/worktree-container.git
cd worktree-container
go mod download
go build -o worktree-container ./cmd/worktree-container
```

### 動作確認

```bash
./worktree-container --version
```

## ビルド方法

### バイナリのビルド

```bash
go build -o worktree-container ./cmd/worktree-container
```

### クロスコンパイル

```bash
# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o worktree-container-darwin-arm64 ./cmd/worktree-container

# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -o worktree-container-linux-amd64 ./cmd/worktree-container

# Windows (amd64)
GOOS=windows GOARCH=amd64 go build -o worktree-container-windows-amd64.exe ./cmd/worktree-container
```

### GoReleaser でのスナップショットビルド

```bash
goreleaser release --snapshot --clean
```

## コーディング規約

### Go コメント規約

プロジェクトオーナーが Go 言語に不慣れであること、およびオープンソースプロジェクトとして
外部コントリビュータの参入を容易にすることから、以下のコメント規約を適用します。

#### 1. GoDoc コメント（全エクスポートシンボル）

すべての公開（大文字始まり）の関数、型、定数、変数に GoDoc 形式のコメントを記述してください。

```go
// Allocator manages port allocation for worktree environments.
// It ensures that no two environments share the same host port
// by tracking allocations via Docker container labels.
type Allocator struct { ... }
```

#### 2. 非公開関数の意図説明

非公開関数であっても「何をするか」「なぜそうするか」を説明してください。

```go
// shiftPort calculates the shifted host port for a given worktree index.
// We use an offset-based approach (index * 10000) to provide predictable,
// non-overlapping port ranges for up to 10 concurrent environments.
func shiftPort(originalPort, worktreeIndex int) int { ... }
```

#### 3. Go イディオムの解説

ゴルーチン、チャネル、インターフェース、defer、エラーハンドリングパターン等の
Go 固有の概念を使用する箇所では、そのパターンの目的と動作を説明してください。

```go
// Use defer to ensure the Docker client connection is always closed
// when this function returns, even if an error occurs midway.
// This is a standard Go pattern for resource cleanup.
defer client.Close()
```

#### 4. 複雑なロジックのステップ説明

複雑なアルゴリズムやビジネスロジックにはステップごとのコメントを付与してください。

#### コメント言語

ソースコード内のコメントは**英語**で記述してください。

### ファイルパス

Windows 対応のため、ファイルパスの構築には必ず `filepath.Join()` を使用してください。
`/` のハードコードは避けてください。

```go
// Good
configPath := filepath.Join(worktreePath, ".devcontainer", "devcontainer.json")

// Bad
configPath := worktreePath + "/.devcontainer/devcontainer.json"
```

### コミットメッセージ

[Conventional Commits](https://www.conventionalcommits.org/) 形式を使用します。

```
feat: ポートシフトアルゴリズムを実装
fix: Compose override YAML のポート書き換えを修正
docs: README.md にインストール手順を追加
test: ポート衝突検証のユニットテストを追加
chore: golangci-lint 設定を更新
refactor: Docker ラベル操作をヘルパー関数に分離
```

## PR ワークフロー

### 1. ブランチの作成

フィーチャーブランチは `###-feature-name` 形式で命名してください。

```bash
git checkout -b 002-port-allocator
```

### 2. 変更の実装

- テストを先に書き、失敗することを確認してから実装してください（テストファースト）
- コメント規約に従い、詳細なコメントを記述してください
- `go test ./...` と `golangci-lint run` がパスすることを確認してください

### 3. コミット

```bash
git add <変更ファイル>
git commit -m "feat: <変更の説明>"
```

### 4. プルリクエストの作成

- PR タイトルは Conventional Commits 形式にしてください
- PR の説明に変更の目的と影響範囲を記載してください
- 1つの PR は1つの関心事に限定してください

### PR チェックリスト

- [ ] `go test ./...` がパスする
- [ ] `golangci-lint run` がパスする
- [ ] 新規の公開関数・型に GoDoc コメントがある
- [ ] 非公開関数にも意図を説明するコメントがある
- [ ] Go イディオムの使用箇所にパターンの解説コメントがある
- [ ] ファイルパスに `filepath.Join()` を使用している（`/` のハードコードがない）
- [ ] Conventional Commits 形式のコミットメッセージになっている

## テスト

### ユニットテスト

```bash
# 全ユニットテスト
go test ./internal/...

# 特定パッケージのテスト
go test ./internal/port/...
go test ./internal/devcontainer/...

# カバレッジ付きで実行
go test -cover ./internal/...

# 詳細出力
go test -v ./internal/...
```

### 統合テスト

統合テストは Docker が必要です。`//go:build integration` ビルドタグを使用しています。

```bash
# 統合テスト（Docker 必要）
go test -tags=integration ./tests/integration/...

# 特定のテストのみ実行
go test -tags=integration -run TestCreateCommand ./tests/integration/...
```

### テストデータ

テスト用の devcontainer.json サンプルは `tests/testdata/` に配置されています。

```
tests/testdata/
  image-simple/          パターン A: image 指定
  dockerfile-build/      パターン B: Dockerfile ビルド
  compose-single/        パターン C: Compose 単一サービス
  compose-multi/         パターン D: Compose 複数サービス
```

新しいテストケースを追加する場合は、適切なディレクトリにサンプルファイルを配置してください。

### テスト作成のガイドライン

- テストフレームワークには `github.com/stretchr/testify` を使用します
- テーブル駆動テストを推奨します
- モックが必要な場合はインターフェースを定義し、テスト用の実装を作成してください
- 統合テストには必ず `//go:build integration` タグを付与してください

## リリースプロセス

リリースは GoReleaser を使用したタグベースの自動リリースです。

### 1. バージョンタグの作成

[セマンティックバージョニング](https://semver.org/) に従います。

```bash
git tag v0.1.0
git push origin v0.1.0
```

### 2. 自動リリース

タグのプッシュをトリガーに、GitHub Actions がリリースワークフローを実行します。

- GoReleaser が macOS、Linux、Windows のバイナリを生成
- GitHub Release にバイナリとチェンジログを公開
- Homebrew Tap を自動更新

### 3. バージョニング方針

| 種別 | 例 | 説明 |
|------|-----|------|
| MAJOR | v1.0.0 → v2.0.0 | 後方互換性のない変更 |
| MINOR | v1.0.0 → v1.1.0 | 後方互換性のある機能追加 |
| PATCH | v1.0.0 → v1.0.1 | バグ修正 |

### ローカルでのリリース確認

```bash
goreleaser release --snapshot --clean
```

`dist/` ディレクトリに各プラットフォームのバイナリが生成されます。

## プロジェクト構造

```
cmd/
  worktree-container/
    main.go                  エントリポイント

internal/
  cli/                       CLI コマンド定義（cobra）
  devcontainer/              devcontainer.json 解析・生成
  port/                      ポート管理
  worktree/                  Git ワークツリー操作
  docker/                    Docker API ラッパー
  model/                     ドメインモデル

tests/
  unit/                      ユニットテスト
  integration/               統合テスト（Docker 必要）
  testdata/                  テスト用 devcontainer.json サンプル
```

## 質問・サポート

不明な点がある場合は、GitHub Issue で質問してください。
