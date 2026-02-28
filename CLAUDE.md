# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## プロジェクト概要

Git ワークツリーごとに独立した Dev Container 環境を自動構築する CLI ツール。
Homebrew でインストール可能なパッケージとして配布する。

## 開発ワークフロー（SpecKit）

本リポジトリは仕様駆動開発フレームワーク（SpecKit）を採用している。
コードを書く前に仕様→計画→タスクの順で設計ドキュメントを作成する。

```
/speckit.specify  → specs/XXX-feature/spec.md を生成
/speckit.clarify  → spec の曖昧な箇所を質問・解消
/speckit.plan     → plan.md, research.md, data-model.md, contracts/ を生成
/speckit.tasks    → tasks.md（依存関係順のタスクリスト）を生成
/speckit.implement → tasks.md に基づき実装を実行
/speckit.analyze  → spec/plan/tasks 間の整合性チェック
/speckit.constitution → プロジェクト憲法の作成・更新
```

## ブランチ命名規則

フィーチャーブランチは `###-feature-name` 形式（例: `001-worktree-container-cli`）。
SpecKit のスクリプトはこの命名規則に依存して `specs/` ディレクトリとマッピングする。

## 主要ディレクトリ構成

- `.specify/memory/constitution.md` — プロジェクト憲法（最上位の設計制約）
- `.specify/templates/` — spec, plan, tasks 等のテンプレート
- `.specify/scripts/bash/` — SpecKit ワークフロー用ユーティリティスクリプト
- `specs/` — フィーチャーごとの仕様・計画・タスクドキュメント

## 憲法の最優先原則

1. **ポート衝突ゼロ（NON-NEGOTIABLE）** — ワークツリー間のポート競合は絶対に許容しない
2. **ワークツリー-コンテナ分離** — 各ワークツリーに完全独立した Dev Container セット
3. **CLI ファースト** — Homebrew 配布、JSON + 人間可読出力、Unix 終了コード準拠
4. **透過的ネットワーキング** — ポートシフト方式を採用（リバースプロキシは非HTTPプロトコルの制約から不採用）
5. **簡潔性・YAGNI** — 具体的ユースケースで正当化できない抽象化は作らない

## 技術スタック

- **言語**: Go >= 1.22
- **CLI**: `github.com/spf13/cobra`
- **Docker API**: `github.com/docker/docker/client`
- **Compose 解析**: `github.com/compose-spec/compose-go/v2`
- **JSONC**: `github.com/tidwall/jsonc`
- **テスト**: `go test` + `github.com/stretchr/testify`
- **ビルド/配布**: GoReleaser + Homebrew Tap + WinGet

## 技術制約

- 対象: macOS、Linux、Windows
- コンテナランタイム: Docker Engine / Docker Desktop 必須
- Git >= 2.15（worktree 機能）
- devcontainer.json 仕様準拠（4パターン: image, Dockerfile, Compose 単一, Compose 複数）
- ポート管理: ポートシフト方式（リバースプロキシは MVP では不採用）
- Windows 対応: `filepath.Join()` でパス処理、Docker Named Pipe 接続、`/` ハードコード禁止

## コードコメント規約

プロジェクトオーナーが Go 言語に不慣れなため、コメントは厚めに記述すること。
- すべてのエクスポート関数・型・定数に GoDoc コメント必須
- 非公開関数にも処理の意図（「なぜそうするか」）を説明するコメント必須
- Go 固有のイディオム（defer, goroutine, channel, interface, error handling）使用時はパターンの目的を解説
- 複雑なロジックにはステップごとの説明コメントを付与
- コメント言語は英語（ソースコード内の規約）

## オープンソース

- ライセンス: MIT License
- README.md, CONTRIBUTING.md を日本語で整備
- シークレット情報をリポジトリに含めないこと

## コミット規約

Conventional Commits 形式を使用する（`feat:`, `fix:`, `docs:`, `chore:` 等）。

## ドキュメント言語

本プロジェクトのドキュメント・マークダウンおよび AI エージェントへの応答は日本語で記述する。
例外: ソースコード識別子、CLI コマンド名/フラグ名、外部仕様の固有名詞、Conventional Commits プレフィックス。
