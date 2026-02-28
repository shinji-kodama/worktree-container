# データモデル: Worktree Container CLI

**ブランチ**: `001-worktree-container-cli` | **日付**: 2026-02-28

## エンティティ

### WorktreeEnv（ワークツリー環境）

ワークツリーとそれに紐づく Dev Container 環境のセットを表す。

| フィールド | 型 | 説明 | 制約 |
|-----------|-----|------|------|
| Name | string | ワークツリー環境の識別名 | 必須。英数字・ハイフン。一意 |
| Branch | string | Git ブランチ名 | 必須 |
| WorktreePath | string | ワークツリーの絶対パス | 必須。存在するディレクトリ |
| SourceRepoPath | string | 元リポジトリの絶対パス | 必須 |
| Status | WorktreeStatus | 環境の状態 | running / stopped / orphaned |
| ConfigPattern | ConfigPattern | devcontainer.json のパターン種別 | image / dockerfile / compose-single / compose-multi |
| Containers | []ContainerInfo | 環境に属するコンテナ一覧 | 1個以上 |
| PortAllocations | []PortAllocation | ポート割り当て一覧 | 0個以上 |
| CreatedAt | time.Time | 作成日時 | 必須。ISO-8601 |

**状態遷移**:
```
[作成] → running → stopped → [削除]
                ↑          ↓
                └──────────┘
                  (start)

running / stopped → orphaned（Git ワークツリーが手動削除された場合）
```

**一意性ルール**: Name はシステム内で一意。同名のワークツリー環境は作成不可。

### PortAllocation（ポート割り当て）

ワークツリー環境内の1つのポートマッピングを表す。

| フィールド | 型 | 説明 | 制約 |
|-----------|-----|------|------|
| ServiceName | string | コンテナ/サービス名 | 必須 |
| ContainerPort | int | コンテナ内ポート番号 | 必須。1-65535 |
| HostPort | int | ホスト側に割り当てたポート番号 | 必須。1024-65535 |
| Protocol | string | プロトコル | tcp / udp。デフォルト: tcp |
| Label | string | ポートの説明ラベル | 任意。portsAttributes.label から取得 |

**バリデーションルール**:
- HostPort は他のワークツリー環境のいずれの HostPort とも重複不可
- HostPort はシステム上の他プロセスが使用中のポートとも重複不可
- ContainerPort はコンテナ内ポートなので重複チェック不要

### DevContainerConfig（Dev Container 設定）

元の devcontainer.json から派生したワークツリー固有の設定を表す。

| フィールド | 型 | 説明 | 制約 |
|-----------|-----|------|------|
| OriginalPath | string | 元 devcontainer.json の絶対パス | 必須。読み取り専用 |
| WorktreePath | string | ワークツリー側の devcontainer.json パス | 必須 |
| Pattern | ConfigPattern | 設定パターン種別 | image / dockerfile / compose-single / compose-multi |
| OriginalPorts | []PortSpec | 元の設定に含まれるポート定義 | 0個以上 |
| ComposeFiles | []string | Docker Compose YAML ファイルパス一覧 | Compose パターン時のみ |
| PrimaryService | string | メインサービス名 | Compose パターン時のみ |
| AllServices | []string | 全サービス名一覧 | Compose パターン時のみ |
| OverrideYAMLPath | string | 生成した override YAML のパス | Compose パターン時のみ |

### ContainerInfo（コンテナ情報）

Docker コンテナの状態情報。Docker API から動的に取得。

| フィールド | 型 | 説明 | 制約 |
|-----------|-----|------|------|
| ContainerID | string | Docker コンテナ ID | 必須 |
| ContainerName | string | Docker コンテナ名 | 必須 |
| ServiceName | string | Compose サービス名（該当する場合） | 任意 |
| Status | string | Docker コンテナステータス | running / exited / created 等 |
| Labels | map[string]string | Docker ラベル一覧 | ワークツリーメタデータを含む |

## 列挙型

### WorktreeStatus

| 値 | 説明 |
|----|------|
| running | コンテナが稼働中 |
| stopped | コンテナが停止中（設定は保持） |
| orphaned | Git ワークツリーが存在しない（孤立状態） |

### ConfigPattern

| 値 | 説明 | 判別条件 |
|----|------|---------|
| image | image フィールドで直接指定 | `dockerComposeFile` なし、`build` なし |
| dockerfile | Dockerfile でビルド | `dockerComposeFile` なし、`build` あり |
| compose-single | Compose 単一サービス | `dockerComposeFile` あり、サービス1つ |
| compose-multi | Compose 複数サービス | `dockerComposeFile` あり、サービス2つ以上 |

## リレーションシップ

```
WorktreeEnv 1 ──── * ContainerInfo    （環境は1つ以上のコンテナを持つ）
WorktreeEnv 1 ──── * PortAllocation   （環境は0個以上のポート割り当てを持つ）
WorktreeEnv 1 ──── 1 DevContainerConfig（環境は1つの設定を持つ）
```

## Docker ラベルスキーマ（永続化）

外部状態ファイルの代わりに、Docker コンテナのラベルに全メタデータを記録する。

| ラベルキー | 値の例 | 説明 |
|-----------|--------|------|
| `worktree.managed-by` | `worktree-container` | 本ツール管理のコンテナを識別 |
| `worktree.name` | `feature-auth` | ワークツリー環境名 |
| `worktree.branch` | `feature/auth` | Git ブランチ名 |
| `worktree.worktree-path` | `/Users/user/repo-feature-auth` | ワークツリーの絶対パス |
| `worktree.source-repo` | `/Users/user/repo` | 元リポジトリの絶対パス |
| `worktree.original-port.3000` | `13000` | ポートマッピング（元 → シフト後） |
| `worktree.original-port.5432` | `15432` | 同上 |
| `worktree.config-pattern` | `compose-multi` | 設定パターン種別 |
| `worktree.created-at` | `2026-02-28T10:00:00Z` | 作成日時 |
