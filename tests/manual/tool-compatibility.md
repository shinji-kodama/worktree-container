# Dev Container Tool Compatibility Test Procedure

**Last updated**: 2026-02-28

## Prerequisites

- Docker Desktop is running
- Git >= 2.15 is installed
- The `loam` binary has been built
- The following tools under test are installed:
  - VS Code + Dev Containers extension
  - Dev Container CLI (`npm install -g @devcontainers/cli`)
  - DevPod (`brew install devpod`)

## Test Scenarios

### Scenario 1: Pattern A (Image) — VS Code

```bash
# 1. Create a worktree environment in the test repository
cd /path/to/test-repo
loam create test-vscode-image

# 2. Verify the generated devcontainer.json
cat ../test-repo-test-vscode-image/.devcontainer/devcontainer.json

# 3. Open in VS Code
code ../test-repo-test-vscode-image

# 4. In VS Code, press Cmd/Ctrl+Shift+P → "Reopen in Container"

# Verification items:
# [ ] Container starts successfully
# [ ] Ports are correctly shifted
# [ ] Environment variable WORKTREE_NAME is set
# [ ] Filesystem is mounted
```

### Scenario 2: Pattern B (Dockerfile) — Dev Container CLI

```bash
# 1. Create a worktree environment
cd /path/to/test-repo-with-dockerfile
loam create test-devcontainer-cli

# 2. Start with Dev Container CLI
devcontainer up --workspace-folder ../test-repo-with-dockerfile-test-devcontainer-cli

# 3. Execute a command inside the container
devcontainer exec --workspace-folder ../test-repo-with-dockerfile-test-devcontainer-cli bash

# Verification items:
# [ ] Build from Dockerfile succeeds
# [ ] Ports are correctly shifted
# [ ] Relative paths (build.dockerfile, build.context) are resolved
# [ ] Labels from runArgs are applied
```

### Scenario 3: Pattern C (Compose Single) — DevPod

```bash
# 1. Create a worktree environment
cd /path/to/test-repo-with-compose
loam create test-devpod-compose

# 2. Start with DevPod
devpod up ../test-repo-with-compose-test-devpod-compose

# Verification items:
# [ ] docker-compose.worktree.yml is generated
# [ ] Override is added to dockerComposeFile in devcontainer.json
# [ ] Compose project name matches the environment name
# [ ] Ports are correctly shifted
# [ ] shutdownAction: stopCompose is configured
```

### Scenario 4: Pattern D (Compose Multi) — All Tools

```bash
# 1. Create a worktree environment
cd /path/to/test-repo-with-multi-compose
loam create test-multi

# 2. Verify the generated files
cat ../test-repo-test-multi/.devcontainer/devcontainer.json
cat ../test-repo-test-multi/.devcontainer/docker-compose.worktree.yml

# 3. Test startup with each tool
# VS Code:
code ../test-repo-test-multi  # → Reopen in Container

# Dev Container CLI:
devcontainer up --workspace-folder ../test-repo-test-multi

# DevPod:
devpod up ../test-repo-test-multi

# Verification items:
# [ ] All services (app, db, redis) start successfully
# [ ] Ports for each service are correctly shifted
# [ ] DNS resolution between services works (app → db:5432)
# [ ] Volumes are isolated per environment
# [ ] Labels are applied to all containers
```

### Scenario 5: Two Environments Running Simultaneously

```bash
# 1. Create two environments
loam create feature-a
loam create feature-b

# 2. Check the environment list
loam list

# 3. Verify there are no port conflicts
loam list --json | jq '.environments[].services[].hostPort'

# Verification items:
# [ ] Both environments are in running state simultaneously
# [ ] No port duplications at all
# [ ] Each environment is individually accessible
# [ ] Both environments appear in the list command output
```

## Troubleshooting

### VS Code does not show "Reopen in Container"
- Verify that `.devcontainer/devcontainer.json` exists
- Verify that the Dev Containers extension is installed

### DevPod does not detect devcontainer.json
- Specify explicitly with `devpod up <path> --devcontainer-path .devcontainer/devcontainer.json`
- Check DevPod logs: `devpod provider logs`

### Ports differ from expectations
- Check port mappings with `loam list --json`
- Check actual ports with `docker ps --format 'table {{.Names}}\t{{.Ports}}'`

### Communication between Compose services fails
- Verify that the network is created with `docker network ls`
- Check service status with `docker compose -f ... -f docker-compose.worktree.yml ps`
- Verify DNS resolution by service name with `docker exec <container> nslookup <service>`
