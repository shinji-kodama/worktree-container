// Package port implements port scanning and allocation for the
// worktree-container CLI.
//
// The core algorithm is offset-based port shifting:
//
//	shiftedPort = originalPort + (worktreeIndex * 10000)
//
// This deterministic formula ensures each worktree environment gets
// a predictable, non-overlapping port range. The Scanner verifies
// OS-level port availability via net.Listen(), while the Allocator
// combines scanning with cross-environment conflict detection to
// enforce the "port collision zero" guarantee.
//
// When the shifted port exceeds 65535, the allocator falls back to
// dynamic port discovery in the IANA ephemeral range (49152-65535).
package port
