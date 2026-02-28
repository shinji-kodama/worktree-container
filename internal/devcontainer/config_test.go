package devcontainer

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/shinji-kodama/worktree-container/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectRoot returns the absolute path to the project root directory.
// It uses runtime.Caller to locate the source file of this test, then
// navigates up from internal/devcontainer/ to the project root.
// This approach is more robust than os.Getwd() because it doesn't depend
// on which directory the test runner is invoked from.
func projectRoot(t *testing.T) string {
	t.Helper()

	// runtime.Caller(0) returns the file path of the current source file.
	// The second return value is the absolute path to this test file.
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed to return file info")

	// Navigate from internal/devcontainer/config_test.go up two directories
	// to reach the project root.
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	return root
}

// testdataPath returns the absolute path to a specific testdata fixture directory.
// Each fixture directory (e.g., "image-simple") contains a .devcontainer/
// subdirectory with a devcontainer.json file.
func testdataPath(t *testing.T, fixture string) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "tests", "testdata", fixture)
}

// --- LoadConfig tests ---

// TestLoadConfig_ImageSimple verifies that a Pattern A (image-based) devcontainer.json
// is correctly parsed, including JSONC comment stripping and all relevant fields.
func TestLoadConfig_ImageSimple(t *testing.T) {
	path := filepath.Join(testdataPath(t, "image-simple"), ".devcontainer", "devcontainer.json")

	raw, err := LoadConfig(path)
	require.NoError(t, err, "LoadConfig should succeed for a valid devcontainer.json")

	// Verify basic fields.
	assert.Equal(t, "simple-node-app", raw.Name)
	assert.Equal(t, "mcr.microsoft.com/devcontainers/typescript-node:20", raw.Image)

	// Build should be nil for image-based patterns.
	assert.Nil(t, raw.Build, "Build should be nil for image pattern")

	// Compose fields should be empty/nil.
	assert.Nil(t, raw.DockerComposeFile, "DockerComposeFile should be nil for image pattern")
	assert.Empty(t, raw.Service)

	// Verify forwardPorts contains the expected entries.
	// JSON numbers are decoded as float64 when the target type is interface{}.
	require.Len(t, raw.ForwardPorts, 2)
	assert.Equal(t, float64(3000), raw.ForwardPorts[0])
	assert.Equal(t, float64(8080), raw.ForwardPorts[1])

	// Verify appPort is parsed (it's an array in this fixture).
	require.NotNil(t, raw.AppPort)

	// Verify portsAttributes.
	require.Len(t, raw.PortsAttributes, 2)
	assert.Equal(t, "Application", raw.PortsAttributes["3000"].Label)
	assert.Equal(t, "notify", raw.PortsAttributes["3000"].OnAutoForward)
	assert.Equal(t, "API Server", raw.PortsAttributes["8080"].Label)

	// Verify containerEnv.
	require.Len(t, raw.ContainerEnv, 1)
	assert.Equal(t, "development", raw.ContainerEnv["NODE_ENV"])

	// Verify runArgs.
	require.Len(t, raw.RunArgs, 3)
	assert.Equal(t, "--cap-add=SYS_PTRACE", raw.RunArgs[0])
}

// TestLoadConfig_DockerfileBuild verifies that a Pattern B (Dockerfile build)
// devcontainer.json is correctly parsed, including the nested build config.
func TestLoadConfig_DockerfileBuild(t *testing.T) {
	path := filepath.Join(testdataPath(t, "dockerfile-build"), ".devcontainer", "devcontainer.json")

	raw, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "custom-build-app", raw.Name)

	// Image should be empty for Dockerfile build patterns.
	assert.Empty(t, raw.Image, "Image should be empty for dockerfile pattern")

	// Verify the build configuration is fully parsed.
	require.NotNil(t, raw.Build, "Build must be present for dockerfile pattern")
	assert.Equal(t, "Dockerfile", raw.Build.Dockerfile)
	assert.Equal(t, "..", raw.Build.Context)
	require.Len(t, raw.Build.Args, 1)
	assert.Equal(t, "20", raw.Build.Args["NODE_VERSION"])

	// Verify forwardPorts.
	require.Len(t, raw.ForwardPorts, 2)
	assert.Equal(t, float64(3000), raw.ForwardPorts[0])
	assert.Equal(t, float64(5432), raw.ForwardPorts[1])

	// Verify portsAttributes.
	assert.Equal(t, "Web App", raw.PortsAttributes["3000"].Label)
	assert.Equal(t, "PostgreSQL", raw.PortsAttributes["5432"].Label)

	// Verify containerEnv.
	assert.Equal(t, "postgresql://localhost:5432/devdb", raw.ContainerEnv["DATABASE_URL"])
}

// TestLoadConfig_ComposeSingle verifies that a Pattern C (Compose single-service)
// devcontainer.json is correctly parsed.
func TestLoadConfig_ComposeSingle(t *testing.T) {
	path := filepath.Join(testdataPath(t, "compose-single"), ".devcontainer", "devcontainer.json")

	raw, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "compose-single-app", raw.Name)

	// DockerComposeFile should be a string (single file).
	assert.Equal(t, "docker-compose.yml", raw.DockerComposeFile)

	// Verify Compose-specific fields.
	assert.Equal(t, "app", raw.Service)
	assert.Equal(t, "/workspace", raw.WorkspaceFolder)
	assert.Equal(t, "stopCompose", raw.ShutdownAction)

	// RunServices should be nil/empty for single-service patterns
	// (the fixture doesn't specify runServices).
	assert.Empty(t, raw.RunServices)

	// Verify forwardPorts.
	require.Len(t, raw.ForwardPorts, 1)
	assert.Equal(t, float64(3000), raw.ForwardPorts[0])
}

// TestLoadConfig_ComposeMulti verifies that a Pattern D (Compose multi-service)
// devcontainer.json is correctly parsed, including array-form dockerComposeFile
// and "service:port" strings in forwardPorts.
func TestLoadConfig_ComposeMulti(t *testing.T) {
	path := filepath.Join(testdataPath(t, "compose-multi"), ".devcontainer", "devcontainer.json")

	raw, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "compose-multi-app", raw.Name)

	// DockerComposeFile should be an array ([]interface{} from JSON).
	require.NotNil(t, raw.DockerComposeFile)

	// Verify Compose fields.
	assert.Equal(t, "app", raw.Service)
	assert.Equal(t, "/workspace", raw.WorkspaceFolder)

	// Verify runServices lists all services that should be started.
	require.Len(t, raw.RunServices, 3)
	assert.Equal(t, []string{"app", "db", "redis"}, raw.RunServices)

	// Verify forwardPorts contains both integer and "service:port" string entries.
	require.Len(t, raw.ForwardPorts, 3)
	assert.Equal(t, float64(3000), raw.ForwardPorts[0])
	assert.Equal(t, "db:5432", raw.ForwardPorts[1])
	assert.Equal(t, "redis:6379", raw.ForwardPorts[2])
}

// TestLoadConfig_NotFound verifies that LoadConfig returns a CLIError with
// ExitDevContainerNotFound when the file does not exist.
func TestLoadConfig_NotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/devcontainer.json")
	require.Error(t, err)

	// Verify the error is a CLIError with the correct exit code.
	// errors.As is the idiomatic Go 1.13+ way to check error types
	// in a wrapped-error chain.
	var cliErr *model.CLIError
	require.True(t, errors.As(err, &cliErr), "error should be a *model.CLIError")
	assert.Equal(t, model.ExitDevContainerNotFound, cliErr.Code)
}

// --- DetectPattern tests ---

// TestDetectPattern_Image verifies that a configuration with no dockerComposeFile
// and no build field is detected as Pattern A (image).
func TestDetectPattern_Image(t *testing.T) {
	raw := &RawDevContainer{
		Image: "node:20",
	}
	pattern := DetectPattern(raw, 0)
	assert.Equal(t, model.PatternImage, pattern)
}

// TestDetectPattern_Dockerfile verifies that a configuration with a build field
// is detected as Pattern B (dockerfile).
func TestDetectPattern_Dockerfile(t *testing.T) {
	raw := &RawDevContainer{
		Build: &BuildConfig{
			Dockerfile: "Dockerfile",
		},
	}
	pattern := DetectPattern(raw, 0)
	assert.Equal(t, model.PatternDockerfile, pattern)
}

// TestDetectPattern_ComposeSingle verifies that a configuration with
// dockerComposeFile and 1 service is detected as Pattern C (compose-single).
func TestDetectPattern_ComposeSingle(t *testing.T) {
	raw := &RawDevContainer{
		DockerComposeFile: "docker-compose.yml",
		Service:           "app",
	}
	// composeServiceCount == 1 → single service
	pattern := DetectPattern(raw, 1)
	assert.Equal(t, model.PatternComposeSingle, pattern)
}

// TestDetectPattern_ComposeMulti verifies that a configuration with
// dockerComposeFile and 2+ services is detected as Pattern D (compose-multi).
func TestDetectPattern_ComposeMulti(t *testing.T) {
	raw := &RawDevContainer{
		DockerComposeFile: []interface{}{"docker-compose.yml"},
		Service:           "app",
		RunServices:       []string{"app", "db", "redis"},
	}
	// composeServiceCount == 3 → multi service
	pattern := DetectPattern(raw, 3)
	assert.Equal(t, model.PatternComposeMulti, pattern)
}

// --- ExtractPorts tests ---

// TestExtractPorts_ForwardPorts verifies that forwardPorts entries are correctly
// parsed, including both integer (container port only) and "service:port" string formats.
func TestExtractPorts_ForwardPorts(t *testing.T) {
	raw := &RawDevContainer{
		ForwardPorts: []interface{}{
			float64(3000),  // plain integer port
			"db:5432",      // service:port string
			"redis:6379",   // another service:port string
		},
	}

	ports := ExtractPorts(raw, "app")

	require.Len(t, ports, 3)

	// First port: plain integer → uses defaultServiceName.
	assert.Equal(t, "app", ports[0].ServiceName)
	assert.Equal(t, 3000, ports[0].ContainerPort)
	assert.Equal(t, 0, ports[0].HostPort, "forwardPorts int entries should have HostPort 0")
	assert.Equal(t, "tcp", ports[0].Protocol)

	// Second port: "db:5432" → service name is "db".
	assert.Equal(t, "db", ports[1].ServiceName)
	assert.Equal(t, 5432, ports[1].ContainerPort)

	// Third port: "redis:6379" → service name is "redis".
	assert.Equal(t, "redis", ports[2].ServiceName)
	assert.Equal(t, 6379, ports[2].ContainerPort)
}

// TestExtractPorts_AppPort verifies that appPort entries in "hostPort:containerPort"
// string format are correctly parsed.
func TestExtractPorts_AppPort(t *testing.T) {
	raw := &RawDevContainer{
		AppPort: []interface{}{
			"3000:3000", // host:container mapping
			"8080:80",   // different host and container ports
		},
	}

	ports := ExtractPorts(raw, "app")

	require.Len(t, ports, 2)

	// First port: "3000:3000" → both host and container port are 3000.
	assert.Equal(t, "app", ports[0].ServiceName)
	assert.Equal(t, 3000, ports[0].ContainerPort)
	assert.Equal(t, 3000, ports[0].HostPort)
	assert.Equal(t, "tcp", ports[0].Protocol)

	// Second port: "8080:80" → host=8080, container=80.
	assert.Equal(t, "app", ports[1].ServiceName)
	assert.Equal(t, 80, ports[1].ContainerPort)
	assert.Equal(t, 8080, ports[1].HostPort)
}

// TestExtractPorts_WithLabels verifies that portsAttributes labels are
// correctly applied to extracted ports.
func TestExtractPorts_WithLabels(t *testing.T) {
	raw := &RawDevContainer{
		ForwardPorts: []interface{}{float64(3000), float64(8080)},
		PortsAttributes: map[string]PortAttribute{
			"3000": {Label: "Application", OnAutoForward: "notify"},
			"8080": {Label: "API Server", OnAutoForward: "silent"},
		},
	}

	ports := ExtractPorts(raw, "app")

	require.Len(t, ports, 2)
	assert.Equal(t, "Application", ports[0].Label)
	assert.Equal(t, "API Server", ports[1].Label)
}

// --- GetComposeFiles tests ---

// TestGetComposeFiles_String verifies that a single string dockerComposeFile
// is normalized into a single-element string slice.
func TestGetComposeFiles_String(t *testing.T) {
	raw := &RawDevContainer{
		DockerComposeFile: "docker-compose.yml",
	}

	files := GetComposeFiles(raw)

	require.Len(t, files, 1)
	assert.Equal(t, "docker-compose.yml", files[0])
}

// TestGetComposeFiles_Array verifies that an array-form dockerComposeFile
// is correctly extracted into a string slice.
func TestGetComposeFiles_Array(t *testing.T) {
	raw := &RawDevContainer{
		DockerComposeFile: []interface{}{"docker-compose.yml", "docker-compose.override.yml"},
	}

	files := GetComposeFiles(raw)

	require.Len(t, files, 2)
	assert.Equal(t, "docker-compose.yml", files[0])
	assert.Equal(t, "docker-compose.override.yml", files[1])
}

// TestGetComposeFiles_Nil verifies that a nil dockerComposeFile returns nil.
func TestGetComposeFiles_Nil(t *testing.T) {
	raw := &RawDevContainer{
		DockerComposeFile: nil,
	}

	files := GetComposeFiles(raw)
	assert.Nil(t, files)
}

// --- FindDevContainerJSON tests ---

// TestFindDevContainerJSON verifies that the function correctly finds
// devcontainer.json in the .devcontainer/ subdirectory (the standard location).
func TestFindDevContainerJSON(t *testing.T) {
	fixturePath := testdataPath(t, "image-simple")

	found, err := FindDevContainerJSON(fixturePath)
	require.NoError(t, err)

	// The function should return the path to the .devcontainer/ subdirectory variant.
	expectedPath := filepath.Join(fixturePath, ".devcontainer", "devcontainer.json")
	assert.Equal(t, expectedPath, found)
}

// TestFindDevContainerJSON_RootLevel verifies that the function finds
// .devcontainer.json at the project root when the .devcontainer/ subdirectory
// does not contain one.
func TestFindDevContainerJSON_RootLevel(t *testing.T) {
	// Create a temporary directory with only .devcontainer.json at the root level.
	tmpDir := t.TempDir()
	rootFile := filepath.Join(tmpDir, ".devcontainer.json")
	err := os.WriteFile(rootFile, []byte(`{"name": "test"}`), 0644)
	require.NoError(t, err)

	found, err := FindDevContainerJSON(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, rootFile, found)
}

// TestFindDevContainerJSON_NotFound verifies that the function returns a
// CLIError with ExitDevContainerNotFound when no devcontainer.json exists.
func TestFindDevContainerJSON_NotFound(t *testing.T) {
	// Use an empty temporary directory that has no devcontainer.json.
	tmpDir := t.TempDir()

	_, err := FindDevContainerJSON(tmpDir)
	require.Error(t, err)

	var cliErr *model.CLIError
	require.True(t, errors.As(err, &cliErr), "error should be a *model.CLIError")
	assert.Equal(t, model.ExitDevContainerNotFound, cliErr.Code)
}
