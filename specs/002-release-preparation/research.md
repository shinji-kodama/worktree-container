# Research: v0.1.0 リリース準備

**Branch**: `002-release-preparation` | **Date**: 2026-02-28

## R-001: go.mod バージョンと CI の整合性

**Decision**: go.mod の `go 1.25.0` を維持し、CI の Go バージョンを `1.25` に引き上げる

**Rationale**:
- Go 1.21 以降、`go` ディレクティブは必須要件に変更された。Go 1.22/1.23 は `go 1.25.0` を宣言する go.mod を**読み込み拒否**する
- Go 1.25 は安定版としてリリース済み（Go 1.26 も 2026-02-10 にリリース済み）
- 現状の CI マトリクス（Go 1.22/1.23）では**ビルド自体が不可能**

**Alternatives considered**:
- go.mod を `go 1.22` に下げて `toolchain go1.25.0` を追加する方法もあるが、Go 1.25 固有の機能を使用している可能性があるため、シンプルに CI 側を合わせる方が安全

**具体的変更**:

| ファイル | 現在の値 | 修正後 |
|---------|---------|--------|
| `go.mod` | `go 1.25.0` | 変更不要 |
| `ci.yml` マトリクス | `["1.22", "1.23"]` | `["1.25"]` |
| `ci.yml` lint/cross-compile | `"1.23"` | `"1.25"` |
| `release.yml` | `"1.23"` | `"1.25"` |

---

## R-002: GoReleaser v2 の brews セクション互換性

**Decision**: 現在の `brews` セクションを維持する（MVP リリースでは移行不要）

**Rationale**:
- `brews` は GoReleaser v2.10 で deprecated になったが、v2.x 全体では引き続き動作する
- v3（リリース日未定）で削除予定
- 移行先は `homebrew_casks`（`directory: Casks` に変更、`install:` ブロック不要）
- 現在の `repository.owner` / `repository.name` 形式は GoReleaser v2 で正しいスキーマ

**Alternatives considered**:
- 即座に `homebrew_casks` に移行する → Tap リポジトリの `Formula/` → `Casks/` 変更も必要になり、MVP には過剰

---

## R-003: GoReleaser --snapshot の動作

**Decision**: `--snapshot` はすべてのパブリッシュをスキップするため、ローカルテスト時に安全

**Rationale**:
- `--snapshot` は `--skip=announce,publish,validate` を暗黙的に適用
- Homebrew Tap への push、GitHub Releases へのアップロード等はすべてスキップ
- ローカルで `dist/` にアーティファクトが生成されることのみ確認可能

---

## R-004: HOMEBREW_TAP_TOKEN 未設定時の動作

**Decision**: 未設定の場合、GoReleaser はテンプレート展開エラーで**全体が失敗**する

**Rationale**:
- `{{ .Env.HOMEBREW_TAP_TOKEN }}` は環境変数が存在しない場合、即座にエラー
- GitHub Release の作成も含めてすべてが中断される
- `release.yml` には既に `HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}` が設定されている
- **運用タスク**: GitHub Settings > Secrets に PAT を登録する必要がある
- PAT は `mmr-tortoise/homebrew-tap` リポジトリへの書き込み権限が必要

---

## R-005: WinGet マニフェスト形式

**Decision**: 3ファイルマニフェスト形式（version / installer / defaultLocale）を使用

**Rationale**:
- WinGet の標準的なマルチファイルマニフェスト形式
- GoReleaser が生成する Windows 向け zip アーカイブに対応
- `InstallerType: zip` + `NestedInstallerType: portable` パターンが Go CLI ツールの標準

**ディレクトリ構成**（`microsoft/winget-pkgs` リポジトリ内）:
```
manifests/s/mmr-tortoise/worktree-container/0.1.0/
├── mmr-tortoise.worktree-container.yaml              (version)
├── mmr-tortoise.worktree-container.installer.yaml    (installer)
└── mmr-tortoise.worktree-container.locale.en-US.yaml (defaultLocale)
```

**テンプレートの配置先**: `packaging/winget/`（clarify で決定済み）

**プレースホルダー**:

| プレースホルダー | 説明 | 例 |
|---|---|---|
| `{{VERSION}}` | セマンティックバージョン | `0.1.0` |
| `{{RELEASE_DATE}}` | リリース日 (YYYY-MM-DD) | `2026-03-01` |
| `{{SHA256_X64}}` | x64 zip の SHA256 | 64文字 hex |
| `{{SHA256_ARM64}}` | arm64 zip の SHA256 | 64文字 hex |

**ManifestVersion**: 1.9.0（2026年2月時点の主流バージョン）
