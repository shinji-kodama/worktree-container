// Package cli — create.go implements the "worktree-container create" command.
//
// The create command is the primary user-facing operation (US1 / MVP).
// It orchestrates the full workflow of creating a Git worktree and launching
// its associated Dev Container environment with shifted ports.
//
// Orchestration steps:
//  1. Validate inputs and detect source repository
//  2. Create Git worktree (or use existing one)
//  3. Find and parse devcontainer.json
//  4. Detect configuration pattern (A/B/C/D)
//  5. Allocate shifted ports based on worktree index
//  6. Copy and rewrite devcontainer configuration
//  7. Start containers (unless --no-start)
//  8. Output results (text or JSON)
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shinji-kodama/worktree-container/internal/devcontainer"
	"github.com/shinji-kodama/worktree-container/internal/docker"
	"github.com/shinji-kodama/worktree-container/internal/model"
	"github.com/shinji-kodama/worktree-container/internal/port"
	"github.com/shinji-kodama/worktree-container/internal/worktree"
)

// createFlags holds the flag values for the create command.
// These are bound to cobra flags in NewCreateCommand.
type createFlags struct {
	base    string // --base: base commit/branch for the worktree
	path    string // --path: custom worktree directory path
	name    string // --name: custom environment name
	noStart bool   // --no-start: skip container startup
}

// NewCreateCommand creates the "create" cobra command.
// It is called from NewRootCommand to register as a subcommand.
func NewCreateCommand() *cobra.Command {
	flags := &createFlags{}

	cmd := &cobra.Command{
		Use:   "create <branch-name>",
		Short: "Create a new worktree environment with Dev Containers",
		Long: `Create a new Git worktree and launch its associated Dev Container environment.

The command automatically:
  - Creates a Git worktree for the specified branch
  - Detects the devcontainer.json configuration pattern
  - Allocates non-conflicting ports for the new environment
  - Starts the Dev Container with shifted ports

Examples:
  worktree-container create feature-auth
  worktree-container create --base main bugfix-login
  worktree-container create --path ~/dev/feature-auth feature-auth
  worktree-container create --no-start feature-auth`,

		// Args validates that exactly one positional argument (branch name) is provided.
		Args: cobra.ExactArgs(1),

		// RunE is used instead of Run so we can return errors. Cobra will
		// pass them to the Execute error handler in root.go.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), args[0], flags)
		},
	}

	// Register command-specific flags.
	cmd.Flags().StringVar(&flags.base, "base", "", "Base commit/branch for the worktree (default: HEAD)")
	cmd.Flags().StringVar(&flags.path, "path", "", "Worktree directory path (default: ../<repo>-<branch>)")
	cmd.Flags().StringVar(&flags.name, "name", "", "Environment name (default: sanitized branch name)")
	cmd.Flags().BoolVar(&flags.noStart, "no-start", false, "Create worktree only, don't start containers")

	return cmd
}

// runCreate is the main orchestration function for the create command.
// It coordinates all the steps needed to create a worktree environment.
func runCreate(ctx context.Context, branchName string, flags *createFlags) error {
	// Step 1: Determine the source repository path.
	// We need the repo root to create worktrees relative to it.
	wm := worktree.NewManager()

	cwd, err := os.Getwd()
	if err != nil {
		return model.WrapCLIError(model.ExitGeneralError, "failed to get current directory", err)
	}

	repoRoot, err := wm.GetRepoRoot(cwd)
	if err != nil {
		return model.WrapCLIError(model.ExitGitError, "not inside a Git repository", err)
	}
	VerboseLog("Source repository: %s", repoRoot)

	// Step 2: Determine environment name.
	// Default: sanitize the branch name by replacing slashes with hyphens.
	envName := flags.name
	if envName == "" {
		envName = sanitizeBranchName(branchName)
	}
	if validateErr := model.ValidateName(envName); validateErr != nil {
		return model.WrapCLIError(model.ExitGeneralError, "invalid environment name", validateErr)
	}
	VerboseLog("Environment name: %s", envName)

	// Step 3: Determine worktree path.
	// Default: sibling directory named <repo>-<envName>.
	worktreePath := flags.path
	if worktreePath == "" {
		repoName := filepath.Base(repoRoot)
		worktreePath = filepath.Join(filepath.Dir(repoRoot), repoName+"-"+envName)
	}
	// Resolve to absolute path for consistency across the codebase.
	worktreePath, err = filepath.Abs(worktreePath)
	if err != nil {
		return model.WrapCLIError(model.ExitGeneralError, "failed to resolve worktree path", err)
	}
	VerboseLog("Worktree path: %s", worktreePath)

	// Step 4: Create Git worktree.
	VerboseLog("Creating Git worktree for branch %q...", branchName)
	if addErr := wm.Add(repoRoot, branchName, worktreePath, flags.base); addErr != nil {
		return model.WrapCLIError(model.ExitGitError, "failed to create worktree", addErr)
	}
	VerboseLog("Git worktree created successfully")

	// Step 5: Find and parse devcontainer.json in the source repo.
	// We look in the source repo (not the worktree) for the original config,
	// as the worktree might not have .devcontainer/ yet.
	devcontainerPath, err := devcontainer.FindDevContainerJSON(repoRoot)
	if err != nil {
		return err // FindDevContainerJSON already returns CLIError
	}
	VerboseLog("Found devcontainer.json: %s", devcontainerPath)

	rawConfig, err := devcontainer.LoadConfig(devcontainerPath)
	if err != nil {
		return err
	}

	// Read the raw file bytes for later use by rewrite functions.
	// The rewrite functions take raw bytes (not parsed structs) so they can
	// preserve unknown fields and JSONC comments through a map-based approach.
	rawJSON, err := os.ReadFile(devcontainerPath)
	if err != nil {
		return model.WrapCLIError(model.ExitDevContainerNotFound, "failed to read devcontainer.json", err)
	}

	// Step 6: Detect configuration pattern.
	// For Compose patterns, we need to count services from the Compose file.
	composeServiceCount := 0
	composeFiles := devcontainer.GetComposeFiles(rawConfig)
	if len(composeFiles) > 0 {
		composeServiceCount = countComposeServices(rawConfig)
	}

	pattern := devcontainer.DetectPattern(rawConfig, composeServiceCount)
	VerboseLog("Detected pattern: %s", pattern)

	// Step 7: Extract ports and allocate shifted ports.
	defaultServiceName := envName
	if rawConfig.Service != "" {
		defaultServiceName = rawConfig.Service
	}
	originalPorts := devcontainer.ExtractPorts(rawConfig, defaultServiceName)
	VerboseLog("Found %d port(s) to allocate", len(originalPorts))

	// Determine worktree index by counting existing environments.
	worktreeIndex, err := determineWorktreeIndex(ctx)
	if err != nil {
		VerboseLog("Could not determine worktree index, using 1: %v", err)
		worktreeIndex = 1
	}
	VerboseLog("Worktree index: %d", worktreeIndex)

	scanner := port.NewScanner()
	allocator := port.NewAllocator(scanner)

	// Load existing allocations from running containers to avoid conflicts.
	existingAllocs, err := loadExistingAllocations(ctx)
	if err != nil {
		VerboseLog("Could not load existing allocations: %v", err)
	} else {
		allocator.SetExistingAllocations(existingAllocs)
	}

	portAllocations, err := allocator.AllocatePorts(originalPorts, worktreeIndex)
	if err != nil {
		return model.WrapCLIError(model.ExitPortAllocationFailed, "port allocation failed", err)
	}

	for _, pa := range portAllocations {
		VerboseLog("Port allocated: %s", pa.String())
	}

	// Step 8: Build labels for the environment.
	env := &model.WorktreeEnv{
		Name:            envName,
		Branch:          branchName,
		WorktreePath:    worktreePath,
		SourceRepoPath:  repoRoot,
		Status:          model.StatusRunning,
		ConfigPattern:   pattern,
		PortAllocations: portAllocations,
		CreatedAt:       time.Now().UTC(),
	}
	labels := docker.BuildLabels(env)

	// Step 9: Copy .devcontainer directory and rewrite configuration.
	srcDevcontainerDir := filepath.Dir(devcontainerPath)
	dstDevcontainerDir := filepath.Join(worktreePath, ".devcontainer")

	VerboseLog("Copying .devcontainer directory to worktree...")
	if err := devcontainer.CopyDevContainerDir(srcDevcontainerDir, dstDevcontainerDir); err != nil {
		return model.WrapCLIError(model.ExitGeneralError, "failed to copy .devcontainer directory", err)
	}

	if pattern.IsCompose() {
		// Pattern C/D: Generate Compose override YAML.
		VerboseLog("Generating Compose override YAML...")

		// Determine all services for the override.
		services := rawConfig.RunServices
		if len(services) == 0 && rawConfig.Service != "" {
			services = []string{rawConfig.Service}
		}

		overrideData, err := devcontainer.GenerateComposeOverride(envName, services, portAllocations, labels)
		if err != nil {
			return model.WrapCLIError(model.ExitGeneralError, "failed to generate Compose override", err)
		}

		overridePath := filepath.Join(dstDevcontainerDir, "docker-compose.worktree.yml")
		if writeErr := devcontainer.WriteComposeOverride(overridePath, overrideData); writeErr != nil {
			return model.WrapCLIError(model.ExitGeneralError, "failed to write Compose override", writeErr)
		}
		VerboseLog("Compose override written to: %s", overridePath)

		// Rewrite devcontainer.json to include the override file.
		rewrittenJSON, err := devcontainer.RewriteComposeConfig(rawJSON, envName, "docker-compose.worktree.yml")
		if err != nil {
			return model.WrapCLIError(model.ExitGeneralError, "failed to rewrite devcontainer.json for Compose", err)
		}

		dstDevcontainerJSON := filepath.Join(dstDevcontainerDir, "devcontainer.json")
		if err := devcontainer.WriteRewrittenConfig(dstDevcontainerJSON, rewrittenJSON); err != nil {
			return model.WrapCLIError(model.ExitGeneralError, "failed to write rewritten devcontainer.json", err)
		}
	} else {
		// Pattern A/B: Rewrite devcontainer.json directly.
		VerboseLog("Rewriting devcontainer.json for pattern %s...", pattern)
		rewrittenJSON, err := devcontainer.RewriteConfig(rawJSON, envName, worktreeIndex, portAllocations, labels)
		if err != nil {
			return model.WrapCLIError(model.ExitGeneralError, "failed to rewrite devcontainer.json", err)
		}

		dstDevcontainerJSON := filepath.Join(dstDevcontainerDir, "devcontainer.json")
		if err := devcontainer.WriteRewrittenConfig(dstDevcontainerJSON, rewrittenJSON); err != nil {
			return model.WrapCLIError(model.ExitGeneralError, "failed to write rewritten devcontainer.json", err)
		}
	}

	// Step 10: Start containers (unless --no-start).
	if !flags.noStart {
		VerboseLog("Starting containers...")
		if err := startContainers(ctx, pattern, dstDevcontainerDir, composeFiles, envName, rawConfig); err != nil {
			return err
		}
		env.Status = model.StatusRunning
	} else {
		env.Status = model.StatusStopped
		VerboseLog("Skipping container startup (--no-start)")
	}

	// Step 11: Output results.
	printCreateResult(env)
	return nil
}

// sanitizeBranchName converts a Git branch name to a valid environment name.
// Replaces "/" with "-" and strips invalid characters.
func sanitizeBranchName(branch string) string {
	// Replace common branch name separators with hyphens.
	name := strings.ReplaceAll(branch, "/", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Remove any characters that aren't alphanumeric or hyphens.
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	name = result.String()

	// Trim leading/trailing hyphens.
	name = strings.Trim(name, "-")

	if name == "" {
		name = "worktree"
	}
	return name
}

// countComposeServices returns the number of services defined in the
// devcontainer's Compose configuration. For Pattern C, this is 1.
// For Pattern D, this is 2 or more.
func countComposeServices(raw *devcontainer.RawDevContainer) int {
	// The runServices field explicitly lists which services to start.
	// If present, its length gives us the service count.
	if len(raw.RunServices) > 0 {
		return len(raw.RunServices)
	}

	// If runServices is not set, there's at least the primary service.
	if raw.Service != "" {
		return 1
	}

	return 0
}

// determineWorktreeIndex counts existing managed environments to determine
// the index for the new environment. Index 0 is reserved for the primary
// worktree (main branch), so new environments start at index 1.
func determineWorktreeIndex(ctx context.Context) (int, error) {
	// Try to connect to Docker to count existing environments.
	cli, err := docker.NewClient()
	if err != nil {
		return 1, err
	}
	defer func() { _ = cli.Close() }()

	containers, err := docker.ListManagedContainers(ctx, cli)
	if err != nil {
		return 1, err
	}

	groups := docker.GroupContainersByEnv(containers)

	// New environment gets the next index after existing ones.
	// Minimum index is 1 (index 0 is for the primary worktree).
	index := len(groups) + 1
	if index > 9 {
		return 0, fmt.Errorf("maximum of 10 environments reached (currently %d)", len(groups))
	}
	return index, nil
}

// loadExistingAllocations fetches port allocations from all currently
// managed containers. This is used to prevent port collisions with
// already-running environments.
func loadExistingAllocations(ctx context.Context) ([]model.PortAllocation, error) {
	cli, err := docker.NewClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = cli.Close() }()

	containers, err := docker.ListManagedContainers(ctx, cli)
	if err != nil {
		return nil, err
	}

	var allocs []model.PortAllocation
	for _, c := range containers {
		portAllocs, err := docker.ParsePortLabels(c.Labels)
		if err != nil {
			continue // Skip containers with invalid labels
		}
		allocs = append(allocs, portAllocs...)
	}
	return allocs, nil
}

// startContainers launches the Dev Container based on the detected pattern.
func startContainers(ctx context.Context, pattern model.ConfigPattern, devcontainerDir string, composeFiles []string, envName string, raw *devcontainer.RawDevContainer) error {
	if pattern.IsCompose() {
		// Pattern C/D: Use docker compose with the override file.
		// Build the full list of compose files: originals + override.
		allComposeFiles := make([]string, 0, len(composeFiles)+1)
		allComposeFiles = append(allComposeFiles, composeFiles...)
		allComposeFiles = append(allComposeFiles, "docker-compose.worktree.yml")

		envVars := map[string]string{
			"COMPOSE_PROJECT_NAME": envName,
		}

		VerboseLog("Running docker compose up with files: %v", allComposeFiles)
		if err := docker.ComposeUp(ctx, devcontainerDir, allComposeFiles, envVars); err != nil {
			return model.WrapCLIError(model.ExitDockerNotRunning, "failed to start Compose services", err)
		}
	} else {
		// Pattern A/B: Use docker run or devcontainer CLI.
		// For now, we use `devcontainer up` which handles building and starting.
		VerboseLog("Starting container for pattern %s...", pattern)
		if err := runDevcontainerUp(ctx, filepath.Dir(devcontainerDir)); err != nil {
			return model.WrapCLIError(model.ExitDockerNotRunning, "failed to start container", err)
		}
	}
	return nil
}

// runDevcontainerUp runs `devcontainer up` command for Pattern A/B containers.
// This delegates to the Dev Container CLI which handles image pulling,
// building, and container creation.
func runDevcontainerUp(ctx context.Context, workspaceFolder string) error {
	// Use docker compose with a single-container setup, or docker run.
	// For simplicity, we use os/exec to call `docker run` with the args
	// from the rewritten devcontainer.json.
	VerboseLog("Using devcontainer up --workspace-folder %s", workspaceFolder)

	// Try devcontainer CLI first.
	return docker.ComposeUp(ctx, workspaceFolder, nil, nil)
}

// printCreateResult outputs the create command results in text or JSON format.
func printCreateResult(env *model.WorktreeEnv) {
	if IsJSONOutput() {
		printCreateResultJSON(env)
	} else {
		printCreateResultText(env)
	}
}

// printCreateResultJSON outputs the create result as structured JSON.
func printCreateResultJSON(env *model.WorktreeEnv) {
	type serviceJSON struct {
		Name          string `json:"name"`
		ContainerPort int    `json:"containerPort"`
		HostPort      int    `json:"hostPort"`
		Protocol      string `json:"protocol"`
	}

	type resultJSON struct {
		Name          string        `json:"name"`
		Branch        string        `json:"branch"`
		WorktreePath  string        `json:"worktreePath"`
		Status        string        `json:"status"`
		ConfigPattern string        `json:"configPattern"`
		Services      []serviceJSON `json:"services"`
	}

	result := resultJSON{
		Name:          env.Name,
		Branch:        env.Branch,
		WorktreePath:  env.WorktreePath,
		Status:        env.Status.String(),
		ConfigPattern: env.ConfigPattern.String(),
	}

	for _, pa := range env.PortAllocations {
		result.Services = append(result.Services, serviceJSON{
			Name:          pa.ServiceName,
			ContainerPort: pa.ContainerPort,
			HostPort:      pa.HostPort,
			Protocol:      pa.Protocol,
		})
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

// printCreateResultText outputs the create result as human-readable text.
func printCreateResultText(env *model.WorktreeEnv) {
	serviceCount := len(env.PortAllocations)
	patternDesc := env.ConfigPattern.String()
	if serviceCount > 0 {
		patternDesc = fmt.Sprintf("%s (%d services)", patternDesc, serviceCount)
	}

	fmt.Printf("Created worktree environment %q\n", env.Name)
	fmt.Printf("  Branch:    %s\n", env.Branch)
	fmt.Printf("  Path:      %s\n", env.WorktreePath)
	fmt.Printf("  Pattern:   %s\n", patternDesc)

	if serviceCount > 0 {
		fmt.Println()
		fmt.Println("  Services:")
		for _, pa := range env.PortAllocations {
			// Format the URL/address based on whether it looks like an HTTP service.
			addr := formatServiceAddress(pa)
			fmt.Printf("    %-8s %s  (container: %d)\n",
				pa.ServiceName, addr, pa.ContainerPort)
		}
	}
}

// formatServiceAddress formats a port allocation as a user-friendly address.
// HTTP-like ports (80, 443, 3000, 8080, etc.) get http:// prefix.
func formatServiceAddress(pa model.PortAllocation) string {
	// Common HTTP port numbers that likely serve web content.
	httpPorts := map[int]bool{
		80: true, 443: true, 3000: true, 3001: true,
		4200: true, 5000: true, 5173: true, 8000: true,
		8080: true, 8443: true, 8888: true, 9000: true,
	}

	if httpPorts[pa.ContainerPort] {
		return fmt.Sprintf("http://localhost:%d", pa.HostPort)
	}
	return fmt.Sprintf("localhost:%d", pa.HostPort)
}
