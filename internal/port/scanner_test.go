package port

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsPortAvailable_FreePort verifies that IsPortAvailable returns true
// for a port that no process is currently using. We pick a high ephemeral
// port (59123) that is extremely unlikely to be in use during tests.
func TestIsPortAvailable_FreePort(t *testing.T) {
	scanner := NewScanner()

	// Use FindAvailablePort to get a port we know is free, rather than
	// hardcoding a port number that might be in use on some CI machines.
	freePort, err := scanner.FindAvailablePort(50000, 50100, "tcp")
	require.NoError(t, err, "should find at least one free port in 50000-50100")

	available := scanner.IsPortAvailable(freePort, "tcp")
	assert.True(t, available, "port %d should be available", freePort)
}

// TestIsPortAvailable_UsedPort verifies that IsPortAvailable returns false
// when a port is already bound by another listener.
//
// The test starts its own TCP listener, then checks the same port.
// This simulates a real-world scenario where another process (e.g., a database)
// is already using the port.
func TestIsPortAvailable_UsedPort(t *testing.T) {
	// Start a TCP listener on an OS-assigned port (":0" lets the OS pick a free port).
	// This avoids test flakiness from hardcoded ports.
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to start test listener")
	// defer ensures cleanup even if the test fails partway through.
	defer func() { _ = listener.Close() }()

	// Extract the actual port the OS assigned to our listener.
	// listener.Addr() returns a net.Addr; for TCP it's a *net.TCPAddr.
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	port := tcpAddr.Port

	scanner := NewScanner()
	available := scanner.IsPortAvailable(port, "tcp")
	assert.False(t, available, "port %d should be in use (we have a listener on it)", port)
}

// TestIsPortAvailable_UDP verifies UDP port scanning works correctly.
// We start a UDP listener and confirm IsPortAvailable reports it as used.
func TestIsPortAvailable_UDP(t *testing.T) {
	// Open a UDP socket on an OS-assigned port.
	conn, err := net.ListenPacket("udp", ":0")
	require.NoError(t, err, "failed to start test UDP listener")
	defer func() { _ = conn.Close() }()

	udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	require.True(t, ok)
	port := udpAddr.Port

	scanner := NewScanner()
	available := scanner.IsPortAvailable(port, "udp")
	assert.False(t, available, "UDP port %d should be in use", port)
}

// TestIsPortAvailable_UnknownProtocol verifies that an unrecognized protocol
// string causes IsPortAvailable to return false (fail-safe behavior).
func TestIsPortAvailable_UnknownProtocol(t *testing.T) {
	scanner := NewScanner()
	available := scanner.IsPortAvailable(50000, "sctp")
	assert.False(t, available, "unknown protocol should return false (fail-safe)")
}

// TestFindAvailablePort verifies that FindAvailablePort successfully finds
// a free port within a given range.
func TestFindAvailablePort(t *testing.T) {
	scanner := NewScanner()

	// Search in a high range that's unlikely to have many listeners.
	port, err := scanner.FindAvailablePort(50000, 50100, "tcp")
	require.NoError(t, err, "should find an available port in range 50000-50100")

	// The returned port must be within the requested range.
	assert.GreaterOrEqual(t, port, 50000)
	assert.LessOrEqual(t, port, 50100)

	// Double-check: the port should actually be available.
	assert.True(t, scanner.IsPortAvailable(port, "tcp"))
}

// TestFindAvailablePort_NoneAvailable verifies that FindAvailablePort returns
// an error when every port in the range is occupied.
//
// We create a tiny 3-port range and bind listeners to all of them, then verify
// that FindAvailablePort correctly reports failure.
func TestFindAvailablePort_NoneAvailable(t *testing.T) {
	scanner := NewScanner()

	// Find a free port to use as the base of our small range.
	basePort, err := scanner.FindAvailablePort(51000, 51100, "tcp")
	require.NoError(t, err)

	// Occupy a small range of consecutive ports by starting listeners on each.
	// We use a slice to hold all listeners so we can clean them up with defer.
	rangeSize := 3
	listeners := make([]net.Listener, 0, rangeSize)
	actualEnd := basePort // Track how many we actually managed to bind.

	for i := 0; i < rangeSize; i++ {
		ln, listenErr := net.Listen("tcp", fmt.Sprintf(":%d", basePort+i))
		if listenErr != nil {
			// If we can't bind even one port (maybe something else grabbed it),
			// skip this test rather than producing a false failure.
			if i == 0 {
				t.Skip("could not bind base port, skipping")
			}
			break
		}
		listeners = append(listeners, ln)
		actualEnd = basePort + i
	}
	// Clean up all listeners when the test completes.
	defer func() {
		for _, ln := range listeners {
			_ = ln.Close()
		}
	}()

	// Now search only within the occupied range. This should fail.
	_, err = scanner.FindAvailablePort(basePort, actualEnd, "tcp")
	assert.Error(t, err, "should fail when all ports in range are occupied")
	assert.Contains(t, err.Error(), "no available")
}

// TestGetUsedPorts verifies that GetUsedPorts correctly identifies ports
// that are in use within a range.
func TestGetUsedPorts(t *testing.T) {
	scanner := NewScanner()

	// Start a listener on an OS-assigned port.
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	port := tcpAddr.Port

	// Scan a range that includes our occupied port.
	used := scanner.GetUsedPorts(port, port)

	// Our listener's port should appear in the results.
	assert.Contains(t, used, port, "the port with an active listener should be reported as used")
}
