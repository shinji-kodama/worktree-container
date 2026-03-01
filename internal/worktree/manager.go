// Package worktree provides Git worktree management operations.
//
// This package wraps Git CLI commands (via os/exec) to create, list,
// remove, and inspect Git worktrees. It serves as the Git integration
// layer for the worktree-container CLI, where each worktree is paired
// with an independent Dev Container environment.
//
// Design decisions:
//   - We shell out to `git` rather than using a Go Git library (e.g., go-git)
//     because worktree operations require full Git CLI compatibility, and
//     go-git's worktree support is limited.
//   - The Manager struct is currently stateless but exists as a receiver to
//     allow future extension (e.g., custom git binary path, logging).
//   - All errors from Git commands are wrapped in model.CLIError with
//     ExitGitError to enable proper CLI exit code handling.
package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mmr-tortoise/worktree-container/internal/model"
)

// WorktreeInfo holds metadata about a single Git worktree entry
// as parsed from `git worktree list --porcelain` output.
//
// Example porcelain output for a single worktree block:
//
//	worktree /path/to/feature-branch
//	HEAD abc123def456
//	branch refs/heads/feature-branch
type WorktreeInfo struct {
	// Path is the absolute filesystem path to the worktree directory.
	Path string

	// Branch is the full branch reference (e.g., "refs/heads/main").
	// Empty if the worktree is in a detached HEAD state.
	Branch string

	// HEAD is the commit SHA that the worktree currently points to.
	HEAD string

	// IsBare indicates whether this worktree entry represents a bare repository.
	// Bare repositories appear in `git worktree list` output with a "bare" marker.
	IsBare bool
}

// Manager provides Git worktree operations by invoking the git CLI.
//
// It is currently stateless — all methods receive the repository path
// as a parameter. The struct exists as a receiver to support future
// extensions such as configurable git binary path or logging middleware.
type Manager struct{}

// NewManager creates a new worktree Manager instance.
//
// Currently there is no initialization logic, but this constructor
// follows Go convention and allows us to add setup code later
// (e.g., verifying git is installed, setting a custom git path)
// without breaking callers.
func NewManager() *Manager {
	return &Manager{}
}

// Add creates a new Git worktree at the specified path on a new branch.
//
// This method handles two cases:
//  1. If the branch does NOT already exist: creates a new branch from baseBranch
//     using `git worktree add -b <branch> <worktreePath> <baseBranch>`.
//  2. If the branch already exists: checks out the existing branch into the
//     new worktree using `git worktree add <worktreePath> <branch>`.
//
// If baseBranch is empty, HEAD is used as the starting point for the new branch.
//
// Parameters:
//   - repoPath: absolute path to the main Git repository (used as working directory)
//   - branch: the branch name to create or check out
//   - worktreePath: absolute path where the new worktree will be created
//   - baseBranch: the branch to base the new branch on (empty string means HEAD)
func (m *Manager) Add(repoPath, branch, worktreePath, baseBranch string) error {
	// Check if the branch already exists to decide which git command form to use.
	// If the branch exists, we cannot use -b (it would fail with "already exists").
	if m.BranchExists(repoPath, branch) {
		// Branch exists — just create a worktree that checks out the existing branch.
		_, err := runGit(repoPath, "worktree", "add", worktreePath, branch)
		return err
	}

	// Branch does not exist — create a new branch at the specified base.
	// Build the command arguments: git worktree add -b <branch> <worktreePath> [baseBranch]
	args := []string{"worktree", "add", "-b", branch, worktreePath}
	if baseBranch != "" {
		args = append(args, baseBranch)
	}
	// When baseBranch is empty, git defaults to HEAD as the starting point.

	_, err := runGit(repoPath, args...)
	return err
}

// List returns information about all worktrees associated with the given repository.
//
// It runs `git worktree list --porcelain` which produces machine-parseable output.
// Each worktree block is separated by a blank line. Within a block, each line
// is a space-separated key-value pair:
//
//	worktree /path/to/dir
//	HEAD abc123
//	branch refs/heads/main
//
// Special markers like "bare" or "detached" appear as standalone keywords.
func (m *Manager) List(repoPath string) ([]WorktreeInfo, error) {
	output, err := runGit(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	return parsePorcelainOutput(output), nil
}

// Remove deletes a Git worktree at the specified path.
//
// This runs `git worktree remove <worktreePath>`, which removes the worktree
// directory and its administrative files. If force is true, the --force flag
// is added to allow removal of worktrees with uncommitted changes.
//
// Note: This only removes the Git worktree. Docker containers associated
// with the worktree must be cleaned up separately.
func (m *Manager) Remove(repoPath, worktreePath string, force bool) error {
	args := []string{"worktree", "remove", worktreePath}
	if force {
		// --force allows removing worktrees that have untracked files or
		// uncommitted changes. Without it, git refuses to remove "dirty" worktrees.
		args = []string{"worktree", "remove", "--force", worktreePath}
	}

	_, err := runGit(repoPath, args...)
	return err
}

// IsWorktree checks whether the given path is a Git worktree (as opposed to
// a main repository working directory).
//
// Git worktrees are identified by having a .git FILE (not directory) that
// contains a "gitdir:" pointer to the main repository's .git/worktrees/<name>
// directory. In contrast, the main working directory has a .git DIRECTORY.
//
// This distinction is important because the CLI needs to identify whether
// a user is working in a worktree or the main repo checkout.
func (m *Manager) IsWorktree(path string) bool {
	gitPath := filepath.Join(path, ".git")

	// Use os.Lstat instead of os.Stat to avoid following symlinks.
	// We specifically need to check if .git is a file (worktree) vs directory (main repo).
	info, err := os.Lstat(gitPath)
	if err != nil {
		// .git doesn't exist at all — not a git repository or worktree.
		return false
	}

	// If .git is a directory, this is the main repository, not a worktree.
	if info.IsDir() {
		return false
	}

	// .git is a file — read it to verify it contains "gitdir:" which confirms
	// this is a Git worktree (or a submodule, but for our purposes both count).
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return false
	}

	return strings.HasPrefix(string(content), "gitdir:")
}

// GetRepoRoot returns the absolute path to the top-level directory of the
// Git repository containing the given path.
//
// This uses `git rev-parse --show-toplevel` which works correctly for both
// the main repository and worktrees — it returns the root of whichever
// working tree contains the specified path.
//
// Note: For worktrees, this returns the worktree root, NOT the main repo root.
// Use `git rev-parse --git-common-dir` if you need the main repo's .git directory.
func (m *Manager) GetRepoRoot(path string) (string, error) {
	output, err := runGit(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	// Trim whitespace/newline from git output.
	return strings.TrimSpace(output), nil
}

// GetCurrentBranch returns the name of the currently checked-out branch
// at the given path.
//
// Uses `git rev-parse --abbrev-ref HEAD` which returns the short branch name
// (e.g., "main" instead of "refs/heads/main"). Returns "HEAD" if the
// repository is in a detached HEAD state.
func (m *Manager) GetCurrentBranch(path string) (string, error) {
	output, err := runGit(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// BranchExists checks whether a branch with the given name exists in the repository.
//
// This uses `git rev-parse --verify <branch>` which exits with code 0 if the
// ref exists and non-zero otherwise. We only care about the exit code, not
// the output (which would be the commit SHA).
//
// This check is used by Add() to decide whether to create a new branch (-b)
// or check out an existing one.
func (m *Manager) BranchExists(repoPath, branch string) bool {
	_, err := runGit(repoPath, "rev-parse", "--verify", branch)
	return err == nil
}

// runGit executes a git command with the given arguments in the specified directory.
//
// It captures both stdout and stderr. On success (exit code 0), it returns
// the stdout output. On failure, it returns a model.CLIError with ExitGitError
// code, including the stderr output in the error message for debugging.
//
// The repoPath parameter is passed to git via the -C flag, which causes git
// to change to that directory before doing anything else. This avoids the need
// to change the process's working directory (which would be problematic in
// concurrent scenarios).
func runGit(repoPath string, args ...string) (string, error) {
	// Prepend -C <repoPath> to make git operate in the target directory.
	// This is safer than using exec.Command().Dir because -C is handled
	// by git itself and works correctly with all git subcommands.
	fullArgs := append([]string{"-C", repoPath}, args...)

	// #nosec G204 — args are constructed internally, not from user input
	cmd := exec.Command("git", fullArgs...)

	// Capture stdout and stderr separately so we can include stderr
	// in error messages while returning stdout on success.
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Wrap the git error in a CLIError with the Git-specific exit code.
		// Include both the git error message and stderr output for diagnostics.
		stderrStr := strings.TrimSpace(stderr.String())
		message := fmt.Sprintf("git %s failed", strings.Join(args, " "))
		if stderrStr != "" {
			message = fmt.Sprintf("%s: %s", message, stderrStr)
		}
		return "", model.WrapCLIError(model.ExitGitError, message, err)
	}

	return stdout.String(), nil
}

// parsePorcelainOutput parses the output of `git worktree list --porcelain`
// into a slice of WorktreeInfo structs.
//
// The porcelain format uses blank lines to separate worktree blocks.
// Each block contains key-value pairs (space-separated) and optional
// standalone markers like "bare" or "detached".
//
// Example input:
//
//	worktree /path/to/main
//	HEAD abc123
//	branch refs/heads/main
//
//	worktree /path/to/feature
//	HEAD def456
//	branch refs/heads/feature
//	<empty line at end>
func parsePorcelainOutput(output string) []WorktreeInfo {
	var worktrees []WorktreeInfo

	// Split on double-newline to get individual worktree blocks.
	// The trailing newline may produce an empty last element, which we skip.
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	var current *WorktreeInfo
	for _, line := range lines {
		// A blank line signals the end of a worktree block.
		if line == "" {
			if current != nil {
				worktrees = append(worktrees, *current)
				current = nil
			}
			continue
		}

		// Parse key-value pairs. The key is the first word, the value is everything after.
		// Some entries (like "bare" or "detached") are standalone keywords with no value.
		key, value, _ := strings.Cut(line, " ")

		switch key {
		case "worktree":
			// Start a new worktree block.
			current = &WorktreeInfo{Path: value}
		case "HEAD":
			if current != nil {
				current.HEAD = value
			}
		case "branch":
			if current != nil {
				current.Branch = value
			}
		case "bare":
			if current != nil {
				current.IsBare = true
			}
			// "detached" is another possible marker — we don't need to track it
			// explicitly because a detached HEAD simply has an empty Branch field.
		}
	}

	// Handle the last block if the output doesn't end with a blank line.
	if current != nil {
		worktrees = append(worktrees, *current)
	}

	return worktrees
}
