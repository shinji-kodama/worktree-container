# Implementation Plan: devcontainer オプショナル化

**Branch**: `003-optional-devcontainer` | **Date**: 2026-03-01 | **Spec**: [spec.md](./spec.md)

## Summary

devcontainer.json が存在しないリポジトリでも `create` コマンドで Git worktree を作成可能にする。マーカーファイル（`.worktree-container`）による管理対象の識別を導入し、Docker ラベルとのデュアルソース方式で `list` コマンドを拡張する。既存の devcontainer あり環境の動作は完全に維持する。

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: spf13/cobra, docker/docker/client, compose-spec/compose-go/v2, tidwall/jsonc, stretchr/testify
**Storage**: マーカーファイル（JSON）+ Docker コンテナラベル（既存）
**Testing**: go test + testify
**Target Platform**: macOS, Linux, Windows
**Project Type**: CLI ツール
**Constraints**: 後方互換性 100%、devcontainer なし時は Docker 不要

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原則 | 状態 | 根拠 |
|------|------|------|
| I. CLI ファースト配布 | ✅ 準拠 | CLI コマンドの拡張のみ。出力は人間可読+JSON 両対応 |
| II. ワークツリー-コンテナ分離 | ⚠️ 要注意 | devcontainer なし環境はコンテナを持たない。原則の「独立した Dev Container セットを持たなければならない」に対し、devcontainer が存在しない場合は例外として扱う |
| III. ポート衝突ゼロ | ✅ 準拠 | devcontainer なし環境はポートを使用しないため衝突しない |
| IV. 透過的ネットワーキング | ✅ 該当なし | devcontainer なし環境にはネットワーキングなし |
| V. 言語非依存の実用主義 | ✅ 準拠 | Go で実装済み |
| VI. 簡潔性・YAGNI | ✅ 準拠 | マーカーファイルは最小限の JSON。新規抽象化なし |
| VII. テストファースト | ✅ 準拠 | 新規テストケースを先行して作成 |
| VIII. 日本語ドキュメント | ✅ 準拠 | ドキュメントは日本語 |
| IX. オープンソース配布 | ✅ 準拠 | 影響なし |
| X. 詳細コードコメント | ✅ 準拠 | 新規コードに詳細コメント付与 |

**原則 II への例外正当化**: 憲法の「独立した Dev Container セットを持たなければならない」は devcontainer.json が存在するリポジトリを前提としている。devcontainer.json が存在しないリポジトリではそもそも Dev Container を構成できないため、Git worktree のみの作成は原則の精神（ワークツリー間の分離）に反しない。将来的に憲法改正で「devcontainer.json が存在する場合は」の条件を明記することを推奨。

## Project Structure

### Documentation (this feature)

```text
specs/003-optional-devcontainer/
├── plan.md              # This file
├── research.md          # Phase 0: 調査結果
├── data-model.md        # Phase 1: データモデル変更
├── quickstart.md        # Phase 1: 動作確認手順
├── contracts/
│   └── cli-commands.md  # Phase 1: CLI コマンドコントラクト
└── tasks.md             # Phase 2: タスクリスト（/speckit.tasks で生成）
```

### Source Code (repository root)

```text
internal/
├── model/
│   └── types.go           # ConfigPattern に "none" 追加、WorktreeStatus に "no-container" 追加
├── cli/
│   ├── create.go          # devcontainer.json 検出をオプショナル化、マーカーファイル配置
│   ├── list.go            # デュアルソース検出（Docker ラベル + マーカーファイル）
│   ├── start.go           # ConfigPattern=none のハンドリング
│   ├── stop.go            # ConfigPattern=none のハンドリング
│   └── remove.go          # devcontainer なし環境の削除対応
├── devcontainer/
│   └── config.go          # FindDevContainerJSON のオプショナル化
├── docker/
│   ├── label.go           # 変更なし
│   └── container.go       # 変更なし
└── worktree/
    └── manager.go         # マーカーファイル読み書き機能追加

tests/
└── testdata/
    └── no-devcontainer/   # devcontainer.json のないテスト用ディレクトリ（新規）
```

**Structure Decision**: 既存のディレクトリ構造を維持。新規パッケージの追加はなし。変更は既存ファイルへの追加・修正のみ。`worktree/manager.go` にマーカーファイル関連の機能を追加する（worktree ディレクトリの管理に関する責務として適切）。

## Implementation Design

### 変更箇所の概要

| 優先度 | コンポーネント | ファイル | 変更内容 |
|--------|--------------|---------|---------|
| P0 | データモデル | `model/types.go` | `PatternNone` 定数追加、`StatusNoContainer` 追加 |
| P1 | マーカーファイル | `worktree/manager.go` | マーカーファイル読み書き関数追加 |
| P1 | devcontainer 検出 | `devcontainer/config.go` | `FindDevContainerJSON` をオプショナル化（エラーではなく nil 返却） |
| P1 | create コマンド | `cli/create.go` | 分岐追加（devcontainer あり/なし）、マーカーファイル配置、Docker 接続の遅延化 |
| P2 | list コマンド | `cli/list.go` | デュアルソース検出ロジック追加 |
| P2 | start/stop | `cli/start.go`, `cli/stop.go` | ConfigPattern=none のハンドリング |
| P2 | remove | `cli/remove.go` | devcontainer なし環境の削除対応 |
| P3 | テスト | `*_test.go` | 新規テストケース 5+ |

### create コマンドの改修フロー

```
改修前:
  RepoRoot → EnvName → WorktreePath → Docker接続 → Worktree作成
  → devcontainer検索（必須）→ Pattern検出 → ポート割当 → コピー → 起動

改修後:
  RepoRoot → EnvName → WorktreePath → Worktree作成
  → マーカーファイル配置（ConfigPattern=none で仮作成）
  → devcontainer検索（オプショナル）
  ├─ あり → Docker接続 → Pattern検出 → ポート割当 → コピー → 起動
  │         → マーカーファイル更新（ConfigPattern=実パターン）
  └─ なし → 成功メッセージ（Branch, Path のみ）
```

**重要**: Docker 接続を devcontainer.json 検出の後に移動。これにより devcontainer なしの場合は Docker が不要になる。

### list コマンドの改修フロー

```
改修前:
  Docker接続 → ListManagedContainers → GroupByEnv → BuildWorktreeEnv → 出力

改修後:
  1. git worktree list → 各パスでマーカーファイル検索 → マーカー環境マップ
  2. Docker接続（失敗してもマーカー環境は表示可能）
     → ListManagedContainers → GroupByEnv → BuildWorktreeEnv → Docker 環境マップ
  3. マージ:
     - Docker 環境マップにある環境 → Docker 情報優先
     - マーカーのみの環境 → ConfigPattern=none, Status=no-container
  4. 出力
```
