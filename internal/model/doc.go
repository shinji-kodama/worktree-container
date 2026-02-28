// Package model defines the domain types and value objects for the
// worktree-container CLI.
//
// This package contains pure data structures with no external dependencies.
// All entities (WorktreeEnv, PortAllocation, DevContainerConfig, etc.)
// are transient representations reconstructed from Docker container labels
// at runtime — there are no persistent state files.
//
// The package also defines exit codes (ExitCode) and a custom error type
// (CLIError) that carries exit codes for proper OS process exit handling.
package model
