// container.go implements Docker container lifecycle operations for the
// worktree-container CLI. It provides functions for listing, grouping,
// starting, stopping, and removing containers that are managed by this tool.
//
// Container management follows two patterns:
//   - Pattern A/B (image/Dockerfile): single container managed via Docker SDK
//   - Pattern C/D (Compose): multiple containers managed via docker compose CLI
//
// All managed containers are identified by the "worktree.managed-by" label,
// which enables filtering them from unrelated containers on the same host.
package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	// Docker API types for container listing results.
	// types.Container is the struct returned by ContainerList.
	"github.com/docker/docker/api/types"

	// container package provides ListOptions, StopOptions, RemoveOptions
	// for Docker container operations.
	"github.com/docker/docker/api/types/container"

	// filters package provides Args type for building Docker API query filters.
	"github.com/docker/docker/api/types/filters"

	"github.com/shinji-kodama/worktree-container/internal/model"
)

// ListManagedContainers queries the Docker daemon for all containers that have
// the "worktree.managed-by=worktree-container" label. It returns a slice of
// ContainerInfo representing each managed container, including stopped ones.
//
// This function is the primary entry point for discovering what worktree
// environments currently exist. All state is derived from Docker labels
// rather than any external database.
//
// The function lists ALL containers (including stopped ones) because
// a worktree environment may have stopped containers that still need
// to be tracked (e.g., for "wt list" or "wt destroy" commands).
func ListManagedContainers(ctx context.Context, cli *Client) ([]model.ContainerInfo, error) {
	// Build a Docker API filter that matches only containers with our
	// management label. This is more efficient than listing all containers
	// and filtering in Go, because Docker performs the filtering server-side.
	filterArgs := filters.NewArgs(
		filters.Arg("label", LabelManagedBy+"="+ManagedByValue),
	)

	// List containers using the Docker SDK. The All flag ensures we also
	// get stopped/exited containers, not just running ones.
	containers, err := cli.Inner().ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, model.WrapCLIError(
			model.ExitDockerNotRunning,
			"failed to list Docker containers",
			err,
		)
	}

	// Convert Docker API types.Container structs to our domain model
	// ContainerInfo structs. This decouples the rest of the application
	// from the Docker SDK types.
	result := make([]model.ContainerInfo, 0, len(containers))
	for _, c := range containers {
		result = append(result, containerToInfo(c))
	}

	return result, nil
}

// containerToInfo converts a Docker API Container struct to our domain
// model ContainerInfo. This is a pure mapping function with no side effects.
//
// The Docker API returns container names with a leading "/" prefix
// (e.g., "/my-container"), which we strip for cleaner display in CLI output.
// The State field from the Docker API is a short string like "running",
// "exited", or "created".
func containerToInfo(c types.Container) model.ContainerInfo {
	// Extract the container name. Docker returns names as a slice,
	// and each name has a leading "/" that we strip for readability.
	name := ""
	if len(c.Names) > 0 {
		// Docker container names always start with "/". We remove it
		// because it's an artifact of the API, not meaningful to users.
		name = strings.TrimPrefix(c.Names[0], "/")
	}

	// Extract the Compose service name from Docker Compose labels.
	// Docker Compose adds a "com.docker.compose.service" label to each
	// container it creates, which tells us which service definition
	// in the YAML this container belongs to.
	serviceName := c.Labels["com.docker.compose.service"]

	return model.ContainerInfo{
		ContainerID:   c.ID,
		ContainerName: name,
		ServiceName:   serviceName,
		Status:        c.State,
		Labels:        c.Labels,
	}
}

// GroupContainersByEnv groups a slice of ContainerInfo by their
// "worktree.name" label value. This is useful for the "wt list" command,
// which needs to display containers organized by worktree environment.
//
// Containers without a "worktree.name" label are silently skipped,
// since they cannot be attributed to any environment. This should not
// happen in practice because ListManagedContainers already filters for
// containers with worktree labels.
//
// Returns a map where keys are environment names and values are slices
// of ContainerInfo belonging to that environment.
func GroupContainersByEnv(containers []model.ContainerInfo) map[string][]model.ContainerInfo {
	groups := make(map[string][]model.ContainerInfo)

	for _, c := range containers {
		// Look up the environment name from the container's labels.
		envName, ok := c.Labels[LabelName]
		if !ok || envName == "" {
			// Skip containers that don't have the environment name label.
			// This is a defensive check — it shouldn't happen with
			// properly labeled containers.
			continue
		}
		groups[envName] = append(groups[envName], c)
	}

	return groups
}

// BuildWorktreeEnv constructs a WorktreeEnv domain object from a group of
// containers that belong to the same worktree environment.
//
// It uses ParseLabels (from label.go) on the first container's labels to
// extract the base environment metadata (name, branch, paths, etc.), and
// uses ParsePortLabels to get port allocations.
//
// The overall environment status is determined by:
//  1. If the worktree path does not exist on disk → orphaned
//  2. If any container has status "running" → running
//  3. Otherwise → stopped
//
// Returns an error if the containers slice is empty or if label parsing fails.
func BuildWorktreeEnv(envName string, containers []model.ContainerInfo) (*model.WorktreeEnv, error) {
	// Guard: at least one container is required to extract labels from.
	if len(containers) == 0 {
		return nil, fmt.Errorf("cannot build WorktreeEnv %q: no containers provided", envName)
	}

	// Parse the base environment metadata from the first container's labels.
	// All containers in the same environment should have identical worktree
	// labels, so using the first one is sufficient.
	env, err := ParseLabels(containers[0].Labels)
	if err != nil {
		return nil, fmt.Errorf("failed to parse labels for environment %q: %w", envName, err)
	}

	// Attach all containers to the environment for downstream use
	// (e.g., displaying container details in "wt list --json").
	env.Containers = containers

	// Determine the overall environment status based on container states
	// and whether the worktree directory still exists on disk.
	env.Status = determineStatus(containers, env.WorktreePath)

	return env, nil
}

// determineStatus calculates the aggregate status of a worktree environment
// based on its containers' states and whether the worktree path exists.
//
// The priority order is:
//  1. Orphaned: worktree path no longer exists → containers are orphaned
//  2. Running: at least one container is running → environment is running
//  3. Stopped: all containers are stopped/exited → environment is stopped
//
// This logic supports the lifecycle model described in the data-model spec:
//
//	[Created] → Running → Stopped ⇄ Running → [Deleted]
//	Running/Stopped → Orphaned (when Git worktree is manually deleted)
func determineStatus(containers []model.ContainerInfo, worktreePath string) model.WorktreeStatus {
	// Check if the worktree directory exists on disk. If not, the environment
	// is orphaned — the user likely deleted the worktree directory manually
	// without cleaning up the Docker containers.
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return model.StatusOrphaned
	}

	// Check if any container is currently running. A single running
	// container is enough to consider the whole environment as "running".
	for _, c := range containers {
		if c.Status == "running" {
			return model.StatusRunning
		}
	}

	// No containers are running, so the environment is stopped.
	return model.StatusStopped
}

// ComposeUp starts containers using docker compose. It executes
// "docker compose -f file1 -f file2 up -d" in the specified project
// directory with the given environment variables.
//
// This function is used for Pattern C/D (Compose-based configurations).
// The -d flag runs containers in detached mode so the CLI doesn't block.
//
// The envVars parameter allows passing environment variables to the
// docker compose process, which is essential for injecting worktree-specific
// values like shifted port numbers via variable substitution in YAML files.
//
// Returns a CLIError with ExitDockerNotRunning if the command fails,
// since compose failures most commonly stem from Docker daemon issues.
func ComposeUp(ctx context.Context, projectDir string, composeFiles []string, envVars map[string]string) error {
	// Build the docker compose command arguments.
	// Each compose file gets its own -f flag, which docker compose
	// merges in order (later files override earlier ones).
	args := buildComposeArgs(composeFiles)
	args = append(args, "up", "-d")

	return runCompose(ctx, projectDir, args, envVars)
}

// ComposeStop stops containers managed by docker compose without removing
// them. It executes "docker compose -f file1 -f file2 stop" in the
// specified project directory.
//
// This preserves container state and data, allowing them to be restarted
// later with ComposeUp. This maps to the "wt stop" CLI command.
func ComposeStop(ctx context.Context, projectDir string, composeFiles []string) error {
	args := buildComposeArgs(composeFiles)
	args = append(args, "stop")

	return runCompose(ctx, projectDir, args, nil)
}

// ComposeDown stops and removes containers, networks, and optionally volumes
// created by docker compose. It executes "docker compose -f file1 -f file2 down"
// with an optional -v flag for volume removal.
//
// This is used by the "wt destroy" CLI command to completely clean up
// all Docker resources associated with a worktree environment.
//
// When removeVolumes is true, the -v flag is added to also remove named
// volumes declared in the Compose file and anonymous volumes attached
// to containers. This ensures complete cleanup with no leftover data.
func ComposeDown(ctx context.Context, projectDir string, composeFiles []string, removeVolumes bool) error {
	args := buildComposeArgs(composeFiles)
	args = append(args, "down")

	// Optionally remove volumes for complete cleanup.
	if removeVolumes {
		args = append(args, "-v")
	}

	return runCompose(ctx, projectDir, args, nil)
}

// buildComposeArgs constructs the common arguments for docker compose commands.
// Each compose file is specified with a -f flag. Docker compose merges
// multiple files in the order specified, with later files taking precedence.
func buildComposeArgs(composeFiles []string) []string {
	args := make([]string, 0, len(composeFiles)*2+2)
	// "compose" is the subcommand for "docker compose" (plugin-style invocation).
	args = append(args, "compose")
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	return args
}

// runCompose executes a docker compose command as a child process.
// It runs "docker" with the given arguments in the specified working directory,
// optionally injecting extra environment variables.
//
// The function captures both stdout and stderr for error reporting.
// On failure, it returns a CLIError with ExitDockerNotRunning because
// compose failures most commonly indicate Docker daemon problems.
func runCompose(ctx context.Context, projectDir string, args []string, envVars map[string]string) error {
	// Create the command with context so it can be cancelled if needed.
	// We use "docker" as the binary and "compose" as the first argument
	// rather than "docker-compose" (legacy standalone binary), because
	// modern Docker ships compose as a plugin subcommand.
	cmd := exec.CommandContext(ctx, "docker", args...)

	// Set the working directory for the compose command.
	// docker compose resolves relative paths in YAML files relative to
	// this directory, so it must be the project root.
	cmd.Dir = projectDir

	// Inherit the current process environment and add any extra variables.
	// os.Environ() returns a copy, so modifications don't affect this process.
	cmd.Env = os.Environ()
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// CombinedOutput runs the command and captures both stdout and stderr
	// into a single byte slice. This is useful for error messages.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return model.WrapCLIError(
			model.ExitDockerNotRunning,
			fmt.Sprintf("docker compose failed: %s", strings.TrimSpace(string(output))),
			err,
		)
	}

	return nil
}

// RunContainer starts a single container using "docker run -d" for
// Pattern A/B (image-based or Dockerfile-based) configurations.
//
// The runArgs parameter should contain all Docker run flags including
// label flags (--label), port mappings (-p), volume mounts (-v), and
// the container name (--name). These arguments are constructed by the
// config rewrite step that precedes container creation.
//
// The function uses os/exec rather than the Docker SDK for simplicity,
// because the Docker SDK's ContainerCreate + ContainerStart workflow
// requires constructing complex Config/HostConfig structs, while
// "docker run" accepts the same CLI flags users are familiar with.
func RunContainer(ctx context.Context, cli *Client, imageName string, containerName string, runArgs []string) error {
	// Build the full argument list for "docker run -d".
	// The -d flag runs the container in detached mode (background).
	args := make([]string, 0, len(runArgs)+4)
	args = append(args, "run", "-d")
	args = append(args, "--name", containerName)
	args = append(args, runArgs...)
	args = append(args, imageName)

	// Execute "docker run" as a child process.
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return model.WrapCLIError(
			model.ExitDockerNotRunning,
			fmt.Sprintf("docker run failed for container %q: %s",
				containerName, strings.TrimSpace(string(output))),
			err,
		)
	}

	return nil
}

// StartContainer starts a stopped container by its ID using the Docker SDK.
// It sends a start request to the Docker daemon, which resumes the container's
// main process. If the container is already running, Docker returns an error.
//
// This is used for Pattern A/B containers that are managed individually
// rather than through docker compose. The "start" command uses this to
// restart previously stopped environments.
func StartContainer(ctx context.Context, cli *Client, containerID string) error {
	// container.StartOptions is currently empty in the Docker SDK but is
	// included for forward compatibility with future Docker API versions.
	err := cli.Inner().ContainerStart(ctx, containerID, container.StartOptions{})
	if err != nil {
		return model.WrapCLIError(
			model.ExitDockerNotRunning,
			fmt.Sprintf("failed to start container %q", containerID),
			err,
		)
	}
	return nil
}

// StopContainer stops a running container by its ID using the Docker SDK.
// It sends a SIGTERM signal to the container's main process and waits
// for it to exit gracefully. If the container does not stop within the
// Docker daemon's default timeout (typically 10 seconds), it is forcefully
// killed with SIGKILL.
//
// This is used for Pattern A/B containers that are managed individually
// rather than through docker compose.
func StopContainer(ctx context.Context, cli *Client, containerID string) error {
	// StopOptions with nil Timeout uses Docker's default timeout (10 seconds).
	// This gives the container a chance to shut down gracefully.
	err := cli.Inner().ContainerStop(ctx, containerID, container.StopOptions{})
	if err != nil {
		return model.WrapCLIError(
			model.ExitDockerNotRunning,
			fmt.Sprintf("failed to stop container %q", containerID),
			err,
		)
	}
	return nil
}

// RemoveContainer removes a container by its ID using the Docker SDK.
// The container must be stopped first unless force is true.
//
// When force is true, Docker will first kill the container (SIGKILL)
// and then remove it. This is useful for cleanup operations where
// graceful shutdown is not required (e.g., "wt destroy --force").
//
// This is used for Pattern A/B containers that are managed individually
// rather than through docker compose.
func RemoveContainer(ctx context.Context, cli *Client, containerID string, force bool) error {
	err := cli.Inner().ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force: force,
	})
	if err != nil {
		return model.WrapCLIError(
			model.ExitDockerNotRunning,
			fmt.Sprintf("failed to remove container %q", containerID),
			err,
		)
	}
	return nil
}
