package devcontainer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shinji-kodama/worktree-container/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- RewriteConfig tests ---

// TestRewriteConfig_PatternA verifies the rewriting behavior for a Pattern A
// (image-based) devcontainer.json. It checks that all five modifications are
// correctly applied:
//  1. name → changed to envName
//  2. runArgs → label flags appended
//  3. appPort → rewritten with shifted ports
//  4. portsAttributes → keys updated to shifted host ports
//  5. containerEnv → WORKTREE_NAME and WORKTREE_INDEX added
func TestRewriteConfig_PatternA(t *testing.T) {
	// Arrange: create a minimal Pattern A devcontainer.json with JSONC comments.
	// The comment on the first line verifies that JSONC stripping works correctly.
	rawJSON := []byte(`{
		// This is a JSONC comment that should be stripped
		"name": "original-name",
		"image": "node:20",
		"runArgs": ["--cap-add=SYS_PTRACE"],
		"appPort": ["3000:3000", "8080:8080"],
		"forwardPorts": [3000, 8080],
		"portsAttributes": {
			"3000": {"label": "Application", "onAutoForward": "notify"},
			"8080": {"label": "API Server", "onAutoForward": "silent"}
		},
		"containerEnv": {
			"NODE_ENV": "development"
		}
	}`)

	portAllocations := []model.PortAllocation{
		{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
		{ServiceName: "app", ContainerPort: 8080, HostPort: 18080, Protocol: "tcp"},
	}

	labels := map[string]string{
		"worktree.managed-by": "worktree-container",
		"worktree.name":       "feature-auth",
	}

	// Act
	result, err := RewriteConfig(rawJSON, "feature-auth", 1, portAllocations, labels)
	require.NoError(t, err, "RewriteConfig should succeed for valid Pattern A input")

	// Parse the result back into a map for assertion.
	var resultMap map[string]interface{}
	err = json.Unmarshal(result, &resultMap)
	require.NoError(t, err, "result should be valid JSON")

	// Assert 1: name is changed to envName.
	assert.Equal(t, "feature-auth", resultMap["name"],
		"name should be changed to the environment name")

	// Assert 2: runArgs has the original args plus label flags.
	runArgs, ok := resultMap["runArgs"].([]interface{})
	require.True(t, ok, "runArgs should be a JSON array")
	// Original had 1 arg, we added 2 labels (each with --label and key=value).
	// So total = 1 + (2 * 2) = 5
	assert.Len(t, runArgs, 5, "runArgs should have original args plus label flags")
	assert.Equal(t, "--cap-add=SYS_PTRACE", runArgs[0],
		"original runArgs should be preserved")
	// Check that --label flags are present (order of labels may vary due to map iteration).
	runArgsStrs := make([]string, len(runArgs))
	for i, v := range runArgs {
		runArgsStrs[i], _ = v.(string)
	}
	assert.Contains(t, runArgsStrs, "--label",
		"runArgs should contain --label flag")
	assert.Contains(t, runArgsStrs, "worktree.managed-by=worktree-container",
		"runArgs should contain the managed-by label")
	assert.Contains(t, runArgsStrs, "worktree.name=feature-auth",
		"runArgs should contain the name label")

	// Assert 3: appPort is rewritten with shifted ports.
	appPort, ok := resultMap["appPort"].([]interface{})
	require.True(t, ok, "appPort should be a JSON array")
	assert.Len(t, appPort, 2, "appPort should have 2 port mappings")
	assert.Equal(t, "13000:3000", appPort[0], "first port should be shifted")
	assert.Equal(t, "18080:8080", appPort[1], "second port should be shifted")

	// Assert 4: portsAttributes keys are updated to shifted ports.
	portsAttrs, ok := resultMap["portsAttributes"].(map[string]interface{})
	require.True(t, ok, "portsAttributes should be a JSON object")
	assert.Len(t, portsAttrs, 2, "portsAttributes should have 2 entries")
	// The key "3000" should now be "13000".
	attr13000, ok := portsAttrs["13000"].(map[string]interface{})
	require.True(t, ok, "portsAttributes should have entry for shifted port 13000")
	assert.Equal(t, "Application", attr13000["label"],
		"label should be preserved when key is shifted")
	// The key "8080" should now be "18080".
	attr18080, ok := portsAttrs["18080"].(map[string]interface{})
	require.True(t, ok, "portsAttributes should have entry for shifted port 18080")
	assert.Equal(t, "API Server", attr18080["label"],
		"label should be preserved when key is shifted")

	// Assert 5: containerEnv has the original env vars plus worktree additions.
	envMap, ok := resultMap["containerEnv"].(map[string]interface{})
	require.True(t, ok, "containerEnv should be a JSON object")
	assert.Equal(t, "development", envMap["NODE_ENV"],
		"original containerEnv entries should be preserved")
	assert.Equal(t, "feature-auth", envMap["WORKTREE_NAME"],
		"WORKTREE_NAME should be set")
	assert.Equal(t, "1", envMap["WORKTREE_INDEX"],
		"WORKTREE_INDEX should be the worktree index as string")

	// Assert 6: image field is preserved (unknown fields should not be lost).
	assert.Equal(t, "node:20", resultMap["image"],
		"image field should be preserved through rewriting")
}

// TestRewriteConfig_PatternB verifies the rewriting behavior for a Pattern B
// (Dockerfile build) devcontainer.json. The key difference from Pattern A is
// the presence of the "build" configuration, which must be preserved.
func TestRewriteConfig_PatternB(t *testing.T) {
	// Arrange: Pattern B config with build section.
	rawJSON := []byte(`{
		"name": "build-app",
		"build": {
			"dockerfile": "Dockerfile",
			"context": "..",
			"args": {
				"NODE_VERSION": "20"
			}
		},
		"forwardPorts": [3000, 5432],
		"appPort": [3000],
		"portsAttributes": {
			"3000": {"label": "Web App"},
			"5432": {"label": "PostgreSQL"}
		}
	}`)

	portAllocations := []model.PortAllocation{
		{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
		{ServiceName: "app", ContainerPort: 5432, HostPort: 15432, Protocol: "tcp"},
	}

	labels := map[string]string{
		"worktree.managed-by": "worktree-container",
		"worktree.name":       "feature-db",
	}

	// Act
	result, err := RewriteConfig(rawJSON, "feature-db", 1, portAllocations, labels)
	require.NoError(t, err)

	var resultMap map[string]interface{}
	err = json.Unmarshal(result, &resultMap)
	require.NoError(t, err)

	// Assert: build configuration is fully preserved.
	build, ok := resultMap["build"].(map[string]interface{})
	require.True(t, ok, "build section should be preserved")
	assert.Equal(t, "Dockerfile", build["dockerfile"],
		"build.dockerfile should be preserved")
	assert.Equal(t, "..", build["context"],
		"build.context should be preserved")
	buildArgs, ok := build["args"].(map[string]interface{})
	require.True(t, ok, "build.args should be preserved")
	assert.Equal(t, "20", buildArgs["NODE_VERSION"],
		"build.args.NODE_VERSION should be preserved")

	// Assert: name is changed.
	assert.Equal(t, "feature-db", resultMap["name"])

	// Assert: appPort is shifted.
	appPort, ok := resultMap["appPort"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, appPort, "13000:3000")
	assert.Contains(t, appPort, "15432:5432")

	// Assert: portsAttributes keys are shifted.
	portsAttrs, ok := resultMap["portsAttributes"].(map[string]interface{})
	require.True(t, ok)
	_, has13000 := portsAttrs["13000"]
	_, has15432 := portsAttrs["15432"]
	assert.True(t, has13000, "should have shifted key 13000")
	assert.True(t, has15432, "should have shifted key 15432")
	// Original keys should no longer exist.
	_, has3000 := portsAttrs["3000"]
	_, has5432 := portsAttrs["5432"]
	assert.False(t, has3000, "original key 3000 should be replaced")
	assert.False(t, has5432, "original key 5432 should be replaced")
}

// TestRewriteConfig_EmptyPorts verifies that RewriteConfig works correctly
// when there are no port allocations. This is a valid scenario for containers
// that don't expose any ports (e.g., a pure build/test environment).
func TestRewriteConfig_EmptyPorts(t *testing.T) {
	rawJSON := []byte(`{
		"name": "no-ports-app",
		"image": "golang:1.22",
		"appPort": [3000],
		"portsAttributes": {
			"3000": {"label": "App"}
		}
	}`)

	// No port allocations — simulates a worktree with no ports to forward.
	var portAllocations []model.PortAllocation

	labels := map[string]string{
		"worktree.managed-by": "worktree-container",
	}

	// Act
	result, err := RewriteConfig(rawJSON, "no-ports", 0, portAllocations, labels)
	require.NoError(t, err)

	var resultMap map[string]interface{}
	err = json.Unmarshal(result, &resultMap)
	require.NoError(t, err)

	// Assert: appPort should be removed when there are no port allocations.
	_, hasAppPort := resultMap["appPort"]
	assert.False(t, hasAppPort,
		"appPort should be removed when there are no port allocations")

	// Assert: name is still changed.
	assert.Equal(t, "no-ports", resultMap["name"])

	// Assert: containerEnv is added with worktree vars.
	envMap, ok := resultMap["containerEnv"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "no-ports", envMap["WORKTREE_NAME"])
	assert.Equal(t, "0", envMap["WORKTREE_INDEX"])
}

// TestRewriteConfig_NoExistingRunArgs verifies that label flags are correctly
// added even when the original config has no runArgs field at all.
func TestRewriteConfig_NoExistingRunArgs(t *testing.T) {
	rawJSON := []byte(`{
		"name": "minimal",
		"image": "node:20"
	}`)

	labels := map[string]string{
		"worktree.name": "minimal-env",
	}

	result, err := RewriteConfig(rawJSON, "minimal-env", 0, nil, labels)
	require.NoError(t, err)

	var resultMap map[string]interface{}
	err = json.Unmarshal(result, &resultMap)
	require.NoError(t, err)

	// runArgs should be created with just the label flags.
	runArgs, ok := resultMap["runArgs"].([]interface{})
	require.True(t, ok, "runArgs should be created even when not originally present")
	assert.Len(t, runArgs, 2, "runArgs should have --label and key=value pair")
	assert.Equal(t, "--label", runArgs[0])
	assert.Equal(t, "worktree.name=minimal-env", runArgs[1])
}

// TestRewriteConfig_NoExistingContainerEnv verifies that containerEnv is
// correctly created when the original config doesn't have one.
func TestRewriteConfig_NoExistingContainerEnv(t *testing.T) {
	rawJSON := []byte(`{
		"name": "no-env",
		"image": "node:20"
	}`)

	result, err := RewriteConfig(rawJSON, "new-env", 3, nil, map[string]string{})
	require.NoError(t, err)

	var resultMap map[string]interface{}
	err = json.Unmarshal(result, &resultMap)
	require.NoError(t, err)

	envMap, ok := resultMap["containerEnv"].(map[string]interface{})
	require.True(t, ok, "containerEnv should be created")
	assert.Equal(t, "new-env", envMap["WORKTREE_NAME"])
	assert.Equal(t, "3", envMap["WORKTREE_INDEX"])
}

// --- WriteRewrittenConfig tests ---

// TestWriteRewrittenConfig verifies that WriteRewrittenConfig correctly creates
// the output file with the provided content, including creating parent directories.
func TestWriteRewrittenConfig(t *testing.T) {
	// Use t.TempDir() for automatic cleanup — Go's testing framework
	// removes the temp dir after the test completes.
	tmpDir := t.TempDir()

	// Use a nested path to verify that MkdirAll creates intermediate directories.
	outputPath := filepath.Join(tmpDir, "worktree", ".devcontainer", "devcontainer.json")

	content := []byte(`{"name": "test-env"}`)

	// Act
	err := WriteRewrittenConfig(outputPath, content)
	require.NoError(t, err, "WriteRewrittenConfig should succeed")

	// Assert: file exists and has the correct content.
	readBack, err := os.ReadFile(outputPath)
	require.NoError(t, err, "file should exist after writing")
	assert.Equal(t, content, readBack, "file content should match what was written")
}

// TestWriteRewrittenConfig_OverwriteExisting verifies that WriteRewrittenConfig
// overwrites an existing file rather than appending or failing.
func TestWriteRewrittenConfig_OverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "devcontainer.json")

	// Write initial content.
	err := os.WriteFile(outputPath, []byte("old content"), 0644)
	require.NoError(t, err)

	// Overwrite with new content.
	newContent := []byte(`{"name": "updated"}`)
	err = WriteRewrittenConfig(outputPath, newContent)
	require.NoError(t, err)

	readBack, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, newContent, readBack,
		"file should contain the new content, not the old")
}

// --- CopyDevContainerDir tests ---

// TestCopyDevContainerDir verifies that the entire .devcontainer directory is
// copied to the destination, EXCEPT for devcontainer.json which is skipped
// because it will be rewritten separately.
func TestCopyDevContainerDir(t *testing.T) {
	// Arrange: create a source .devcontainer directory with multiple files.
	srcDir := t.TempDir()

	// Create files that should be copied.
	files := map[string]string{
		"Dockerfile":             "FROM node:20\nRUN npm install",
		"setup.sh":               "#!/bin/bash\necho hello",
		"docker-compose.yml":     "version: '3'\nservices:\n  app:\n    build: .",
		"scripts/post-create.sh": "#!/bin/bash\necho post-create",
	}

	for path, content := range files {
		fullPath := filepath.Join(srcDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create devcontainer.json which should be SKIPPED.
	devcontainerJSON := filepath.Join(srcDir, "devcontainer.json")
	err := os.WriteFile(devcontainerJSON, []byte(`{"name": "original"}`), 0644)
	require.NoError(t, err)

	// Create destination directory.
	dstDir := t.TempDir()
	dstSubDir := filepath.Join(dstDir, ".devcontainer")

	// Act
	err = CopyDevContainerDir(srcDir, dstSubDir)
	require.NoError(t, err, "CopyDevContainerDir should succeed")

	// Assert: all non-devcontainer.json files are copied with correct content.
	for path, expectedContent := range files {
		dstPath := filepath.Join(dstSubDir, path)
		readBack, readErr := os.ReadFile(dstPath)
		require.NoError(t, readErr, "file %s should exist in destination", path)
		assert.Equal(t, expectedContent, string(readBack),
			"content of %s should match the source", path)
	}

	// Assert: devcontainer.json is NOT copied.
	_, err = os.Stat(filepath.Join(dstSubDir, "devcontainer.json"))
	assert.True(t, os.IsNotExist(err),
		"devcontainer.json should NOT be copied (it will be rewritten separately)")
}

// TestCopyDevContainerDir_EmptyDir verifies that copying an empty directory
// (no files at all) works without errors.
func TestCopyDevContainerDir_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dest")

	err := CopyDevContainerDir(srcDir, dstDir)
	require.NoError(t, err, "copying an empty directory should succeed")

	// Destination directory should exist.
	info, err := os.Stat(dstDir)
	require.NoError(t, err, "destination directory should be created")
	assert.True(t, info.IsDir(), "destination should be a directory")
}

// TestCopyDevContainerDir_PreservesFilePermissions verifies that executable
// scripts maintain their permissions after copying.
func TestCopyDevContainerDir_PreservesFilePermissions(t *testing.T) {
	srcDir := t.TempDir()

	// Create an executable script.
	scriptPath := filepath.Join(srcDir, "setup.sh")
	err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho hello"), 0755)
	require.NoError(t, err)

	dstDir := filepath.Join(t.TempDir(), "dest")

	err = CopyDevContainerDir(srcDir, dstDir)
	require.NoError(t, err)

	// Check that the copied file has executable permissions.
	info, err := os.Stat(filepath.Join(dstDir, "setup.sh"))
	require.NoError(t, err)
	// Check that owner execute bit is set (0100).
	assert.NotZero(t, info.Mode()&0100,
		"executable permission should be preserved on copied files")
}
