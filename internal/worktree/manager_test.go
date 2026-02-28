package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a temporary directory with an initialized Git repository
// containing a single commit. This provides a realistic baseline for testing
// worktree operations, since most git worktree commands require at least one
// commit to exist.
//
// The function uses t.TempDir() which automatically cleans up after the test.
// It also configures a local user.name and user.email so that `git commit`
// works in CI environments where global git config may not be set.
//
// Returns the absolute path to the temporary repository.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Initialize a new Git repository.
	runTestGit(t, dir, "init")

	// Configure user identity at the repo level so `git commit` works
	// even in environments without a global Git configuration (e.g., CI).
	runTestGit(t, dir, "config", "user.email", "test@example.com")
	runTestGit(t, dir, "config", "user.name", "Test User")

	// Create an initial commit. Git worktree commands require at least one
	// commit to exist, because a worktree needs a branch, and a branch
	// needs at least one commit to point to.
	initialFile := filepath.Join(dir, "README.md")
	err := os.WriteFile(initialFile, []byte("# Test Repo\n"), 0644)
	require.NoError(t, err, "failed to create initial file")

	runTestGit(t, dir, "add", ".")
	runTestGit(t, dir, "commit", "-m", "initial commit")

	return dir
}

// runTestGit is a test helper that runs a git command in the specified directory
// and fails the test immediately if the command exits with a non-zero status.
// This keeps test setup code concise by avoiding repetitive error checks.
func runTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
	return string(output)
}

// TestAdd verifies that Manager.Add creates a new worktree with a new branch.
// It checks both that the worktree directory is created on disk and that
// git reports it in the worktree list.
func TestAdd(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	worktreePath := filepath.Join(t.TempDir(), "feature-branch")

	// Add a worktree on a new branch based on HEAD (empty baseBranch = HEAD).
	err := m.Add(repoPath, "feature-branch", worktreePath, "")
	require.NoError(t, err, "Add should succeed for a new branch")

	// Verify the worktree directory was created on disk.
	_, statErr := os.Stat(worktreePath)
	assert.NoError(t, statErr, "worktree directory should exist after Add")

	// Verify the branch was checked out in the new worktree.
	branch, err := m.GetCurrentBranch(worktreePath)
	require.NoError(t, err)
	assert.Equal(t, "feature-branch", branch)
}

// TestAddExistingBranch verifies that Manager.Add works correctly when the
// branch already exists. In this case, it should check out the existing branch
// without the -b flag (which would fail if the branch already exists).
func TestAddExistingBranch(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	// Create a branch first (without a worktree).
	runTestGit(t, repoPath, "branch", "existing-branch")

	worktreePath := filepath.Join(t.TempDir(), "existing-branch-wt")

	// Add should detect the existing branch and use `git worktree add <path> <branch>`
	// without -b.
	err := m.Add(repoPath, "existing-branch", worktreePath, "")
	require.NoError(t, err, "Add should succeed for an existing branch")

	branch, err := m.GetCurrentBranch(worktreePath)
	require.NoError(t, err)
	assert.Equal(t, "existing-branch", branch)
}

// TestAddWithBaseBranch verifies that Manager.Add creates a new branch based
// on a specified base branch rather than HEAD.
func TestAddWithBaseBranch(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	// Get the current branch name to use as baseBranch.
	mainBranch, err := m.GetCurrentBranch(repoPath)
	require.NoError(t, err)

	worktreePath := filepath.Join(t.TempDir(), "from-base")

	err = m.Add(repoPath, "from-base", worktreePath, mainBranch)
	require.NoError(t, err, "Add with explicit baseBranch should succeed")

	branch, err := m.GetCurrentBranch(worktreePath)
	require.NoError(t, err)
	assert.Equal(t, "from-base", branch)
}

// TestList verifies that Manager.List returns all worktrees including the main
// repository and any additional worktrees that have been created.
func TestList(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	// Create two additional worktrees.
	wt1 := filepath.Join(t.TempDir(), "wt1")
	wt2 := filepath.Join(t.TempDir(), "wt2")

	err := m.Add(repoPath, "branch-1", wt1, "")
	require.NoError(t, err)

	err = m.Add(repoPath, "branch-2", wt2, "")
	require.NoError(t, err)

	// List should return the main repo + 2 worktrees = 3 entries.
	worktrees, err := m.List(repoPath)
	require.NoError(t, err)
	assert.Len(t, worktrees, 3, "should list main repo + 2 worktrees")

	// Collect all paths from the listing for verification.
	paths := make([]string, len(worktrees))
	for i, wt := range worktrees {
		paths[i] = wt.Path
	}

	// Verify each worktree path appears in the listing.
	// We resolve symlinks using filepath.EvalSymlinks because on macOS,
	// t.TempDir() returns a path under /var which is a symlink to /private/var,
	// and git may resolve this differently.
	resolvedRepo, _ := filepath.EvalSymlinks(repoPath)
	resolvedWT1, _ := filepath.EvalSymlinks(wt1)
	resolvedWT2, _ := filepath.EvalSymlinks(wt2)

	assert.Contains(t, paths, resolvedRepo, "listing should include main repo")
	assert.Contains(t, paths, resolvedWT1, "listing should include worktree 1")
	assert.Contains(t, paths, resolvedWT2, "listing should include worktree 2")

	// Verify branch information is populated.
	for _, wt := range worktrees {
		assert.NotEmpty(t, wt.HEAD, "each worktree should have a HEAD commit")
		// Branch can be empty for detached HEAD, but our test worktrees all have branches.
		assert.NotEmpty(t, wt.Branch, "each worktree should have a branch ref")
	}
}

// TestRemove verifies that Manager.Remove deletes a worktree from both the
// filesystem and git's worktree registry.
func TestRemove(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	worktreePath := filepath.Join(t.TempDir(), "to-remove")
	err := m.Add(repoPath, "to-remove", worktreePath, "")
	require.NoError(t, err)

	// Verify it exists before removal.
	_, statErr := os.Stat(worktreePath)
	require.NoError(t, statErr, "worktree should exist before removal")

	// Remove the worktree (non-forced).
	err = m.Remove(repoPath, worktreePath, false)
	require.NoError(t, err, "Remove should succeed for a clean worktree")

	// Verify the directory no longer exists.
	_, statErr = os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree directory should be deleted after removal")

	// Verify the worktree is no longer listed.
	worktrees, err := m.List(repoPath)
	require.NoError(t, err)

	resolvedWT, _ := filepath.EvalSymlinks(worktreePath)
	for _, wt := range worktrees {
		assert.NotEqual(t, resolvedWT, wt.Path, "removed worktree should not appear in list")
	}
}

// TestRemoveForce verifies that Manager.Remove with force=true can remove
// a worktree that has uncommitted changes (which would fail without --force).
func TestRemoveForce(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	worktreePath := filepath.Join(t.TempDir(), "dirty-wt")
	err := m.Add(repoPath, "dirty-branch", worktreePath, "")
	require.NoError(t, err)

	// Make the worktree "dirty" by adding an untracked file.
	dirtyFile := filepath.Join(worktreePath, "untracked.txt")
	err = os.WriteFile(dirtyFile, []byte("dirty"), 0644)
	require.NoError(t, err)

	// Force removal should succeed even with untracked files.
	err = m.Remove(repoPath, worktreePath, true)
	require.NoError(t, err, "Force Remove should succeed even with untracked files")

	_, statErr := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree directory should be deleted after forced removal")
}

// TestGetRepoRoot verifies that GetRepoRoot returns the correct top-level
// directory for a Git repository.
func TestGetRepoRoot(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	root, err := m.GetRepoRoot(repoPath)
	require.NoError(t, err)

	// Resolve symlinks on both sides for comparison because macOS uses
	// /var -> /private/var symlinks in temporary directories.
	resolvedRepo, _ := filepath.EvalSymlinks(repoPath)
	resolvedRoot, _ := filepath.EvalSymlinks(root)
	assert.Equal(t, resolvedRepo, resolvedRoot, "GetRepoRoot should return the repo path")
}

// TestGetRepoRootFromSubdirectory verifies that GetRepoRoot works correctly
// when called from a subdirectory within the repository.
func TestGetRepoRootFromSubdirectory(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	// Create a subdirectory within the repo.
	subDir := filepath.Join(repoPath, "sub", "dir")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	root, err := m.GetRepoRoot(subDir)
	require.NoError(t, err)

	resolvedRepo, _ := filepath.EvalSymlinks(repoPath)
	resolvedRoot, _ := filepath.EvalSymlinks(root)
	assert.Equal(t, resolvedRepo, resolvedRoot,
		"GetRepoRoot from subdirectory should return the repo root")
}

// TestGetCurrentBranch verifies that GetCurrentBranch returns the correct
// branch name. After `git init`, the default branch is typically "main" or
// "master" depending on the git configuration.
func TestGetCurrentBranch(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	branch, err := m.GetCurrentBranch(repoPath)
	require.NoError(t, err)

	// The default branch name depends on git configuration (init.defaultBranch).
	// It's typically "main" or "master". We accept either.
	assert.True(t, branch == "main" || branch == "master",
		"expected 'main' or 'master', got %q", branch)
}

// TestBranchExists verifies that BranchExists correctly detects the presence
// or absence of branches.
func TestBranchExists(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	// The default branch (created during setupTestRepo) should exist.
	mainBranch, err := m.GetCurrentBranch(repoPath)
	require.NoError(t, err)

	assert.True(t, m.BranchExists(repoPath, mainBranch),
		"BranchExists should return true for the default branch")

	// A non-existent branch should return false.
	assert.False(t, m.BranchExists(repoPath, "non-existent-branch-xyz"),
		"BranchExists should return false for a branch that doesn't exist")
}

// TestBranchExistsAfterCreation verifies that BranchExists returns true
// for a branch that was created after repository initialization.
func TestBranchExistsAfterCreation(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	// Create a new branch.
	runTestGit(t, repoPath, "branch", "new-feature")

	assert.True(t, m.BranchExists(repoPath, "new-feature"),
		"BranchExists should return true for a newly created branch")
}

// TestIsWorktree verifies that IsWorktree correctly distinguishes between
// a worktree directory (which has a .git file) and the main repository
// (which has a .git directory).
func TestIsWorktree(t *testing.T) {
	repoPath := setupTestRepo(t)
	m := NewManager()

	// The main repository should NOT be identified as a worktree.
	// It has a .git directory, not a .git file.
	assert.False(t, m.IsWorktree(repoPath),
		"main repo should not be identified as a worktree")

	// Create a worktree and verify it IS identified as a worktree.
	worktreePath := filepath.Join(t.TempDir(), "wt-check")
	err := m.Add(repoPath, "wt-check-branch", worktreePath, "")
	require.NoError(t, err)

	assert.True(t, m.IsWorktree(worktreePath),
		"worktree path should be identified as a worktree")
}

// TestIsWorktreeNonGitDirectory verifies that IsWorktree returns false
// for a directory that is not part of any Git repository.
func TestIsWorktreeNonGitDirectory(t *testing.T) {
	m := NewManager()

	nonGitDir := t.TempDir()
	assert.False(t, m.IsWorktree(nonGitDir),
		"non-git directory should not be identified as a worktree")
}

// TestParsePorcelainOutput directly tests the parsePorcelainOutput function
// with known porcelain format strings to verify correct parsing logic.
func TestParsePorcelainOutput(t *testing.T) {
	// Simulate typical `git worktree list --porcelain` output.
	input := `worktree /path/to/main
HEAD abc123def456
branch refs/heads/main

worktree /path/to/feature
HEAD def789abc012
branch refs/heads/feature

`
	result := parsePorcelainOutput(input)
	require.Len(t, result, 2, "should parse two worktree entries")

	// Verify first worktree (main).
	assert.Equal(t, "/path/to/main", result[0].Path)
	assert.Equal(t, "abc123def456", result[0].HEAD)
	assert.Equal(t, "refs/heads/main", result[0].Branch)
	assert.False(t, result[0].IsBare)

	// Verify second worktree (feature).
	assert.Equal(t, "/path/to/feature", result[1].Path)
	assert.Equal(t, "def789abc012", result[1].HEAD)
	assert.Equal(t, "refs/heads/feature", result[1].Branch)
	assert.False(t, result[1].IsBare)
}

// TestParsePorcelainOutputBare verifies that the parser correctly handles
// bare repository entries in the porcelain output.
func TestParsePorcelainOutputBare(t *testing.T) {
	input := `worktree /path/to/bare-repo
HEAD abc123
bare

`
	result := parsePorcelainOutput(input)
	require.Len(t, result, 1)

	assert.Equal(t, "/path/to/bare-repo", result[0].Path)
	assert.True(t, result[0].IsBare, "bare marker should set IsBare to true")
	assert.Empty(t, result[0].Branch, "bare worktree should have no branch")
}

// TestParsePorcelainOutputDetached verifies parsing of worktrees in a
// detached HEAD state (no branch line present).
func TestParsePorcelainOutputDetached(t *testing.T) {
	input := `worktree /path/to/detached
HEAD abc123
detached

`
	result := parsePorcelainOutput(input)
	require.Len(t, result, 1)

	assert.Equal(t, "/path/to/detached", result[0].Path)
	assert.Empty(t, result[0].Branch, "detached HEAD should have no branch")
	assert.False(t, result[0].IsBare)
}

// TestParsePorcelainOutputEmpty verifies that an empty string input
// produces no results without panicking.
func TestParsePorcelainOutputEmpty(t *testing.T) {
	result := parsePorcelainOutput("")
	assert.Empty(t, result, "empty input should produce empty result")
}
