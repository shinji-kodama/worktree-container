// Package cli — start.go implements the "worktree-container start" command.
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

	"github.com/shinji-kodama/worktree-container/internal/docker"
	"github.com/shinji-kodama/worktree-container/internal/model"
	"github.com/shinji-kodama/worktree-container/internal/port"
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
  worktree-container start feature-auth
  worktree-container start --json feature-auth`,

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
