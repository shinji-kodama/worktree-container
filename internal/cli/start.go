// Package cli — start.go implements the "loam start" command.
//
// The start command restarts a previously stopped worktree environment.
// Before starting containers, it verifies that all allocated host ports
// are still available. If any port is in use by another process, the
// command fails with exit code 4 (port conflict) instead of silently
// starting containers with broken port mappings.
//
// For Compose-based patterns (C/D), it uses docker compose up -d.
// For non-Compose patterns (A/B), it uses the Docker SDK to start
// each container individually.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mmr-tortoise/loam/internal/devcontainer"
	"github.com/mmr-tortoise/loam/internal/docker"
	"github.com/mmr-tortoise/loam/internal/model"
	"github.com/mmr-tortoise/loam/internal/port"
)

// NewStartCommand creates the "start" cobra command.
// It is called from NewRootCommand to register as a subcommand.
func NewStartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start a stopped worktree environment",
		Long: `Start all containers in a previously stopped worktree environment.

Before starting, the command verifies that all allocated host ports
are still available. If any port conflict is detected, the command
exits with code 4 and reports which ports are in use.

Examples:
  loam start feature-auth
  loam start --json feature-auth`,

		// Exactly one positional argument (environment name) is required.
		Args: cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd.Context(), args[0])
		},
	}

	return cmd
}

// runStart is the main logic function for the start command.
// It finds the named environment, checks port availability, and starts
// all containers.
func runStart(ctx context.Context, envName string) error {
	// Step 1: Try to connect to Docker daemon.
	// Docker may not be needed for PatternNone environments, so connection
	// failure is deferred until we know the pattern.
	cli, err := docker.NewClient()
	if err != nil {
		VerboseLog("Warning: Docker not available: %v", err)
		cli = nil
	} else {
		defer func() { _ = cli.Close() }()
		VerboseLog("Connected to Docker daemon")
	}

	// Step 2: Find the target environment.
	env, containers, err := findEnvironment(ctx, cli, envName)
	if err != nil {
		return err
	}

	VerboseLog("Found environment %q with %d containers", envName, len(containers))

	// Step 2.5: Handle environments with no container configuration.
	// Check if a devcontainer.json has been added since the environment was created.
	// If found, inform the user to run `remove --keep-worktree` + `create` to set up
	// the full container environment (port allocation, config rewrite, etc.).
	if env.ConfigPattern == model.PatternNone {
		VerboseLog("Environment %q has PatternNone, checking for newly added devcontainer.json...", envName)

		devcontainerPath, findErr := devcontainer.FindDevContainerJSON(env.WorktreePath)
		if findErr != nil {
			VerboseLog("Warning: error searching for devcontainer.json: %v", findErr)
		}

		if devcontainerPath == "" {
			// No devcontainer.json found — nothing to start.
			fmt.Printf("Environment %q has no container configuration.\n", envName)
			fmt.Println("To add containers, create a .devcontainer/devcontainer.json in the worktree.")
			return nil
		}

		// devcontainer.json found, but start cannot perform the full setup
		// (port allocation, config rewrite, container creation) that create does.
		// Guide the user to re-create the environment.
		fmt.Printf("Environment %q has a devcontainer.json but was created without container support.\n", envName)
		fmt.Println("To set up containers, re-create the environment:")
		fmt.Printf("  loam remove --force --keep-worktree %s\n", envName)
		fmt.Printf("  loam create %s\n", env.Branch)
		return nil
	}

	// Step 2.6: Guard against nil Docker client for non-None patterns.
	// If Docker is not available but the environment requires containers,
	// return a clear error instead of proceeding to panic on Docker SDK calls.
	if cli == nil {
		return model.WrapCLIError(model.ExitDockerNotRunning,
			fmt.Sprintf("Docker is required to start environment %q (pattern: %s) but is not available",
				envName, env.ConfigPattern), nil)
	}

	// Step 3: Verify port availability before starting.
	// This prevents starting containers that would fail to bind ports or
	// silently shadow other services already using those ports.
	scanner := port.NewScanner()
	var conflictingPorts []int
	for _, pa := range env.PortAllocations {
		if !scanner.IsPortAvailable(pa.HostPort, pa.Protocol) {
			conflictingPorts = append(conflictingPorts, pa.HostPort)
		}
	}

	if len(conflictingPorts) > 0 {
		return model.NewCLIError(model.ExitPortAllocationFailed,
			fmt.Sprintf("port conflict: the following ports are already in use: %v", conflictingPorts))
	}

	// Step 4: Start containers based on the configuration pattern.
	if env.ConfigPattern.IsCompose() {
		// Pattern C/D: Use docker compose up -d for coordinated startup.
		// Compose handles service dependency ordering and network creation.
		VerboseLog("Starting Compose environment %q...", envName)

		devcontainerDir := filepath.Join(env.WorktreePath, ".devcontainer")
		envVars := map[string]string{
			"COMPOSE_PROJECT_NAME": envName,
		}
		if err := docker.ComposeUp(ctx, devcontainerDir, nil, envVars); err != nil {
			return model.WrapCLIError(model.ExitGeneralError,
				fmt.Sprintf("failed to start environment %q", envName), err)
		}
	} else {
		// Pattern A/B: Start each container individually via Docker SDK.
		VerboseLog("Starting %d container(s) for environment %q...", len(containers), envName)
		for _, c := range containers {
			VerboseLog("Starting container %s (%s)...", c.ContainerName, c.ContainerID[:12])
			if err := docker.StartContainer(ctx, cli, c.ContainerID); err != nil {
				return model.WrapCLIError(model.ExitGeneralError,
					fmt.Sprintf("failed to start container %q", c.ContainerName), err)
			}
		}
	}

	// Step 5: Output the result with service details.
	printStartResult(env)
	return nil
}

// printStartResult outputs the start command result in text or JSON format.
func printStartResult(env *model.WorktreeEnv) {
	if IsJSONOutput() {
		printStartResultJSON(env)
	} else {
		printStartResultText(env)
	}
}

// printStartResultJSON outputs the start result as structured JSON.
func printStartResultJSON(env *model.WorktreeEnv) {
	type serviceJSON struct {
		Name          string `json:"name"`
		ContainerPort int    `json:"containerPort"`
		HostPort      int    `json:"hostPort"`
	}

	type resultJSON struct {
		Name     string        `json:"name"`
		Action   string        `json:"action"`
		Services []serviceJSON `json:"services"`
	}

	result := resultJSON{
		Name:     env.Name,
		Action:   "started",
		Services: make([]serviceJSON, 0, len(env.PortAllocations)),
	}

	for _, pa := range env.PortAllocations {
		result.Services = append(result.Services, serviceJSON{
			Name:          pa.ServiceName,
			ContainerPort: pa.ContainerPort,
			HostPort:      pa.HostPort,
		})
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

// printStartResultText outputs the start result as human-readable text,
// including a service table with port mappings.
func printStartResultText(env *model.WorktreeEnv) {
	fmt.Printf("Started worktree environment %q\n", env.Name)

	if len(env.PortAllocations) > 0 {
		fmt.Println()
		fmt.Println("  Services:")
		for _, pa := range env.PortAllocations {
			// Format the URL/address based on whether it looks like an HTTP service.
			// Reuse the formatServiceAddress function from create.go.
			addr := formatServiceAddress(pa)
			fmt.Printf("    %-8s %s  (container: %d)\n",
				pa.ServiceName, addr, pa.ContainerPort)
		}
	}
}
