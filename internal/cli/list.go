// Package cli — list.go implements the "worktree-container list" command.
//
// The list command displays all managed worktree environments by querying
// Docker for containers with the "worktree.managed-by=worktree-container"
// label. Containers are grouped by environment name and presented as a
// text table or JSON array, depending on the --json flag.
//
// An optional --status flag allows filtering by environment lifecycle state
// (running, stopped, orphaned, or all).
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mmr-tortoise/worktree-container/internal/docker"
	"github.com/mmr-tortoise/worktree-container/internal/model"
)

// listFlags holds the flag values for the list command.
// These are bound to cobra flags in NewListCommand.
type listFlags struct {
	// status filters environments by their lifecycle state.
	// Valid values: "running", "stopped", "orphaned", "all" (default).
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
  worktree-container list
  worktree-container list --status running
  worktree-container list --json`,

		// No positional arguments are required for the list command.
		Args: cobra.NoArgs,

		// RunE returns an error to the root command's error handler.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), flags)
		},
	}

	// Register the --status flag with a default value of "all".
	cmd.Flags().StringVar(&flags.status, "status", "all",
		"Filter by status: running, stopped, orphaned, all (default: all)")

	return cmd
}

// runList is the main logic function for the list command.
// It connects to Docker, discovers managed environments, applies the
// status filter, and outputs results in the appropriate format.
func runList(ctx context.Context, flags *listFlags) error {
	// Step 1: Validate the --status flag value.
	statusFilter := flags.status
	if statusFilter != "all" {
		if _, err := model.ParseWorktreeStatus(statusFilter); err != nil {
			return model.WrapCLIError(model.ExitGeneralError,
				fmt.Sprintf("invalid status filter %q: valid values are running, stopped, orphaned, all", statusFilter), nil)
		}
	}

	// Step 2: Connect to Docker and verify the daemon is available.
	cli, err := docker.NewClient()
	if err != nil {
		return err // NewClient already returns CLIError with ExitDockerNotRunning
	}
	// defer ensures the Docker client is closed when this function returns,
	// releasing the underlying HTTP connection and resources.
	defer func() { _ = cli.Close() }()

	VerboseLog("Connected to Docker daemon")

	// Step 3: List all containers that are managed by worktree-container.
	containers, err := docker.ListManagedContainers(ctx, cli)
	if err != nil {
		return err // ListManagedContainers already returns CLIError
	}
	VerboseLog("Found %d managed containers", len(containers))

	// Step 4: Group containers by environment name.
	// Each environment may have one or more containers (e.g., app + db).
	groups := docker.GroupContainersByEnv(containers)

	// Step 5: Build WorktreeEnv domain objects for each group.
	var envs []*model.WorktreeEnv
	for envName, containerGroup := range groups {
		env, err := docker.BuildWorktreeEnv(envName, containerGroup)
		if err != nil {
			// Log the error but continue processing other environments.
			// A single corrupted environment should not prevent listing others.
			VerboseLog("Warning: skipping environment %q: %v", envName, err)
			continue
		}
		envs = append(envs, env)
	}

	// Step 6: Sort environments alphabetically by name for consistent output.
	sort.Slice(envs, func(i, j int) bool {
		return envs[i].Name < envs[j].Name
	})

	// Step 7: Apply the --status filter if specified.
	if statusFilter != "all" {
		filteredEnvs := make([]*model.WorktreeEnv, 0, len(envs))
		for _, env := range envs {
			if env.Status.String() == statusFilter {
				filteredEnvs = append(filteredEnvs, env)
			}
		}
		envs = filteredEnvs
	}

	// Step 8: Output results in the appropriate format.
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
