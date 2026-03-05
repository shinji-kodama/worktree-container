// Package cli — stop.go implements the "loam stop" command.
//
// The stop command gracefully stops all containers in a named worktree
// environment. For Compose-based patterns (C/D), it delegates to
// `docker compose stop`. For non-Compose patterns (A/B), it uses the
// Docker SDK to stop each container individually.
//
// Stopping preserves container state and data, allowing the environment
// to be restarted later with the "start" command.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/mmr-tortoise/loam/internal/docker"
	"github.com/mmr-tortoise/loam/internal/model"
	"github.com/mmr-tortoise/loam/internal/worktree"
)

// NewStopCommand creates the "stop" cobra command.
// It is called from NewRootCommand to register as a subcommand.
func NewStopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a worktree environment",
		Long: `Stop all containers in the specified worktree environment.

The environment's containers are gracefully stopped but not removed.
Data and configuration are preserved, and the environment can be
restarted later with the "start" command.

Examples:
  loam stop feature-auth
  loam stop --json feature-auth`,

		// Exactly one positional argument (environment name) is required.
		Args: cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(cmd.Context(), args[0])
		},
	}

	return cmd
}

// runStop is the main logic function for the stop command.
// It finds the named environment, determines the appropriate stop strategy
// (Compose vs. individual containers), and executes the stop operation.
func runStop(ctx context.Context, envName string) error {
	// Step 1: Try to connect to Docker daemon.
	// Docker may not be needed for PatternNone environments.
	cli, err := docker.NewClient()
	if err != nil {
		VerboseLog("Warning: Docker not available: %v", err)
		cli = nil
	} else {
		defer func() { _ = cli.Close() }()
		VerboseLog("Connected to Docker daemon")
	}

	// Step 2: Find the target environment by listing all managed containers
	// and looking for ones with the matching environment name.
	env, containers, err := findEnvironment(ctx, cli, envName)
	if err != nil {
		return err
	}

	VerboseLog("Found environment %q with %d containers", envName, len(containers))

	// Step 2.5: Handle environments with no container configuration.
	// PatternNone environments have no containers to stop.
	if env.ConfigPattern == model.PatternNone {
		fmt.Printf("Environment %q has no container configuration. Nothing to stop.\n", envName)
		return nil
	}

	// Step 2.6: Guard against nil Docker client for non-None patterns.
	// If Docker is not available but the environment requires containers,
	// return a clear error instead of proceeding to panic on Docker SDK calls.
	if cli == nil {
		return model.WrapCLIError(model.ExitDockerNotRunning,
			fmt.Sprintf("Docker is required to stop environment %q (pattern: %s) but is not available",
				envName, env.ConfigPattern), nil)
	}

	// Step 3: Stop containers based on the configuration pattern.
	if env.ConfigPattern.IsCompose() {
		// Pattern C/D: Use docker compose stop for coordinated shutdown.
		// Compose handles service dependency ordering during stop.
		VerboseLog("Stopping Compose environment %q...", envName)

		// The devcontainer directory is at <worktreePath>/.devcontainer
		devcontainerDir := filepath.Join(env.WorktreePath, ".devcontainer")
		if err := docker.ComposeStop(ctx, devcontainerDir, nil); err != nil {
			return model.WrapCLIError(model.ExitGeneralError,
				fmt.Sprintf("failed to stop environment %q", envName), err)
		}
	} else {
		// Pattern A/B: Stop each container individually via Docker SDK.
		VerboseLog("Stopping %d container(s) for environment %q...", len(containers), envName)
		for _, c := range containers {
			VerboseLog("Stopping container %s (%s)...", c.ContainerName, c.ContainerID[:12])
			if err := docker.StopContainer(ctx, cli, c.ContainerID); err != nil {
				return model.WrapCLIError(model.ExitGeneralError,
					fmt.Sprintf("failed to stop container %q", c.ContainerName), err)
			}
		}
	}

	// Step 4: Output the result.
	printStopResult(envName, len(containers))
	return nil
}

// printStopResult outputs the stop command result in text or JSON format.
func printStopResult(envName string, containerCount int) {
	if IsJSONOutput() {
		printStopResultJSON(envName, containerCount)
	} else {
		printStopResultText(envName, containerCount)
	}
}

// printStopResultJSON outputs the stop result as structured JSON.
func printStopResultJSON(envName string, containerCount int) {
	result := map[string]interface{}{
		"name":           envName,
		"action":         "stopped",
		"containerCount": containerCount,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

// printStopResultText outputs the stop result as human-readable text.
func printStopResultText(envName string, containerCount int) {
	fmt.Printf("Stopped worktree environment %q (%d containers)\n",
		envName, containerCount)
}

// findEnvironment looks up a worktree environment by name.
// It first searches Docker containers, then falls back to marker files
// if Docker doesn't have the environment (e.g., PatternNone environments
// that have no containers).
//
// Returns the environment metadata, the list of containers belonging to it
// (may be empty for marker-only environments), and an error if the
// environment is not found.
//
// This is a shared helper used by stop, start, and remove commands.
func findEnvironment(ctx context.Context, cli *docker.Client, envName string) (*model.WorktreeEnv, []model.ContainerInfo, error) {
	// Step 1: Try Docker-based lookup first (has live container state).
	if cli != nil {
		allContainers, err := docker.ListManagedContainers(ctx, cli)
		if err != nil {
			VerboseLog("Warning: could not list Docker containers: %v", err)
		} else {
			groups := docker.GroupContainersByEnv(allContainers)
			containers, ok := groups[envName]
			if ok && len(containers) > 0 {
				env, err := docker.BuildWorktreeEnv(envName, containers)
				if err != nil {
					return nil, nil, model.WrapCLIError(model.ExitGeneralError,
						fmt.Sprintf("failed to parse environment %q metadata", envName), err)
				}
				return env, containers, nil
			}
		}
	}

	// Step 2: Fall back to marker file search.
	// Scan all worktrees in the repository for a matching marker file.
	env, err := findEnvironmentFromMarker(envName)
	if err != nil {
		return nil, nil, err
	}
	if env != nil {
		return env, nil, nil
	}

	return nil, nil, model.NewCLIError(model.ExitEnvNotFound,
		fmt.Sprintf("worktree environment %q not found", envName))
}

// findEnvironmentFromMarker searches for an environment by name using marker
// files in worktree directories. Returns nil, nil if not found.
func findEnvironmentFromMarker(envName string) (*model.WorktreeEnv, error) {
	wm := worktree.NewManager()

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not get current directory: %w", err)
	}

	repoRoot, err := wm.GetRepoRoot(cwd)
	if err != nil {
		// Not being inside a Git repository is a legitimate scenario
		// (e.g., running from $HOME). Return nil, nil to indicate "not found".
		VerboseLog("Warning: not inside a Git repository: %v", err)
		return nil, nil
	}

	wtPaths, err := wm.ListPaths(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("could not list worktrees: %w", err)
	}

	for _, wtPath := range wtPaths {
		marker, err := worktree.ReadMarkerFile(wtPath)
		if err != nil || marker == nil {
			continue
		}

		// Skip markers not written by this tool.
		if marker.ManagedBy != "loam" {
			continue
		}

		if marker.Name != envName {
			continue
		}

		// Found a matching marker — build a WorktreeEnv from it.
		createdAt, parseErr := time.Parse(time.RFC3339, marker.CreatedAt)
		if parseErr != nil {
			VerboseLog("Warning: could not parse createdAt %q in marker at %s: %v", marker.CreatedAt, wtPath, parseErr)
		}

		// Use config pattern from marker directly (typed as model.ConfigPattern).
		// Skip markers with invalid config patterns instead of silently falling
		// back to PatternNone, which could mask data corruption.
		configPattern := marker.ConfigPattern
		if !configPattern.IsValid() {
			VerboseLog("Warning: ignoring marker at %s for %q due to invalid configPattern %q", wtPath, envName, marker.ConfigPattern)
			continue
		}

		// Determine status heuristically based on config pattern.
		// Without Docker, we cannot know the actual container state, so:
		// - PatternNone → StatusNoContainer (no containers exist)
		// - Any other pattern → StatusStopped (best guess; containers may
		//   actually be running or removed, but "stopped" is the safest
		//   assumption for marker-only lookup without Docker).
		status := model.StatusNoContainer
		if configPattern != model.PatternNone {
			status = model.StatusStopped
		}

		env := &model.WorktreeEnv{
			Name:           marker.Name,
			Branch:         marker.Branch,
			WorktreePath:   wtPath,
			SourceRepoPath: marker.SourceRepoPath,
			Status:         status,
			ConfigPattern:  configPattern,
			CreatedAt:      createdAt,
		}
		return env, nil
	}

	return nil, nil
}
