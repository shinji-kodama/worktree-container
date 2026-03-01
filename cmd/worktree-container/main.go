// Package main is the entry point for the worktree-container CLI.
//
// This binary provides commands to manage Dev Container environments
// associated with Git worktrees. It delegates all functionality to the
// internal/cli package, which defines cobra commands.
//
// Build-time variables (version, commit, date) are injected via ldflags
// by GoReleaser during the release process. During development, they
// default to "dev", "none", and "unknown" respectively.
package main

import (
	"github.com/mmr-tortoise/worktree-container/internal/cli"
)

// version, commit, and date are set by GoReleaser at build time
// via ldflags (see .goreleaser.yml). They provide binary identification
// for the --version flag output.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Inject build-time version info into the CLI package.
	// This decouples the build system (GoReleaser ldflags) from the
	// CLI framework (cobra), keeping main.go minimal.
	cli.Version = version
	cli.Commit = commit
	cli.Date = date

	// Create the root command with all subcommands registered,
	// then execute it. Execute handles error formatting and exit codes.
	rootCmd := cli.NewRootCommand()
	cli.Execute(rootCmd)
}
