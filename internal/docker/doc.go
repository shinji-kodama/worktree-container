// Package docker provides Docker Engine API wrappers and container
// lifecycle management for the worktree-container CLI.
//
// This package handles:
//   - Docker client initialization with automatic socket detection
//     (Linux, macOS, Windows)
//   - Container label management for persisting worktree metadata
//     (Docker labels are the sole state storage mechanism — FR-011)
//   - Container lifecycle operations: list, start, stop, remove
//   - Docker Compose operations: up, stop, down
//
// The package uses github.com/docker/docker/client as the underlying
// Docker SDK, with version negotiation enabled for broad compatibility.
package docker
