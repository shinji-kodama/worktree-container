# Contract: リリースパイプライン

**Date**: 2026-02-28

本フィーチャーは新しい外部インターフェースを追加しない（CLI コマンドやAPIの変更なし）。
以下は、リリースパイプラインが満たすべき契約を定義する。

## GitHub Actions Release ワークフロー

**トリガー**: `v*` タグの push

**入力**:
- `GITHUB_TOKEN`: GitHub が自動提供
- `HOMEBREW_TAP_TOKEN`: GitHub Secrets から取得

**出力**:
- GitHub Release（タグ名 = バージョン）
- 6 アーティファクト（darwin/linux × amd64/arm64 の tar.gz + windows_amd64 の zip）
- `checksums.txt`
- Homebrew Formula が `shinji-kodama/homebrew-tap` リポジトリの `Formula/` に自動 push

## GoReleaser アーティファクト命名規則

```
worktree-container_<Version>_<OS>_<Arch>.<ext>
```

| OS | Arch | Extension |
|---|---|---|
| darwin | amd64, arm64 | .tar.gz |
| linux | amd64, arm64 | .tar.gz |
| windows | amd64 | .zip |

## バージョン情報の注入

```
-X main.version={{.Version}}
-X main.commit={{.Commit}}
-X main.date={{.Date}}
```

`worktree-container --version` の出力: `worktree-container version <Version>`

## Homebrew Formula 契約

- **配置先**: `shinji-kodama/homebrew-tap` リポジトリ `Formula/worktree-container.rb`
- **インストールコマンド**: `brew install shinji-kodama/tap/worktree-container`
- **テスト**: `worktree-container --version` が成功すること

## WinGet マニフェスト契約

- **PackageIdentifier**: `shinji-kodama.worktree-container`
- **InstallerType**: `zip`（`NestedInstallerType: portable`）
- **提出先**: `microsoft/winget-pkgs` リポジトリに PR
- **インストールコマンド**: `winget install shinji-kodama.worktree-container`
