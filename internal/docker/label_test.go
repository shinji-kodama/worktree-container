package docker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mmr-tortoise/worktree-container/internal/model"
)

// TestBuildLabels verifies that BuildLabels correctly converts a WorktreeEnv
// into a Docker label map with all required keys and values.
func TestBuildLabels(t *testing.T) {
	// Arrange: create a WorktreeEnv with known values including port allocations.
	createdAt := time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC)
	env := &model.WorktreeEnv{
		Name:           "feature-auth",
		Branch:         "feature/auth",
		WorktreePath:   "/Users/user/repo-feature-auth",
		SourceRepoPath: "/Users/user/repo",
		ConfigPattern:  model.PatternComposeMulti,
		PortAllocations: []model.PortAllocation{
			{ServiceName: "app", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
			{ServiceName: "db", ContainerPort: 5432, HostPort: 15432, Protocol: "tcp"},
		},
		CreatedAt: createdAt,
	}

	// Act
	labels := BuildLabels(env)

	// Assert: verify all static labels are present and correct.
	assert.Equal(t, ManagedByValue, labels[LabelManagedBy],
		"managed-by label should always be set to the constant value")
	assert.Equal(t, "feature-auth", labels[LabelName])
	assert.Equal(t, "feature/auth", labels[LabelBranch])
	assert.Equal(t, "/Users/user/repo-feature-auth", labels[LabelWorktreePath])
	assert.Equal(t, "/Users/user/repo", labels[LabelSourceRepo])
	assert.Equal(t, "compose-multi", labels[LabelConfigPattern])
	assert.Equal(t, "2026-02-28T10:00:00Z", labels[LabelCreatedAt])

	// Assert: verify port allocation labels.
	assert.Equal(t, "13000", labels["worktree.original-port.3000"],
		"port 3000 should be mapped to host port 13000")
	assert.Equal(t, "15432", labels["worktree.original-port.5432"],
		"port 5432 should be mapped to host port 15432")

	// Assert: verify total label count (7 static + 2 port = 9).
	assert.Len(t, labels, 9, "expected 7 static labels + 2 port labels")
}

// TestBuildLabels_NoPorts verifies that BuildLabels works correctly
// when there are no port allocations (e.g., a container with no
// forwarded ports).
func TestBuildLabels_NoPorts(t *testing.T) {
	env := &model.WorktreeEnv{
		Name:           "simple-env",
		Branch:         "main",
		WorktreePath:   "/tmp/worktree",
		SourceRepoPath: "/tmp/repo",
		ConfigPattern:  model.PatternImage,
		CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	labels := BuildLabels(env)

	// Should have only the 7 static labels, no port labels.
	assert.Len(t, labels, 7)
	assert.Equal(t, "image", labels[LabelConfigPattern])
}

// TestParseLabels verifies that ParseLabels correctly reconstructs a
// WorktreeEnv from a Docker label map. This is the inverse of BuildLabels.
func TestParseLabels(t *testing.T) {
	// Arrange: create a label map matching what BuildLabels would produce.
	labels := map[string]string{
		LabelManagedBy:                ManagedByValue,
		LabelName:                     "feature-auth",
		LabelBranch:                   "feature/auth",
		LabelWorktreePath:             "/Users/user/repo-feature-auth",
		LabelSourceRepo:               "/Users/user/repo",
		LabelConfigPattern:            "compose-multi",
		LabelCreatedAt:                "2026-02-28T10:00:00Z",
		"worktree.original-port.3000": "13000",
		"worktree.original-port.5432": "15432",
	}

	// Act
	env, err := ParseLabels(labels)

	// Assert: no error and all fields are correctly populated.
	require.NoError(t, err, "ParseLabels should succeed with valid labels")
	assert.Equal(t, "feature-auth", env.Name)
	assert.Equal(t, "feature/auth", env.Branch)
	assert.Equal(t, "/Users/user/repo-feature-auth", env.WorktreePath)
	assert.Equal(t, "/Users/user/repo", env.SourceRepoPath)
	assert.Equal(t, model.PatternComposeMulti, env.ConfigPattern)
	assert.Equal(t, time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC), env.CreatedAt)

	// Assert: port allocations were parsed correctly.
	// Note: map iteration order is not guaranteed, so we check by finding
	// the expected ports rather than relying on slice order.
	require.Len(t, env.PortAllocations, 2, "should have 2 port allocations")

	portMap := make(map[int]int) // containerPort → hostPort
	for _, pa := range env.PortAllocations {
		portMap[pa.ContainerPort] = pa.HostPort
	}
	assert.Equal(t, 13000, portMap[3000], "container port 3000 should map to host port 13000")
	assert.Equal(t, 15432, portMap[5432], "container port 5432 should map to host port 15432")
}

// TestParseLabels_NoPorts verifies that ParseLabels works when there
// are no port labels — the PortAllocations slice should be empty.
func TestParseLabels_NoPorts(t *testing.T) {
	labels := map[string]string{
		LabelManagedBy:     ManagedByValue,
		LabelName:          "simple",
		LabelBranch:        "main",
		LabelWorktreePath:  "/tmp/worktree",
		LabelSourceRepo:    "/tmp/repo",
		LabelConfigPattern: "image",
		LabelCreatedAt:     "2026-01-01T00:00:00Z",
	}

	env, err := ParseLabels(labels)
	require.NoError(t, err)
	assert.Empty(t, env.PortAllocations, "should have no port allocations")
}

// TestParseLabels_MissingRequired verifies that ParseLabels returns an
// error when required labels are missing from the label map.
func TestParseLabels_MissingRequired(t *testing.T) {
	// Sub-test table: each test case removes one required label to verify
	// that its absence is detected.
	testCases := []struct {
		name       string
		missingKey string
	}{
		{"missing managed-by", LabelManagedBy},
		{"missing name", LabelName},
		{"missing branch", LabelBranch},
		{"missing worktree-path", LabelWorktreePath},
		{"missing source-repo", LabelSourceRepo},
		{"missing config-pattern", LabelConfigPattern},
		{"missing created-at", LabelCreatedAt},
	}

	for _, tc := range testCases {
		// t.Run creates a named sub-test, which makes test output clearer
		// and allows running individual sub-tests with -run flag.
		t.Run(tc.name, func(t *testing.T) {
			// Start with a complete valid label set.
			labels := map[string]string{
				LabelManagedBy:     ManagedByValue,
				LabelName:          "test",
				LabelBranch:        "main",
				LabelWorktreePath:  "/tmp/wt",
				LabelSourceRepo:    "/tmp/repo",
				LabelConfigPattern: "image",
				LabelCreatedAt:     "2026-01-01T00:00:00Z",
			}

			// Remove the label under test.
			delete(labels, tc.missingKey)

			_, err := ParseLabels(labels)
			require.Error(t, err, "should fail when %s is missing", tc.missingKey)
			assert.Contains(t, err.Error(), tc.missingKey,
				"error message should mention the missing label key")
		})
	}
}

// TestParseLabels_InvalidManagedBy verifies that ParseLabels rejects
// containers with an unexpected managed-by value.
func TestParseLabels_InvalidManagedBy(t *testing.T) {
	labels := map[string]string{
		LabelManagedBy:     "some-other-tool",
		LabelName:          "test",
		LabelBranch:        "main",
		LabelWorktreePath:  "/tmp/wt",
		LabelSourceRepo:    "/tmp/repo",
		LabelConfigPattern: "image",
		LabelCreatedAt:     "2026-01-01T00:00:00Z",
	}

	_, err := ParseLabels(labels)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected value")
}

// TestParseLabels_InvalidConfigPattern verifies that ParseLabels returns
// an error when the config-pattern label has an invalid value.
func TestParseLabels_InvalidConfigPattern(t *testing.T) {
	labels := map[string]string{
		LabelManagedBy:     ManagedByValue,
		LabelName:          "test",
		LabelBranch:        "main",
		LabelWorktreePath:  "/tmp/wt",
		LabelSourceRepo:    "/tmp/repo",
		LabelConfigPattern: "invalid-pattern",
		LabelCreatedAt:     "2026-01-01T00:00:00Z",
	}

	_, err := ParseLabels(labels)
	require.Error(t, err)
	assert.Contains(t, err.Error(), LabelConfigPattern)
}

// TestParseLabels_InvalidCreatedAt verifies that ParseLabels returns
// an error when the created-at label has an unparseable timestamp.
func TestParseLabels_InvalidCreatedAt(t *testing.T) {
	labels := map[string]string{
		LabelManagedBy:     ManagedByValue,
		LabelName:          "test",
		LabelBranch:        "main",
		LabelWorktreePath:  "/tmp/wt",
		LabelSourceRepo:    "/tmp/repo",
		LabelConfigPattern: "image",
		LabelCreatedAt:     "not-a-timestamp",
	}

	_, err := ParseLabels(labels)
	require.Error(t, err)
	assert.Contains(t, err.Error(), LabelCreatedAt)
}

// TestBuildPortLabel verifies that BuildPortLabel generates the correct
// label key format for various port numbers.
func TestBuildPortLabel(t *testing.T) {
	testCases := []struct {
		containerPort int
		expected      string
	}{
		{3000, "worktree.original-port.3000"},
		{5432, "worktree.original-port.5432"},
		{80, "worktree.original-port.80"},
		{443, "worktree.original-port.443"},
		{8080, "worktree.original-port.8080"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := BuildPortLabel(tc.containerPort)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestParsePortLabels verifies that ParsePortLabels correctly extracts
// port allocations from a label map containing mixed labels (both port
// and non-port labels).
func TestParsePortLabels(t *testing.T) {
	labels := map[string]string{
		// Non-port labels should be ignored.
		LabelManagedBy: ManagedByValue,
		LabelName:      "test",
		// Port labels to be parsed.
		"worktree.original-port.3000": "13000",
		"worktree.original-port.5432": "15432",
		"worktree.original-port.8080": "18080",
	}

	allocations, err := ParsePortLabels(labels)
	require.NoError(t, err)
	assert.Len(t, allocations, 3, "should extract exactly 3 port allocations")

	// Build a map for order-independent assertion.
	portMap := make(map[int]int)
	for _, pa := range allocations {
		portMap[pa.ContainerPort] = pa.HostPort
		// Verify default protocol is set.
		assert.Equal(t, "tcp", pa.Protocol,
			"protocol should default to tcp")
	}

	assert.Equal(t, 13000, portMap[3000])
	assert.Equal(t, 15432, portMap[5432])
	assert.Equal(t, 18080, portMap[8080])
}

// TestParsePortLabels_Empty verifies that ParsePortLabels returns an
// empty slice when no port labels are present.
func TestParsePortLabels_Empty(t *testing.T) {
	labels := map[string]string{
		LabelManagedBy: ManagedByValue,
		LabelName:      "test",
	}

	allocations, err := ParsePortLabels(labels)
	require.NoError(t, err)
	assert.Empty(t, allocations, "should return empty slice when no port labels exist")
}

// TestParsePortLabels_InvalidFormat verifies that ParsePortLabels returns
// errors for malformed port labels (non-numeric port numbers or values).
func TestParsePortLabels_InvalidFormat(t *testing.T) {
	t.Run("non-numeric container port", func(t *testing.T) {
		labels := map[string]string{
			"worktree.original-port.abc": "13000",
		}

		_, err := ParsePortLabels(labels)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid container port",
			"error should describe the issue with the container port")
	})

	t.Run("non-numeric host port", func(t *testing.T) {
		labels := map[string]string{
			"worktree.original-port.3000": "not-a-port",
		}

		_, err := ParsePortLabels(labels)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid host port",
			"error should describe the issue with the host port value")
	})
}

// TestFilterLabels verifies that FilterLabels returns the correct
// filter map for listing managed containers.
func TestFilterLabels(t *testing.T) {
	filters := FilterLabels()

	// The filter should contain exactly one entry: the managed-by label.
	require.Len(t, filters, 1, "filter should contain exactly one label")
	assert.Equal(t, ManagedByValue, filters[LabelManagedBy],
		"filter should match the managed-by label value")
}

// TestBuildAndParseLabelRoundTrip verifies that building labels from a
// WorktreeEnv and parsing them back produces an equivalent WorktreeEnv.
// This is a critical integration-style test that ensures the two functions
// are inverse operations.
func TestBuildAndParseLabelRoundTrip(t *testing.T) {
	createdAt := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	original := &model.WorktreeEnv{
		Name:           "roundtrip-test",
		Branch:         "feature/roundtrip",
		WorktreePath:   "/home/user/projects/my-app-roundtrip",
		SourceRepoPath: "/home/user/projects/my-app",
		ConfigPattern:  model.PatternDockerfile,
		PortAllocations: []model.PortAllocation{
			{ServiceName: "web", ContainerPort: 3000, HostPort: 13000, Protocol: "tcp"},
		},
		CreatedAt: createdAt,
	}

	// Build labels, then parse them back.
	labels := BuildLabels(original)
	parsed, err := ParseLabels(labels)
	require.NoError(t, err)

	// Compare the fields that are preserved through labels.
	// Note: Status and Containers are NOT persisted in labels, so they
	// are excluded from comparison.
	assert.Equal(t, original.Name, parsed.Name)
	assert.Equal(t, original.Branch, parsed.Branch)
	assert.Equal(t, original.WorktreePath, parsed.WorktreePath)
	assert.Equal(t, original.SourceRepoPath, parsed.SourceRepoPath)
	assert.Equal(t, original.ConfigPattern, parsed.ConfigPattern)
	assert.Equal(t, original.CreatedAt.UTC(), parsed.CreatedAt.UTC())

	// Port allocations: compare using maps since order may differ.
	require.Len(t, parsed.PortAllocations, len(original.PortAllocations))
	for _, origPA := range original.PortAllocations {
		found := false
		for _, parsedPA := range parsed.PortAllocations {
			if parsedPA.ContainerPort == origPA.ContainerPort {
				assert.Equal(t, origPA.HostPort, parsedPA.HostPort)
				found = true
				break
			}
		}
		assert.True(t, found, "port allocation for container port %d should be preserved", origPA.ContainerPort)
	}
}
