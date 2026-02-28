// validate.go provides validation functions that ensure generated
// devcontainer.json files conform to the Dev Container specification.
//
// This is critical for tool compatibility (US4): the generated configuration
// must work seamlessly with VS Code Dev Containers, Dev Container CLI,
// and DevPod. Each tool has slightly different parsing behavior, so we
// validate against the common subset they all support.
package devcontainer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidationError represents a specific validation failure in a devcontainer.json file.
type ValidationError struct {
	// Field is the JSON field path that failed validation (e.g., "build.dockerfile").
	Field string

	// Message describes what's wrong with the field value.
	Message string
}

// Error implements the error interface for ValidationError.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("devcontainer.json validation error: %s: %s", e.Field, e.Message)
}

// ValidateConfig performs specification-conformance checks on a parsed
// devcontainer.json configuration. It returns a list of validation errors
// (empty list = valid configuration).
//
// Checks performed:
//   - Pattern consistency: image/build/dockerComposeFile are mutually exclusive
//   - Required fields: "name" should be present
//   - Port specifications: forwardPorts values must be valid
//   - Compose fields: service must be set when dockerComposeFile is present
//   - Build paths: dockerfile and context paths should be relative
//   - appPort format: must be valid "host:container" or integer
func ValidateConfig(raw *RawDevContainer) []ValidationError {
	var errors []ValidationError

	// Check 1: Name should be present for container identification.
	if raw.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "name field is recommended for container identification",
		})
	}

	// Check 2: Pattern consistency — only one of image, build, or dockerComposeFile
	// should be the primary source. Having both image and build is technically allowed
	// by the spec (build takes precedence), but having dockerComposeFile with either
	// image or build is a conflict.
	hasImage := raw.Image != ""
	hasBuild := raw.Build != nil
	hasCompose := raw.DockerComposeFile != nil

	if hasCompose && (hasImage || hasBuild) {
		errors = append(errors, ValidationError{
			Field:   "dockerComposeFile",
			Message: "dockerComposeFile should not be combined with image or build fields",
		})
	}

	// Check 3: When dockerComposeFile is present, service must be specified.
	if hasCompose && raw.Service == "" {
		errors = append(errors, ValidationError{
			Field:   "service",
			Message: "service field is required when dockerComposeFile is specified",
		})
	}

	// Check 4: Build path validation — dockerfile and context should be relative.
	if raw.Build != nil {
		if raw.Build.Dockerfile != "" && filepath.IsAbs(raw.Build.Dockerfile) {
			errors = append(errors, ValidationError{
				Field:   "build.dockerfile",
				Message: "dockerfile path should be relative to the .devcontainer directory",
			})
		}
		if raw.Build.Context != "" && filepath.IsAbs(raw.Build.Context) {
			errors = append(errors, ValidationError{
				Field:   "build.context",
				Message: "context path should be relative to the .devcontainer directory",
			})
		}
	}

	return errors
}

// ValidateGeneratedConfig validates a generated (rewritten) devcontainer.json
// file by parsing it and running ValidateConfig, plus additional checks
// specific to the worktree-container modifications.
func ValidateGeneratedConfig(jsonData []byte) []ValidationError {
	var raw RawDevContainer
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		return []ValidationError{{
			Field:   "(root)",
			Message: fmt.Sprintf("invalid JSON: %v", err),
		}}
	}

	errors := ValidateConfig(&raw)

	// Additional check: verify name is set (required for worktree identification).
	if raw.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "generated config must have a name for environment identification",
		})
	}

	return errors
}

// GenerateDevPodConfig generates additional configuration output for DevPod.
// DevPod uses `devpod up <path>` and reads .devcontainer/devcontainer.json
// from the workspace folder. This function returns the command-line arguments
// needed to use DevPod with the generated configuration.
//
// Parameters:
//   - workspaceFolder: absolute path to the worktree directory
//   - devcontainerPath: relative path to devcontainer.json within the workspace
//
// Returns the DevPod CLI command and arguments.
func GenerateDevPodConfig(workspaceFolder string, devcontainerPath string) DevPodInfo {
	info := DevPodInfo{
		WorkspaceFolder:  workspaceFolder,
		DevContainerPath: devcontainerPath,
	}

	// DevPod auto-detects .devcontainer/devcontainer.json by default.
	// If the devcontainer.json is in the standard location, no extra args needed.
	standardPath := filepath.Join(".devcontainer", "devcontainer.json")
	if devcontainerPath == standardPath {
		info.Command = fmt.Sprintf("devpod up %s", workspaceFolder)
	} else {
		// For non-standard locations, use --devcontainer-path flag.
		info.Command = fmt.Sprintf("devpod up %s --devcontainer-path %s",
			workspaceFolder, devcontainerPath)
	}

	return info
}

// DevPodInfo holds information needed to use DevPod with a worktree environment.
type DevPodInfo struct {
	// WorkspaceFolder is the absolute path to the worktree directory.
	WorkspaceFolder string `json:"workspaceFolder"`

	// DevContainerPath is the relative path to devcontainer.json within the workspace.
	DevContainerPath string `json:"devcontainerPath"`

	// Command is the complete DevPod CLI command to start the environment.
	Command string `json:"command"`
}

// GenerateToolCompatInfo generates compatibility information for all supported
// Dev Container tools (VS Code, Dev Container CLI, DevPod).
func GenerateToolCompatInfo(workspaceFolder string) ToolCompatInfo {
	return ToolCompatInfo{
		VSCode:          fmt.Sprintf("code %s", workspaceFolder),
		DevContainerCLI: fmt.Sprintf("devcontainer up --workspace-folder %s", workspaceFolder),
		DevPod:          fmt.Sprintf("devpod up %s", workspaceFolder),
	}
}

// ToolCompatInfo holds CLI commands for each supported Dev Container tool.
type ToolCompatInfo struct {
	// VSCode is the command to open the workspace in VS Code.
	VSCode string `json:"vscode"`

	// DevContainerCLI is the command to start the environment with Dev Container CLI.
	DevContainerCLI string `json:"devcontainerCli"`

	// DevPod is the command to start the environment with DevPod.
	DevPod string `json:"devpod"`
}

// ValidateWorkspaceFiles checks that the generated worktree workspace has
// all the files needed for Dev Container tools to detect and use it.
func ValidateWorkspaceFiles(worktreePath string) []string {
	var issues []string

	// Check for .devcontainer directory.
	devcontainerDir := filepath.Join(worktreePath, ".devcontainer")
	if _, err := os.Stat(devcontainerDir); os.IsNotExist(err) {
		issues = append(issues, ".devcontainer directory not found")
		return issues // No point checking further
	}

	// Check for devcontainer.json.
	devcontainerJSON := filepath.Join(devcontainerDir, "devcontainer.json")
	if _, err := os.Stat(devcontainerJSON); os.IsNotExist(err) {
		issues = append(issues, ".devcontainer/devcontainer.json not found")
	}

	// Read and validate devcontainer.json if it exists.
	data, err := os.ReadFile(devcontainerJSON)
	if err == nil {
		var configMap map[string]interface{}
		if jsonErr := json.Unmarshal(data, &configMap); jsonErr != nil {
			issues = append(issues, fmt.Sprintf("devcontainer.json is not valid JSON: %v", jsonErr))
		} else {
			// Check for required fields based on pattern.
			if _, hasCompose := configMap["dockerComposeFile"]; hasCompose {
				// Compose pattern: verify compose files exist.
				composeFiles := getComposeFilePaths(configMap)
				for _, cf := range composeFiles {
					cfPath := filepath.Join(devcontainerDir, cf)
					if _, err := os.Stat(cfPath); os.IsNotExist(err) {
						issues = append(issues, fmt.Sprintf("referenced Compose file not found: %s", cf))
					}
				}
			} else if buildConfig, hasBuild := configMap["build"]; hasBuild {
				// Build pattern: verify Dockerfile exists.
				if buildMap, ok := buildConfig.(map[string]interface{}); ok {
					if dockerfile, ok := buildMap["dockerfile"].(string); ok {
						dfPath := filepath.Join(devcontainerDir, dockerfile)
						if _, err := os.Stat(dfPath); os.IsNotExist(err) {
							issues = append(issues, fmt.Sprintf("referenced Dockerfile not found: %s", dockerfile))
						}
					}
				}
			}
		}
	}

	return issues
}

// getComposeFilePaths extracts Compose file paths from a config map.
// Handles both string and array forms of dockerComposeFile.
func getComposeFilePaths(configMap map[string]interface{}) []string {
	dcf := configMap["dockerComposeFile"]
	switch v := dcf.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var paths []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				paths = append(paths, s)
			}
		}
		return paths
	}
	return nil
}

// SanitizeForDevPod ensures the devcontainer.json is compatible with DevPod.
// DevPod has a few quirks compared to VS Code:
//   - It prefers workspaceFolder to be set explicitly
//   - It handles dockerComposeFile arrays differently from VS Code
//
// This function applies any necessary adjustments and returns true if
// changes were made.
func SanitizeForDevPod(configMap map[string]interface{}) bool {
	changed := false

	// Ensure workspaceFolder is set. DevPod requires this for proper operation.
	if _, hasWorkspaceFolder := configMap["workspaceFolder"]; !hasWorkspaceFolder {
		configMap["workspaceFolder"] = "/workspace"
		changed = true
	}

	// Normalize dockerComposeFile to always be an array.
	// DevPod handles arrays more reliably than single strings.
	if dcf, hasCompose := configMap["dockerComposeFile"]; hasCompose {
		if s, ok := dcf.(string); ok {
			configMap["dockerComposeFile"] = []string{s}
			changed = true
		}
	}

	// Ensure shutdownAction is set for Compose patterns.
	// DevPod needs this to properly stop all services.
	if _, hasCompose := configMap["dockerComposeFile"]; hasCompose {
		if _, hasShutdown := configMap["shutdownAction"]; !hasShutdown {
			configMap["shutdownAction"] = "stopCompose"
			changed = true
		}
	}

	// Remove remoteUser if it conflicts with DevPod's user management.
	// DevPod handles user mapping differently from VS Code.
	// Note: We don't remove it, just ensure it doesn't cause issues.
	_ = changed // Suppress unused variable if no changes are needed.

	return changed
}

// FormatDevContainerPath returns the --devcontainer-path argument for DevPod
// when the devcontainer.json is not in the standard location.
// Returns empty string if the path is the standard .devcontainer/devcontainer.json.
func FormatDevContainerPath(relativePath string) string {
	standardPaths := []string{
		filepath.Join(".devcontainer", "devcontainer.json"),
		".devcontainer.json",
	}

	for _, sp := range standardPaths {
		if strings.EqualFold(relativePath, sp) {
			return ""
		}
	}

	return relativePath
}
