// Package devcontainer handles parsing, rewriting, and validation of
// devcontainer.json configuration files for the worktree-container CLI.
//
// This package supports all four devcontainer.json configuration patterns:
//
//   - Pattern A (image): Direct container image reference
//   - Pattern B (dockerfile): Builds from a Dockerfile
//   - Pattern C (compose-single): Docker Compose with one service
//   - Pattern D (compose-multi): Docker Compose with multiple services
//
// The original devcontainer.json is NEVER modified (FR-012). Instead,
// the package generates modified copies in the worktree directory.
// For Pattern A/B, it rewrites the JSON directly. For Pattern C/D,
// it generates a docker-compose override YAML file.
//
// JSONC (JSON with Comments) is supported via github.com/tidwall/jsonc,
// ensuring compatibility with the common practice of commenting
// devcontainer.json files.
package devcontainer
