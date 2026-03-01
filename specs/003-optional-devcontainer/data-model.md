# Data Model: devcontainer オプショナル化

**Branch**: `003-optional-devcontainer` | **Date**: 2026-03-01

## エンティティ変更

### ConfigPattern（変更）

既存の列挙型に `none` を追加。

| 値 | 説明 | 変更種別 |
|----|------|---------|
| `image` | devcontainer.json の image フィールドで Docker イメージを直接指定 | 既存 |
| `dockerfile` | devcontainer.json の build フィールドで Dockerfile からビルド | 既存 |
| `compose-single` | Docker Compose 使用、サービス 1 つ | 既存 |
| `compose-multi` | Docker Compose 使用、サービス 2 つ以上 | 既存 |
| **`none`** | **devcontainer.json なし。Git worktree のみ** | **新規** |

**IsCompose() への影響**: `none` は Compose パターンではないため、既存の `IsCompose()` は変更不要。

### WorktreeEnv（変更）

既存のフィールド構成を維持。`ConfigPattern` が `none` の場合の各フィールドの値:

| フィールド | ConfigPattern=none の場合の値 |
|-----------|------------------------------|
| `Name` | 環境名（通常通り） |
| `Branch` | Git ブランチ名（通常通り） |
| `WorktreePath` | ワークツリーの絶対パス（通常通り） |
| `SourceRepoPath` | ソースリポジトリの絶対パス（通常通り） |
| `Status` | `no-container` |
| `ConfigPattern` | `none` |
| `Containers` | 空（`[]`） |
| `PortAllocations` | 空（`[]`） |
| `CreatedAt` | 作成日時（通常通り） |

### WorktreeStatus（変更）

新しいステータス値を追加。

| 値 | 説明 | 変更種別 |
|----|------|---------|
| `running` | コンテナが稼働中 | 既存 |
| `stopped` | コンテナが停止中 | 既存 |
| `orphaned` | ワークツリーが削除済みだがコンテナが残っている | 既存 |
| **`no-container`** | **コンテナなし（devcontainer 未設定）** | **新規** |

### マーカーファイル（新規）

**ファイル名**: `.worktree-container`（注: リネームは本フィーチャーのスコープ外。別ブランチで対応予定）
**配置場所**: ワークツリーのルートディレクトリ

| フィールド | 型 | 説明 |
|-----------|-----|------|
| `managedBy` | string | ツール識別子（`"worktree-container"`） |
| `name` | string | 環境名 |
| `branch` | string | Git ブランチ名 |
| `sourceRepoPath` | string | ソースリポジトリの絶対パス |
| `configPattern` | string | ConfigPattern の値（`none`, `image`, `dockerfile`, `compose-single`, `compose-multi`） |
| `createdAt` | string | RFC3339 形式の作成日時 |

**JSON 形式の例（devcontainer なし）**:

```json
{
  "managedBy": "worktree-container",
  "name": "feature-auth",
  "branch": "feature/auth",
  "sourceRepoPath": "/Users/user/myproject",
  "configPattern": "none",
  "createdAt": "2026-03-01T10:00:00Z"
}
```

**JSON 形式の例（devcontainer あり）**:

```json
{
  "managedBy": "worktree-container",
  "name": "feature-auth",
  "branch": "feature/auth",
  "sourceRepoPath": "/Users/user/myproject",
  "configPattern": "compose-multi",
  "createdAt": "2026-03-01T10:00:00Z"
}
```

## 状態遷移

### ワークツリー環境のライフサイクル

```
create（devcontainer なし）:
  → [no-container] ──remove──→ [削除済み]

create（devcontainer あり）:
  → [running] ──stop──→ [stopped] ──start──→ [running]
                                   ──remove──→ [削除済み]
                         ──remove──→ [削除済み]
```

## Docker ラベルスキーマ（変更なし）

既存の Docker ラベルスキーマは変更しない。devcontainer なしのワークツリーは Docker コンテナを持たないため、ラベルは存在しない。既存のラベルスキーマは devcontainer あり環境でのみ使用される。

## list コマンドのデータソースマージ

```
データソース 1: Docker ラベル（既存）
  → コンテナ付き環境の検出
  → ConfigPattern: image/dockerfile/compose-single/compose-multi

データソース 2: git worktree list + マーカーファイル（新規）
  → 全ワークツリーのパスを取得
  → 各パスでマーカーファイルを検索
  → マーカーファイルから環境情報を読み取り

マージルール:
  Docker ラベルとマーカーファイルの両方で検出 → Docker ラベル優先
  マーカーファイルのみ → ConfigPattern=none, Status=no-container
  Docker ラベルのみ → 従来通り（後方互換性）
```
