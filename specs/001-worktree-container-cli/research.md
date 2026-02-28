# リサーチ結果: Worktree Container CLI

**ブランチ**: `001-worktree-container-cli` | **日付**: 2026-02-28
**仕様**: [spec.md](./spec.md)

## 1. 実装言語の選定

### 決定: Go

**根拠**:
- Docker 自体が Go で記述されており、Docker SDK（`docker/docker/client`）が最も成熟している
- クロスコンパイルが `GOOS=darwin/linux GOARCH=amd64/arm64` のみで完結
- GoReleaser + Homebrew Tap パイプラインが確立されており、`brew install` 配布が容易
- CLI フレームワーク（cobra）が Go エコシステムの事実上の標準
- 単一バイナリ配布、高速な起動時間

**評価した代替案**:

| 言語 | 評価 | 不採用理由 |
|------|------|-----------|
| Rust | MEDIUM | Docker Compose オーケストレーション層が存在しない。devcontainer.json パーサークレートがない。git2 が libgit2（C依存）を必要とし、クロスコンパイルが複雑化 |
| Python | LOW | 単一バイナリ配布が困難。起動時間が遅い。Homebrew 配布が複雑 |
| TypeScript/Deno | LOW | ネイティブバイナリ生成が不安定。Docker SDK が成熟していない |
| Swift | LOW | Linux サポートが限定的。Docker SDK が存在しない |

### 主要ライブラリ

| 用途 | ライブラリ | 理由 |
|------|-----------|------|
| CLI フレームワーク | `github.com/spf13/cobra` | Go CLI の事実上の標準。サブコマンド、フラグ、ヘルプ生成 |
| Docker API | `github.com/docker/docker/client` | 公式 Docker SDK。コンテナ・ネットワーク・ボリューム操作 |
| Compose ファイル解析 | `github.com/compose-spec/compose-go/v2` | 公式 Compose 仕様パーサー。マージ戦略対応 |
| JSONC パーサー | `github.com/tidwall/jsonc` | devcontainer.json のコメント付き JSON 解析 |
| JSON 操作 | `encoding/json`（標準ライブラリ） | JSON シリアライズ/デシリアライズ |
| Git 操作 | `os/exec`（標準ライブラリ） | `git worktree add/list/remove` の実行。外部依存なし |
| テスト | `testing`（標準ライブラリ）+ `testify` | ユニット・統合テスト |
| ビルド・配布 | GoReleaser + Homebrew Tap | クロスコンパイル + `brew install` 対応 |

## 2. ポート管理方式

### 決定: ポートシフト（Method A）

**根拠**:
- PostgreSQL（5432）、Redis（6379）等の非 HTTP プロトコルはホスト名ベースルーティングに TLS（SNI）が必要で、開発環境では非実用的
- macOS で `*.localhost` DNS 解決を機能させるには dnsmasq + sudo 権限が必要で、UX を損なう
- ポートシフトは `docker-compose.override.yml` の `ports` セクションで容易に実現可能
- 憲法原則 VI（簡潔性・YAGNI）に最も適合

**評価した代替案**:

| 方式 | 評価 | 不採用理由 |
|------|------|-----------|
| リバースプロキシ（Traefik/Caddy） | 不採用 | 非 HTTP プロトコルの制約。macOS DNS 設定が煩雑。追加コンテナのリソース消費 |
| ハイブリッド（HTTP はプロキシ、非 HTTP はシフト） | 延期 | 複雑性が MVP に見合わない。将来的な拡張オプションとして保留 |

### ポートシフト戦略の詳細

- **割り当てアルゴリズム**: ワークツリーインデックス × オフセット（例: ベースポート 3000 → 環境1: 3000, 環境2: 13000, 環境3: 23000）
- **衝突検出**: `net.Listen()` で事前にポート使用状況を確認
- **ホスト側ポート制御方法**:
  - パターン A/B（単一コンテナ）: `appPort` の `"hostPort:containerPort"` 形式
  - パターン C/D（Compose）: Override YAML の `ports` セクション
- **メタデータ記録**: Docker コンテナラベルに元ポートとシフト後ポートの対応を記録

## 3. devcontainer.json 設定パターンと対応方針

### パターン分類

devcontainer.json には大きく4つの設定パターンが存在し、それぞれで書き換え対象フィールドと方法が異なる。

#### パターン A: 単一コンテナ（image 指定）

```json
{
  "name": "My Project",
  "image": "mcr.microsoft.com/devcontainers/typescript-node:20",
  "forwardPorts": [3000],
  "appPort": ["3000:3000"],
  "runArgs": ["--cap-add=SYS_PTRACE"]
}
```

**書き換え対象**:
- `name`: ワークツリー名を含む名前に変更
- `runArgs`: `--name`（コンテナ名衝突回避）、`--label`（メタデータ）を追加
- `appPort`: ホスト側ポートをシフト（例: `"13000:3000"`）
- `portsAttributes`: キーをシフト後ポートに変更。`requireLocalPort: true` → `false`
- `containerEnv`: ワークツリー固有の環境変数を追加

#### パターン B: 単一コンテナ（Dockerfile ビルド）

```json
{
  "name": "My Project",
  "build": {
    "dockerfile": "Dockerfile",
    "context": ".."
  },
  "forwardPorts": [3000, 5432],
  "appPort": ["3000:3000"]
}
```

**書き換え対象**: パターン A と同様に加え:
- `build.dockerfile`: 相対パスがワークツリーから正しく解決されることを検証
- `build.context`: 同上
- `workspaceMount`: 絶対パスの場合はワークツリーパスに書き換え

#### パターン C: Docker Compose（単一サービス）

```json
{
  "name": "My Project",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "forwardPorts": [3000],
  "shutdownAction": "stopCompose"
}
```

**書き換え対象**:
- `name`: ワークツリー名を含む名前に変更
- `dockerComposeFile`: 配列に変換し、override YAML を追加
- `forwardPorts`: シフト後ポートへの更新（ただし `"service:port"` 形式は維持可能）

**生成する Override YAML**:
```yaml
name: project-feature-branch
services:
  app:
    ports:
      - "13000:3000"
    labels:
      worktree.name: "feature-branch"
    volumes:
      - /path/to/worktree:/workspace
```

#### パターン D: Docker Compose（複数サービス）

```json
{
  "name": "Full Stack App",
  "dockerComposeFile": ["docker-compose.yml", "docker-compose.dev.yml"],
  "service": "app",
  "runServices": ["app", "db", "redis"],
  "forwardPorts": [3000, "db:5432", "redis:6379"]
}
```

**書き換え対象**: パターン C と同様に加え:
- Override YAML で全サービスのポートをシフト

**生成する Override YAML**:
```yaml
name: project-feature-branch
services:
  app:
    ports:
      - "13000:3000"
    labels:
      worktree.name: "feature-branch"
      worktree.original-ports: "3000,5432,6379"
    volumes:
      - /path/to/worktree:/workspace
  db:
    ports:
      - "15432:5432"
    labels:
      worktree.name: "feature-branch"
  redis:
    ports:
      - "16379:6379"
    labels:
      worktree.name: "feature-branch"
```

### パターン判別ロジック

```
devcontainer.json を読み込み:
  ├─ "dockerComposeFile" フィールドが存在する？
  │   ├─ YES → "runServices" に複数サービスがある、または
  │   │        Compose YAML に複数サービスが定義されている？
  │   │   ├─ YES → パターン D（Compose 複数サービス）
  │   │   └─ NO  → パターン C（Compose 単一サービス）
  │   └─ NO  → "build" フィールドが存在する？
  │       ├─ YES → パターン B（Dockerfile ビルド）
  │       └─ NO  → パターン A（image 指定）
```

### 重要な技術的発見

#### forwardPorts の制限
`forwardPorts` にはホスト側ポートを明示指定する構文がない。ホスト側ポート番号はツール（VS Code, Dev Container CLI）側の判断で決まる。そのため、ポート衝突回避には以下の方法を使用する:

- **パターン A/B**: `appPort`（`"hostPort:containerPort"` 形式）
- **パターン C/D**: Compose override YAML の `ports` セクション

#### Docker Compose のマージ戦略
`ports` はシーケンス型のため、override ファイルで定義すると**元の定義を完全に置換する**。したがって、override ファイルには元のポートマッピング（ホスト側のみ変更）もすべて含める必要がある。

#### COMPOSE_PROJECT_NAME による分離
`COMPOSE_PROJECT_NAME`（または YAML の `name` フィールド）を変更するだけで:
- コンテナ名が `{project-name}_{service}_{instance}` に変わる
- ネットワーク名が `{project-name}_default` に変わる
- 名前付きボリュームが `{project-name}_{volume}` に変わる
- **サービス間の DNS 解決は引き続き機能する**（サービス名は不変）

#### devcontainer CLI / VS Code / DevPod の互換性
ワークツリーの `.devcontainer/` にコピー済みの devcontainer.json を配置すれば、
3つのツールすべてで自動検出される:

| ツール | 認識方法 |
|--------|---------|
| Dev Container CLI | `devcontainer up --workspace-folder <worktree-path>` |
| VS Code | ワークツリーフォルダを開き「Reopen in Container」 |
| DevPod | `devpod up <worktree-path>` |

## 4. Docker ラベルによる状態管理

### ラベルスキーマ

```
worktree.managed-by: "worktree-container"
worktree.name: "<worktree-name>"
worktree.branch: "<branch-name>"
worktree.original-port.<container-port>: "<host-port>"
worktree.worktree-path: "<absolute-path>"
worktree.created-at: "<ISO-8601>"
```

### 状態検出の仕組み

`docker ps --filter label=worktree.managed-by=worktree-container` で本ツールが管理する
コンテナを検出し、ラベル情報からワークツリー環境の状態を動的に構築する。
外部状態ファイルは一切不要（FR-011 準拠）。

## 5. プロジェクト構造

### 決定: 単一プロジェクト構造

```text
cmd/
└── worktree-container/
    └── main.go              # エントリポイント

internal/
├── cli/                     # CLI コマンド定義（cobra）
│   ├── root.go
│   ├── create.go
│   ├── list.go
│   ├── start.go
│   ├── stop.go
│   └── remove.go
├── devcontainer/            # devcontainer.json 解析・生成
│   ├── config.go            # 設定パターン判別・読み込み
│   ├── rewrite.go           # ワークツリー用設定書き換え
│   └── compose.go           # Compose override YAML 生成
├── port/                    # ポート管理
│   ├── allocator.go         # ポート割り当てアルゴリズム
│   └── scanner.go           # 使用中ポートスキャン
├── worktree/                # Git ワークツリー操作
│   └── manager.go           # git worktree add/list/remove
├── docker/                  # Docker API ラッパー
│   ├── client.go            # Docker クライアント初期化
│   ├── container.go         # コンテナライフサイクル操作
│   └── label.go             # ラベル操作・状態検出
└── model/                   # ドメインモデル
    └── types.go             # WorktreeEnv, PortAllocation 等

tests/
├── unit/                    # ユニットテスト
├── integration/             # 統合テスト（Docker 必要）
└── testdata/                # テスト用 devcontainer.json サンプル
    ├── image-simple/
    ├── dockerfile-build/
    ├── compose-single/
    └── compose-multi/
```
