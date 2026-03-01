# Feature Specification: devcontainer オプショナル化

**Feature Branch**: `003-optional-devcontainer`
**Created**: 2026-03-01
**Status**: Draft
**Input**: devcontainer なしでも worktree 作成可能にする。devcontainer.json が存在しない場合でも Git worktree を作成できるようにし、存在する場合は従来通り Dev Container 環境も自動構築する。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - devcontainer なしのリポジトリでワークツリーを作成する (Priority: P1)

開発者として、devcontainer.json を持たないリポジトリでも `create` コマンドでワークツリーを作成し、別ブランチでの並行開発を行いたい。

**Why this priority**: devcontainer.json のないリポジトリは非常に多い。この機能がなければツールの利用対象が限定され、「Git ワークツリー管理ツール」としての価値が大幅に低下する。

**Independent Test**: devcontainer.json のないリポジトリで `create` コマンドを実行し、Git ワークツリーが正常に作成されることを確認する。

**Acceptance Scenarios**:

1. **Given** devcontainer.json が存在しない Git リポジトリ, **When** `create feature-branch` を実行, **Then** Git ワークツリーが作成され、成功メッセージが表示される（コンテナ関連の処理はスキップされる）
2. **Given** devcontainer.json が存在しない Git リポジトリ, **When** `create --base main feature-branch` を実行, **Then** main ブランチをベースとした Git ワークツリーが作成される
3. **Given** devcontainer.json が存在しない Git リポジトリ, **When** `create --path ~/dev/feature feature-branch` を実行, **Then** 指定パスに Git ワークツリーが作成される

---

### User Story 2 - devcontainer ありのリポジトリで従来通り動作する (Priority: P1)

開発者として、devcontainer.json を持つリポジトリでは従来通りワークツリー作成と Dev Container 環境の自動構築が行われることを期待する。

**Why this priority**: 既存機能の後方互換性は最重要。既存ユーザーの体験を損なわないことが前提条件。

**Independent Test**: devcontainer.json のあるリポジトリで `create` コマンドを実行し、ワークツリー作成とコンテナ起動が従来通り行われることを確認する。

**Acceptance Scenarios**:

1. **Given** devcontainer.json が存在する Git リポジトリ（image パターン）, **When** `create feature-branch` を実行, **Then** Git ワークツリーが作成され、Dev Container が起動し、ポート割り当てが行われる
2. **Given** devcontainer.json が存在する Git リポジトリ（Compose パターン）, **When** `create feature-branch` を実行, **Then** Git ワークツリーが作成され、Compose サービス群が起動し、全ポートがシフトされる

---

### User Story 3 - devcontainer なしのワークツリーを一覧・管理する (Priority: P2)

開発者として、devcontainer なしで作成したワークツリーも `list` コマンドで確認し、`remove` コマンドで削除できるようにしたい。

**Why this priority**: 作成したワークツリーを管理できなければ、ゴミが残る。ただしコンテナがないため `start`/`stop` は対象外。

**Independent Test**: devcontainer なしのワークツリーを作成後、`list` で表示され、`remove` で削除できることを確認する。

**Acceptance Scenarios**:

1. **Given** devcontainer なしで作成されたワークツリーが存在, **When** `list` を実行, **Then** そのワークツリーが一覧に表示される（ステータスは "no-container"、サービス数は 0、ポートは "-"）
2. **Given** devcontainer なしで作成されたワークツリーが存在, **When** `list --json` を実行, **Then** JSON 出力にそのワークツリーが含まれ、`configPattern` が `none`、`services` が空配列で表示される
3. **Given** devcontainer なしで作成されたワークツリーが存在, **When** `remove <name>` を実行, **Then** Git ワークツリーが削除される（確認プロンプトあり）
4. **Given** devcontainer なしで作成されたワークツリーが存在, **When** `start <name>` を実行, **Then** 「この環境にはコンテナがありません」というメッセージが表示される

---

### User Story 4 - ワークツリー作成後に devcontainer を追加する (Priority: P3)

開発者として、devcontainer なしで作成したワークツリーに後から devcontainer.json を追加し、コンテナ環境をセットアップしたい。

**Why this priority**: 段階的な開発フローをサポート。最初は軽量にワークツリーだけ作成し、必要に応じてコンテナ環境を追加できる柔軟性を提供する。

**Independent Test**: devcontainer なしのワークツリーに devcontainer.json を追加後、`start` で Dev Container が起動できることを確認する。

**Acceptance Scenarios**:

1. **Given** devcontainer なしで作成されたワークツリー, **When** ワークツリー内に `.devcontainer/devcontainer.json` を作成し `start <name>` を実行, **Then** Dev Container が検出・起動され、ポート割り当てが行われる

---

### Edge Cases

- devcontainer.json が壊れている（不正な JSON）場合 → 従来通りパースエラーとして報告する
- `--no-start` フラグと devcontainer なしの組み合わせ → ワークツリーのみ作成される（コンテナ処理がないため `--no-start` は実質無影響）
- devcontainer.json が `.devcontainer.json`（ルート直置き）にのみ存在するケース → 従来の検索ロジックで検出される
- 同名のワークツリーが既に存在する場合 → 従来通りエラーとなる
- devcontainer なしのワークツリーと devcontainer ありのワークツリーが混在する `list` 出力 → 両方が区別可能な形式で表示される
- devcontainer なしのワークツリーに対する `stop` → `start` と同様に「コンテナがありません」メッセージを表示する

## Clarifications

### Session 2026-03-01

- Q: このツールで管理するワークツリーの識別方法は？ → A: ツールで作成したワークツリーにマーカーファイル `.worktree-container` を配置して識別する
- Q: devcontainer なしの `create` 成功時の出力フォーマットは？ → A: Branch と Path のみ表示し、Services セクションは省略する

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `create` コマンドは、devcontainer.json が存在しないリポジトリでも Git ワークツリーを正常に作成しなければならない（MUST）
- **FR-002**: `create` コマンドは、devcontainer.json が存在する場合、従来通り Dev Container 環境の構築（ポート割り当て、設定コピー、コンテナ起動）を行わなければならない（MUST）
- **FR-003**: devcontainer.json が存在しない場合、`create` コマンドはコンテナ関連の処理（ポート割り当て、.devcontainer コピー、コンテナ起動）をすべてスキップしなければならない（MUST）
- **FR-004**: `list` コマンドは、devcontainer なしで作成されたワークツリーを含むすべてのワークツリー環境を表示しなければならない（MUST）
- **FR-005**: devcontainer なしのワークツリーは、`list` 出力でコンテナ付きの環境と視覚的に区別できなければならない（MUST）
- **FR-006**: `remove` コマンドは、devcontainer なしのワークツリーを削除できなければならない（MUST）
- **FR-007**: `start` コマンドは、devcontainer なしのワークツリーに対して実行された場合、コンテナが存在しない旨の明確なメッセージを表示し、終了コード 0 を返さなければならない（MUST）
- **FR-008**: `stop` コマンドは、devcontainer なしのワークツリーに対して実行された場合、コンテナが存在しない旨の明確なメッセージを表示し、終了コード 0 を返さなければならない（MUST）
- **FR-009**: `create` コマンドの成功時の出力は、devcontainer の有無に応じて適切なフォーマットで表示しなければならない（MUST）。devcontainer なしの場合は Branch と Path のみ表示し、Pattern・Services セクションは省略する
- **FR-010**: devcontainer.json が存在しない状況は正常動作であり、エラーとして扱ってはならない（MUST）
- **FR-011**: `create` コマンドは、ワークツリー作成時にマーカーファイル `.worktree-container` をワークツリーディレクトリ内に配置しなければならない（MUST）。これはdevcontainer の有無に関わらず全ワークツリーに適用される
- **FR-012**: `list` コマンドは、マーカーファイルの存在と Docker ラベルの両方を検索ソースとし、管理対象のワークツリーを網羅的に検出しなければならない（MUST）

### Assumptions

- devcontainer なしのワークツリーの識別は、ワークツリーディレクトリ内に配置するマーカーファイル `.worktree-container` に基づく。`create` コマンドはワークツリー作成時にこのファイルを配置し、`list` コマンドはマーカーファイルの存在で管理対象を判別する。Docker ラベルがある環境はコンテナ付き、マーカーファイルのみの環境はコンテナなしとして扱う
- 既存の終了コード体系は維持する。終了コード 2 は「devcontainer.json のパースに失敗した」場合にのみ使用され、「devcontainer.json が見つからない」はエラーではなくなる
- `--no-start` フラグの動作: devcontainer ありの場合は従来通り（ワークツリー作成 + devcontainer コピーのみ）、devcontainer なしの場合はワークツリー作成のみ
- devcontainer なしのワークツリーは Docker に依存しない。Docker が起動していなくてもワークツリー作成が可能

### Key Entities

- **ワークツリー環境（Environment）**: Git ワークツリーと、オプショナルな Dev Container 環境の組み合わせ。`configPattern` が `none` の場合はコンテナなし
- **設定パターン（ConfigPattern）**: 既存の `image`, `dockerfile`, `compose-single`, `compose-multi` に加え、新たに `none`（devcontainer なし）を追加

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: devcontainer.json のないリポジトリで `create` コマンドが 5 秒以内に完了すること（コンテナ関連の処理がスキップされるため高速）
- **SC-002**: 既存の全テストケース（devcontainer ありのシナリオ）が変更なく通過すること（後方互換性 100%）
- **SC-003**: `list` コマンドの出力で、devcontainer なしのワークツリーとありのワークツリーが混在する場合、ユーザーが各環境の状態を即座に識別できること
- **SC-004**: devcontainer なし・あり合わせて最大 10 環境を同時管理でき、すべてのコマンドが正常に動作すること
- **SC-005**: devcontainer なしのシナリオが少なくとも 5 つの新規テストケースでカバーされていること
