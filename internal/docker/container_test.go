package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shinji-kodama/worktree-container/internal/model"
)

// makeTestContainer is a helper that creates a model.ContainerInfo with
// worktree management labels. This avoids repetitive label construction
// across multiple test cases.
//
// Parameters:
//   - id: Docker container ID (shortened hash)
//   - name: Docker container name
//   - service: Docker Compose service name (can be empty for Pattern A/B)
//   - status: container state (e.g., "running", "exited")
//   - envName: the worktree environment name (worktree.name label)
//   - worktreePath: filesystem path to the worktree directory
//
// Returns a ContainerInfo populated with all required worktree labels.
func makeTestContainer(id, name, service, status, envName, worktreePath string) model.ContainerInfo {
	return model.ContainerInfo{
		ContainerID:   id,
		ContainerName: name,
		ServiceName:   service,
		Status:        status,
		Labels: map[string]string{
			LabelManagedBy:     ManagedByValue,
			LabelName:          envName,
			LabelBranch:        "feature/test",
			LabelWorktreePath:  worktreePath,
			LabelSourceRepo:    "/tmp",
			LabelConfigPattern: "image",
			LabelCreatedAt:     "2026-02-28T10:00:00Z",
		},
	}
}

// TestGroupContainersByEnv verifies that GroupContainersByEnv correctly
// groups 3 containers into 2 separate environments based on their
// "worktree.name" label values.
//
// This test uses containers from two environments ("env-alpha" and "env-beta")
// to verify that the grouping logic correctly separates them.
func TestGroupContainersByEnv(t *testing.T) {
	// Arrange: create 3 containers across 2 environments.
	// env-alpha has 2 containers (app and db), env-beta has 1 container.
	containers := []model.ContainerInfo{
		makeTestContainer("aaa111", "alpha-app-1", "app", "running", "env-alpha", "/tmp"),
		makeTestContainer("bbb222", "alpha-db-1", "db", "running", "env-alpha", "/tmp"),
		makeTestContainer("ccc333", "beta-app-1", "app", "running", "env-beta", "/tmp"),
	}

	// Act: group containers by environment name.
	groups := GroupContainersByEnv(containers)

	// Assert: there should be 2 groups.
	require.Len(t, groups, 2, "should have 2 environment groups")

	// Assert: env-alpha should have 2 containers.
	alphaGroup, ok := groups["env-alpha"]
	require.True(t, ok, "env-alpha group should exist")
	assert.Len(t, alphaGroup, 2, "env-alpha should have 2 containers")

	// Assert: env-beta should have 1 container.
	betaGroup, ok := groups["env-beta"]
	require.True(t, ok, "env-beta group should exist")
	assert.Len(t, betaGroup, 1, "env-beta should have 1 container")

	// Verify the correct containers are in each group by checking IDs.
	alphaIDs := make(map[string]bool)
	for _, c := range alphaGroup {
		alphaIDs[c.ContainerID] = true
	}
	assert.True(t, alphaIDs["aaa111"], "env-alpha should contain container aaa111")
	assert.True(t, alphaIDs["bbb222"], "env-alpha should contain container bbb222")

	assert.Equal(t, "ccc333", betaGroup[0].ContainerID,
		"env-beta should contain container ccc333")
}

// TestGroupContainersByEnv_Empty verifies that GroupContainersByEnv
// returns an empty map when given an empty input slice.
// This is a boundary condition that must be handled gracefully.
func TestGroupContainersByEnv_Empty(t *testing.T) {
	// Act: group an empty slice.
	groups := GroupContainersByEnv([]model.ContainerInfo{})

	// Assert: the result should be an empty (but non-nil) map.
	require.NotNil(t, groups, "result should be a non-nil map")
	assert.Empty(t, groups, "result should have no groups")
}

// TestBuildWorktreeEnv_Running verifies that BuildWorktreeEnv correctly
// sets the status to "running" when at least one container in the
// environment has a "running" state.
//
// The test uses /tmp as the worktree path because it always exists
// on Unix systems, which prevents the orphan detection from triggering.
func TestBuildWorktreeEnv_Running(t *testing.T) {
	// Arrange: create containers where one is running and one is exited.
	// The environment should be considered "running" because at least
	// one container is active.
	containers := []model.ContainerInfo{
		{
			ContainerID:   "abc123",
			ContainerName: "test-app-1",
			ServiceName:   "app",
			Status:        "running",
			Labels: map[string]string{
				LabelManagedBy:                ManagedByValue,
				LabelName:                     "test-env",
				LabelBranch:                   "feature/test",
				LabelWorktreePath:             "/tmp",
				LabelSourceRepo:               "/tmp",
				LabelConfigPattern:            "image",
				LabelCreatedAt:                "2026-02-28T10:00:00Z",
				"worktree.original-port.3000": "13000",
			},
		},
		{
			ContainerID:   "def456",
			ContainerName: "test-db-1",
			ServiceName:   "db",
			Status:        "exited",
			Labels: map[string]string{
				LabelManagedBy:                ManagedByValue,
				LabelName:                     "test-env",
				LabelBranch:                   "feature/test",
				LabelWorktreePath:             "/tmp",
				LabelSourceRepo:               "/tmp",
				LabelConfigPattern:            "image",
				LabelCreatedAt:                "2026-02-28T10:00:00Z",
				"worktree.original-port.5432": "15432",
			},
		},
	}

	// Act: build the WorktreeEnv from the containers.
	env, err := BuildWorktreeEnv("test-env", containers)

	// Assert: no error and status is running.
	require.NoError(t, err, "BuildWorktreeEnv should succeed with valid containers")
	assert.Equal(t, model.StatusRunning, env.Status,
		"status should be 'running' when at least one container is running")

	// Assert: basic fields are populated correctly from labels.
	assert.Equal(t, "test-env", env.Name)
	assert.Equal(t, "feature/test", env.Branch)
	assert.Equal(t, "/tmp", env.WorktreePath)

	// Assert: all containers are attached to the environment.
	assert.Len(t, env.Containers, 2, "should have 2 containers attached")

	// Assert: port allocations were parsed from labels.
	assert.NotEmpty(t, env.PortAllocations, "should have port allocations from labels")
}

// TestBuildWorktreeEnv_Stopped verifies that BuildWorktreeEnv correctly
// sets the status to "stopped" when all containers in the environment
// are in non-running states (e.g., "exited", "created").
func TestBuildWorktreeEnv_Stopped(t *testing.T) {
	// Arrange: create containers that are all stopped/exited.
	containers := []model.ContainerInfo{
		{
			ContainerID:   "abc123",
			ContainerName: "test-app-1",
			ServiceName:   "app",
			Status:        "exited",
			Labels: map[string]string{
				LabelManagedBy:     ManagedByValue,
				LabelName:          "test-env",
				LabelBranch:        "feature/test",
				LabelWorktreePath:  "/tmp",
				LabelSourceRepo:    "/tmp",
				LabelConfigPattern: "image",
				LabelCreatedAt:     "2026-02-28T10:00:00Z",
			},
		},
		{
			ContainerID:   "def456",
			ContainerName: "test-db-1",
			ServiceName:   "db",
			Status:        "exited",
			Labels: map[string]string{
				LabelManagedBy:     ManagedByValue,
				LabelName:          "test-env",
				LabelBranch:        "feature/test",
				LabelWorktreePath:  "/tmp",
				LabelSourceRepo:    "/tmp",
				LabelConfigPattern: "image",
				LabelCreatedAt:     "2026-02-28T10:00:00Z",
			},
		},
	}

	// Act
	env, err := BuildWorktreeEnv("test-env", containers)

	// Assert: status should be "stopped" since no container is running
	// and the worktree path (/tmp) exists.
	require.NoError(t, err)
	assert.Equal(t, model.StatusStopped, env.Status,
		"status should be 'stopped' when all containers are stopped and worktree path exists")
}

// TestBuildWorktreeEnv_Orphaned verifies that BuildWorktreeEnv correctly
// sets the status to "orphaned" when the worktree path no longer exists
// on disk. This simulates the scenario where a user manually deletes
// their Git worktree directory without cleaning up the Docker containers.
func TestBuildWorktreeEnv_Orphaned(t *testing.T) {
	// Arrange: use a non-existent path as the worktree path.
	// This simulates a worktree that was deleted from disk.
	nonExistentPath := "/tmp/worktree-container-test-nonexistent-path-12345"

	containers := []model.ContainerInfo{
		{
			ContainerID:   "abc123",
			ContainerName: "test-app-1",
			ServiceName:   "app",
			Status:        "running",
			Labels: map[string]string{
				LabelManagedBy:     ManagedByValue,
				LabelName:          "orphan-env",
				LabelBranch:        "feature/orphan",
				LabelWorktreePath:  nonExistentPath,
				LabelSourceRepo:    "/tmp",
				LabelConfigPattern: "image",
				LabelCreatedAt:     "2026-02-28T10:00:00Z",
			},
		},
	}

	// Act
	env, err := BuildWorktreeEnv("orphan-env", containers)

	// Assert: status should be "orphaned" because the worktree path
	// does not exist, even though the container is running.
	// Orphan detection takes priority over running status.
	require.NoError(t, err)
	assert.Equal(t, model.StatusOrphaned, env.Status,
		"status should be 'orphaned' when worktree path does not exist on disk")
	assert.Equal(t, nonExistentPath, env.WorktreePath,
		"worktree path should still be preserved from labels")
}

// TestBuildWorktreeEnv_NoContainers verifies that BuildWorktreeEnv returns
// an error when called with an empty container slice. This is a programming
// error guard — every environment must have at least one container.
func TestBuildWorktreeEnv_NoContainers(t *testing.T) {
	// Act: try to build an env from an empty slice.
	env, err := BuildWorktreeEnv("empty-env", []model.ContainerInfo{})

	// Assert: should return an error and a nil env.
	require.Error(t, err, "should fail when no containers are provided")
	assert.Nil(t, env, "returned env should be nil on error")
	assert.Contains(t, err.Error(), "no containers provided",
		"error message should explain the problem")
}

// TestGroupContainersByEnv_SkipsNoLabel verifies that containers
// without the "worktree.name" label are silently excluded from grouping.
// This is a defensive behavior — in practice all managed containers
// should have this label.
func TestGroupContainersByEnv_SkipsNoLabel(t *testing.T) {
	// Arrange: one container with a valid label, one without.
	containers := []model.ContainerInfo{
		makeTestContainer("aaa111", "valid-app", "app", "running", "valid-env", "/tmp"),
		{
			// Container with no worktree.name label.
			ContainerID:   "bbb222",
			ContainerName: "no-label-container",
			Status:        "running",
			Labels:        map[string]string{},
		},
	}

	// Act
	groups := GroupContainersByEnv(containers)

	// Assert: only the container with a valid label should be grouped.
	require.Len(t, groups, 1, "should have 1 group, skipping unlabeled container")
	assert.Len(t, groups["valid-env"], 1, "valid-env should have 1 container")
}

// TestDetermineStatus_Running verifies the internal determineStatus function
// returns "running" when at least one container is in the "running" state
// and the worktree path exists.
func TestDetermineStatus_Running(t *testing.T) {
	containers := []model.ContainerInfo{
		{Status: "running"},
		{Status: "exited"},
	}

	status := determineStatus(containers, "/tmp")
	assert.Equal(t, model.StatusRunning, status,
		"should be running when at least one container is running")
}

// TestDetermineStatus_Stopped verifies the internal determineStatus function
// returns "stopped" when no containers are running and the worktree path exists.
func TestDetermineStatus_Stopped(t *testing.T) {
	containers := []model.ContainerInfo{
		{Status: "exited"},
		{Status: "created"},
	}

	status := determineStatus(containers, "/tmp")
	assert.Equal(t, model.StatusStopped, status,
		"should be stopped when no containers are running")
}

// TestDetermineStatus_Orphaned verifies the internal determineStatus function
// returns "orphaned" when the worktree path does not exist on disk,
// regardless of container states.
func TestDetermineStatus_Orphaned(t *testing.T) {
	containers := []model.ContainerInfo{
		{Status: "running"},
	}

	// Use a path that definitely does not exist.
	status := determineStatus(containers, "/tmp/worktree-container-nonexistent-path-99999")
	assert.Equal(t, model.StatusOrphaned, status,
		"should be orphaned when worktree path does not exist, even if containers are running")
}
