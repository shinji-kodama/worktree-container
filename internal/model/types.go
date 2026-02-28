// Package model defines the domain types for the worktree-container CLI.
//
// All entities in this package represent the core data structures described
// in the data-model.md specification. These types are used throughout the
// application for passing data between components.
//
// Key design decision: All state is persisted via Docker container labels
// (FR-011), so these types are transient representations reconstructed
// from Docker API queries at runtime.
package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// WorktreeStatus represents the lifecycle state of a worktree environment.
// The state transitions are:
//
//	[Created] → Running → Stopped ⇄ Running → [Deleted]
//	Running/Stopped → Orphaned (when Git worktree is manually deleted)
type WorktreeStatus string

const (
	// StatusRunning indicates all containers in the environment are running.
	StatusRunning WorktreeStatus = "running"

	// StatusStopped indicates containers exist but are not running.
	// Configuration and data are preserved.
	StatusStopped WorktreeStatus = "stopped"

	// StatusOrphaned indicates the Git worktree directory no longer exists,
	// but Docker containers/resources remain. This typically happens when
	// a user manually deletes the worktree directory.
	StatusOrphaned WorktreeStatus = "orphaned"
)

// String returns the string representation of WorktreeStatus.
// This method satisfies the fmt.Stringer interface, enabling
// human-readable output in CLI commands and logging.
func (s WorktreeStatus) String() string {
	return string(s)
}

// IsValid checks whether the WorktreeStatus value is one of the
// predefined valid states.
func (s WorktreeStatus) IsValid() bool {
	switch s {
	case StatusRunning, StatusStopped, StatusOrphaned:
		return true
	default:
		return false
	}
}

// ParseWorktreeStatus converts a string to a WorktreeStatus.
// Returns an error if the string does not match any valid status.
func ParseWorktreeStatus(s string) (WorktreeStatus, error) {
	status := WorktreeStatus(strings.ToLower(s))
	if !status.IsValid() {
		return "", fmt.Errorf("invalid worktree status: %q (valid: running, stopped, orphaned)", s)
	}
	return status, nil
}

// ConfigPattern represents the type of devcontainer.json configuration.
// The pattern determines how the CLI generates worktree-specific configuration
// files and manages containers.
//
// Pattern detection logic (from spec):
//   - No dockerComposeFile + No build → PatternImage (A)
//   - No dockerComposeFile + build present → PatternDockerfile (B)
//   - dockerComposeFile + 1 service → PatternComposeSingle (C)
//   - dockerComposeFile + 2+ services → PatternComposeMulti (D)
type ConfigPattern string

const (
	// PatternImage (Pattern A) uses a pre-built container image directly.
	// Example: {"image": "mcr.microsoft.com/devcontainers/typescript-node:20"}
	PatternImage ConfigPattern = "image"

	// PatternDockerfile (Pattern B) builds an image from a Dockerfile.
	// Example: {"build": {"dockerfile": "Dockerfile", "context": ".."}}
	PatternDockerfile ConfigPattern = "dockerfile"

	// PatternComposeSingle (Pattern C) uses Docker Compose with a single service.
	// Example: {"dockerComposeFile": "docker-compose.yml", "service": "app"}
	PatternComposeSingle ConfigPattern = "compose-single"

	// PatternComposeMulti (Pattern D) uses Docker Compose with multiple services.
	// Example: {"dockerComposeFile": ["docker-compose.yml"], "service": "app", "runServices": ["app", "db"]}
	PatternComposeMulti ConfigPattern = "compose-multi"
)

// String returns the string representation of ConfigPattern.
func (p ConfigPattern) String() string {
	return string(p)
}

// IsValid checks whether the ConfigPattern value is one of the
// predefined valid patterns.
func (p ConfigPattern) IsValid() bool {
	switch p {
	case PatternImage, PatternDockerfile, PatternComposeSingle, PatternComposeMulti:
		return true
	default:
		return false
	}
}

// IsCompose returns true if the pattern uses Docker Compose.
// This is useful for branching logic that applies only to
// Compose-based configurations (e.g., override YAML generation).
func (p ConfigPattern) IsCompose() bool {
	return p == PatternComposeSingle || p == PatternComposeMulti
}

// ParseConfigPattern converts a string to a ConfigPattern.
// Returns an error if the string does not match any valid pattern.
func ParseConfigPattern(s string) (ConfigPattern, error) {
	pattern := ConfigPattern(strings.ToLower(s))
	if !pattern.IsValid() {
		return "", fmt.Errorf("invalid config pattern: %q (valid: image, dockerfile, compose-single, compose-multi)", s)
	}
	return pattern, nil
}

// WorktreeEnv represents a worktree environment — a Git worktree paired with
// its Dev Container setup. This is the primary aggregate entity in the domain.
//
// All fields are reconstructed at runtime from Docker container labels
// (see Docker label schema in data-model.md). There is no persistent
// state file on disk.
type WorktreeEnv struct {
	// Name is the unique identifier for this worktree environment.
	// Must contain only alphanumeric characters and hyphens.
	Name string `json:"name"`

	// Branch is the Git branch name associated with this worktree.
	Branch string `json:"branch"`

	// WorktreePath is the absolute filesystem path to the Git worktree directory.
	WorktreePath string `json:"worktreePath"`

	// SourceRepoPath is the absolute filesystem path to the original Git repository.
	SourceRepoPath string `json:"sourceRepoPath"`

	// Status is the current lifecycle state of the environment.
	Status WorktreeStatus `json:"status"`

	// ConfigPattern indicates which devcontainer.json pattern (A/B/C/D) is used.
	ConfigPattern ConfigPattern `json:"configPattern"`

	// Containers holds information about all Docker containers belonging
	// to this environment. Must contain at least one container.
	Containers []ContainerInfo `json:"containers,omitempty"`

	// PortAllocations holds all port mappings for this environment.
	// May be empty if no ports are forwarded.
	PortAllocations []PortAllocation `json:"portAllocations,omitempty"`

	// CreatedAt is the timestamp when this environment was created.
	CreatedAt time.Time `json:"createdAt"`
}

// nameRegex validates environment names: alphanumeric + hyphens only,
// must start and end with alphanumeric.
var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)

// ValidateName checks if the given name is a valid worktree environment name.
// Valid names contain only alphanumeric characters and hyphens,
// and must start/end with an alphanumeric character.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("environment name must not be empty")
	}
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("invalid environment name %q: must contain only alphanumeric characters and hyphens, and start/end with alphanumeric", name)
	}
	return nil
}

// PortAllocation represents a single port mapping between a container port
// and a host port within a worktree environment.
//
// The port shifting algorithm assigns host ports using the formula:
//
//	shiftedPort = originalPort + (worktreeIndex * 10000)
//
// If the result exceeds 65535, dynamic port discovery is used via net.Listen().
type PortAllocation struct {
	// ServiceName is the Docker container or Compose service name
	// that owns this port mapping.
	ServiceName string `json:"serviceName"`

	// ContainerPort is the port number inside the container (1-65535).
	ContainerPort int `json:"containerPort"`

	// HostPort is the port number on the host machine (1024-65535).
	// Must be unique across all worktree environments and not conflict
	// with ports used by other system processes.
	HostPort int `json:"hostPort"`

	// Protocol is the network protocol for the port mapping.
	// Defaults to "tcp". Also supports "udp".
	Protocol string `json:"protocol"`

	// Label is an optional human-readable description for this port,
	// typically sourced from portsAttributes.label in devcontainer.json.
	Label string `json:"label,omitempty"`
}

// Validate checks whether the PortAllocation has valid field values.
// It verifies port number ranges and protocol values.
func (p *PortAllocation) Validate() error {
	if p.ServiceName == "" {
		return fmt.Errorf("port allocation: service name must not be empty")
	}
	if p.ContainerPort < 1 || p.ContainerPort > 65535 {
		return fmt.Errorf("port allocation: container port %d out of range (1-65535)", p.ContainerPort)
	}
	if p.HostPort < 1024 || p.HostPort > 65535 {
		return fmt.Errorf("port allocation: host port %d out of range (1024-65535)", p.HostPort)
	}
	if p.Protocol == "" {
		p.Protocol = "tcp"
	}
	if p.Protocol != "tcp" && p.Protocol != "udp" {
		return fmt.Errorf("port allocation: invalid protocol %q (valid: tcp, udp)", p.Protocol)
	}
	return nil
}

// String returns a human-readable representation of the port allocation.
// Format: "service:containerPort → hostPort/protocol"
func (p *PortAllocation) String() string {
	proto := p.Protocol
	if proto == "" {
		proto = "tcp"
	}
	return fmt.Sprintf("%s:%d → %d/%s", p.ServiceName, p.ContainerPort, p.HostPort, proto)
}

// ValidatePortAllocations checks a slice of PortAllocations for
// individual validity and cross-allocation host port uniqueness.
// This enforces the "port collision zero" constitution principle.
func ValidatePortAllocations(allocations []PortAllocation) error {
	// Track seen host ports to detect duplicates within the same environment.
	// Key: "hostPort/protocol", Value: service name that owns it.
	seen := make(map[string]string)

	for i := range allocations {
		// Validate each allocation individually first.
		if err := allocations[i].Validate(); err != nil {
			return err
		}

		// Build a unique key combining port and protocol to detect duplicates.
		// Different protocols on the same port are allowed (e.g., 3000/tcp and 3000/udp).
		key := fmt.Sprintf("%d/%s", allocations[i].HostPort, allocations[i].Protocol)
		if existingService, exists := seen[key]; exists {
			return fmt.Errorf("port allocation: host port %s is used by both %q and %q",
				key, existingService, allocations[i].ServiceName)
		}
		seen[key] = allocations[i].ServiceName
	}
	return nil
}

// ContainerInfo holds runtime information about a Docker container.
// This data is fetched dynamically from the Docker API, not persisted.
type ContainerInfo struct {
	// ContainerID is the unique Docker container identifier (SHA-256 hash prefix).
	ContainerID string `json:"containerId"`

	// ContainerName is the human-readable Docker container name.
	ContainerName string `json:"containerName"`

	// ServiceName is the Docker Compose service name, if applicable.
	// Empty for non-Compose containers (Pattern A/B).
	ServiceName string `json:"serviceName,omitempty"`

	// Status is the Docker container status (e.g., "running", "exited", "created").
	Status string `json:"status"`

	// Labels is the full set of Docker labels on the container.
	// Includes worktree-container management labels (worktree.* prefix).
	Labels map[string]string `json:"labels,omitempty"`
}

// DevContainerConfig represents the parsed and transformed devcontainer.json
// configuration for a specific worktree environment.
//
// The original devcontainer.json is NEVER modified (FR-012). Instead,
// a copy is created in the worktree directory with environment-specific
// modifications (port shifts, labels, container names).
type DevContainerConfig struct {
	// OriginalPath is the absolute path to the source devcontainer.json.
	// This file is treated as read-only.
	OriginalPath string `json:"originalPath"`

	// WorktreePath is the absolute path to the worktree-specific
	// devcontainer.json (a modified copy of the original).
	WorktreePath string `json:"worktreePath"`

	// Pattern is the detected configuration pattern (A/B/C/D).
	Pattern ConfigPattern `json:"pattern"`

	// OriginalPorts contains port definitions extracted from the original
	// devcontainer.json (from forwardPorts, appPort, etc.).
	OriginalPorts []PortSpec `json:"originalPorts,omitempty"`

	// ComposeFiles lists Docker Compose YAML file paths.
	// Only populated for Compose patterns (C/D).
	ComposeFiles []string `json:"composeFiles,omitempty"`

	// PrimaryService is the main service name from the "service" field
	// in devcontainer.json. Only populated for Compose patterns (C/D).
	PrimaryService string `json:"primaryService,omitempty"`

	// AllServices lists all service names defined in the Compose file(s).
	// Only populated for Compose patterns (C/D).
	AllServices []string `json:"allServices,omitempty"`

	// OverrideYAMLPath is the path to the generated docker-compose override
	// YAML file. Only populated for Compose patterns (C/D).
	OverrideYAMLPath string `json:"overrideYamlPath,omitempty"`
}

// PortSpec represents a port definition extracted from devcontainer.json.
// This is a normalized representation that abstracts over the different
// port specification formats found in devcontainer.json (forwardPorts,
// appPort, Compose ports).
type PortSpec struct {
	// ServiceName is the container/service that owns this port.
	// For Pattern A/B, this defaults to the container name.
	// For Pattern C/D, this is the Compose service name.
	ServiceName string `json:"serviceName"`

	// ContainerPort is the port number inside the container.
	ContainerPort int `json:"containerPort"`

	// HostPort is the originally specified host port.
	// May be 0 if only a container port was specified (e.g., forwardPorts).
	HostPort int `json:"hostPort"`

	// Protocol is the network protocol (tcp/udp). Defaults to "tcp".
	Protocol string `json:"protocol"`

	// Label is an optional description from portsAttributes.
	Label string `json:"label,omitempty"`
}

// ExitCode defines standard CLI exit codes per the contracts specification.
// These codes allow scripts and CI systems to programmatically determine
// the outcome of a command.
type ExitCode int

const (
	// ExitSuccess indicates the command completed successfully.
	ExitSuccess ExitCode = 0

	// ExitGeneralError indicates an unspecified error occurred.
	ExitGeneralError ExitCode = 1

	// ExitDevContainerNotFound indicates devcontainer.json was not found
	// in the expected location.
	ExitDevContainerNotFound ExitCode = 2

	// ExitDockerNotRunning indicates the Docker daemon is not accessible.
	ExitDockerNotRunning ExitCode = 3

	// ExitPortAllocationFailed indicates a port could not be allocated
	// without conflicting with existing allocations.
	ExitPortAllocationFailed ExitCode = 4

	// ExitGitError indicates a Git operation (worktree add/remove) failed.
	ExitGitError ExitCode = 5

	// ExitEnvNotFound indicates the specified worktree environment
	// does not exist.
	ExitEnvNotFound ExitCode = 6

	// ExitUserCancelled indicates the user cancelled an interactive prompt.
	ExitUserCancelled ExitCode = 7
)

// CLIError is a custom error type that carries an exit code.
// This allows the CLI layer to translate domain errors into
// appropriate process exit codes.
type CLIError struct {
	// Code is the exit code to return to the OS.
	Code ExitCode

	// Message is the human-readable error description.
	Message string

	// Err is the underlying error, if any.
	Err error
}

// Error satisfies the error interface. It returns the human-readable
// error message, optionally including the underlying error.
func (e *CLIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error for use with errors.Is/errors.As.
// This follows Go's error wrapping convention introduced in Go 1.13.
func (e *CLIError) Unwrap() error {
	return e.Err
}

// NewCLIError creates a new CLIError with the given exit code and message.
func NewCLIError(code ExitCode, message string) *CLIError {
	return &CLIError{Code: code, Message: message}
}

// WrapCLIError creates a new CLIError that wraps an existing error.
func WrapCLIError(code ExitCode, message string, err error) *CLIError {
	return &CLIError{Code: code, Message: message, Err: err}
}
