# Dev Container ツール互換性テスト手順書

**最終更新**: 2026-02-28

## 前提条件

- Docker Desktop が起動していること
- Git >= 2.15 がインストールされていること
- `worktree-container` バイナリがビルド済みであること
- テスト対象ツールがインストールされていること:
  - VS Code + Dev Containers 拡張機能
  - Dev Container CLI (`npm install -g @devcontainers/cli`)
  - DevPod (`brew install devpod`)

## テストシナリオ

### シナリオ 1: Pattern A (Image) — VS Code

```bash
# 1. テスト用リポジトリでワークツリー環境を作成
cd /path/to/test-repo
worktree-container create test-vscode-image

# 2. 生成された devcontainer.json を確認
cat ../test-repo-test-vscode-image/.devcontainer/devcontainer.json

# 3. VS Code で開く
code ../test-repo-test-vscode-image

# 4. VS Code で Cmd/Ctrl+Shift+P → "Reopen in Container"

# 検証項目:
# [ ] コンテナが起動する
# [ ] ポートが正しくシフトされている
# [ ] 環境変数 WORKTREE_NAME が設定されている
# [ ] ファイルシステムがマウントされている
```

### シナリオ 2: Pattern B (Dockerfile) — Dev Container CLI

```bash
# 1. ワークツリー環境を作成
cd /path/to/test-repo-with-dockerfile
worktree-container create test-devcontainer-cli

# 2. Dev Container CLI で起動
devcontainer up --workspace-folder ../test-repo-with-dockerfile-test-devcontainer-cli

# 3. コンテナ内でコマンド実行
devcontainer exec --workspace-folder ../test-repo-with-dockerfile-test-devcontainer-cli bash

# 検証項目:
# [ ] Dockerfile からビルドが成功する
# [ ] ポートが正しくシフトされている
# [ ] 相対パス（build.dockerfile, build.context）が解決される
# [ ] runArgs のラベルが適用されている
```

### シナリオ 3: Pattern C (Compose Single) — DevPod

```bash
# 1. ワークツリー環境を作成
cd /path/to/test-repo-with-compose
worktree-container create test-devpod-compose

# 2. DevPod で起動
devpod up ../test-repo-with-compose-test-devpod-compose

# 検証項目:
# [ ] docker-compose.worktree.yml が生成されている
# [ ] devcontainer.json の dockerComposeFile に override が追加されている
# [ ] Compose プロジェクト名が環境名になっている
# [ ] ポートが正しくシフトされている
# [ ] shutdownAction: stopCompose が設定されている
```

### シナリオ 4: Pattern D (Compose Multi) — 全ツール

```bash
# 1. ワークツリー環境を作成
cd /path/to/test-repo-with-multi-compose
worktree-container create test-multi

# 2. 生成されたファイルを確認
cat ../test-repo-test-multi/.devcontainer/devcontainer.json
cat ../test-repo-test-multi/.devcontainer/docker-compose.worktree.yml

# 3. 各ツールで起動テスト
# VS Code:
code ../test-repo-test-multi  # → Reopen in Container

# Dev Container CLI:
devcontainer up --workspace-folder ../test-repo-test-multi

# DevPod:
devpod up ../test-repo-test-multi

# 検証項目:
# [ ] 全サービス（app, db, redis）が起動する
# [ ] 各サービスのポートが正しくシフトされている
# [ ] サービス間のDNS解決が動作する（app → db:5432）
# [ ] ボリュームが環境ごとに分離されている
# [ ] ラベルが全コンテナに適用されている
```

### シナリオ 5: 2環境同時起動

```bash
# 1. 2つの環境を作成
worktree-container create feature-a
worktree-container create feature-b

# 2. 環境一覧を確認
worktree-container list

# 3. ポート衝突がないことを確認
worktree-container list --json | jq '.environments[].services[].hostPort'

# 検証項目:
# [ ] 2環境が同時に running 状態
# [ ] ポートが一切重複していない
# [ ] 各環境に個別にアクセスできる
# [ ] list コマンドで両方表示される
```

## トラブルシューティング

### VS Code が "Reopen in Container" を表示しない
- `.devcontainer/devcontainer.json` が存在するか確認
- Dev Containers 拡張機能がインストールされているか確認

### DevPod が devcontainer.json を検出しない
- `devpod up <path> --devcontainer-path .devcontainer/devcontainer.json` で明示指定
- DevPod のログを確認: `devpod provider logs`

### ポートが想定と異なる
- `worktree-container list --json` でポートマッピングを確認
- `docker ps --format 'table {{.Names}}\t{{.Ports}}'` で実際のポートを確認

### Compose サービス間の通信が失敗
- `docker network ls` でネットワークが作成されているか確認
- `docker compose -f ... -f docker-compose.worktree.yml ps` でサービス状態を確認
- サービス名でのDNS解決を `docker exec <container> nslookup <service>` で確認
