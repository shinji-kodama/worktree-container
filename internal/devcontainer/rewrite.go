// rewrite.go handles the generation of worktree-specific devcontainer.json files
// for Pattern A (image) and Pattern B (dockerfile) configurations.
//
// The original devcontainer.json is NEVER modified (FR-012). Instead, this module:
//  1. Parses the comment-stripped JSON into a generic map[string]interface{}
//  2. Applies worktree-specific modifications (name, labels, port shifts, env vars)
//  3. Serializes back to JSON and writes to the worktree's .devcontainer/ directory
//
// Using a map-based approach (instead of the typed RawDevContainer struct) ensures
// that unknown fields from the original devcontainer.json are preserved in the
// rewritten output. The RawDevContainer struct only captures fields we care about,
// so marshaling it back would lose everything else.
package devcontainer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mmr-tortoise/worktree-container/internal/model"
	"github.com/tidwall/jsonc"
)

// RewriteConfig takes the raw bytes of a devcontainer.json file (with JSONC
// comments), applies worktree-specific modifications, and returns the
// modified JSON as formatted bytes.
//
// The function works in three phases:
//  1. Strip JSONC comments and parse into a generic map
//  2. Apply modifications: name, runArgs labels, appPort shifts,
//     portsAttributes key updates, and containerEnv additions
//  3. Re-serialize with indentation for human readability
//
// Parameters:
//   - rawJSON: the original devcontainer.json file contents (may include JSONC comments)
//   - envName: the worktree environment name to use as the container name
//   - worktreeIndex: the 0-based worktree index, stored in WORKTREE_INDEX env var
//   - portAllocations: the shifted port assignments for this worktree
//   - labels: Docker labels to inject via --label runArgs flags
//
// Returns the modified JSON bytes, or an error if parsing/serialization fails.
func RewriteConfig(rawJSON []byte, envName string, worktreeIndex int, portAllocations []model.PortAllocation, labels map[string]string) ([]byte, error) {
	// Phase 1: Strip JSONC comments and parse into a generic map.
	// Using map[string]interface{} preserves ALL fields from the original JSON,
	// not just the ones defined in RawDevContainer. This is critical because
	// devcontainer.json has many optional fields we don't explicitly model.
	cleanJSON := jsonc.ToJSON(rawJSON)

	var configMap map[string]interface{}
	if err := json.Unmarshal(cleanJSON, &configMap); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer.json for rewriting: %w", err)
	}

	// Phase 2: Apply modifications.

	// 2a. Update the container name to the worktree environment name.
	// This makes it easy to identify which worktree a container belongs to
	// when listing containers with `docker ps`.
	configMap["name"] = envName

	// 2b. Append Docker label flags to runArgs.
	// Labels are injected as `--label key=value` pairs in the runArgs array.
	// This is the mechanism for non-Compose patterns (A/B) to tag containers
	// with worktree metadata, since there's no docker-compose.yml to add labels to.
	applyRunArgsLabels(configMap, labels)

	// 2c. Rewrite appPort with shifted host ports.
	// The appPort field specifies port mappings published from the container.
	// We replace the original port mappings with shifted ones based on the
	// port allocations computed by the allocator.
	applyAppPortShift(configMap, portAllocations)

	// 2d. Update portsAttributes keys to use shifted host ports.
	// The portsAttributes map is keyed by port number (as string). When we
	// shift ports, the keys need to be updated to match the new host ports
	// so that VS Code / Dev Container tooling applies the correct metadata
	// (labels, autoForward behavior) to the shifted ports.
	applyPortsAttributesShift(configMap, portAllocations)

	// 2e. Add worktree environment variables to containerEnv.
	// These env vars allow code running inside the container to detect
	// that it's in a worktree environment and determine which one.
	applyContainerEnv(configMap, envName, worktreeIndex)

	// Phase 3: Re-serialize with 2-space indentation.
	// The indentation matches the typical devcontainer.json formatting.
	result, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize rewritten devcontainer.json: %w", err)
	}

	// Append a trailing newline for POSIX compliance (many editors and linters
	// expect text files to end with a newline).
	result = append(result, '\n')

	return result, nil
}

// applyRunArgsLabels appends Docker --label flags to the runArgs array.
// Each label is added as two separate entries: "--label" and "key=value".
//
// Example: for label {"worktree.name": "my-env"}, this appends:
//
//	"--label", "worktree.name=my-env"
//
// If runArgs doesn't exist yet in the config, it is created as a new array.
func applyRunArgsLabels(configMap map[string]interface{}, labels map[string]string) {
	// Retrieve the existing runArgs, or start with an empty slice.
	var runArgs []interface{}
	if existing, ok := configMap["runArgs"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			runArgs = arr
		}
	}

	// Append --label flags for each label entry.
	// Map iteration order is non-deterministic in Go, so we sort the keys
	// to produce stable, reproducible output across runs.
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		runArgs = append(runArgs, "--label", fmt.Sprintf("%s=%s", key, labels[key]))
	}

	configMap["runArgs"] = runArgs
}

// applyAppPortShift replaces the appPort field with shifted port mappings.
// The output format is an array of "hostPort:containerPort" strings.
//
// Example output: ["13000:3000", "18080:8080"]
//
// If there are no port allocations, appPort is removed from the config
// to avoid an empty array, which some tools might interpret incorrectly.
func applyAppPortShift(configMap map[string]interface{}, portAllocations []model.PortAllocation) {
	if len(portAllocations) == 0 {
		// No ports to map — remove appPort entirely to keep the config clean.
		delete(configMap, "appPort")
		return
	}

	// Build the shifted appPort array in "hostPort:containerPort" format.
	// This is the standard Docker port mapping format that devcontainer tooling
	// understands for Pattern A/B configurations.
	appPorts := make([]interface{}, 0, len(portAllocations))
	for _, pa := range portAllocations {
		appPorts = append(appPorts, fmt.Sprintf("%d:%d", pa.HostPort, pa.ContainerPort))
	}

	configMap["appPort"] = appPorts
}

// applyPortsAttributesShift updates the portsAttributes map keys from
// original container ports to shifted host ports.
//
// The portsAttributes field in devcontainer.json is keyed by port number
// (as a string). When port shifting is applied, we need to re-key the
// attributes so they match the new host ports that the developer will
// actually see and interact with.
//
// Example: if port 3000 is shifted to 13000, the attribute entry for
// "3000" becomes "13000" (preserving the label and other metadata).
func applyPortsAttributesShift(configMap map[string]interface{}, portAllocations []model.PortAllocation) {
	existing, ok := configMap["portsAttributes"]
	if !ok {
		return
	}

	// The portsAttributes value is a map of port-number-strings to attribute objects.
	// When parsed from JSON into interface{}, it's map[string]interface{}.
	oldAttrs, ok := existing.(map[string]interface{})
	if !ok {
		return
	}

	// Build a lookup from container port to host port for quick access.
	portMapping := make(map[string]int) // containerPort(string) → hostPort
	for _, pa := range portAllocations {
		portMapping[strconv.Itoa(pa.ContainerPort)] = pa.HostPort
	}

	// Create a new attributes map with shifted keys.
	newAttrs := make(map[string]interface{})
	for portKey, attrValue := range oldAttrs {
		if hostPort, found := portMapping[portKey]; found {
			// This port was shifted — use the new host port as the key.
			newAttrs[strconv.Itoa(hostPort)] = attrValue
		} else {
			// This port wasn't in our allocation list — preserve as-is.
			// This can happen if portsAttributes references ports that
			// aren't in forwardPorts/appPort (e.g., wildcard attributes).
			newAttrs[portKey] = attrValue
		}
	}

	configMap["portsAttributes"] = newAttrs
}

// applyContainerEnv adds worktree-specific environment variables to the
// containerEnv map.
//
// Two variables are always added:
//   - WORKTREE_NAME: the environment name (e.g., "feature-auth")
//   - WORKTREE_INDEX: the 0-based worktree index as a string (e.g., "1")
//
// These environment variables allow application code and scripts inside
// the container to detect and adapt to the worktree environment. For example,
// a startup script might use WORKTREE_INDEX to compute database names.
//
// If containerEnv doesn't exist yet, it is created as a new map.
// Existing entries in containerEnv are preserved.
func applyContainerEnv(configMap map[string]interface{}, envName string, worktreeIndex int) {
	// Retrieve or create the containerEnv map.
	var envMap map[string]interface{}
	if existing, ok := configMap["containerEnv"]; ok {
		if m, ok := existing.(map[string]interface{}); ok {
			envMap = m
		} else {
			envMap = make(map[string]interface{})
		}
	} else {
		envMap = make(map[string]interface{})
	}

	// Add the worktree-specific environment variables.
	envMap["WORKTREE_NAME"] = envName
	envMap["WORKTREE_INDEX"] = strconv.Itoa(worktreeIndex)

	configMap["containerEnv"] = envMap
}

// WriteRewrittenConfig writes the rewritten devcontainer.json bytes to the
// specified output path, creating parent directories if they don't exist.
//
// The file is written with 0644 permissions (owner read/write, group/others
// read-only), which is the standard permission for non-executable config files.
//
// Parameters:
//   - outputPath: the absolute path where the rewritten file should be saved
//   - data: the JSON bytes to write (typically from RewriteConfig)
func WriteRewrittenConfig(outputPath string, data []byte) error {
	// Create the parent directory tree if it doesn't exist.
	// os.MkdirAll is a no-op if the directory already exists, and creates
	// all necessary parent directories (like `mkdir -p`).
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write the file contents atomically (from the caller's perspective).
	// os.WriteFile handles open-write-close in a single call.
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write rewritten devcontainer.json to %s: %w", outputPath, err)
	}

	return nil
}

// CopyDevContainerDir copies the entire .devcontainer directory from a source
// location to a destination, preserving all supporting files (Dockerfiles,
// shell scripts, etc.) that the devcontainer.json references.
//
// IMPORTANT: The devcontainer.json file itself is SKIPPED during the copy,
// because it will be rewritten separately by RewriteConfig + WriteRewrittenConfig.
// This is a core design requirement (FR-012: never modify the original).
//
// The function performs a shallow recursive copy — it copies files and
// directories but does NOT follow symbolic links (they are skipped).
//
// Parameters:
//   - srcDir: the source .devcontainer directory path
//   - dstDir: the destination .devcontainer directory path (will be created)
func CopyDevContainerDir(srcDir, dstDir string) error {
	// Walk the source directory tree and copy each entry.
	// filepath.Walk visits every file and directory recursively.
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		// If the Walk function itself encountered an error accessing a path
		// (e.g., permission denied), propagate it immediately.
		if walkErr != nil {
			return fmt.Errorf("error walking source directory at %s: %w", path, walkErr)
		}

		// Compute the relative path from the source root, then join it
		// with the destination root to get the target path.
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %s: %w", path, err)
		}
		dstPath := filepath.Join(dstDir, relPath)

		// Skip symbolic links to avoid potential circular references and
		// to keep the copy behavior predictable.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Handle directories: create them in the destination.
		if info.IsDir() {
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dstPath, err)
			}
			return nil
		}

		// Skip devcontainer.json — it will be rewritten separately.
		// We check the filename (not the full path) because the file could
		// be at any level within the .devcontainer directory, though in
		// practice it's always at the root level.
		if strings.EqualFold(filepath.Base(path), "devcontainer.json") {
			return nil
		}

		// Copy the file contents from source to destination.
		if err := copyFile(path, dstPath, info.Mode()); err != nil {
			return err
		}

		return nil
	})
}

// copyFile copies a single file from src to dst, preserving the file mode.
// This is a helper used by CopyDevContainerDir for individual file copies.
//
// The function uses io.Copy for efficient streaming — the entire file is
// not loaded into memory, which matters for large Dockerfiles or scripts.
func copyFile(src, dst string, mode os.FileMode) error {
	// Open the source file for reading.
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", src, err)
	}
	// defer ensures the file is closed even if an error occurs below.
	// This is a common Go pattern for resource cleanup.
	defer func() { _ = srcFile.Close() }()

	// Create the destination file with the same permissions as the source.
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %w", dst, err)
	}
	defer func() { _ = dstFile.Close() }()

	// Stream the file contents. io.Copy reads from src and writes to dst
	// in chunks, avoiding loading the entire file into memory.
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", src, dst, err)
	}

	return nil
}
