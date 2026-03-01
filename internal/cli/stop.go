// Package cli — stop.go implements the "worktree-container stop" command.
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
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mmr-tortoise/worktree-container/internal/docker"
	"github.com/mmr-tortoise/worktree-container/internal/model"
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
  worktree-container stop feature-auth
  worktree-container stop --json feature-auth`,

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
	// Step 1: Connect to Docker daemon.
	cli, err := docker.NewClient()
	if err != nil {
		return err // NewClient already returns CLIError with ExitDockerNotRunning
	}
	defer func() { _ = cli.Close() }()

	VerboseLog("Connected to Docker daemon")

	// Step 2: Find the target environment by listing all managed containers
	// and looking for ones with the matching environment name.
	env, containers, err := findEnvironment(ctx, cli, envName)
	if err != nil {
		return err
	}

	VerboseLog("Found environment %q with %d containers", envName, len(containers))

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

// findEnvironment looks up a worktree environment by name from Docker containers.
// It returns the environment metadata, the list of containers belonging to it,
// and an error if the environment is not found or Docker operations fail.
//
// This is a shared helper used by stop, start, and remove commands.
func findEnvironment(ctx context.Context, cli *docker.Client, envName string) (*model.WorktreeEnv, []model.ContainerInfo, error) {
	// List all managed containers.
	allContainers, err := docker.ListManagedContainers(ctx, cli)
	if err != nil {
		return nil, nil, err
	}

	// Group by environment name.
	groups := docker.GroupContainersByEnv(allContainers)

	// Look up the target environment.
	containers, ok := groups[envName]
	if !ok || len(containers) == 0 {
		return nil, nil, model.NewCLIError(model.ExitEnvNotFound,
			fmt.Sprintf("worktree environment %q not found", envName))
	}

	// Build the WorktreeEnv domain object from container labels.
	env, err := docker.BuildWorktreeEnv(envName, containers)
	if err != nil {
		return nil, nil, model.WrapCLIError(model.ExitGeneralError,
			fmt.Sprintf("failed to parse environment %q metadata", envName), err)
	}

	return env, containers, nil
}
