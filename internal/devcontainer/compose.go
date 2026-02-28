// compose.go handles the generation of Docker Compose override YAML files and
// the rewriting of devcontainer.json for Compose-based patterns (C and D).
//
// For Compose patterns, the worktree isolation strategy differs from Pattern A/B:
//   - Instead of injecting --label flags into runArgs, labels go into the
//     Compose override YAML's service definitions.
//   - Instead of rewriting appPort, port mappings go into the override YAML's
//     service ports sections.
//   - The devcontainer.json is rewritten to include the override YAML path
//     in the dockerComposeFile array.
//   - The top-level `name` field in the override YAML sets COMPOSE_PROJECT_NAME,
//     which automatically isolates container names, network names, and volumes.
//
// This approach leverages Docker Compose's native override mechanism:
// multiple Compose files are merged in order, with later files overriding
// earlier ones. The override YAML only specifies the fields that need to
// change (ports, labels, project name), leaving everything else untouched.
package devcontainer

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/shinji-kodama/worktree-container/internal/model"
	"github.com/tidwall/jsonc"
	"gopkg.in/yaml.v3"
)

// composeOverride represents the structure of the generated docker-compose
// override YAML file. This struct is used for YAML serialization via the
// yaml.v3 library.
//
// The override file contains:
//   - A top-level `name` that sets COMPOSE_PROJECT_NAME for isolation
//   - Per-service port mappings with shifted host ports
//   - Per-service worktree management labels
type composeOverride struct {
	// Name sets the Compose project name. Docker Compose uses this to prefix
	// container names, network names, and volume names, providing automatic
	// namespace isolation between worktree environments.
	Name string `yaml:"name"`

	// Services maps service names to their override configurations.
	// Each service gets its shifted ports and worktree labels.
	Services map[string]composeServiceOverride `yaml:"services"`
}

// composeServiceOverride represents the override configuration for a single
// Docker Compose service. Only the fields that need to be overridden are
// included — Docker Compose merges these with the base service definition.
type composeServiceOverride struct {
	// Ports lists the port mappings in "hostPort:containerPort" format.
	// This REPLACES the service's port list from the base Compose file.
	// Only present for services that have port allocations.
	Ports []string `yaml:"ports,omitempty"`

	// Labels contains worktree management labels applied to the service's
	// containers. These labels enable container discovery and metadata
	// reconstruction from Docker API queries.
	Labels map[string]string `yaml:"labels"`
}

// GenerateComposeOverride creates a docker-compose override YAML that applies
// worktree-specific port shifts and management labels to Compose services.
//
// The generated YAML follows Docker Compose's override file convention and is
// designed to be included in the dockerComposeFile array in devcontainer.json
// as the LAST entry (so it takes precedence over the base Compose file).
//
// Key behaviors:
//   - The top-level `name` field sets COMPOSE_PROJECT_NAME for complete isolation
//     of container names, networks, and volumes across worktree environments.
//   - Port mappings are a COMPLETE REPLACEMENT — all ports for each service must
//     be included with their shifted host ports.
//   - ALL services receive worktree labels, even those without port mappings,
//     to ensure every container in the environment can be discovered via labels.
//
// Parameters:
//   - envName: the worktree environment name, used as the Compose project name
//   - services: list of ALL service names defined in the Compose file(s)
//   - portAllocations: the shifted port assignments for this worktree
//   - labels: worktree management labels to apply to all services
//
// Returns the YAML bytes with a header comment, or an error if serialization fails.
func GenerateComposeOverride(envName string, services []string, portAllocations []model.PortAllocation, labels map[string]string) ([]byte, error) {
	// Build a mapping from service name to its port allocations for quick lookup.
	// A single service may have multiple port allocations (e.g., app → [3000, 8080]).
	servicePorts := make(map[string][]model.PortAllocation)
	for _, pa := range portAllocations {
		servicePorts[pa.ServiceName] = append(servicePorts[pa.ServiceName], pa)
	}

	// Build the override structure with all services.
	override := composeOverride{
		Name:     envName,
		Services: make(map[string]composeServiceOverride),
	}

	// Sort service names for deterministic output order.
	// This makes the generated YAML reproducible and easier to diff.
	sortedServices := make([]string, len(services))
	copy(sortedServices, services)
	sort.Strings(sortedServices)

	for _, svc := range sortedServices {
		svcOverride := composeServiceOverride{
			// Every service gets ALL worktree labels for container discovery.
			Labels: make(map[string]string),
		}

		// Copy all labels to this service.
		for k, v := range labels {
			svcOverride.Labels[k] = v
		}

		// Add port mappings if this service has any allocated ports.
		if ports, ok := servicePorts[svc]; ok {
			for _, pa := range ports {
				// Use the standard Docker port mapping format: "hostPort:containerPort".
				svcOverride.Ports = append(svcOverride.Ports, fmt.Sprintf("%d:%d", pa.HostPort, pa.ContainerPort))
			}
		}

		override.Services[svc] = svcOverride
	}

	// Serialize to YAML with the yaml.v3 library.
	yamlBytes, err := yaml.Marshal(&override)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize compose override YAML: %w", err)
	}

	// Prepend a header comment explaining the file's purpose and warning
	// against manual edits. This is important because the file is auto-generated
	// and will be overwritten on each `create` or `start` command.
	header := fmt.Sprintf(
		"# Auto-generated by worktree-container for environment %q\n# DO NOT EDIT - this file is regenerated on each create/start\n",
		envName,
	)

	return []byte(header + string(yamlBytes)), nil
}

// WriteComposeOverride writes the generated Compose override YAML bytes to
// the specified output path.
//
// This is a thin wrapper around WriteRewrittenConfig (from rewrite.go) since
// the file-writing logic (create parent dirs, write with 0644 permissions)
// is identical for both JSON and YAML output files.
//
// Parameters:
//   - outputPath: the absolute path where the override YAML should be saved
//   - data: the YAML bytes to write (typically from GenerateComposeOverride)
func WriteComposeOverride(outputPath string, data []byte) error {
	// Reuse the same write logic from rewrite.go — it handles MkdirAll
	// and os.WriteFile with appropriate permissions.
	return WriteRewrittenConfig(outputPath, data)
}

// RewriteComposeConfig takes the raw bytes of a devcontainer.json file (with
// JSONC comments) and rewrites it for Compose patterns by:
//  1. Updating the `name` field to the worktree environment name
//  2. Appending the override YAML path to the `dockerComposeFile` array
//
// This function is used for Pattern C and D configurations. Unlike RewriteConfig
// (for Pattern A/B), it does NOT modify runArgs, appPort, or portsAttributes,
// because those concerns are handled by the Compose override YAML instead.
//
// The override YAML path is added as the LAST entry in the dockerComposeFile
// array, which is critical because Docker Compose processes files in order
// and later files override earlier ones.
//
// Parameters:
//   - rawJSON: the original devcontainer.json file contents (may include JSONC comments)
//   - envName: the worktree environment name
//   - overrideYAMLPath: the relative path to the generated override YAML file
//     (relative to the devcontainer.json location, e.g., "docker-compose.worktree.yml")
//
// Returns the modified JSON bytes, or an error if parsing/serialization fails.
func RewriteComposeConfig(rawJSON []byte, envName, overrideYAMLPath string) ([]byte, error) {
	// Strip JSONC comments and parse into a generic map.
	// Same approach as RewriteConfig — we use a map to preserve unknown fields.
	cleanJSON := jsonc.ToJSON(rawJSON)

	var configMap map[string]interface{}
	if err := json.Unmarshal(cleanJSON, &configMap); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer.json for compose rewriting: %w", err)
	}

	// Update the container/environment name.
	configMap["name"] = envName

	// Append the override YAML path to the dockerComposeFile array.
	// The dockerComposeFile field can be either a string or an array of strings.
	// We normalize it to an array and append the override path.
	configMap["dockerComposeFile"] = appendComposeFile(configMap["dockerComposeFile"], overrideYAMLPath)

	// Re-serialize with 2-space indentation for readability.
	result, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize rewritten devcontainer.json: %w", err)
	}

	// Append trailing newline for POSIX compliance.
	result = append(result, '\n')

	return result, nil
}

// appendComposeFile normalizes the dockerComposeFile field to an array and
// appends the override YAML path if it's not already present.
//
// The devcontainer.json spec allows dockerComposeFile to be either:
//   - A single string: "docker-compose.yml"
//   - An array of strings: ["docker-compose.yml", "docker-compose.override.yml"]
//
// This function handles both cases and always returns an array.
// If the override path is already in the array (e.g., from a previous run),
// it is NOT added again to avoid duplicates.
func appendComposeFile(existing interface{}, overridePath string) []interface{} {
	var files []interface{}

	switch v := existing.(type) {
	case string:
		// Single string → convert to array with one element.
		files = []interface{}{v}
	case []interface{}:
		// Already an array → use as-is.
		files = v
	default:
		// nil or unexpected type → start with an empty array.
		files = []interface{}{}
	}

	// Check if the override path is already present to prevent duplicates.
	// This can happen if the user runs `create` multiple times on the same worktree.
	for _, f := range files {
		if s, ok := f.(string); ok && s == overridePath {
			// Already present — return the array unchanged.
			return files
		}
	}

	// Append the override path as the last entry.
	// Being last is important: Docker Compose processes files in order,
	// and the override must come after the base file to take effect.
	return append(files, overridePath)
}
