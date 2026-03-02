// Package cli — create_test.go contains unit tests for the create command's
// pure helper functions. These tests verify data transformation and marker
// file logic without requiring a Docker daemon.
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mmr-tortoise/worktree-container/internal/model"
	"github.com/mmr-tortoise/worktree-container/internal/worktree"
)

// setupTestRepo creates a temporary directory with an initialized Git repository
// containing a single commit. This mirrors the helper in manager_test.go.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	runTestGit(t, dir, "init")
	runTestGit(t, dir, "config", "user.email", "test@example.com")
	runTestGit(t, dir, "config", "user.name", "Test User")

	initialFile := filepath.Join(dir, "README.md")
	err := os.WriteFile(initialFile, []byte("# Test Repo\n"), 0644)
	require.NoError(t, err)

	runTestGit(t, dir, "add", ".")
	runTestGit(t, dir, "commit", "-m", "initial commit")

	return dir
}

// runTestGit is a test helper that runs a git command in the specified directory.
func runTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
	return string(output)
}

// TestCreateNoDevcontainer_WorktreeAndMarker verifies that running the create
// workflow on a repo without devcontainer.json:
//  1. Creates the Git worktree
//  2. Writes a marker file with configPattern="none"
//  3. Does not require Docker (no Docker operations are invoked)
//
// This test exercises the worktree creation + marker file logic directly,
// rather than going through cobra, to avoid Docker dependencies.
func TestCreateNoDevcontainer_WorktreeAndMarker(t *testing.T) {
	repoPath := setupTestRepo(t)
	wm := worktree.NewManager()

	// Create a worktree (simulating what runCreate does before devcontainer detection).
	branchName := "feature-no-dc"
	envName := sanitizeBranchName(branchName)
	worktreePath := filepath.Join(t.TempDir(), "wt-no-dc")

	err := wm.Add(repoPath, branchName, worktreePath, "")
	require.NoError(t, err, "worktree creation should succeed")

	// Verify worktree exists.
	_, statErr := os.Stat(worktreePath)
	require.NoError(t, statErr, "worktree directory should exist")

	// Write marker file (as create.go Step 4.5 does).
	marker := worktree.MarkerFile{
		ManagedBy:      "worktree-container",
		Name:           envName,
		Branch:         branchName,
		SourceRepoPath: repoPath,
		ConfigPattern:  model.PatternNone,
		CreatedAt:      "2026-03-02T00:00:00Z",
	}
	err = worktree.WriteMarkerFile(worktreePath, marker)
	require.NoError(t, err, "WriteMarkerFile should succeed")

	// Read back and verify.
	readMarker, err := worktree.ReadMarkerFile(worktreePath)
	require.NoError(t, err)
	require.NotNil(t, readMarker)

	assert.Equal(t, "worktree-container", readMarker.ManagedBy)
	assert.Equal(t, envName, readMarker.Name)
	assert.Equal(t, branchName, readMarker.Branch)
	assert.Equal(t, repoPath, readMarker.SourceRepoPath)
	assert.Equal(t, model.PatternNone, readMarker.ConfigPattern)
}

// TestCreateWithDevcontainer_MarkerFile verifies that when a devcontainer.json
// exists, the marker file is updated with the actual config pattern (not "none").
func TestCreateWithDevcontainer_MarkerFile(t *testing.T) {
	repoPath := setupTestRepo(t)
	wm := worktree.NewManager()

	// Add a simple devcontainer.json (Pattern A: image).
	devcontainerDir := filepath.Join(repoPath, ".devcontainer")
	err := os.MkdirAll(devcontainerDir, 0755)
	require.NoError(t, err)

	devcontainerJSON := `{
		"name": "test-app",
		"image": "node:20",
		"forwardPorts": [3000]
	}`
	err = os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(devcontainerJSON), 0644)
	require.NoError(t, err)

	// Create worktree.
	branchName := "feature-with-dc"
	envName := sanitizeBranchName(branchName)
	worktreePath := filepath.Join(t.TempDir(), "wt-with-dc")

	err = wm.Add(repoPath, branchName, worktreePath, "")
	require.NoError(t, err)

	// Write initial marker (as create.go Step 4.5 does).
	marker := worktree.MarkerFile{
		ManagedBy:      "worktree-container",
		Name:           envName,
		Branch:         branchName,
		SourceRepoPath: repoPath,
		ConfigPattern:  model.PatternNone,
		CreatedAt:      "2026-03-02T00:00:00Z",
	}
	err = worktree.WriteMarkerFile(worktreePath, marker)
	require.NoError(t, err)

	// Simulate what create.go does after finding devcontainer.json:
	// update the marker with the actual pattern.
	marker.ConfigPattern = model.PatternImage
	err = worktree.WriteMarkerFile(worktreePath, marker)
	require.NoError(t, err)

	// Read back and verify configPattern was updated.
	readMarker, err := worktree.ReadMarkerFile(worktreePath)
	require.NoError(t, err)
	require.NotNil(t, readMarker)

	assert.Equal(t, model.PatternImage, readMarker.ConfigPattern,
		"marker file should be updated from 'none' to 'image'")
	assert.Equal(t, envName, readMarker.Name)
}

// TestLateDevcontainerAddition verifies that when a devcontainer.json is added
// to a worktree that was initially created without one, the marker file can be
// updated from PatternNone to the actual pattern.
func TestLateDevcontainerAddition(t *testing.T) {
	repoPath := setupTestRepo(t)
	wm := worktree.NewManager()

	// Create worktree without devcontainer.
	branchName := "feature-late-dc"
	envName := sanitizeBranchName(branchName)
	worktreePath := filepath.Join(t.TempDir(), "wt-late-dc")

	err := wm.Add(repoPath, branchName, worktreePath, "")
	require.NoError(t, err)

	// Write initial marker with PatternNone.
	marker := worktree.MarkerFile{
		ManagedBy:      "worktree-container",
		Name:           envName,
		Branch:         branchName,
		SourceRepoPath: repoPath,
		ConfigPattern:  model.PatternNone,
		CreatedAt:      "2026-03-02T00:00:00Z",
	}
	err = worktree.WriteMarkerFile(worktreePath, marker)
	require.NoError(t, err)

	// Verify initial state is PatternNone.
	readMarker, err := worktree.ReadMarkerFile(worktreePath)
	require.NoError(t, err)
	assert.Equal(t, model.PatternNone, readMarker.ConfigPattern)

	// Now add a devcontainer.json to the worktree.
	devcontainerDir := filepath.Join(worktreePath, ".devcontainer")
	err = os.MkdirAll(devcontainerDir, 0755)
	require.NoError(t, err)

	devcontainerJSON := `{
		"name": "late-addition",
		"image": "node:20",
		"forwardPorts": [3000]
	}`
	err = os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(devcontainerJSON), 0644)
	require.NoError(t, err)

	// Simulate updating the marker file to reflect a new config pattern.
	marker.ConfigPattern = model.PatternImage
	err = worktree.WriteMarkerFile(worktreePath, marker)
	require.NoError(t, err)

	// Verify the marker was updated.
	readMarker, err = worktree.ReadMarkerFile(worktreePath)
	require.NoError(t, err)
	assert.Equal(t, model.PatternImage, readMarker.ConfigPattern,
		"marker should be updated to 'image' after late devcontainer addition")
}

// TestFindEnvironmentFromMarker_Found verifies that findEnvironmentFromMarker
// discovers an environment by name from marker files in the repository's
// worktrees. This test uses os.Chdir to set the working directory inside the
// repo, so it must NOT use t.Parallel().
func TestFindEnvironmentFromMarker_Found(t *testing.T) {
	repoPath := setupTestRepo(t)
	wm := worktree.NewManager()

	// Create a worktree with a marker file.
	branchName := "feature-find-marker"
	envName := "feature-find-marker"
	worktreePath := filepath.Join(t.TempDir(), "wt-find-marker")

	err := wm.Add(repoPath, branchName, worktreePath, "")
	require.NoError(t, err)

	marker := worktree.MarkerFile{
		ManagedBy:      "worktree-container",
		Name:           envName,
		Branch:         branchName,
		SourceRepoPath: repoPath,
		ConfigPattern:  model.PatternImage,
		CreatedAt:      "2026-03-02T12:00:00Z",
	}
	err = worktree.WriteMarkerFile(worktreePath, marker)
	require.NoError(t, err)

	// Save and restore cwd to avoid affecting other tests.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	err = os.Chdir(repoPath)
	require.NoError(t, err)

	// findEnvironmentFromMarker should find the environment.
	env, findErr := findEnvironmentFromMarker(envName)
	require.NoError(t, findErr)
	require.NotNil(t, env, "should find environment by name from marker")

	assert.Equal(t, envName, env.Name)
	assert.Equal(t, branchName, env.Branch)
	assert.Equal(t, model.PatternImage, env.ConfigPattern)
}

// TestFindEnvironmentFromMarker_NotFound verifies that findEnvironmentFromMarker
// returns nil, nil when the specified environment name does not match any marker.
func TestFindEnvironmentFromMarker_NotFound(t *testing.T) {
	repoPath := setupTestRepo(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	err = os.Chdir(repoPath)
	require.NoError(t, err)

	env, findErr := findEnvironmentFromMarker("nonexistent-env")
	assert.NoError(t, findErr)
	assert.Nil(t, env, "should return nil for non-existent environment")
}

// TestFindEnvironmentFromMarker_StatusByPattern verifies that the status
// heuristic in findEnvironmentFromMarker correctly maps:
// - PatternNone → StatusNoContainer
// - PatternImage → StatusStopped
func TestFindEnvironmentFromMarker_StatusByPattern(t *testing.T) {
	repoPath := setupTestRepo(t)
	wm := worktree.NewManager()

	// Create two worktrees with different patterns.
	wtNone := filepath.Join(t.TempDir(), "wt-status-none")
	wtImage := filepath.Join(t.TempDir(), "wt-status-image")

	err := wm.Add(repoPath, "status-none", wtNone, "")
	require.NoError(t, err)
	err = wm.Add(repoPath, "status-image", wtImage, "")
	require.NoError(t, err)

	// Write markers.
	err = worktree.WriteMarkerFile(wtNone, worktree.MarkerFile{
		ManagedBy:      "worktree-container",
		Name:           "status-none",
		Branch:         "status-none",
		SourceRepoPath: repoPath,
		ConfigPattern:  model.PatternNone,
		CreatedAt:      "2026-03-02T12:00:00Z",
	})
	require.NoError(t, err)

	err = worktree.WriteMarkerFile(wtImage, worktree.MarkerFile{
		ManagedBy:      "worktree-container",
		Name:           "status-image",
		Branch:         "status-image",
		SourceRepoPath: repoPath,
		ConfigPattern:  model.PatternImage,
		CreatedAt:      "2026-03-02T12:00:00Z",
	})
	require.NoError(t, err)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	err = os.Chdir(repoPath)
	require.NoError(t, err)

	// PatternNone → StatusNoContainer
	envNone, err := findEnvironmentFromMarker("status-none")
	require.NoError(t, err)
	require.NotNil(t, envNone)
	assert.Equal(t, model.StatusNoContainer, envNone.Status,
		"PatternNone should map to StatusNoContainer")

	// PatternImage → StatusStopped
	envImage, err := findEnvironmentFromMarker("status-image")
	require.NoError(t, err)
	require.NotNil(t, envImage)
	assert.Equal(t, model.StatusStopped, envImage.Status,
		"PatternImage should map to StatusStopped (best guess without Docker)")
}
