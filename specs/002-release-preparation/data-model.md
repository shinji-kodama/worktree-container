# Data Model: v0.1.0 リリース準備

**Branch**: `002-release-preparation` | **Date**: 2026-02-28

本フィーチャーはリリースプロセスの準備であり、アプリケーションのデータモデルに変更はない。
以下は、リリースパイプラインで管理されるエンティティの構造を記述する。

## リリースアーティファクト

GoReleaser が生成する成果物。

| フィールド | 説明 | 例 |
|---|---|---|
| ProjectName | プロジェクト名 | `worktree-container` |
| Version | セマンティックバージョン | `0.1.0` |
| OS | ターゲット OS | `darwin`, `linux`, `windows` |
| Arch | ターゲットアーキテクチャ | `amd64`, `arm64` |
| Format | アーカイブ形式 | `tar.gz`（darwin/linux）、`zip`（windows） |

**命名規則**: `worktree-container_<Version>_<OS>_<Arch>.<Format>`

**生成ファイル一覧**:
- `worktree-container_0.1.0_darwin_amd64.tar.gz`
- `worktree-container_0.1.0_darwin_arm64.tar.gz`
- `worktree-container_0.1.0_linux_amd64.tar.gz`
- `worktree-container_0.1.0_linux_arm64.tar.gz`
- `worktree-container_0.1.0_windows_amd64.zip`
- `checksums.txt`

## Homebrew Formula

GoReleaser が自動生成し、Tap リポジトリに push する。

| フィールド | 説明 |
|---|---|
| class name | `WorktreeContainer`（Ruby クラス名） |
| url | GitHub Release のダウンロード URL |
| sha256 | アーカイブの SHA256 チェックサム |
| bin.install | `worktree-container` |
| test | `system "#{bin}/worktree-container", "--version"` |

## WinGet マニフェスト

3ファイル構成。リリース後に手動で `microsoft/winget-pkgs` に PR 提出。

| ファイル | ManifestType | 主要フィールド |
|---|---|---|
| `*.yaml` | version | PackageIdentifier, PackageVersion, DefaultLocale |
| `*.installer.yaml` | installer | InstallerType(zip), Architecture, InstallerUrl, InstallerSha256 |
| `*.locale.en-US.yaml` | defaultLocale | Publisher, PackageName, License, ShortDescription |

## GitHub Secrets

| Secret 名 | 用途 | 必要な権限 |
|---|---|---|
| `GITHUB_TOKEN` | GitHub Release 作成 | 自動提供（`contents: write`） |
| `HOMEBREW_TAP_TOKEN` | Homebrew Tap への Formula push | `mmr-tortoise/homebrew-tap` への書き込み |
