# Contributing Guide

We welcome contributions to Loam.
This document explains the steps from setting up your development environment to creating a pull request.

## Setting Up the Development Environment

### Required Tools

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.25 | Build & test |
| Docker Engine / Docker Desktop | Latest recommended | Container operations & integration tests |
| Git | >= 2.15 | Worktree functionality |
| golangci-lint | Latest recommended | Static analysis |
| GoReleaser | Latest recommended | Release builds (optional) |

### Cloning the Repository and Building

```bash
git clone https://github.com/mmr-tortoise/loam.git
cd loam
go mod download
go build -o loam ./cmd/loam
```

### Verifying the Installation

```bash
./loam --version
```

## How to Build

### Building the Binary

```bash
go build -o loam ./cmd/loam
```

### Cross-Compilation

```bash
# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o loam-darwin-arm64 ./cmd/loam

# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -o loam-linux-amd64 ./cmd/loam

# Windows (amd64)
GOOS=windows GOARCH=amd64 go build -o loam-windows-amd64.exe ./cmd/loam
```

### Snapshot Build with GoReleaser

```bash
goreleaser release --snapshot --clean
```

## Coding Conventions

### Go Comment Conventions

Since the project owner is not deeply experienced with Go, and to make it easier for external contributors to get involved in this open-source project, the following comment conventions apply.

#### 1. GoDoc Comments (All Exported Symbols)

All public (capitalized) functions, types, constants, and variables must have GoDoc-style comments.

```go
// Allocator manages port allocation for worktree environments.
// It ensures that no two environments share the same host port
// by tracking allocations via Docker container labels.
type Allocator struct { ... }
```

#### 2. Intent Explanation for Unexported Functions

Even for unexported functions, explain "what it does" and "why it does it."

```go
// shiftPort calculates the shifted host port for a given worktree index.
// We use an offset-based approach (index * 10000) to provide predictable,
// non-overlapping port ranges for up to 10 concurrent environments.
func shiftPort(originalPort, worktreeIndex int) int { ... }
```

#### 3. Explanation of Go Idioms

When using Go-specific concepts such as goroutines, channels, interfaces, defer, and error handling patterns, explain the purpose and behavior of the pattern.

```go
// Use defer to ensure the Docker client connection is always closed
// when this function returns, even if an error occurs midway.
// This is a standard Go pattern for resource cleanup.
defer client.Close()
```

#### 4. Step-by-Step Explanation of Complex Logic

Add step-by-step comments for complex algorithms and business logic.

#### Comment Language

Comments in source code must be written in **English**.

### File Paths

For Windows compatibility, always use `filepath.Join()` to construct file paths.
Avoid hardcoding `/`.

```go
// Good
configPath := filepath.Join(worktreePath, ".devcontainer", "devcontainer.json")

// Bad
configPath := worktreePath + "/.devcontainer/devcontainer.json"
```

### Commit Messages

Use the [Conventional Commits](https://www.conventionalcommits.org/) format.

```
feat: implement port-shift algorithm
fix: correct port rewriting in Compose override YAML
docs: add installation instructions to README.md
test: add unit tests for port collision detection
chore: update golangci-lint configuration
refactor: extract Docker label operations into helper functions
```

## PR Workflow

### 1. Create a Branch

Feature branches should follow the `###-feature-name` naming convention.

```bash
git checkout -b 002-port-allocator
```

### 2. Implement Changes

- Write tests first and confirm they fail before implementing (test-first approach)
- Follow the comment conventions and write detailed comments
- Ensure `go test ./...` and `golangci-lint run` pass

### 3. Commit

```bash
git add <changed files>
git commit -m "feat: <description of changes>"
```

### 4. Create a Pull Request

- Use Conventional Commits format for the PR title
- Describe the purpose and scope of the changes in the PR description
- Limit each PR to a single concern

### PR Checklist

- [ ] `go test ./...` passes
- [ ] `golangci-lint run` passes
- [ ] New public functions and types have GoDoc comments
- [ ] Unexported functions have comments explaining their intent
- [ ] Go idiom usage has explanatory comments about the pattern
- [ ] File paths use `filepath.Join()` (no hardcoded `/`)
- [ ] Commit messages follow Conventional Commits format

## Testing

### Unit Tests

```bash
# All unit tests
go test ./internal/...

# Tests for a specific package
go test ./internal/port/...
go test ./internal/devcontainer/...

# Run with coverage
go test -cover ./internal/...

# Verbose output
go test -v ./internal/...
```

### Integration Tests

Integration tests require Docker. They use the `//go:build integration` build tag.

```bash
# Integration tests (requires Docker)
go test -tags=integration ./tests/integration/...

# Run a specific test only
go test -tags=integration -run TestCreateCommand ./tests/integration/...
```

### Test Data

Sample devcontainer.json files for testing are located in `tests/testdata/`.

```
tests/testdata/
  image-simple/          Pattern A: image specification
  dockerfile-build/      Pattern B: Dockerfile build
  compose-single/        Pattern C: Compose single service
  compose-multi/         Pattern D: Compose multiple services
```

When adding new test cases, place sample files in the appropriate directory.

### Test Writing Guidelines

- Use `github.com/stretchr/testify` as the testing framework
- Table-driven tests are recommended
- When mocking is needed, define interfaces and create test implementations
- Integration tests must always have the `//go:build integration` tag

## Release Process

Releases use tag-based automated releases via GoReleaser.

### 1. Create a Version Tag

Follow [Semantic Versioning](https://semver.org/).

```bash
git tag v0.1.0
git push origin v0.1.0
```

### 2. Automated Release

Pushing a tag triggers the release workflow in GitHub Actions.

- GoReleaser generates binaries for macOS, Linux, and Windows
- Binaries and changelog are published to GitHub Releases
- Homebrew Tap is automatically updated

### 3. Versioning Policy

| Type | Example | Description |
|------|---------|-------------|
| MAJOR | v1.0.0 → v2.0.0 | Breaking changes |
| MINOR | v1.0.0 → v1.1.0 | Backward-compatible feature additions |
| PATCH | v1.0.0 → v1.0.1 | Bug fixes |

### Local Release Verification

```bash
goreleaser release --snapshot --clean
```

Binaries for each platform will be generated in the `dist/` directory.

## Project Structure

```
cmd/
  loam/
    main.go                  Entry point

internal/
  cli/                       CLI command definitions (cobra)
  devcontainer/              devcontainer.json parsing & generation
  port/                      Port management
  worktree/                  Git worktree operations
  docker/                    Docker API wrapper
  model/                     Domain models

tests/
  unit/                      Unit tests
  integration/               Integration tests (requires Docker)
  testdata/                  Sample devcontainer.json files for testing
```

## Questions & Support

If you have any questions, please ask via GitHub Issues.
