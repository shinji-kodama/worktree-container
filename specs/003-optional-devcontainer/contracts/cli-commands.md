# CLI コマンドコントラクト: devcontainer オプショナル化

**Branch**: `003-optional-devcontainer` | **Date**: 2026-03-01

## create コマンド（変更）

### 入力

変更なし。既存のフラグ・引数をそのまま使用。

### 出力（devcontainer なし）

**テキスト形式**:

```
Created worktree environment "feature-auth"
  Branch:    feature/auth
  Path:      /Users/user/myproject-feature-auth
```

**JSON 形式**:

```json
{
  "name": "feature-auth",
  "branch": "feature/auth",
  "worktreePath": "/Users/user/myproject-feature-auth",
  "status": "no-container",
  "configPattern": "none",
  "services": []
}
```

### 出力（devcontainer あり）

変更なし。既存の出力フォーマットを維持。

### 終了コード

| コード | 変更 | 意味 |
|--------|------|------|
| 0 | 変更なし | 成功（devcontainer あり・なし両方） |
| 2 | **意味変更** | devcontainer.json のパースエラー（見つからない場合はエラーではなくなる） |
| 3 | 変更なし | Docker 未起動（devcontainer なしの場合は発生しない） |
| 5 | 変更なし | Git 操作エラー |

## list コマンド（変更）

### 出力（devcontainer なし環境を含む場合）

**テキスト形式**:

```
NAME           BRANCH          STATUS         SERVICES  PORTS
feature-auth   feature/auth    running        3         13000,15432,16379
bugfix-login   bugfix/login    no-container   0         -
```

**JSON 形式**:

```json
[
  {
    "name": "feature-auth",
    "branch": "feature/auth",
    "worktreePath": "/Users/user/myproject-feature-auth",
    "status": "running",
    "configPattern": "compose-multi",
    "services": [
      { "name": "app", "containerPort": 3000, "hostPort": 13000, "protocol": "tcp" }
    ]
  },
  {
    "name": "bugfix-login",
    "branch": "bugfix/login",
    "worktreePath": "/Users/user/myproject-bugfix-login",
    "status": "no-container",
    "configPattern": "none",
    "services": []
  }
]
```

## start コマンド（変更）

### devcontainer なし環境に対して実行した場合

**テキスト出力**:

```
Environment "bugfix-login" has no container configuration.
To add containers, create a .devcontainer/devcontainer.json in the worktree.
```

**終了コード**: 0（エラーではない）

## stop コマンド（変更）

### devcontainer なし環境に対して実行した場合

**テキスト出力**:

```
Environment "bugfix-login" has no container configuration. Nothing to stop.
```

**終了コード**: 0（エラーではない）

## remove コマンド（変更）

### devcontainer なし環境の削除

Docker コンテナ削除をスキップし、以下のみ実行:
1. 確認プロンプト（`--force` でスキップ可能）
2. マーカーファイル削除
3. Git worktree 削除（`--keep-worktree` でスキップ可能）

**テキスト出力**:

```
Removed worktree environment "bugfix-login"
  Worktree removed: /Users/user/myproject-bugfix-login
```

## マーカーファイルコントラクト（新規）

### ファイル名

`.worktree-container`

### パス

`<worktreePath>/.worktree-container`

### JSON スキーマ

```json
{
  "managedBy": "worktree-container",
  "name": "<environment-name>",
  "branch": "<git-branch>",
  "sourceRepoPath": "<absolute-path-to-source-repo>",
  "configPattern": "none|image|dockerfile|compose-single|compose-multi",
  "createdAt": "<RFC3339-timestamp>"
}
```

### ライフサイクル

| 操作 | マーカーファイルへの影響 |
|------|------------------------|
| `create` | 作成（worktree 作成直後） |
| `list` | 読み取りのみ |
| `start` | 読み取りのみ |
| `stop` | 変更なし |
| `remove` | worktree 削除時に自動削除 |
