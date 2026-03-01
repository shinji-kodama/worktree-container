# Research: devcontainer オプショナル化

**Branch**: `003-optional-devcontainer` | **Date**: 2026-03-01

## R-001: devcontainer なしワークツリーの検出方法

### Decision: マーカーファイル方式

ワークツリーディレクトリ内に `.worktree-container` マーカーファイルを配置し、このツールで管理されたワークツリーを識別する。

### Rationale

現在の `list` コマンドは Docker ラベル（`worktree.managed-by`）のみで管理対象を検出する。devcontainer なしのワークツリーは Docker コンテナを持たないため、Docker ラベル経由では検出できない。マーカーファイルにより Docker 非依存の検出が可能になる。

### Alternatives Considered

| 方式 | メリット | デメリット | 判定 |
|------|---------|-----------|------|
| マーカーファイル | Docker 不要、シンプル、worktree 内に閉じる | ファイル削除で追跡不可 | **採用** |
| ローカル設定ファイル（~/.config/grove/） | 一元管理 | ファイル移動で不整合、マルチユーザー非対応 | 不採用 |
| git worktree list の全表示 | 実装最小 | ツール外で作成した worktree も表示される | 不採用 |

### マーカーファイルの仕様

- **ファイル名**: `.worktree-container`（リネームは本フィーチャーのスコープ外。別ブランチで対応予定）
- **配置場所**: ワークツリーのルートディレクトリ
- **形式**: JSON
- **内容**: 環境名、ブランチ名、ソースリポジトリパス、作成日時、ConfigPattern
- **作成タイミング**: `create` コマンドの worktree 作成直後
- **削除タイミング**: `remove` コマンドの worktree 削除前

## R-002: list コマンドの環境検出ロジック改修

### Decision: マーカーファイル + Docker ラベルのデュアルソース方式

`list` コマンドは以下の 2 ソースから環境情報をマージする:

1. **Docker ラベル**: 既存の `ListManagedContainers()` → コンテナ付き環境
2. **マーカーファイル**: `git worktree list` → 各ワークツリーパスでマーカーファイルを検索 → コンテナなし環境

### Rationale

Docker ラベルのみでは devcontainer なしのワークツリーを検出できない。`git worktree list` は全ワークツリーを返すが、このツール管理外のものも含む。マーカーファイルで管理対象をフィルタリングすることで、正確な検出が可能。

### マージルール

- Docker ラベルとマーカーファイルの両方で検出された環境: Docker ラベル情報を優先（コンテナ状態が正確）
- マーカーファイルのみで検出された環境: configPattern=none、status=no-container
- Docker ラベルのみで検出された環境: 従来通り（後方互換性）

## R-003: create コマンドの分岐設計

### Decision: FindDevContainerJSON のエラーを条件分岐に変更

現在の `FindDevContainerJSON()` は devcontainer.json が見つからない場合にエラー（exit code 2）を返す。これを「見つからなかった」を正常な分岐条件として扱うように変更する。

### Rationale

`create` コマンドの処理フロー（create.go L86-287）で、worktree 作成（L131）の後に devcontainer.json 検出（L139）が行われる。見つからない場合、以降のコンテナ関連処理（L145-282）をすべてスキップし、マーカーファイル配置と成功メッセージ表示のみ行う。

### 設計

```
create コマンド:
  1. リポジトリルート取得
  2. 環境名・ワークツリーパス決定
  3. Git worktree 作成
  4. マーカーファイル配置（ConfigPattern=none で仮作成）
  5. devcontainer.json 検索
     ├─ 見つかった → 従来フロー（Pattern検出→ポート割当→コピー→起動）
     │                 マーカーファイル更新（ConfigPattern=実パターン）
     └─ 見つからない → 成功メッセージ（Branch, Path のみ）
```

## R-004: start/stop コマンドの ConfigPattern=none ハンドリング

### Decision: findEnvironment をマーカーファイル対応に拡張

現在の `findEnvironment()` は Docker ラベルのみから環境を検索する。devcontainer なしの環境はコンテナがないため見つからない。マーカーファイルからも検索するように拡張する。

### Rationale

`start` / `stop` コマンドで devcontainer なしの環境名を指定された場合、「環境が見つかりません」（exit code 6）ではなく「この環境にはコンテナがありません」という適切なメッセージを返すべき。

## R-005: Docker 未起動時の動作

### Decision: devcontainer なしの場合は Docker 接続を省略

現在の `create` コマンドは早い段階で Docker クライアント接続を行い、失敗すると exit code 3 で終了する。devcontainer なしの場合、Docker は不要なので接続自体をスキップする。

### Rationale

devcontainer.json のないリポジトリでは Docker が起動していなくても worktree を作成できるべき（仕様 Assumptions に明記）。ただし、`create` コマンドの現在の処理順序では Docker 接続が worktree 作成より前に行われる可能性がある。処理順序の調整が必要。

### 設計

```
改修後の create コマンド処理順序:
  1. リポジトリルート取得（Git のみ）
  2. 環境名・ワークツリーパス決定
  3. Git worktree 作成
  4. マーカーファイル配置
  5. devcontainer.json 検索
     ├─ 見つかった → Docker 接続 → 従来フロー
     └─ 見つからない → Docker 不要、即座に成功
```
