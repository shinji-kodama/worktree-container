// Package cli — remove.go implements the "worktree-container remove" command.
//
// The remove command completely destroys a worktree environment by:
//  1. Stopping and removing all Docker containers and resources
//  2. Optionally removing the Git worktree directory
//
// For Compose-based patterns (C/D), it uses `docker compose down -v` which
// removes containers, networks, and volumes. For non-Compose patterns (A/B),
// it stops and removes each container individually.
//
// By default, the command prompts for confirmation before proceeding.
// The --force flag skips the confirmation prompt. The --keep-worktree flag
// preserves the Git worktree directory while still removing containers.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mmr-tortoise/worktree-container/internal/docker"
	"github.com/mmr-tortoise/worktree-container/internal/model"
	"github.com/mmr-tortoise/worktree-container/internal/worktree"
)

// removeFlags holds the flag values for the remove command.
type removeFlags struct {
	// force skips the interactive confirmation prompt when true.
	force bool

	// keepWorktree preserves the Git worktree directory when true.
	// Only Docker containers and resources are removed.
	keepWorktree bool
}

// NewRemoveCommand creates the "remove" cobra command.
// It is called from NewRootCommand to register as a subcommand.
func NewRemoveCommand() *cobra.Command {
	flags := &removeFlags{}

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a worktree environment",
		Long: `Remove a worktree environment, including all Docker containers and resources.

By default, the Git worktree directory is also removed. Use --keep-worktree
to preserve the directory while removing only the Docker resources.

Unless --force is specified, the command prompts for confirmation.

Examples:
  worktree-container remove feature-auth
  worktree-container remove --force feature-auth
  worktree-container remove --keep-worktree feature-auth`,

		// Exactly one positional argument (environment name) is required.
		Args: cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd.Context(), args[0], flags)
		},
	}

	// Register command-specific flags.
	cmd.Flags().BoolVarP(&flags.force, "force", "f", false, "Remove without confirmation")
	cmd.Flags().BoolVar(&flags.keepWorktree, "keep-worktree", false, "Keep Git worktree directory")

	return cmd
}

// runRemove is the main logic function for the remove command.
// It finds the environment, optionally prompts for confirmation, removes
// Docker resources, and optionally removes the Git worktree.
func runRemove(ctx context.Context, envName string, flags *removeFlags) error {
	// Step 1: Connect to Docker daemon.
	cli, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	VerboseLog("Connected to Docker daemon")

	// Step 2: Find the target environment.
	env, containers, err := findEnvironment(ctx, cli, envName)
	if err != nil {
		return err
	}

	VerboseLog("Found environment %q with %d containers", envName, len(containers))

	// Step 3: Prompt for confirmation unless --force is specified.
	if !flags.force {
		confirmed, err := promptConfirmation(envName, len(containers), env.WorktreePath, flags.keepWorktree)
		if err != nil {
			return model.WrapCLIError(model.ExitGeneralError, "failed to read user input", err)
		}
		if !confirmed {
			return model.NewCLIError(model.ExitUserCancelled, "operation cancelled by user")
		}
	}

	// Step 4: Remove Docker containers and resources.
	if env.ConfigPattern.IsCompose() {
		// Pattern C/D: Use docker compose down with volume removal.
		// This removes containers, networks, and named volumes in one operation.
		VerboseLog("Running docker compose down for environment %q...", envName)

		devcontainerDir := filepath.Join(env.WorktreePath, ".devcontainer")
		if err := docker.ComposeDown(ctx, devcontainerDir, nil, true); err != nil {
			return model.WrapCLIError(model.ExitGeneralError,
				fmt.Sprintf("failed to remove environment %q containers", envName), err)
		}
	} else {
		// Pattern A/B: Stop and remove each container individually.
		VerboseLog("Removing %d container(s) for environment %q...", len(containers), envName)
		for _, c := range containers {
			VerboseLog("Removing container %s (%s)...", c.ContainerName, c.ContainerID[:12])
			// Use force=true to handle containers that might still be running.
			if err := docker.RemoveContainer(ctx, cli, c.ContainerID, true); err != nil {
				return model.WrapCLIError(model.ExitGeneralError,
					fmt.Sprintf("failed to remove container %q", c.ContainerName), err)
			}
		}
	}

	// Step 5: Optionally remove the Git worktree.
	worktreeRemoved := false
	if !flags.keepWorktree {
		VerboseLog("Removing Git worktree at %s...", env.WorktreePath)
		wm := worktree.NewManager()

		// Use the source repo path (stored in labels) to run git worktree remove.
		// The source repo is where the worktree was originally created from.
		if err := wm.Remove(env.SourceRepoPath, env.WorktreePath, true); err != nil {
			// Git worktree removal failure is not fatal — the containers are
			// already cleaned up. Log the error and continue.
			VerboseLog("Warning: failed to remove Git worktree: %v", err)

			// If the worktree directory still exists, report the git error.
			if _, statErr := os.Stat(env.WorktreePath); statErr == nil {
				return model.WrapCLIError(model.ExitGitError,
					fmt.Sprintf("failed to remove Git worktree at %s", env.WorktreePath), err)
			}
			// Directory already gone — the worktree was likely already removed manually.
		} else {
			worktreeRemoved = true
		}
	}

	// Step 6: Output the result.
	printRemoveResult(envName, len(containers), env.WorktreePath, worktreeRemoved)
	return nil
}

// promptConfirmation asks the user to confirm the remove operation.
// It reads a single line from stdin and checks for "y" or "yes".
// Returns true if the user confirmed, false otherwise.
func promptConfirmation(envName string, containerCount int, worktreePath string, keepWorktree bool) (bool, error) {
	fmt.Printf("About to remove worktree environment %q:\n", envName)
	fmt.Printf("  - %d container(s) will be removed\n", containerCount)
	if !keepWorktree {
		fmt.Printf("  - Git worktree at %s will be removed\n", worktreePath)
	}
	fmt.Print("\nContinue? [y/N] ")

	// Read a line from stdin. bufio.Scanner handles different line endings
	// across platforms (LF on Unix, CRLF on Windows).
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes", nil
	}

	// If stdin is closed or an error occurred, treat it as "no".
	if err := scanner.Err(); err != nil {
		return false, err
	}

	return false, nil
}

// printRemoveResult outputs the remove command result in text or JSON format.
func printRemoveResult(envName string, containerCount int, worktreePath string, worktreeRemoved bool) {
	if IsJSONOutput() {
		printRemoveResultJSON(envName, containerCount, worktreePath, worktreeRemoved)
	} else {
		printRemoveResultText(envName, containerCount, worktreePath, worktreeRemoved)
	}
}

// printRemoveResultJSON outputs the remove result as structured JSON.
func printRemoveResultJSON(envName string, containerCount int, worktreePath string, worktreeRemoved bool) {
	result := map[string]interface{}{
		"name":            envName,
		"action":          "removed",
		"containerCount":  containerCount,
		"worktreeRemoved": worktreeRemoved,
		"worktreePath":    worktreePath,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

// printRemoveResultText outputs the remove result as human-readable text.
func printRemoveResultText(envName string, containerCount int, worktreePath string, worktreeRemoved bool) {
	fmt.Printf("Removed worktree environment %q\n", envName)
	fmt.Printf("  Removed %d containers\n", containerCount)
	if worktreeRemoved {
		fmt.Printf("  Removed git worktree at %s\n", worktreePath)
	}
}
