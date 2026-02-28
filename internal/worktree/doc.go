// Package worktree provides Git worktree management operations for
// the worktree-container CLI.
//
// All Git operations are performed via os/exec calls to the git binary,
// rather than using a Git library like go-git. This approach:
//   - Avoids CGO dependencies (libgit2)
//   - Uses the exact same Git behavior the user sees in their terminal
//   - Requires Git >= 2.15 (when worktree support matured)
//
// The Manager struct provides methods for adding, listing, and removing
// worktrees, as well as querying branch and repository information.
package worktree
