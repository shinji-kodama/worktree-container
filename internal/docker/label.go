package docker

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shinji-kodama/worktree-container/internal/model"
)

// Label key constants define the Docker label keys used to persist
// worktree environment metadata on containers. These labels serve as
// the sole persistence mechanism — there is no external state file.
//
// All keys share the "worktree." prefix to namespace them and avoid
// collisions with labels set by other tools (Docker Compose, VS Code, etc.).
const (
	// LabelPrefix is the common prefix for all worktree-container labels.
	// Using a consistent prefix enables efficient label-based filtering
	// when listing containers via the Docker API.
	LabelPrefix = "worktree."

	// LabelManagedBy identifies containers managed by worktree-container.
	// This is the primary label used for filtering and discovery.
	// Key: "worktree.managed-by", Value: always "worktree-container".
	LabelManagedBy = LabelPrefix + "managed-by"

	// LabelName stores the worktree environment's unique identifier.
	// Key: "worktree.name", Value: environment name (e.g., "feature-auth").
	LabelName = LabelPrefix + "name"

	// LabelBranch stores the Git branch associated with this worktree.
	// Key: "worktree.branch", Value: branch name (e.g., "feature/auth").
	LabelBranch = LabelPrefix + "branch"

	// LabelWorktreePath stores the absolute filesystem path to the Git worktree.
	// Key: "worktree.worktree-path", Value: absolute path.
	LabelWorktreePath = LabelPrefix + "worktree-path"

	// LabelSourceRepo stores the absolute filesystem path to the original
	// Git repository (the one from which the worktree was created).
	// Key: "worktree.source-repo", Value: absolute path.
	LabelSourceRepo = LabelPrefix + "source-repo"

	// LabelOriginalPortPrefix is the prefix for per-port labels.
	// Each port mapping gets its own label with the container port appended:
	//   "worktree.original-port.3000" = "13000"
	// This allows reconstructing the full port mapping table from labels.
	LabelOriginalPortPrefix = LabelPrefix + "original-port."

	// LabelConfigPattern stores the detected devcontainer.json pattern type.
	// Key: "worktree.config-pattern", Value: one of "image", "dockerfile",
	// "compose-single", "compose-multi".
	LabelConfigPattern = LabelPrefix + "config-pattern"

	// LabelCreatedAt stores the ISO-8601 timestamp of environment creation.
	// Key: "worktree.created-at", Value: RFC3339 formatted timestamp.
	LabelCreatedAt = LabelPrefix + "created-at"
)

// ManagedByValue is the constant value for the LabelManagedBy label.
// All containers created by this CLI are tagged with this value,
// enabling discovery via Docker API label filters.
const ManagedByValue = "worktree-container"

// BuildLabels constructs a Docker label map from a WorktreeEnv.
// These labels are applied to every container in the environment,
// allowing full reconstruction of the WorktreeEnv from container
// inspection alone (no external state file needed).
//
// Port allocations are encoded as individual labels using the format:
//
//	"worktree.original-port.<containerPort>" = "<hostPort>"
//
// This per-port label design avoids encoding/parsing complex structures
// in a single label value, keeping the labels human-readable when
// inspecting containers with `docker inspect`.
func BuildLabels(env *model.WorktreeEnv) map[string]string {
	labels := map[string]string{
		LabelManagedBy:    ManagedByValue,
		LabelName:         env.Name,
		LabelBranch:       env.Branch,
		LabelWorktreePath: env.WorktreePath,
		LabelSourceRepo:   env.SourceRepoPath,
		LabelConfigPattern: env.ConfigPattern.String(),
		// time.RFC3339 produces ISO-8601 compatible timestamps like
		// "2026-02-28T10:00:00Z". Using UTC ensures consistency
		// regardless of the host machine's timezone.
		LabelCreatedAt: env.CreatedAt.UTC().Format(time.RFC3339),
	}

	// Encode each port allocation as a separate label.
	// This approach trades label count for simplicity — each port
	// mapping is self-contained and independently parseable.
	for _, pa := range env.PortAllocations {
		key := BuildPortLabel(pa.ContainerPort)
		labels[key] = strconv.Itoa(pa.HostPort)
	}

	return labels
}

// ParseLabels reconstructs a WorktreeEnv from Docker container labels.
// This is the inverse of BuildLabels and is used when listing or
// inspecting containers to rebuild the domain model.
//
// Required labels: managed-by, name, branch, worktree-path, source-repo,
// config-pattern, created-at. Missing required labels cause an error.
//
// Note: Status and Containers are NOT reconstructed from labels because
// they are determined at runtime from Docker container state, not from
// static label values.
func ParseLabels(labels map[string]string) (*model.WorktreeEnv, error) {
	// Validate that all required labels are present.
	// We check them all at once rather than failing on the first missing one,
	// so the error message can list all missing labels for easier debugging.
	requiredKeys := []string{
		LabelManagedBy,
		LabelName,
		LabelBranch,
		LabelWorktreePath,
		LabelSourceRepo,
		LabelConfigPattern,
		LabelCreatedAt,
	}

	var missing []string
	for _, key := range requiredKeys {
		if _, ok := labels[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required Docker labels: %s", strings.Join(missing, ", "))
	}

	// Verify this container is actually managed by worktree-container.
	if labels[LabelManagedBy] != ManagedByValue {
		return nil, fmt.Errorf(
			"label %s has unexpected value %q (expected %q)",
			LabelManagedBy, labels[LabelManagedBy], ManagedByValue,
		)
	}

	// Parse the config pattern string into the typed enum.
	pattern, err := model.ParseConfigPattern(labels[LabelConfigPattern])
	if err != nil {
		return nil, fmt.Errorf("invalid label %s: %w", LabelConfigPattern, err)
	}

	// Parse the ISO-8601 timestamp.
	// time.RFC3339 is Go's constant for the ISO-8601 / RFC-3339 format.
	createdAt, err := time.Parse(time.RFC3339, labels[LabelCreatedAt])
	if err != nil {
		return nil, fmt.Errorf("invalid label %s: %w", LabelCreatedAt, err)
	}

	// Extract port allocations from labels.
	ports, err := ParsePortLabels(labels)
	if err != nil {
		return nil, fmt.Errorf("failed to parse port labels: %w", err)
	}

	return &model.WorktreeEnv{
		Name:            labels[LabelName],
		Branch:          labels[LabelBranch],
		WorktreePath:    labels[LabelWorktreePath],
		SourceRepoPath:  labels[LabelSourceRepo],
		ConfigPattern:   pattern,
		PortAllocations: ports,
		CreatedAt:       createdAt,
	}, nil
}

// BuildPortLabel generates a Docker label key for a specific container port.
// The format is "worktree.original-port.<containerPort>", for example:
//
//	BuildPortLabel(3000) → "worktree.original-port.3000"
//
// This key is paired with the host port as the value in the label map.
func BuildPortLabel(containerPort int) string {
	return fmt.Sprintf("%s%d", LabelOriginalPortPrefix, containerPort)
}

// ParsePortLabels extracts all port allocation entries from a Docker
// label map. It scans for labels with the LabelOriginalPortPrefix and
// parses both the container port (from the key suffix) and the host
// port (from the label value).
//
// Returns an empty slice (not nil) if no port labels are found.
// Returns an error if any port label has a malformed key or value.
func ParsePortLabels(labels map[string]string) ([]model.PortAllocation, error) {
	// Pre-allocate with zero length but some capacity to avoid repeated
	// slice growth in the common case of 1-5 port mappings.
	allocations := make([]model.PortAllocation, 0, 4)

	for key, value := range labels {
		// Check if this label key starts with the port prefix.
		if !strings.HasPrefix(key, LabelOriginalPortPrefix) {
			continue
		}

		// Extract the container port from the key suffix.
		// For "worktree.original-port.3000", the suffix is "3000".
		portStr := strings.TrimPrefix(key, LabelOriginalPortPrefix)
		containerPort, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid container port in label key %q: %w", key, err,
			)
		}

		// Parse the host port from the label value.
		hostPort, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid host port in label %q=%q: %w", key, value, err,
			)
		}

		allocations = append(allocations, model.PortAllocation{
			ContainerPort: containerPort,
			HostPort:      hostPort,
			// Protocol defaults to "tcp" as specified in the data model.
			// The protocol is not stored in labels because the vast majority
			// of use cases are TCP. UDP ports would need a separate label
			// scheme if ever needed in the future.
			Protocol: "tcp",
		})
	}

	return allocations, nil
}

// FilterLabels returns a label filter map suitable for use with the Docker
// API's container listing endpoint. The returned map filters for containers
// that have the LabelManagedBy label set to ManagedByValue, effectively
// listing only containers managed by worktree-container.
//
// Usage with Docker SDK:
//
//	filters := docker.FilterLabels()
//	containers, err := cli.ContainerList(ctx, container.ListOptions{
//	    Filters: filters.NewArgs(filters.Arg("label", ...)),
//	})
func FilterLabels() map[string]string {
	return map[string]string{
		LabelManagedBy: ManagedByValue,
	}
}
