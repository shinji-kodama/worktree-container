// Package docker provides a wrapper around the Docker Engine SDK client
// for managing containers associated with Git worktree environments.
//
// The primary purpose of this package is to abstract Docker API interactions
// and provide worktree-container-specific functionality such as label-based
// container filtering and automatic Docker socket detection.
package docker

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/docker/docker/client"

	"github.com/shinji-kodama/worktree-container/internal/model"
)

// defaultPingTimeout is the maximum duration to wait for a Docker daemon
// response during a Ping operation. 5 seconds is generous enough for most
// environments, including Docker Desktop on macOS which can be slower
// than native Linux Docker.
const defaultPingTimeout = 5 * time.Second

// Client wraps the Docker Engine SDK client to provide worktree-container
// specific functionality. It handles automatic Docker socket detection
// across platforms (Linux, macOS, Windows) and provides methods for
// verifying Docker daemon connectivity.
//
// Usage:
//
//	c, err := docker.NewClient()
//	if err != nil { /* handle */ }
//	defer c.Close()  // Always close to release resources
//	if err := c.Ping(ctx); err != nil { /* Docker not running */ }
type Client struct {
	// inner is the underlying Docker SDK client. We wrap it rather than
	// embedding it to control the exposed API surface and add
	// worktree-specific behavior.
	inner *client.Client
}

// NewClient creates a new Docker client with automatic socket detection.
//
// The detection strategy follows this priority order:
//  1. DOCKER_HOST environment variable (if set, used as-is)
//  2. Platform-specific default socket paths:
//     - Linux: /var/run/docker.sock
//     - macOS: /var/run/docker.sock, then ~/.docker/run/docker.sock
//     - Windows: npipe:////./pipe/docker_engine (Docker Named Pipe)
//
// Returns a model.CLIError with ExitDockerNotRunning if no Docker socket
// is found or the client cannot be created.
func NewClient() (*Client, error) {
	// Step 1: Check if the user has explicitly set DOCKER_HOST.
	// When DOCKER_HOST is set, we respect it unconditionally and let
	// the Docker SDK handle the connection string parsing.
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost != "" {
		return newClientWithHost(dockerHost)
	}

	// Step 2: Auto-detect the Docker socket based on the current platform.
	// runtime.GOOS returns the OS at compile time, which matches the
	// running binary's target platform.
	host, err := detectDockerHost()
	if err != nil {
		return nil, model.WrapCLIError(
			model.ExitDockerNotRunning,
			"Docker socket not found",
			err,
		)
	}

	return newClientWithHost(host)
}

// newClientWithHost creates a Docker client connected to the specified host.
// The host parameter should be a valid Docker connection string (e.g.,
// "unix:///var/run/docker.sock" or "npipe:////./pipe/docker_engine").
func newClientWithHost(host string) (*Client, error) {
	// client.NewClientWithOpts creates a Docker SDK client.
	// - client.WithHost sets the Docker daemon address.
	// - client.WithAPIVersionNegotiation enables automatic API version
	//   negotiation, which ensures compatibility across different Docker
	//   daemon versions without hardcoding a specific API version.
	c, err := client.NewClientWithOpts(
		client.WithHost(host),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, model.WrapCLIError(
			model.ExitDockerNotRunning,
			fmt.Sprintf("failed to create Docker client for host %q", host),
			err,
		)
	}

	return &Client{inner: c}, nil
}

// detectDockerHost determines the Docker socket path for the current platform.
// It probes known socket paths and returns the first one that exists.
//
// Design note: We check for socket file existence rather than attempting
// a connection, because existence checks are fast and don't require
// a running daemon. The Ping() method handles connectivity verification.
func detectDockerHost() (string, error) {
	switch runtime.GOOS {
	case "linux":
		// Linux uses the standard Docker socket path.
		return detectUnixSocket([]string{
			"/var/run/docker.sock",
		})

	case "darwin":
		// macOS has two possible socket locations:
		// 1. /var/run/docker.sock - Standard path (Docker Desktop creates a symlink here)
		// 2. ~/.docker/run/docker.sock - Alternative path used by newer Docker Desktop
		//    versions or when the symlink at /var/run/docker.sock is not created.
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// Fall back to just the standard path if home directory is unavailable.
			return detectUnixSocket([]string{
				"/var/run/docker.sock",
			})
		}
		return detectUnixSocket([]string{
			"/var/run/docker.sock",
			homeDir + "/.docker/run/docker.sock",
		})

	case "windows":
		// Windows uses Named Pipes for Docker communication.
		// The pipe path is fixed by Docker Desktop and cannot be customized
		// via filesystem location. We check if the pipe exists by attempting
		// a brief dial, since os.Stat does not work on Windows named pipes.
		pipePath := `//./pipe/docker_engine`
		conn, err := net.DialTimeout("pipe", pipePath, 1*time.Second)
		if err == nil {
			// Pipe is reachable — close the probe connection immediately.
			// defer is not used here because we want explicit control over
			// the close timing before returning.
			conn.Close()
			return "npipe://" + pipePath, nil
		}
		return "", fmt.Errorf("Docker named pipe not found at %s: %w", pipePath, err)

	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// detectUnixSocket probes a list of Unix socket paths and returns the
// Docker host URI for the first socket that exists on the filesystem.
//
// The paths are checked in order, so callers should list them from
// most-preferred to least-preferred.
func detectUnixSocket(paths []string) (string, error) {
	for _, path := range paths {
		// os.Stat checks filesystem metadata. For Unix sockets, a successful
		// Stat confirms the socket file exists, though it does not guarantee
		// the Docker daemon is listening on it.
		if _, err := os.Stat(path); err == nil {
			return "unix://" + path, nil
		}
	}
	return "", fmt.Errorf(
		"Docker socket not found at any of: %v — is Docker running?",
		paths,
	)
}

// Ping verifies that the Docker daemon is reachable and responsive.
// It sends a lightweight ping request to the Docker API and waits
// up to defaultPingTimeout for a response.
//
// Returns a model.CLIError with ExitDockerNotRunning if the daemon
// does not respond or returns an error.
func (c *Client) Ping(ctx context.Context) error {
	// Create a child context with timeout to prevent hanging indefinitely
	// if the Docker daemon is unresponsive (e.g., Docker Desktop is paused).
	pingCtx, cancel := context.WithTimeout(ctx, defaultPingTimeout)
	// defer ensures cancel() is called even if Ping returns early due to
	// an error, preventing a context leak. This is a standard Go pattern
	// for managing context lifecycle.
	defer cancel()

	_, err := c.inner.Ping(pingCtx)
	if err != nil {
		return model.WrapCLIError(
			model.ExitDockerNotRunning,
			"Docker daemon is not responding — is Docker running?",
			err,
		)
	}
	return nil
}

// Close releases all resources held by the Docker client.
// This should be called when the client is no longer needed,
// typically via defer immediately after NewClient().
//
// Close is safe to call multiple times.
func (c *Client) Close() error {
	if c.inner != nil {
		return c.inner.Close()
	}
	return nil
}

// Inner returns the underlying Docker SDK client for advanced operations
// that are not exposed through the Client wrapper. This escape hatch
// allows other packages to access Docker API methods directly when needed,
// while still benefiting from the automatic socket detection in NewClient().
//
// Callers should prefer using Client methods over Inner() when possible.
func (c *Client) Inner() *client.Client {
	return c.inner
}
