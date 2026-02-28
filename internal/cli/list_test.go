// Package cli — list_test.go contains unit tests for the pure formatting
// functions used by the list command and other CLI output helpers.
//
// These tests verify data transformation logic without requiring a Docker
// daemon or any external dependencies.
package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shinji-kodama/worktree-container/internal/model"
)

// TestFormatPortsList verifies that FormatPortsList correctly converts
// a slice of PortAllocations into a comma-separated string of host ports.
func TestFormatPortsList(t *testing.T) {
	tests := []struct {
		name        string
		allocations []model.PortAllocation
		want        string
	}{
		{
			name:        "empty allocations returns dash",
			allocations: []model.PortAllocation{},
			want:        "-",
		},
		{
			name:        "nil allocations returns dash",
			allocations: nil,
			want:        "-",
		},
		{
			name: "single port",
			allocations: []model.PortAllocation{
				{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
			},
			want: "13000",
		},
		{
			name: "multiple ports sorted",
			allocations: []model.PortAllocation{
				{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
				{ServiceName: "db", ContainerPort: 5432, HostPort: 15432, Protocol: "tcp"},
				{ServiceName: "cache", ContainerPort: 6379, HostPort: 16379, Protocol: "tcp"},
			},
			want: "13000,15432,16379",
		},
		{
			name: "ports are sorted numerically by string representation",
			allocations: []model.PortAllocation{
				{ServiceName: "cache", ContainerPort: 6379, HostPort: 16379, Protocol: "tcp"},
				{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
				{ServiceName: "db", ContainerPort: 5432, HostPort: 15432, Protocol: "tcp"},
			},
			// Sorted as strings: "13000", "15432", "16379"
			want: "13000,15432,16379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPortsList(tt.allocations)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSanitizeBranchName verifies that sanitizeBranchName correctly converts
// Git branch names to valid environment names. The function is defined in
// create.go but tested here as it is a shared utility across the CLI.
func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   string
	}{
		{
			name:   "simple branch name",
			branch: "feature-auth",
			want:   "feature-auth",
		},
		{
			name:   "branch with slashes",
			branch: "feature/auth",
			want:   "feature-auth",
		},
		{
			name:   "branch with underscores",
			branch: "feature_auth",
			want:   "feature-auth",
		},
		{
			name:   "branch with mixed separators",
			branch: "feature/auth_login",
			want:   "feature-auth-login",
		},
		{
			name:   "branch with special characters",
			branch: "feature/auth@v2.0",
			want:   "feature-authv20",
		},
		{
			name:   "branch with leading slash",
			branch: "/feature-auth",
			want:   "feature-auth",
		},
		{
			name:   "branch with trailing slash",
			branch: "feature-auth/",
			want:   "feature-auth",
		},
		{
			name:   "empty branch becomes worktree",
			branch: "",
			want:   "worktree",
		},
		{
			name:   "all special characters becomes worktree",
			branch: "///",
			want:   "worktree",
		},
		{
			name:   "single character branch",
			branch: "a",
			want:   "a",
		},
		{
			name:   "numeric branch",
			branch: "123",
			want:   "123",
		},
		{
			name:   "deeply nested branch",
			branch: "org/team/feature/auth",
			want:   "org-team-feature-auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeBranchName(tt.branch)
			assert.Equal(t, tt.want, got)
		})
	}
}
