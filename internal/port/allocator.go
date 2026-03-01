package port

import (
	"fmt"

	"github.com/mmr-tortoise/worktree-container/internal/model"
)

const (
	// portShiftMultiplier is the offset multiplied by the worktree index to
	// compute the shifted port. Each worktree index gets its own 10000-port
	// "band" to avoid collisions deterministically.
	//
	// Example: worktreeIndex=1, originalPort=3000 → 3000 + (1*10000) = 13000
	portShiftMultiplier = 10000

	// maxPort is the highest valid TCP/UDP port number (2^16 - 1).
	maxPort = 65535

	// dynamicRangeStart is the start of the IANA dynamic/private port range.
	// When the shifted port overflows maxPort, we fall back to searching for
	// a free port in this range (49152-65535).
	dynamicRangeStart = 49152

	// dynamicRangeEnd is the end of the dynamic port range.
	dynamicRangeEnd = 65535

	// maxWorktreeIndex is the maximum supported worktree index (0-9).
	// This gives us 10 concurrent environments, which is the design limit
	// documented in the spec. Index 0 uses original ports unchanged.
	maxWorktreeIndex = 9
)

// Allocator computes host port assignments for worktree environments using
// an offset-based port shifting strategy.
//
// The core algorithm is simple: shiftedPort = originalPort + (worktreeIndex * 10000).
// This deterministic formula means users can predict which ports their services
// will use without running any commands. For example, a developer working on
// worktree index 2 knows their app on port 3000 will be at 23000.
//
// The Allocator holds a reference to a Scanner for verifying port availability
// at allocation time, and tracks existing allocations from other worktrees to
// prevent cross-environment conflicts.
type Allocator struct {
	// scanner is used to probe the OS for actual port availability.
	// Injected via constructor to allow test doubles if needed.
	scanner *Scanner

	// existingAllocations tracks ports already assigned to other worktree
	// environments. The allocator checks new allocations against this list
	// to enforce the zero-collision guarantee across environments.
	existingAllocations []model.PortAllocation
}

// NewAllocator creates a new Allocator with the given Scanner.
// The scanner must not be nil — it is required for port availability checks.
func NewAllocator(scanner *Scanner) *Allocator {
	return &Allocator{
		scanner: scanner,
	}
}

// SetExistingAllocations registers port allocations from other worktree
// environments. The allocator will avoid assigning any port that conflicts
// with these existing allocations.
//
// This should be called before AllocatePort/AllocatePorts with data gathered
// from Docker labels on running containers (see FR-011 in the spec).
func (a *Allocator) SetExistingAllocations(allocs []model.PortAllocation) {
	a.existingAllocations = allocs
}

// AllocatePort computes a host port for a single container port using the
// offset-based shifting formula.
//
// Algorithm:
//  1. If worktreeIndex == 0, use the original port unchanged (index 0 is the
//     "primary" worktree, typically the main branch).
//  2. Compute shiftedPort = originalPort + (worktreeIndex * 10000).
//  3. If shiftedPort > 65535, skip to step 5 (overflow).
//  4. Verify shiftedPort is available (not used by OS, not in existingAllocations).
//     If available, return it. If not, search upward within the same 10000-block.
//  5. Fall back: search the IANA dynamic range (49152-65535) for any free port.
//
// Parameters:
//   - originalPort: the port number from the container/Compose definition
//   - worktreeIndex: 0-based environment index (0-9)
//   - serviceName: Docker service name, used for labeling the allocation
//   - protocol: "tcp" or "udp"
//
// Returns the allocated PortAllocation or an error if no port could be assigned.
func (a *Allocator) AllocatePort(originalPort, worktreeIndex int, serviceName, protocol string) (*model.PortAllocation, error) {
	// Validate the worktree index against the design limit.
	if worktreeIndex < 0 || worktreeIndex > maxWorktreeIndex {
		return nil, fmt.Errorf("worktree index %d out of range (0-%d)", worktreeIndex, maxWorktreeIndex)
	}

	// Default protocol to TCP if unspecified, matching Docker's default behavior.
	if protocol == "" {
		protocol = "tcp"
	}

	var hostPort int

	if worktreeIndex == 0 {
		// Index 0 is the primary worktree — use the original port as-is.
		// This means the first worktree behaves identically to a standard
		// devcontainer setup, reducing surprise for users.
		hostPort = originalPort
	} else {
		// Apply the deterministic shift formula.
		hostPort = originalPort + (worktreeIndex * portShiftMultiplier)
	}

	// Check if the shifted port exceeds the valid range.
	if hostPort > maxPort {
		// Overflow case: the shifted port doesn't fit in the 16-bit port space.
		// Fall back to dynamic discovery in the ephemeral range.
		fallbackPort, err := a.findAvailablePortExcludingExisting(dynamicRangeStart, dynamicRangeEnd, protocol)
		if err != nil {
			return nil, fmt.Errorf("port overflow: %d+(%d*%d)=%d exceeds %d, and fallback failed: %w",
				originalPort, worktreeIndex, portShiftMultiplier,
				originalPort+(worktreeIndex*portShiftMultiplier), maxPort, err)
		}
		hostPort = fallbackPort
	} else if !a.isPortAvailableForAllocation(hostPort, protocol) {
		// The shifted port is within range but already in use. Try to find
		// the next available port within the same 10000-block.
		//
		// The block boundaries ensure we don't accidentally step into another
		// worktree's port range. For index 1, the block is 10000-19999.
		blockStart := hostPort
		blockEnd := hostPort + portShiftMultiplier - 1
		if blockEnd > maxPort {
			blockEnd = maxPort
		}

		found := false
		for candidate := blockStart + 1; candidate <= blockEnd; candidate++ {
			if a.isPortAvailableForAllocation(candidate, protocol) {
				hostPort = candidate
				found = true
				break
			}
		}

		// If nothing was found in the block, fall back to the dynamic range.
		if !found {
			fallbackPort, err := a.findAvailablePortExcludingExisting(dynamicRangeStart, dynamicRangeEnd, protocol)
			if err != nil {
				return nil, fmt.Errorf("port %d (shifted from %d) is in use and no alternative found: %w",
					hostPort, originalPort, err)
			}
			hostPort = fallbackPort
		}
	}

	return &model.PortAllocation{
		ServiceName:   serviceName,
		ContainerPort: originalPort,
		HostPort:      hostPort,
		Protocol:      protocol,
	}, nil
}

// AllocatePorts processes multiple port specifications for a single worktree
// environment and returns the complete set of port allocations.
//
// It tracks newly allocated ports within this batch to prevent intra-environment
// collisions (e.g., two services both wanting port 3000 at the same index).
// Each successful allocation is temporarily added to existingAllocations so
// subsequent ports in the same batch can see it.
func (a *Allocator) AllocatePorts(ports []model.PortSpec, worktreeIndex int) ([]model.PortAllocation, error) {
	allocations := make([]model.PortAllocation, 0, len(ports))

	for _, ps := range ports {
		// Use ContainerPort as the base for shifting. The HostPort in PortSpec
		// may be 0 (e.g., from forwardPorts which only specifies container ports),
		// so we always shift based on ContainerPort for consistency.
		proto := ps.Protocol
		if proto == "" {
			proto = "tcp"
		}

		alloc, err := a.AllocatePort(ps.ContainerPort, worktreeIndex, ps.ServiceName, proto)
		if err != nil {
			return nil, fmt.Errorf("failed to allocate port for %s:%d: %w", ps.ServiceName, ps.ContainerPort, err)
		}

		// Copy the label from the original port spec.
		alloc.Label = ps.Label

		// Register this allocation so subsequent ports in the same batch
		// won't collide with it. This is critical for correctness when
		// multiple services expose ports that would shift to the same value.
		a.existingAllocations = append(a.existingAllocations, *alloc)

		allocations = append(allocations, *alloc)
	}

	return allocations, nil
}

// isPortAvailableForAllocation checks both the OS-level availability via Scanner
// AND that the port doesn't conflict with any existing allocations from other
// worktree environments.
//
// This two-layer check is necessary because:
//   - Scanner catches ports used by non-worktree processes (e.g., a local MySQL)
//   - existingAllocations catches ports used by other worktree environments that
//     might be stopped (containers not running, so Scanner wouldn't detect them)
func (a *Allocator) isPortAvailableForAllocation(port int, protocol string) bool {
	// First, check against known allocations from other worktree environments.
	for _, alloc := range a.existingAllocations {
		if alloc.HostPort == port && alloc.Protocol == protocol {
			return false
		}
	}

	// Then, check the OS to see if the port is actually free on the host.
	return a.scanner.IsPortAvailable(port, protocol)
}

// findAvailablePortExcludingExisting searches a port range for the first port
// that is both OS-available and not in the existing allocations list.
//
// This is used as a fallback when the deterministic shift formula can't find
// a suitable port (either due to overflow or conflict).
func (a *Allocator) findAvailablePortExcludingExisting(startPort, endPort int, protocol string) (int, error) {
	for port := startPort; port <= endPort; port++ {
		if a.isPortAvailableForAllocation(port, protocol) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available %s port found in range %d-%d", protocol, startPort, endPort)
}
