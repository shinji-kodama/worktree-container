# Implementation Plan: v0.1.0 リリース準備

**Branch**: `002-release-preparation` | **Date**: 2026-02-28 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/002-release-preparation/spec.md`

## Summary

初回リリース v0.1.0 に必要な全工程を洗い出し、CI/CD パイプラインの修正・検証、GoReleaser スナップショットビルドの確認、Homebrew Tap リポジトリの準備、WinGet マニフェストテンプレートの作成、リリース手順書の整備を行う。主な技術的課題は go.mod と CI の Go バージョン不整合の解消。

## Technical Context

**Language/Version**: Go 1.25（go.mod: `go 1.25.0`）
**Primary Dependencies**: GoReleaser v2, golangci-lint, GitHub Actions
**Storage**: N/A（リリースプロセスのみ）
**Testing**: `go test` + `github.com/stretchr/testify`（既存テストの通過確認）
**Target Platform**: macOS (darwin), Linux, Windows — amd64/arm64
**Project Type**: CLI ツール（リリースパイプラインの準備）
**Performance Goals**: N/A
**Constraints**: CGO_ENABLED=0（クロスコンパイル必須）
**Scale/Scope**: 設定ファイル修正 + テンプレートファイル作成 + 外部リポジトリ準備

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原則 | ステータス | 備考 |
|---|---|---|
| I. CLI ファースト配布 | ✅ 準拠 | Homebrew + WinGet での配布を準備する |
| II. ワークツリー-コンテナ分離 | ✅ 該当なし | 本フィーチャーでは変更なし |
| III. ポート衝突ゼロ | ✅ 該当なし | 本フィーチャーでは変更なし |
| IV. 透過的ネットワーキング | ✅ 該当なし | 本フィーチャーでは変更なし |
| V. 言語非依存の実用主義 | ✅ 準拠 | Go が選定済み（001 で決定・文書化済み） |
| VI. 簡潔性・YAGNI | ✅ 準拠 | `brews` → `homebrew_casks` 移行は MVP では不要と判断 |
| VII. テストファースト | ✅ 準拠 | 既存テストの全通過を確認する |
| VIII. 日本語ドキュメント | ✅ 準拠 | リリース手順書・マークダウンは日本語で記述 |
| IX. オープンソース配布 | ✅ 準拠 | MIT ライセンス確認済み、シークレットはリポジトリに含めない |
| X. 詳細コードコメント | ✅ 該当なし | 本フィーチャーではソースコード変更なし |

**技術制約チェック**:
- macOS/Linux: Homebrew Tap 準備 ✅
- Windows: WinGet マニフェスト準備 ✅
- パッケージマネージャ配布: 両方準備 ✅

**ゲート結果**: ✅ 全原則に準拠。違反なし。

## Project Structure

### Documentation (this feature)

```text
specs/002-release-preparation/
├── plan.md              # This file
├── research.md          # Phase 0 output — 5つの調査結果
├── data-model.md        # Phase 1 output — リリースエンティティ定義
├── quickstart.md        # Phase 1 output — リリース手順の概要
├── contracts/
│   └── release-pipeline.md  # リリースパイプラインの契約定義
├── checklists/
│   └── requirements.md  # 仕様品質チェックリスト
└── tasks.md             # Phase 2 output (by /speckit.tasks)
```

### Source Code (repository root)

```text
.github/workflows/
├── ci.yml               # 修正: Go バージョンを 1.25 に引き上げ
└── release.yml          # 修正: Go バージョンを 1.25 に引き上げ

packaging/
└── winget/
    ├── shinji-kodama.worktree-container.yaml           # version manifest
    ├── shinji-kodama.worktree-container.installer.yaml # installer manifest
    └── shinji-kodama.worktree-container.locale.en-US.yaml  # defaultLocale manifest

docs/
└── RELEASE.md           # リリース手順チェックリスト（新規作成）
```

**Structure Decision**: 既存のリポジトリ構造を維持し、`packaging/winget/` と `docs/RELEASE.md` を追加する。ソースコード（`cmd/`, `internal/`）への変更は不要。

## Implementation Phases

### Phase 1: CI/CD 修正（P1）

1. `.github/workflows/ci.yml` の Go バージョンを修正
   - マトリクス: `["1.22", "1.23"]` → `["1.25"]`
   - lint ジョブ: `"1.23"` → `"1.25"`
   - cross-compile ジョブ: `"1.23"` → `"1.25"`
2. `.github/workflows/release.yml` の Go バージョンを修正
   - `"1.23"` → `"1.25"`
3. ブランチを push して CI の全ジョブ通過を確認

### Phase 2: GoReleaser スナップショットビルド（P1）

1. GoReleaser のインストール確認（`goreleaser --version`）
2. `goreleaser release --snapshot --clean` を実行
3. `dist/` ディレクトリのアーティファクト生成を確認
4. ローカルバイナリで `--version` の出力を確認

### Phase 3: Homebrew Tap 準備（P2）

1. `shinji-kodama/homebrew-tap` リポジトリの存在確認（なければ作成）
2. `Formula/` ディレクトリの作成
3. `HOMEBREW_TAP_TOKEN` の GitHub Secrets 設定（**ユーザー手動操作**）

### Phase 4: WinGet マニフェスト準備（P2）

1. `packaging/winget/` ディレクトリの作成
2. 3つのマニフェストテンプレートファイルの作成
   - `shinji-kodama.worktree-container.yaml`（version）
   - `shinji-kodama.worktree-container.installer.yaml`（installer）
   - `shinji-kodama.worktree-container.locale.en-US.yaml`（defaultLocale）
3. プレースホルダー（`{{VERSION}}`, `{{SHA256_X64}}` 等）を含むテンプレートとして作成

### Phase 5: リリース手順書（P3）

1. `docs/RELEASE.md` にリリースチェックリストを作成
2. 初回リリース固有の手順と通常リリースの手順を分離
3. WinGet 提出手順を含める

## Risk Assessment

| リスク | 影響 | 緩和策 |
|---|---|---|
| CI の Go バージョン変更でテストが失敗 | 高 | ローカルで事前に `go test` を実行して確認 |
| GoReleaser v2 の `brews` が予期せず動作しない | 中 | snapshot ビルドで事前検証 |
| `HOMEBREW_TAP_TOKEN` の権限不足 | 中 | PAT の作成手順をドキュメント化 |
| WinGet マニフェストのスキーマエラー | 低 | GoReleaser の既存 PR を参考にテンプレート作成 |

## Complexity Tracking

違反なし。Complexity Tracking は不要。
