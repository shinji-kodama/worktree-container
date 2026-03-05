# Loam

*Fertile soil for your worktrees* 🌱

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A CLI tool that automatically builds isolated Dev Container environments for each Git worktree.

When developing across multiple branches simultaneously, you can create a fully isolated Dev Container environment for each worktree with a single command -- no need to worry about port conflicts or container collisions.
Delegate work on other branches to coding agents (such as Claude Code) while continuing to use your main branch's development environment without interruption.

## Features

- **Zero Port Collision Guarantee** -- No port conflicts even with up to 10 environments running simultaneously. The port-shift algorithm handles all port assignments automatically
- **All 4 devcontainer.json Patterns Supported** -- Works with image references, Dockerfile builds, single-service Compose, and multi-service Compose configurations
- **Dual-Source State Management** -- Docker container labels store runtime metadata, while lightweight `.loam` marker files in each worktree enable fast environment discovery without querying Docker
- **Multiple Tool Support** -- Connect from VS Code Dev Containers, Dev Container CLI, or DevPod
- **Cross-Platform** -- Supports macOS, Linux, and Windows
- **Non-Destructive to Original Config** -- The original project's devcontainer.json is read-only. A copy is generated in the worktree and modified there

## Installation

### Homebrew (macOS / Linux)

```bash
brew install mmr-tortoise/tap/loam
```

### go install

```bash
go install github.com/mmr-tortoise/loam/cmd/loam@latest
```

### Build from Source

```bash
git clone https://github.com/mmr-tortoise/loam.git
cd loam
go build -o loam ./cmd/loam
```

### WinGet (Windows)

```powershell
winget install mmr-tortoise.loam
```

## Prerequisites

- Docker Engine or Docker Desktop must be running
- Git >= 2.15
- The target project must contain a `.devcontainer/devcontainer.json`

## Quick Start

### 1. Create a Worktree Environment

Run from the root of a project that has a devcontainer.json.

```bash
# Create a worktree environment on a new branch
loam create feature-auth

# Create a worktree environment based on an existing branch
loam create --base main bugfix-login

# Specify the destination path
loam create --path ~/dev/feature-auth feature-auth
```

### 2. List Worktree Environments

```bash
# Text output
loam list

# JSON format
loam list --json

# Show only running environments
loam list --status running
```

### 3. Stop and Restart Worktree Environments

```bash
# Stop
loam stop feature-auth

# Restart
loam start feature-auth
```

### 4. Remove a Worktree Environment

```bash
# Remove with interactive confirmation
loam remove feature-auth

# Remove without confirmation
loam remove --force feature-auth

# Remove containers only, keeping the Git worktree
loam remove --keep-worktree feature-auth
```

## Command Reference

```
loam <command> [flags]

Commands:
  create    Create and start a new worktree environment
  list      List worktree environments
  start     Restart a stopped worktree environment
  stop      Stop a running worktree environment
  remove    Remove a worktree environment

Global Flags:
  --json            Output in JSON format
  --verbose, -v     Enable verbose logging
  --help, -h        Show help
  --version         Show version
```

### `loam create`

Creates a new Git worktree and launches a dedicated Dev Container environment for it.

```
loam create <branch-name> [flags]

Flags:
  --base <ref>       Base commit/branch for the worktree (default: HEAD)
  --path <dir>       Destination path for the worktree (default: ../<repo>-<branch-name>)
  --name <name>      Identifier for the worktree environment (default: <branch-name>)
  --no-start         Create the worktree only without starting containers
```

**Example Output (Text):**

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

**Example Output (JSON):**

```json
{
  "name": "feature-auth",
  "branch": "feature/auth",
  "worktreePath": "/Users/user/myproject-feature-auth",
  "status": "running",
  "configPattern": "compose-multi",
  "services": [
    { "name": "app", "containerPort": 3000, "hostPort": 13000, "protocol": "tcp" },
    { "name": "db", "containerPort": 5432, "hostPort": 15432, "protocol": "tcp" },
    { "name": "redis", "containerPort": 6379, "hostPort": 16379, "protocol": "tcp" }
  ]
}
```

### `loam list`

Lists all worktree environments.

```
loam list [flags]

Flags:
  --status <status>  Filter: running / stopped / orphaned / all (default: all)
```

**Example Output:**

```
NAME           BRANCH          STATUS    SERVICES  PORTS
feature-auth   feature/auth    running   3         13000,15432,16379
bugfix-login   bugfix/login    stopped   1         -
old-branch     old/branch      orphaned  0         -
```

### `loam stop`

Stops the containers of a running worktree environment.

```
loam stop <name>
```

### `loam start`

Restarts the containers of a stopped worktree environment.

```
loam start <name>
```

### `loam remove`

Removes a worktree environment. Deletes containers, networks, and worktree-dedicated volumes,
and optionally removes the Git worktree as well.

```
loam remove <name> [flags]

Flags:
  --force, -f         Remove without confirmation
  --keep-worktree     Keep the Git worktree instead of removing it
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | devcontainer.json not found |
| 3 | Docker is not running |
| 4 | Port allocation failure |
| 5 | Git operation error |
| 6 | Specified environment not found |
| 7 | Cancelled by user |

## Port Management

Loam automatically assigns host-side ports for each worktree environment using a port-shift algorithm.

### Port-Shift Algorithm

```
shiftedPort = originalPort + (worktreeIndex * 10000)
```

| Environment | Base Port 3000 | Base Port 5432 | Base Port 6379 |
|-------------|----------------|----------------|----------------|
| Original (index 0) | 3000 | 5432 | 6379 |
| Worktree 1 | 13000 | 15432 | 16379 |
| Worktree 2 | 23000 | 25432 | 26379 |
| Worktree 3 | 33000 | 35432 | 36379 |

### Collision Avoidance

1. If a shifted port exceeds 65535, an available port is dynamically discovered
2. Ports in use by other processes are detected via `net.Listen()` and automatically avoided
3. Ports in use by other worktree environments are detected from Docker labels

Users never need to manually specify port numbers.
Use `loam list` to check the access endpoints for each environment.

## Supported devcontainer.json Patterns

### Pattern A: Image Reference

Specifies a Docker image directly using the `image` field.

```json
{
  "name": "My Project",
  "image": "mcr.microsoft.com/devcontainers/typescript-node:18",
  "forwardPorts": [3000]
}
```

### Pattern B: Dockerfile Build

Builds from a Dockerfile using the `build` field.

```json
{
  "name": "My Project",
  "build": {
    "dockerfile": "Dockerfile",
    "context": ".."
  },
  "forwardPorts": [3000, 5000]
}
```

### Pattern C: Docker Compose Single Service

Uses Docker Compose via the `dockerComposeFile` field with a single service.

```json
{
  "name": "My Project",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace"
}
```

### Pattern D: Docker Compose Multiple Services

Uses Docker Compose via the `dockerComposeFile` field with two or more services.
Supports configurations such as app + database + cache.

```json
{
  "name": "My Project",
  "dockerComposeFile": "docker-compose.yml",
  "service": "app",
  "workspaceFolder": "/workspace"
}
```

```yaml
# docker-compose.yml
services:
  app:
    build: .
    ports:
      - "3000:3000"
  db:
    image: postgres:16
    ports:
      - "5432:5432"
  redis:
    image: redis:7
    ports:
      - "6379:6379"
```

## Compatible Tools

After creating a worktree environment, you can connect to the container using any of the following methods.

### VS Code

1. Open the worktree folder in VS Code
2. Command Palette -> "Reopen in Container"

### Dev Container CLI

```bash
devcontainer up --workspace-folder /path/to/worktree
devcontainer exec --workspace-folder /path/to/worktree bash
```

### DevPod

```bash
devpod up /path/to/worktree
```

## Development

### Prerequisites

- Go 1.25
- Docker Engine or Docker Desktop
- Git >= 2.15

### Build

```bash
go build -o loam ./cmd/loam
```

### Test

```bash
# Unit tests
go test ./internal/...

# Integration tests (requires Docker)
go test -tags=integration ./tests/integration/...

# All tests
go test ./...
```

### Lint

```bash
golangci-lint run
```

### Release (GoReleaser)

```bash
goreleaser release --snapshot --clean
```

For detailed development instructions, see [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

[MIT License](./LICENSE)

Copyright (c) 2026 mmr-tortoise
