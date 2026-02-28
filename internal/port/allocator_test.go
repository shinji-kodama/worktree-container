package port

import (
	"fmt"
	"net"
	"testing"

	"github.com/shinji-kodama/worktree-container/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllocatePort_Index0 verifies that worktree index 0 (the primary worktree)
// attempts to use the original port. Index 0 represents the main branch
// environment which should behave identically to a standard devcontainer setup.
// If the original port is already in use by another process, the allocator
// will find the next available port in the same block — this is correct behavior.
func TestAllocatePort_Index0(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	// Use a high port that is very unlikely to be in use on the test machine.
	alloc, err := allocator.AllocatePort(48000, 0, "app", "tcp")
	require.NoError(t, err)

	assert.Equal(t, 48000, alloc.HostPort, "index 0 should use original port when available")
	assert.Equal(t, 48000, alloc.ContainerPort)
	assert.Equal(t, "app", alloc.ServiceName)
	assert.Equal(t, "tcp", alloc.Protocol)
}

// TestAllocatePort_Index1 verifies the basic shifting formula for index 1:
// shiftedPort = 3000 + (1 * 10000) = 13000.
func TestAllocatePort_Index1(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	alloc, err := allocator.AllocatePort(3000, 1, "app", "tcp")
	require.NoError(t, err)

	assert.Equal(t, 13000, alloc.HostPort, "index 1 should shift by 10000")
	assert.Equal(t, 3000, alloc.ContainerPort)
}

// TestAllocatePort_Index2 verifies the shifting formula for index 2:
// shiftedPort = 3000 + (2 * 10000) = 23000.
func TestAllocatePort_Index2(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	alloc, err := allocator.AllocatePort(3000, 2, "app", "tcp")
	require.NoError(t, err)

	assert.Equal(t, 23000, alloc.HostPort, "index 2 should shift by 20000")
	assert.Equal(t, 3000, alloc.ContainerPort)
}

// TestAllocatePort_Overflow verifies that when the shifted port exceeds 65535,
// the allocator falls back to dynamic port discovery in the ephemeral range.
//
// Example: originalPort=8000, worktreeIndex=7 → 8000 + 70000 = 78000 > 65535.
// The allocator should find a free port in 49152-65535 instead.
func TestAllocatePort_Overflow(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	// 8000 + (7 * 10000) = 78000 which exceeds 65535.
	alloc, err := allocator.AllocatePort(8000, 7, "app", "tcp")
	require.NoError(t, err)

	// The host port should be within the dynamic/ephemeral range.
	assert.GreaterOrEqual(t, alloc.HostPort, 49152, "overflow should fall back to dynamic range")
	assert.LessOrEqual(t, alloc.HostPort, 65535, "overflow port should not exceed max port")
	assert.Equal(t, 8000, alloc.ContainerPort, "container port should remain unchanged")
}

// TestAllocatePort_DefaultProtocol verifies that an empty protocol string
// defaults to "tcp", matching Docker's default behavior.
func TestAllocatePort_DefaultProtocol(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	alloc, err := allocator.AllocatePort(3000, 0, "app", "")
	require.NoError(t, err)

	assert.Equal(t, "tcp", alloc.Protocol, "empty protocol should default to tcp")
}

// TestAllocatePort_InvalidIndex verifies that indices outside the valid range
// (0-9) are rejected with an error. This enforces the design limit of 10
// concurrent environments.
func TestAllocatePort_InvalidIndex(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	_, err := allocator.AllocatePort(3000, 10, "app", "tcp")
	assert.Error(t, err, "index 10 should be rejected (max is 9)")
	assert.Contains(t, err.Error(), "out of range")

	_, err = allocator.AllocatePort(3000, -1, "app", "tcp")
	assert.Error(t, err, "negative index should be rejected")
}

// TestAllocatePorts_MultipleServices verifies that allocating ports for
// multiple services at once produces the correct shifted ports.
//
// This simulates a typical Compose setup with app (3000), db (5432),
// and redis (6379) at worktree index 1.
func TestAllocatePorts_MultipleServices(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	ports := []model.PortSpec{
		{ServiceName: "app", ContainerPort: 3000, Protocol: "tcp"},
		{ServiceName: "db", ContainerPort: 5432, Protocol: "tcp"},
		{ServiceName: "redis", ContainerPort: 6379, Protocol: "tcp"},
	}

	allocations, err := allocator.AllocatePorts(ports, 1)
	require.NoError(t, err)
	require.Len(t, allocations, 3, "should return exactly 3 allocations")

	// Verify each service got its expected shifted port:
	// app:  3000 + 10000 = 13000
	// db:   5432 + 10000 = 15432
	// redis: 6379 + 10000 = 16379
	assert.Equal(t, 13000, allocations[0].HostPort, "app should be at 13000")
	assert.Equal(t, "app", allocations[0].ServiceName)

	assert.Equal(t, 15432, allocations[1].HostPort, "db should be at 15432")
	assert.Equal(t, "db", allocations[1].ServiceName)

	assert.Equal(t, 16379, allocations[2].HostPort, "redis should be at 16379")
	assert.Equal(t, "redis", allocations[2].ServiceName)
}

// TestAllocatePorts_ConflictAvoidance verifies that the allocator avoids
// ports already claimed by other worktree environments.
//
// Scenario: Another worktree has already allocated port 13000. When we
// try to allocate port 3000 at index 1 (which would normally shift to 13000),
// the allocator should pick a different port.
func TestAllocatePorts_ConflictAvoidance(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	// Simulate another worktree environment that already uses port 13000.
	allocator.SetExistingAllocations([]model.PortAllocation{
		{
			ServiceName:   "other-app",
			ContainerPort: 3000,
			HostPort:      13000,
			Protocol:      "tcp",
		},
	})

	// Allocate port 3000 at index 1. The natural shift would be 13000,
	// but that's taken, so the allocator should find an alternative.
	alloc, err := allocator.AllocatePort(3000, 1, "app", "tcp")
	require.NoError(t, err)

	// The allocated port must NOT be 13000 (that's already taken).
	assert.NotEqual(t, 13000, alloc.HostPort,
		"should avoid conflicting with existing allocation at 13000")

	// The container port should still reflect the original.
	assert.Equal(t, 3000, alloc.ContainerPort)
	assert.Equal(t, "app", alloc.ServiceName)
}

// TestAllocatePorts_IntraBatchConflictAvoidance verifies that allocating
// multiple ports within the same batch doesn't produce duplicates.
//
// This tests the scenario where two different services happen to have the
// same container port. Each should get a unique host port.
func TestAllocatePorts_IntraBatchConflictAvoidance(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	// Two services both expose port 3000. At index 1, both would want 13000.
	ports := []model.PortSpec{
		{ServiceName: "frontend", ContainerPort: 3000, Protocol: "tcp"},
		{ServiceName: "backend", ContainerPort: 3000, Protocol: "tcp"},
	}

	allocations, err := allocator.AllocatePorts(ports, 1)
	require.NoError(t, err)
	require.Len(t, allocations, 2)

	// The two allocations must have different host ports.
	assert.NotEqual(t, allocations[0].HostPort, allocations[1].HostPort,
		"two services with the same container port must get different host ports")
}

// TestAllocatePort_UDP verifies that UDP port allocation works correctly.
func TestAllocatePort_UDP(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	alloc, err := allocator.AllocatePort(5000, 1, "dns", "udp")
	require.NoError(t, err)

	assert.Equal(t, 15000, alloc.HostPort, "UDP port should shift the same as TCP")
	assert.Equal(t, "udp", alloc.Protocol)
}

// TestAllocatePorts_ThreeEnvironmentsUnique verifies that allocating ports
// for 3 separate worktree environments produces completely unique host ports.
// This is the core "port collision zero" test from the constitution.
//
// Scenario: 3 environments, each with app(3000), db(5432), redis(6379).
// Expected:
//   - Env 1 (index 1): 13000, 15432, 16379
//   - Env 2 (index 2): 23000, 25432, 26379
//   - Env 3 (index 3): 33000, 35432, 36379
func TestAllocatePorts_ThreeEnvironmentsUnique(t *testing.T) {
	scanner := NewScanner()
	ports := []model.PortSpec{
		{ServiceName: "app", ContainerPort: 3000, Protocol: "tcp"},
		{ServiceName: "db", ContainerPort: 5432, Protocol: "tcp"},
		{ServiceName: "redis", ContainerPort: 6379, Protocol: "tcp"},
	}

	// Track all allocated host ports across all environments.
	allHostPorts := make(map[int]string) // hostPort → "env-service"

	for envIndex := 1; envIndex <= 3; envIndex++ {
		allocator := NewAllocator(scanner)

		allocations, err := allocator.AllocatePorts(ports, envIndex)
		require.NoError(t, err, "env %d allocation should succeed", envIndex)
		require.Len(t, allocations, 3, "env %d should have 3 allocations", envIndex)

		for _, alloc := range allocations {
			key := alloc.HostPort
			label := fmt.Sprintf("env%d-%s", envIndex, alloc.ServiceName)

			// Verify no host port is used by another environment.
			existing, conflict := allHostPorts[key]
			assert.False(t, conflict,
				"port %d is used by both %s and %s", key, existing, label)

			allHostPorts[key] = label
		}
	}

	// Verify we have exactly 9 unique host ports (3 envs × 3 services).
	assert.Len(t, allHostPorts, 9, "should have 9 unique host ports across 3 environments")
}

// TestAllocatePorts_ExternalPortOccupied verifies that when an external process
// occupies a port that would be the shifted target, the allocator finds
// an alternative port without error.
func TestAllocatePorts_ExternalPortOccupied(t *testing.T) {
	scanner := NewScanner()

	// Occupy a port that would be the shifted target for port 3000 at index 1.
	// net.Listen on port 13000 simulates an external process.
	listener, err := net.Listen("tcp", ":13000")
	if err != nil {
		// If 13000 is already in use by something else, the test still validates
		// that the allocator avoids it.
		t.Logf("Port 13000 already in use (by external process), test still valid")
	} else {
		defer func() { _ = listener.Close() }()
	}

	allocator := NewAllocator(scanner)
	alloc, err := allocator.AllocatePort(3000, 1, "app", "tcp")
	require.NoError(t, err)

	// The allocator should have found an alternative since 13000 is occupied.
	assert.NotEqual(t, 13000, alloc.HostPort,
		"should avoid port 13000 which is occupied by external process")
	assert.Equal(t, 3000, alloc.ContainerPort)
	assert.Equal(t, "app", alloc.ServiceName)
}

// TestAllocatePorts_ExistingAndExternalConflict verifies the two-layer
// conflict detection: both Docker label allocations AND OS-level port usage
// are checked simultaneously.
func TestAllocatePorts_ExistingAndExternalConflict(t *testing.T) {
	scanner := NewScanner()
	allocator := NewAllocator(scanner)

	// Layer 1: Existing allocation from another worktree at 13000.
	allocator.SetExistingAllocations([]model.PortAllocation{
		{ServiceName: "other-app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
	})

	// Layer 2: External process on 13001 (the likely fallback).
	listener, err := net.Listen("tcp", ":13001")
	if err != nil {
		t.Logf("Port 13001 already in use, test still valid")
	} else {
		defer func() { _ = listener.Close() }()
	}

	alloc, err := allocator.AllocatePort(3000, 1, "app", "tcp")
	require.NoError(t, err)

	// Must avoid both 13000 (existing allocation) and 13001 (external process).
	assert.NotEqual(t, 13000, alloc.HostPort, "should avoid existing allocation")
	assert.NotEqual(t, 13001, alloc.HostPort, "should avoid externally occupied port")
}
