// Package port implements port availability scanning and offset-based port
// allocation for worktree environments.
//
// The port management system ensures zero port collisions between worktree
// environments (the project's NON-NEGOTIABLE #1 principle). It does this by:
//   - Scanning host ports with net.Listen/net.ListenPacket to detect availability
//   - Applying a deterministic offset formula: shiftedPort = originalPort + (worktreeIndex * 10000)
//   - Falling back to dynamic port discovery when shifted ports are unavailable
package port

import (
	"fmt"
	"net"
)

// Scanner checks whether specific ports are available on the host machine.
//
// It uses the operating system's network stack (net.Listen / net.ListenPacket)
// to determine if a port is free. This is the most reliable method because it
// asks the OS directly, rather than parsing /proc/net/* or relying on external
// commands like `lsof` or `ss` which may require elevated permissions.
//
// The struct is currently stateless, but is defined as a struct (rather than
// bare functions) so that future options (e.g., bind address, timeout) can be
// added without breaking the API. It also makes the Scanner injectable as a
// dependency, which improves testability of the Allocator.
type Scanner struct{}

// NewScanner creates a new Scanner instance.
// Currently no configuration is needed, but this constructor follows Go
// convention and allows future expansion (e.g., custom bind address).
func NewScanner() *Scanner {
	return &Scanner{}
}

// IsPortAvailable checks whether a single port is free on the host machine.
//
// For TCP, it attempts net.Listen("tcp", ":port"). For UDP, it attempts
// net.ListenPacket("udp", ":port"). If the listen/bind succeeds, the port
// is available — the listener is immediately closed via defer.
//
// We bind to all interfaces (":port" rather than "127.0.0.1:port") because
// Docker typically publishes ports on 0.0.0.0, so we need to check the same
// address space to avoid false positives.
//
// Parameters:
//   - port: the port number to check (1-65535)
//   - protocol: "tcp" or "udp"
//
// Returns true if the port is free, false if it is already in use or invalid.
func (s *Scanner) IsPortAvailable(port int, protocol string) bool {
	addr := fmt.Sprintf(":%d", port)

	switch protocol {
	case "tcp":
		// net.Listen opens a TCP listener. If the port is already bound by
		// another process, this returns an error (typically "address already in use").
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return false
		}
		// defer ensures the listener is closed even if something panics between
		// here and the return statement. We close immediately because we only
		// needed to test availability, not actually accept connections.
		defer func() { _ = listener.Close() }()
		return true

	case "udp":
		// net.ListenPacket is the UDP equivalent. UDP is connectionless, so we
		// use ListenPacket (which returns a PacketConn) instead of Listen.
		conn, err := net.ListenPacket("udp", addr)
		if err != nil {
			return false
		}
		defer func() { _ = conn.Close() }()
		return true

	default:
		// Unknown protocol — treat as unavailable to fail safe.
		return false
	}
}

// FindAvailablePort scans a port range [startPort, endPort] (inclusive) and
// returns the first port that is available for the given protocol.
//
// The search is sequential from startPort upward. This deterministic ordering
// means the same free port will be selected consistently, which helps with
// reproducibility in testing and debugging.
//
// Returns an error if no available port is found in the entire range. This
// typically indicates heavy port usage on the host, which the CLI should
// report to the user with exit code ExitPortAllocationFailed.
func (s *Scanner) FindAvailablePort(startPort, endPort int, protocol string) (int, error) {
	for port := startPort; port <= endPort; port++ {
		if s.IsPortAvailable(port, protocol) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available %s port found in range %d-%d", protocol, startPort, endPort)
}

// GetUsedPorts returns a slice of port numbers that are currently in use
// within the specified range [startPort, endPort] (inclusive).
//
// This scans using TCP only, because TCP port conflicts are the primary
// concern for web services and databases. If a port fails the TCP availability
// check, it is considered "in use".
//
// This method is useful for the `wt status` command to display which ports
// are occupied on the host.
func (s *Scanner) GetUsedPorts(startPort, endPort int) []int {
	var used []int
	for port := startPort; port <= endPort; port++ {
		// Check TCP availability — if Listen fails, the port is in use.
		if !s.IsPortAvailable(port, "tcp") {
			used = append(used, port)
		}
	}
	return used
}
