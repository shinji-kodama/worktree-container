# CLI コマンドスキーマ: Worktree Container CLI

**ブランチ**: `001-worktree-container-cli` | **日付**: 2026-02-28

## コマンド概要

```
worktree-container <command> [flags]

Commands:
  create    新しいワークツリー環境を作成・起動する
  list      ワークツリー環境の一覧を表示する
  start     停止中のワークツリー環境を再起動する
  stop      稼働中のワークツリー環境を停止する
  remove    ワークツリー環境を削除する

Global Flags:
  --json            JSON 形式で出力する
  --verbose, -v     詳細なログを出力する
  --help, -h        ヘルプを表示する
  --version         バージョンを表示する
```

## コマンド詳細

### `worktree-container create`

新しい Git ワークツリーを作成し、そのワークツリー専用の Dev Container 環境を起動する。

```
Usage:
  worktree-container create <branch-name> [flags]

Args:
  branch-name    作成するブランチ名（既存ブランチ名も指定可）

Flags:
  --base <ref>       ワークツリーのベースとなるコミット/ブランチ（デフォルト: HEAD）
  --path <dir>       ワークツリーの作成先パス（デフォルト: ../<repo>-<branch-name>）
  --name <name>      ワークツリー環境の識別名（デフォルト: <branch-name>）
  --no-start         ワークツリー作成のみ行い、コンテナは起動しない
```

**出力（テキスト）**:
```
Created worktree environment "feature-auth"
  Branch:    feature/auth
  Path:      /Users/user/myproject-feature-auth
  Pattern:   compose-multi (3 services)

  Services:
    app     http://localhost:13000  (container: 3000)
    db      localhost:15432         (container: 5432)
    redis   localhost:16379         (container: 6379)
```

**出力（JSON）**:
```json
{
  "name": "feature-auth",
  "branch": "feature/auth",
  "worktreePath": "/Users/user/myproject-feature-auth",
  "status": "running",
  "configPattern": "compose-multi",
  "services": [
    {
      "name": "app",
      "containerPort": 3000,
      "hostPort": 13000,
      "protocol": "tcp"
    },
    {
      "name": "db",
      "containerPort": 5432,
      "hostPort": 15432,
      "protocol": "tcp"
    },
    {
      "name": "redis",
      "containerPort": 6379,
      "hostPort": 16379,
      "protocol": "tcp"
    }
  ]
}
```

**終了コード**:
| コード | 意味 |
|--------|------|
| 0 | 成功 |
| 1 | 一般エラー |
| 2 | devcontainer.json が見つからない |
| 3 | Docker が起動していない |
| 4 | ポート割り当て失敗（衝突回避不可） |
| 5 | Git 操作エラー |

---

### `worktree-container list`

全ワークツリー環境の一覧を表示する。

```
Usage:
  worktree-container list [flags]

Flags:
  --status <status>  フィルタ: running / stopped / orphaned / all（デフォルト: all）
```

**出力（テキスト）**:
```
NAME           BRANCH          STATUS    SERVICES  PORTS
feature-auth   feature/auth    running   3         13000,15432,16379
bugfix-login   bugfix/login    stopped   1         -
old-branch     old/branch      orphaned  0         -
```

**出力（JSON）**:
```json
{
  "environments": [
    {
      "name": "feature-auth",
      "branch": "feature/auth",
      "status": "running",
      "worktreePath": "/Users/user/myproject-feature-auth",
      "configPattern": "compose-multi",
      "services": [
        {
          "name": "app",
          "containerPort": 3000,
          "hostPort": 13000
        }
      ]
    }
  ]
}
```

**終了コード**:
| コード | 意味 |
|--------|------|
| 0 | 成功 |
| 1 | 一般エラー |
| 3 | Docker が起動していない |

---

### `worktree-container start`

停止中のワークツリー環境のコンテナを再起動する。

```
Usage:
  worktree-container start <name> [flags]
```

**出力（テキスト）**:
```
Started worktree environment "feature-auth"

  Services:
    app     http://localhost:13000  (container: 3000)
    db      localhost:15432         (container: 5432)
    redis   localhost:16379         (container: 6379)
```

**終了コード**:
| コード | 意味 |
|--------|------|
| 0 | 成功 |
| 1 | 一般エラー |
| 3 | Docker が起動していない |
| 4 | ポート割り当て失敗（以前のポートが他プロセスに占有された場合） |
| 6 | 指定された環境が見つからない |

---

### `worktree-container stop`

稼働中のワークツリー環境のコンテナを停止する。

```
Usage:
  worktree-container stop <name> [flags]
```

**出力（テキスト）**:
```
Stopped worktree environment "feature-auth" (3 containers)
```

**終了コード**:
| コード | 意味 |
|--------|------|
| 0 | 成功 |
| 1 | 一般エラー |
| 3 | Docker が起動していない |
| 6 | 指定された環境が見つからない |

---

### `worktree-container remove`

ワークツリー環境を削除する。コンテナ、ネットワーク、ワークツリー専用ボリュームを削除し、
オプションで Git ワークツリーも削除する。

```
Usage:
  worktree-container remove <name> [flags]

Flags:
  --force, -f         確認なしで削除する
  --keep-worktree     Git ワークツリーは削除せず保持する
```

**出力（テキスト）**:
```
Remove worktree environment "feature-auth"? [y/N]: y
  Removed 3 containers
  Removed 1 network
  Removed 2 volumes
  Removed git worktree at /Users/user/myproject-feature-auth
```

**終了コード**:
| コード | 意味 |
|--------|------|
| 0 | 成功 |
| 1 | 一般エラー |
| 3 | Docker が起動していない |
| 5 | Git 操作エラー |
| 6 | 指定された環境が見つからない |
| 7 | ユーザーがキャンセルした |

## エラーメッセージ規約

- エラーメッセージは stderr に出力する
- `--json` フラグ指定時はエラーも JSON 形式で出力する:
  ```json
  {
    "error": {
      "code": "DOCKER_NOT_RUNNING",
      "message": "Docker daemon is not running. Please start Docker and try again."
    }
  }
  ```
- CLI ヘルプテキストは英語（憲法原則 VIII の例外規定に準拠）
