// Package cli — list.go implements the "loam list" command.
//
// The list command displays all managed worktree environments using a
// dual-source approach: marker files (.loam) for local
// worktree discovery, and Docker container labels for live container state.
// This allows listing environments even when Docker is unavailable.
//
// Environments are presented as a text table or JSON array, depending on
// the --json flag. An optional --status flag allows filtering by lifecycle
// state (running, stopped, orphaned, no-container, or all).
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mmr-tortoise/loam/internal/docker"
	"github.com/mmr-tortoise/loam/internal/model"
	"github.com/mmr-tortoise/loam/internal/worktree"
)

// listFlags holds the flag values for the list command.
// These are bound to cobra flags in NewListCommand.
type listFlags struct {
	// status filters environments by their lifecycle state.
	// Valid values: "running", "stopped", "orphaned", "no-container", "all" (default).
	status string
}

// NewListCommand creates the "list" cobra command.
// It is called from NewRootCommand to register as a subcommand.
func NewListCommand() *cobra.Command {
	flags := &listFlags{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all worktree environments",
		Long: `List all managed worktree environments and their status.

Each environment is shown with its name, branch, lifecycle status,
service count, and allocated host ports.

Examples:
  loam list
  loam list --status running
  loam list --json`,

		// No positional arguments are required for the list command.
		Args: cobra.NoArgs,

		// RunE returns an error to the root command's error handler.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), flags)
		},
	}

	// Register the --status flag with a default value of "all".
	cmd.Flags().StringVar(&flags.status, "status", "all",
		"Filter by status: running, stopped, orphaned, no-container, all (default: all)")

	return cmd
}

// runList is the main logic function for the list command.
// It uses a dual-source approach: marker files for local discovery and
// Docker labels for container state. This allows listing environments
// even when Docker is not running (showing marker-only environments).
func runList(ctx context.Context, flags *listFlags) error {
	// Step 1: Validate the --status flag value.
	statusFilter := flags.status
	if statusFilter != "all" {
		if _, err := model.ParseWorktreeStatus(statusFilter); err != nil {
			return model.WrapCLIError(model.ExitGeneralError,
				fmt.Sprintf("invalid status filter %q: valid values are running, stopped, orphaned, no-container, all", statusFilter), nil)
		}
	}

	// Step 2: Discover environments from marker files (local filesystem).
	// Get the repository root so we can enumerate all worktrees.
	wm := worktree.NewManager()
	cwd, err := os.Getwd()
	if err != nil {
		return model.WrapCLIError(model.ExitGeneralError, "failed to get current directory", err)
	}

	repoRoot, err := wm.GetRepoRoot(cwd)
	if err != nil {
		return model.WrapCLIError(model.ExitGitError, "not inside a Git repository", err)
	}

	// Scan all worktree paths for marker files.
	// Build a map of envName → WorktreeEnv from marker data.
	markerEnvs := make(map[string]*model.WorktreeEnv)
	wtPaths, err := wm.ListPaths(repoRoot)
	if err != nil {
		VerboseLog("Warning: could not list worktrees: %v", err)
	} else {
		for _, wtPath := range wtPaths {
			marker, readErr := worktree.ReadMarkerFile(wtPath)
			if readErr != nil {
				VerboseLog("Warning: could not read marker at %s: %v", wtPath, readErr)
				continue
			}
			if marker == nil {
				continue // No marker file — not managed by loam.
			}

			// Validate that this marker was written by loam.
			// Markers from other tools or with corrupted data are silently skipped.
			if marker.ManagedBy != "loam" {
				VerboseLog("Warning: ignoring marker at %s with unexpected managedBy %q", wtPath, marker.ManagedBy)
				continue
			}
			if marker.Name == "" {
				VerboseLog("Warning: ignoring marker at %s with empty name", wtPath)
				continue
			}

			// Parse the creation timestamp from the marker file.
			createdAt, parseErr := time.Parse(time.RFC3339, marker.CreatedAt)
			if parseErr != nil {
				VerboseLog("Warning: could not parse createdAt %q in marker at %s: %v", marker.CreatedAt, wtPath, parseErr)
			}

			// Use config pattern from marker directly (typed as model.ConfigPattern).
			// Default to PatternNone if the stored value is invalid.
			configPattern := marker.ConfigPattern
			if !configPattern.IsValid() {
				configPattern = model.PatternNone
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
			markerEnvs[marker.Name] = env
		}
	}
	VerboseLog("Found %d marker-based environments", len(markerEnvs))

	// Step 3: Connect to Docker and discover container-based environments.
	// Docker connection failure is non-fatal — we fall back to marker-only data.
	var dockerEnvs map[string]*model.WorktreeEnv

	cli, err := docker.NewClient()
	if err != nil {
		VerboseLog("Warning: Docker not available, showing marker-only environments: %v", err)
	} else {
		defer func() { _ = cli.Close() }()
		VerboseLog("Connected to Docker daemon")

		containers, err := docker.ListManagedContainers(ctx, cli)
		if err != nil {
			VerboseLog("Warning: could not list Docker containers: %v", err)
		} else {
			VerboseLog("Found %d managed containers", len(containers))
			groups := docker.GroupContainersByEnv(containers)

			dockerEnvs = make(map[string]*model.WorktreeEnv, len(groups))
			for envName, containerGroup := range groups {
				env, err := docker.BuildWorktreeEnv(envName, containerGroup)
				if err != nil {
					VerboseLog("Warning: skipping environment %q: %v", envName, err)
					continue
				}
				dockerEnvs[envName] = env
			}
		}
	}

	// Step 4: Merge marker and Docker environments.
	// Docker data takes priority (has live container state).
	// Marker-only environments are included with StatusNoContainer.
	merged := make(map[string]*model.WorktreeEnv)

	// Start with marker environments as the base.
	for name, env := range markerEnvs {
		merged[name] = env
	}

	// Overlay Docker environments (takes priority).
	for name, env := range dockerEnvs {
		merged[name] = env
	}

	// Step 5: Convert to sorted slice.
	envs := make([]*model.WorktreeEnv, 0, len(merged))
	for _, env := range merged {
		envs = append(envs, env)
	}

	sort.Slice(envs, func(i, j int) bool {
		return envs[i].Name < envs[j].Name
	})

	// Step 6: Apply the --status filter if specified.
	if statusFilter != "all" {
		filteredEnvs := make([]*model.WorktreeEnv, 0, len(envs))
		for _, env := range envs {
			if env.Status.String() == statusFilter {
				filteredEnvs = append(filteredEnvs, env)
			}
		}
		envs = filteredEnvs
	}

	// Step 7: Output results in the appropriate format.
	printListResult(envs)
	return nil
}

// printListResult outputs the list of environments in text or JSON format,
// depending on the global --json flag.
func printListResult(envs []*model.WorktreeEnv) {
	if IsJSONOutput() {
		printListResultJSON(envs)
	} else {
		printListResultText(envs)
	}
}

// listEnvJSON is the JSON output structure for a single environment
// in the list command. It mirrors the CLI contracts specification.
type listEnvJSON struct {
	Name          string            `json:"name"`
	Branch        string            `json:"branch"`
	Status        string            `json:"status"`
	WorktreePath  string            `json:"worktreePath"`
	ConfigPattern string            `json:"configPattern"`
	Services      []listServiceJSON `json:"services"`
}

// listServiceJSON is the JSON output structure for a service within
// an environment in the list command.
type listServiceJSON struct {
	Name          string `json:"name"`
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
}

// printListResultJSON outputs the environment list as structured JSON.
// The top-level key is "environments" containing an array of environment objects.
func printListResultJSON(envs []*model.WorktreeEnv) {
	type resultJSON struct {
		Environments []listEnvJSON `json:"environments"`
	}

	result := resultJSON{
		// Use an empty slice instead of nil to ensure JSON output shows []
		// instead of null when no environments are found.
		Environments: make([]listEnvJSON, 0, len(envs)),
	}

	for _, env := range envs {
		entry := listEnvJSON{
			Name:          env.Name,
			Branch:        env.Branch,
			Status:        env.Status.String(),
			WorktreePath:  env.WorktreePath,
			ConfigPattern: env.ConfigPattern.String(),
			Services:      make([]listServiceJSON, 0, len(env.PortAllocations)),
		}

		for _, pa := range env.PortAllocations {
			entry.Services = append(entry.Services, listServiceJSON{
				Name:          pa.ServiceName,
				ContainerPort: pa.ContainerPort,
				HostPort:      pa.HostPort,
			})
		}

		result.Environments = append(result.Environments, entry)
	}

	// MarshalIndent produces human-readable JSON with 2-space indentation.
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

// printListResultText outputs the environment list as a human-readable
// text table with aligned columns.
//
// The table format is:
//
//	NAME           BRANCH          STATUS    SERVICES  PORTS
//	feature-auth   feature/auth    running   3         13000,15432,16379
//	bugfix-login   bugfix/login    stopped   1         -
func printListResultText(envs []*model.WorktreeEnv) {
	if len(envs) == 0 {
		fmt.Println("No worktree environments found.")
		return
	}

	// Print header row.
	fmt.Printf("%-20s %-20s %-10s %-10s %s\n",
		"NAME", "BRANCH", "STATUS", "SERVICES", "PORTS")

	for _, env := range envs {
		serviceCount := len(env.PortAllocations)
		portsStr := FormatPortsList(env.PortAllocations)

		// Print one row per environment with fixed-width columns.
		fmt.Printf("%-20s %-20s %-10s %-10d %s\n",
			env.Name,
			env.Branch,
			env.Status.String(),
			serviceCount,
			portsStr,
		)
	}
}

// FormatPortsList converts a slice of PortAllocations into a comma-separated
// string of host ports. Returns "-" if no ports are allocated.
//
// This function is exported for testing purposes (tested in list_test.go).
//
// Example:
//
//	[{HostPort: 13000}, {HostPort: 15432}] → "13000,15432"
//	[]                                       → "-"
func FormatPortsList(allocations []model.PortAllocation) string {
	if len(allocations) == 0 {
		return "-"
	}

	// Collect all host ports as integers for proper numeric sorting.
	portNums := make([]int, 0, len(allocations))
	for _, pa := range allocations {
		portNums = append(portNums, pa.HostPort)
	}

	// Sort numerically to ensure correct ordering (e.g., 3000 before 15432).
	// Lexicographic sort would incorrectly order "15432" before "3000".
	sort.Ints(portNums)

	// Convert sorted integers back to strings for joining.
	ports := make([]string, 0, len(portNums))
	for _, p := range portNums {
		ports = append(ports, strconv.Itoa(p))
	}
	return strings.Join(ports, ",")
}
