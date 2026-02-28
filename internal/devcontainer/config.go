// Package devcontainer handles parsing and analysis of devcontainer.json files.
//
// The devcontainer.json specification supports JSONC (JSON with Comments),
// so this package uses github.com/tidwall/jsonc to strip comments before
// parsing with the standard encoding/json library.
//
// Key responsibilities:
//   - Load and parse devcontainer.json (with JSONC support)
//   - Detect the configuration pattern (image / dockerfile / compose-single / compose-multi)
//   - Extract port specifications from various devcontainer.json fields
//   - Locate devcontainer.json in standard paths
package devcontainer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shinji-kodama/worktree-container/internal/model"
	"github.com/tidwall/jsonc"
)

// RawDevContainer represents the raw JSON structure of a devcontainer.json file.
// Only the fields relevant to worktree-container are included; other fields
// are silently ignored during parsing.
//
// Several fields use interface{} types because the devcontainer.json spec allows
// multiple value types for the same field (e.g., dockerComposeFile can be a
// string or an array of strings).
type RawDevContainer struct {
	// Name is the display name for the dev container.
	Name string `json:"name"`

	// Image is the Docker image to use when the container is created directly
	// from an image (Pattern A).
	Image string `json:"image,omitempty"`

	// Build specifies how to build the Docker image from a Dockerfile (Pattern B).
	Build *BuildConfig `json:"build,omitempty"`

	// DockerComposeFile is the path(s) to Docker Compose file(s).
	// Can be a single string or an array of strings in devcontainer.json.
	// We use interface{} to handle both cases during deserialization.
	DockerComposeFile interface{} `json:"dockerComposeFile,omitempty"`

	// Service is the name of the primary service in the Docker Compose file
	// that the dev container attaches to.
	Service string `json:"service,omitempty"`

	// RunServices lists which Compose services to start. If omitted, all
	// services in the Compose file are started.
	RunServices []string `json:"runServices,omitempty"`

	// WorkspaceFolder is the path inside the container where the project
	// source will be mounted.
	WorkspaceFolder string `json:"workspaceFolder,omitempty"`

	// ForwardPorts lists ports to forward from the container to the host.
	// Each element can be an integer (container port only) or a string
	// like "service:port" for Compose multi-service setups.
	ForwardPorts []interface{} `json:"forwardPorts,omitempty"`

	// AppPort defines ports to publish from the container. Can be a single
	// string ("hostPort:containerPort"), a single integer, or an array of
	// strings/integers. We use interface{} to handle all cases.
	AppPort interface{} `json:"appPort,omitempty"`

	// PortsAttributes provides metadata (labels, auto-forward behavior) for
	// specific ports. The map key is the port number as a string.
	PortsAttributes map[string]PortAttribute `json:"portsAttributes,omitempty"`

	// ContainerEnv sets environment variables inside the container.
	ContainerEnv map[string]string `json:"containerEnv,omitempty"`

	// RunArgs provides additional arguments to pass to `docker run`.
	// Only applicable for non-Compose patterns (A/B).
	RunArgs []string `json:"runArgs,omitempty"`

	// ShutdownAction controls what happens when the dev container is stopped.
	// Common values: "none", "stopCompose".
	ShutdownAction string `json:"shutdownAction,omitempty"`
}

// BuildConfig holds the Dockerfile build configuration.
// This corresponds to the "build" object in devcontainer.json.
type BuildConfig struct {
	// Dockerfile is the relative path to the Dockerfile.
	Dockerfile string `json:"dockerfile,omitempty"`

	// Context is the Docker build context path, relative to devcontainer.json.
	Context string `json:"context,omitempty"`

	// Args are build-time variables passed to the Dockerfile via --build-arg.
	Args map[string]string `json:"args,omitempty"`
}

// PortAttribute holds metadata about a port, sourced from the
// "portsAttributes" field in devcontainer.json. These attributes
// provide display labels and auto-forwarding behavior hints.
type PortAttribute struct {
	// Label is a human-readable description for the port.
	Label string `json:"label,omitempty"`

	// OnAutoForward controls the IDE's behavior when the port is detected.
	// Common values: "notify", "openBrowser", "silent", "ignore".
	OnAutoForward string `json:"onAutoForward,omitempty"`
}

// LoadConfig reads a devcontainer.json file, strips JSONC comments, and
// parses it into a RawDevContainer struct.
//
// The function uses github.com/tidwall/jsonc to handle JSONC (JSON with
// Comments) format, which is common in devcontainer.json files. After
// stripping comments, it uses the standard encoding/json for parsing.
//
// Returns a CLIError with ExitDevContainerNotFound if the file does not exist.
func LoadConfig(devcontainerPath string) (*RawDevContainer, error) {
	// Read the raw file contents. os.ReadFile is preferred over os.Open+io.ReadAll
	// because it handles the open-read-close lifecycle in a single call.
	data, err := os.ReadFile(devcontainerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, model.WrapCLIError(
				model.ExitDevContainerNotFound,
				fmt.Sprintf("devcontainer.json not found: %s", devcontainerPath),
				err,
			)
		}
		return nil, fmt.Errorf("failed to read devcontainer.json: %w", err)
	}

	// Strip JSONC comments (// and /* */) and trailing commas before parsing.
	// The devcontainer.json spec officially supports JSONC, so real-world
	// files frequently contain comments.
	cleanJSON := jsonc.ToJSON(data)

	// Parse the cleaned JSON into our struct. encoding/json silently ignores
	// fields not defined in the struct, which is the desired behavior since
	// we only care about a subset of devcontainer.json fields.
	var raw RawDevContainer
	if err := json.Unmarshal(cleanJSON, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer.json at %s: %w", devcontainerPath, err)
	}

	return &raw, nil
}

// DetectPattern determines the devcontainer configuration pattern based on
// the parsed configuration fields.
//
// The detection logic follows a priority-based approach:
//  1. If dockerComposeFile is present → Compose pattern
//     - composeServiceCount == 1 → PatternComposeSingle (C)
//     - composeServiceCount >= 2 → PatternComposeMulti (D)
//  2. If build field is present → PatternDockerfile (B)
//  3. Otherwise → PatternImage (A) (fallback/default)
//
// The composeServiceCount parameter represents the number of services
// defined in the Docker Compose file(s). This count must be determined
// externally (by parsing the Compose files) because devcontainer.json
// itself does not contain this information.
func DetectPattern(raw *RawDevContainer, composeServiceCount int) model.ConfigPattern {
	// Check for Compose patterns first, since a Compose configuration
	// takes precedence over other fields (a devcontainer.json with both
	// dockerComposeFile and image should be treated as Compose).
	if raw.DockerComposeFile != nil {
		if composeServiceCount >= 2 {
			return model.PatternComposeMulti
		}
		return model.PatternComposeSingle
	}

	// Check for Dockerfile build pattern. The presence of the Build struct
	// (even if partially filled) indicates the user wants to build from
	// a Dockerfile rather than use a pre-built image.
	if raw.Build != nil {
		return model.PatternDockerfile
	}

	// Default to image pattern. This covers both explicit "image" fields
	// and configurations that rely on the default image.
	return model.PatternImage
}

// ExtractPorts collects port specifications from all port-related fields
// in devcontainer.json and returns a normalized list of PortSpec values.
//
// Port sources in devcontainer.json:
//   - forwardPorts: array of int or "service:port" strings
//   - appPort: string "host:container", int, or array of these
//   - portsAttributes: only provides metadata (labels), not port definitions
//
// The defaultServiceName parameter is used as the ServiceName for ports
// that don't specify a service (e.g., plain integers in forwardPorts).
// For Compose patterns, this is typically the primary service name.
func ExtractPorts(raw *RawDevContainer, defaultServiceName string) []model.PortSpec {
	var ports []model.PortSpec

	// Step 1: Parse forwardPorts.
	// Each entry can be:
	//   - A number (int or float64 from JSON): just the container port
	//   - A string like "db:5432": service name and container port
	for _, fp := range raw.ForwardPorts {
		switch v := fp.(type) {
		case float64:
			// JSON numbers are always parsed as float64 in Go's encoding/json
			// when the target type is interface{}. We convert to int.
			ports = append(ports, model.PortSpec{
				ServiceName:   defaultServiceName,
				ContainerPort: int(v),
				Protocol:      "tcp",
			})
		case string:
			// Parse "service:port" format. Split on ":" and extract
			// the service name and port number.
			ps := parseServicePort(v, defaultServiceName)
			if ps != nil {
				ports = append(ports, *ps)
			}
		}
	}

	// Step 2: Parse appPort.
	// The appPort field supports multiple formats:
	//   - Single int: just the container port
	//   - Single string: "hostPort:containerPort"
	//   - Array of ints or strings
	ports = append(ports, parseAppPort(raw.AppPort, defaultServiceName)...)

	// Step 3: Enrich ports with labels from portsAttributes.
	// portsAttributes is keyed by port number (as string) and provides
	// display metadata. We match each port's ContainerPort against the keys.
	if raw.PortsAttributes != nil {
		for i := range ports {
			portKey := strconv.Itoa(ports[i].ContainerPort)
			if attr, ok := raw.PortsAttributes[portKey]; ok {
				ports[i].Label = attr.Label
			}
		}
	}

	return ports
}

// parseServicePort parses a "service:port" string into a PortSpec.
// If the string contains a colon, it's treated as "serviceName:containerPort".
// If parsing fails, returns nil.
func parseServicePort(s string, defaultServiceName string) *model.PortSpec {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		// No colon found — try to parse as a plain port number.
		port, err := strconv.Atoi(s)
		if err != nil {
			return nil
		}
		return &model.PortSpec{
			ServiceName:   defaultServiceName,
			ContainerPort: port,
			Protocol:      "tcp",
		}
	}

	// parts[0] = service name, parts[1] = port number
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}

	return &model.PortSpec{
		ServiceName:   parts[0],
		ContainerPort: port,
		Protocol:      "tcp",
	}
}

// parseAppPort handles the various formats of the appPort field.
// appPort can be:
//   - nil: no ports defined
//   - float64: a single container port number (JSON number → float64 in interface{})
//   - string: "hostPort:containerPort" mapping
//   - []interface{}: an array of the above types
func parseAppPort(appPort interface{}, defaultServiceName string) []model.PortSpec {
	if appPort == nil {
		return nil
	}

	var ports []model.PortSpec

	switch v := appPort.(type) {
	case float64:
		// Single integer port (JSON numbers decode to float64 via interface{}).
		ports = append(ports, model.PortSpec{
			ServiceName:   defaultServiceName,
			ContainerPort: int(v),
			Protocol:      "tcp",
		})
	case string:
		// Single "hostPort:containerPort" string.
		ps := parseAppPortString(v, defaultServiceName)
		if ps != nil {
			ports = append(ports, *ps)
		}
	case []interface{}:
		// Array of ports — each element can be a number or a string.
		for _, item := range v {
			switch iv := item.(type) {
			case float64:
				ports = append(ports, model.PortSpec{
					ServiceName:   defaultServiceName,
					ContainerPort: int(iv),
					Protocol:      "tcp",
				})
			case string:
				ps := parseAppPortString(iv, defaultServiceName)
				if ps != nil {
					ports = append(ports, *ps)
				}
			}
		}
	}

	return ports
}

// parseAppPortString parses a single appPort string entry.
// Format: "hostPort:containerPort" or just "containerPort".
func parseAppPortString(s string, defaultServiceName string) *model.PortSpec {
	parts := strings.SplitN(s, ":", 2)

	if len(parts) == 2 {
		// "hostPort:containerPort" format.
		hostPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil
		}
		containerPort, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil
		}
		return &model.PortSpec{
			ServiceName:   defaultServiceName,
			ContainerPort: containerPort,
			HostPort:      hostPort,
			Protocol:      "tcp",
		}
	}

	// Single port number as string.
	port, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &model.PortSpec{
		ServiceName:   defaultServiceName,
		ContainerPort: port,
		Protocol:      "tcp",
	}
}

// GetComposeFiles extracts and normalizes the dockerComposeFile field
// from a RawDevContainer into a string slice.
//
// The devcontainer.json spec allows dockerComposeFile to be either a
// single string or an array of strings. This function normalizes both
// forms into a consistent []string representation.
//
// Returns nil if dockerComposeFile is not set.
func GetComposeFiles(raw *RawDevContainer) []string {
	if raw.DockerComposeFile == nil {
		return nil
	}

	switch v := raw.DockerComposeFile.(type) {
	case string:
		// Single Compose file specified as a plain string.
		return []string{v}
	case []interface{}:
		// Array of Compose file paths. Each element should be a string,
		// but we handle the type assertion safely.
		files := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				files = append(files, s)
			}
		}
		return files
	default:
		return nil
	}
}

// FindDevContainerJSON searches for devcontainer.json in the standard
// locations within a project directory.
//
// The search order follows the official devcontainer.json spec:
//  1. <projectPath>/.devcontainer/devcontainer.json (preferred, most common)
//  2. <projectPath>/.devcontainer.json (alternative, less common)
//
// Returns the absolute path to the first found file, or a CLIError
// with ExitDevContainerNotFound if neither location contains the file.
func FindDevContainerJSON(projectPath string) (string, error) {
	// Define candidate paths in priority order.
	// The .devcontainer/ subdirectory is the standard location recommended
	// by the devcontainer spec. The root-level .devcontainer.json is a
	// convenience alternative for simple projects.
	candidates := []string{
		filepath.Join(projectPath, ".devcontainer", "devcontainer.json"),
		filepath.Join(projectPath, ".devcontainer.json"),
	}

	for _, path := range candidates {
		// os.Stat checks if the file exists without reading its contents.
		// This is more efficient than os.ReadFile when we only need existence.
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", model.NewCLIError(
		model.ExitDevContainerNotFound,
		fmt.Sprintf("devcontainer.json not found in %s (searched .devcontainer/devcontainer.json and .devcontainer.json)", projectPath),
	)
}
