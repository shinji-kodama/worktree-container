package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorktreeStatus_String verifies that WorktreeStatus values produce
// the expected string representations for CLI output and JSON serialization.
func TestWorktreeStatus_String(t *testing.T) {
	tests := []struct {
		status   WorktreeStatus
		expected string
	}{
		{StatusRunning, "running"},
		{StatusStopped, "stopped"},
		{StatusOrphaned, "orphaned"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

// TestWorktreeStatus_IsValid checks that only defined status values pass validation.
func TestWorktreeStatus_IsValid(t *testing.T) {
	assert.True(t, StatusRunning.IsValid())
	assert.True(t, StatusStopped.IsValid())
	assert.True(t, StatusOrphaned.IsValid())
	assert.False(t, WorktreeStatus("invalid").IsValid())
	assert.False(t, WorktreeStatus("").IsValid())
}

// TestParseWorktreeStatus verifies string-to-status conversion,
// including case normalization and error cases.
func TestParseWorktreeStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected WorktreeStatus
		hasError bool
	}{
		{"running", StatusRunning, false},
		{"stopped", StatusStopped, false},
		{"orphaned", StatusOrphaned, false},
		{"Running", StatusRunning, false}, // case insensitive
		{"STOPPED", StatusStopped, false}, // case insensitive
		{"invalid", "", true},             // unknown value
		{"", "", true},                    // empty string
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWorktreeStatus(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestConfigPattern_String verifies string representation of all config patterns.
func TestConfigPattern_String(t *testing.T) {
	tests := []struct {
		pattern  ConfigPattern
		expected string
	}{
		{PatternImage, "image"},
		{PatternDockerfile, "dockerfile"},
		{PatternComposeSingle, "compose-single"},
		{PatternComposeMulti, "compose-multi"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.pattern.String())
		})
	}
}

// TestConfigPattern_IsValid checks that only defined patterns pass validation.
func TestConfigPattern_IsValid(t *testing.T) {
	assert.True(t, PatternImage.IsValid())
	assert.True(t, PatternDockerfile.IsValid())
	assert.True(t, PatternComposeSingle.IsValid())
	assert.True(t, PatternComposeMulti.IsValid())
	assert.False(t, ConfigPattern("invalid").IsValid())
}

// TestConfigPattern_IsCompose verifies that only Compose-based patterns
// return true, which controls whether override YAML generation is needed.
func TestConfigPattern_IsCompose(t *testing.T) {
	assert.False(t, PatternImage.IsCompose())
	assert.False(t, PatternDockerfile.IsCompose())
	assert.True(t, PatternComposeSingle.IsCompose())
	assert.True(t, PatternComposeMulti.IsCompose())
}

// TestParseConfigPattern verifies string-to-pattern conversion.
func TestParseConfigPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected ConfigPattern
		hasError bool
	}{
		{"image", PatternImage, false},
		{"dockerfile", PatternDockerfile, false},
		{"compose-single", PatternComposeSingle, false},
		{"compose-multi", PatternComposeMulti, false},
		{"IMAGE", PatternImage, false}, // case insensitive
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseConfigPattern(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestValidateName checks environment name validation rules:
// - Must not be empty
// - Alphanumeric + hyphens only
// - Must start and end with alphanumeric
func TestValidateName(t *testing.T) {
	tests := []struct {
		name     string
		hasError bool
	}{
		{"feature-auth", false},    // valid: alphanumeric with hyphen
		{"a", false},               // valid: single character
		{"feature-auth-v2", false}, // valid: multiple hyphens
		{"abc123", false},          // valid: alphanumeric
		{"", true},                 // invalid: empty
		{"-feature", true},         // invalid: starts with hyphen
		{"feature-", true},         // invalid: ends with hyphen
		{"feature auth", true},     // invalid: space
		{"feature_auth", true},     // invalid: underscore
		{"feature.auth", true},     // invalid: dot
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.name)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestPortAllocation_Validate checks individual port allocation validation:
// - ContainerPort range: 1-65535
// - HostPort range: 1024-65535
// - Protocol must be tcp or udp
// - ServiceName must not be empty
func TestPortAllocation_Validate(t *testing.T) {
	tests := []struct {
		name     string
		alloc    PortAllocation
		hasError bool
	}{
		{
			name:     "valid tcp allocation",
			alloc:    PortAllocation{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
			hasError: false,
		},
		{
			name:     "valid udp allocation",
			alloc:    PortAllocation{ServiceName: "app", ContainerPort: 53, HostPort: 10053, Protocol: "udp"},
			hasError: false,
		},
		{
			name:     "defaults empty protocol to tcp",
			alloc:    PortAllocation{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: ""},
			hasError: false,
		},
		{
			name:     "empty service name",
			alloc:    PortAllocation{ServiceName: "", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
			hasError: true,
		},
		{
			name:     "container port too low",
			alloc:    PortAllocation{ServiceName: "app", ContainerPort: 0, HostPort: 13000, Protocol: "tcp"},
			hasError: true,
		},
		{
			name:     "container port too high",
			alloc:    PortAllocation{ServiceName: "app", ContainerPort: 70000, HostPort: 13000, Protocol: "tcp"},
			hasError: true,
		},
		{
			name:     "host port below 1024",
			alloc:    PortAllocation{ServiceName: "app", ContainerPort: 80, HostPort: 80, Protocol: "tcp"},
			hasError: true,
		},
		{
			name:     "host port too high",
			alloc:    PortAllocation{ServiceName: "app", ContainerPort: 3000, HostPort: 70000, Protocol: "tcp"},
			hasError: true,
		},
		{
			name:     "invalid protocol",
			alloc:    PortAllocation{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "sctp"},
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.alloc.Validate()
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestPortAllocation_String verifies the human-readable output format
// used in CLI table displays.
func TestPortAllocation_String(t *testing.T) {
	alloc := PortAllocation{
		ServiceName:   "app",
		ContainerPort: 3000,
		HostPort:      13000,
		Protocol:      "tcp",
	}
	assert.Equal(t, "app:3000 → 13000/tcp", alloc.String())
}

// TestValidatePortAllocations checks cross-allocation validation:
// - Duplicate host port detection within the same protocol
// - Different protocols on the same port are allowed
func TestValidatePortAllocations(t *testing.T) {
	t.Run("valid unique allocations", func(t *testing.T) {
		allocs := []PortAllocation{
			{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
			{ServiceName: "db", ContainerPort: 5432, HostPort: 15432, Protocol: "tcp"},
			{ServiceName: "redis", ContainerPort: 6379, HostPort: 16379, Protocol: "tcp"},
		}
		assert.NoError(t, ValidatePortAllocations(allocs))
	})

	t.Run("duplicate host port same protocol", func(t *testing.T) {
		allocs := []PortAllocation{
			{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
			{ServiceName: "web", ContainerPort: 8080, HostPort: 13000, Protocol: "tcp"},
		}
		err := ValidatePortAllocations(allocs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "13000/tcp")
	})

	t.Run("same port different protocols allowed", func(t *testing.T) {
		allocs := []PortAllocation{
			{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
			{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "udp"},
		}
		assert.NoError(t, ValidatePortAllocations(allocs))
	})

	t.Run("empty allocations valid", func(t *testing.T) {
		assert.NoError(t, ValidatePortAllocations([]PortAllocation{}))
	})

	t.Run("individual validation also checked", func(t *testing.T) {
		allocs := []PortAllocation{
			{ServiceName: "", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
		}
		assert.Error(t, ValidatePortAllocations(allocs))
	})
}

// TestCLIError verifies the custom error type used for exit code mapping.
func TestCLIError(t *testing.T) {
	t.Run("simple error", func(t *testing.T) {
		err := NewCLIError(ExitDockerNotRunning, "Docker daemon is not running")
		assert.Equal(t, ExitDockerNotRunning, err.Code)
		assert.Equal(t, "Docker daemon is not running", err.Error())
		assert.Nil(t, err.Unwrap())
	})

	t.Run("wrapped error", func(t *testing.T) {
		inner := errors.New("connection refused")
		err := WrapCLIError(ExitDockerNotRunning, "Docker daemon is not running", inner)
		assert.Equal(t, ExitDockerNotRunning, err.Code)
		assert.Contains(t, err.Error(), "connection refused")
		assert.Equal(t, inner, err.Unwrap())
	})

	// Verify errors.Is works with unwrapped errors (Go 1.13+ error chain).
	t.Run("errors.Is chain", func(t *testing.T) {
		inner := errors.New("connection refused")
		err := WrapCLIError(ExitDockerNotRunning, "Docker daemon is not running", inner)
		assert.True(t, errors.Is(err, inner))
	})
}
