// Package cli implements the cobra-based CLI commands for worktree-container.
//
// Each subcommand (create, list, start, stop, remove) is defined in its own
// file within this package. This file defines the root command that serves as
// the parent for all subcommands and handles global flags.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shinji-kodama/worktree-container/internal/model"
)

// Global flag variables shared across all subcommands.
// These are bound to cobra persistent flags on the root command,
// which makes them available to every subcommand automatically.
var (
	// jsonOutput controls whether command output is formatted as JSON.
	// When true, all output uses structured JSON format for machine consumption.
	// When false (default), output uses human-readable text format.
	jsonOutput bool

	// verbose enables detailed logging output for debugging.
	// When true, additional information about operations is printed to stderr.
	verbose bool
)

// version, commit, and date are set at build time via ldflags.
// They are injected from the main package to display version information.
var (
	// Version is the semantic version of the binary (e.g., "1.0.0").
	Version = "dev"

	// Commit is the Git commit hash the binary was built from.
	Commit = "none"

	// Date is the build timestamp.
	Date = "unknown"
)

// NewRootCommand creates and configures the root cobra command.
// This is the entry point for the entire CLI application.
//
// The root command itself does not perform any action — it only provides
// help text and global flags. Actual functionality is provided by
// subcommands (create, list, start, stop, remove).
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		// Use is the one-line usage pattern shown in help output.
		Use:   "worktree-container",
		Short: "Git worktree-aware Dev Container environment manager",
		Long: `worktree-container automatically creates isolated Dev Container environments
for each Git worktree, with automatic port management to prevent conflicts.

Each worktree gets its own set of containers with shifted ports, ensuring
zero port collisions between environments.`,

		// SilenceUsage prevents cobra from printing usage on every error.
		// We handle error output ourselves for cleaner UX.
		SilenceUsage: true,

		// SilenceErrors prevents cobra from printing errors automatically.
		// We format errors ourselves (text or JSON based on --json flag).
		SilenceErrors: true,

		// Version is displayed when --version flag is used.
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, Date),
	}

	// PersistentFlags are inherited by all subcommands. This is the cobra
	// mechanism for global flags — any flag defined here is automatically
	// available in every subcommand without re-declaration.
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// Register subcommands. Each subcommand is defined in its own file
	// (create.go, list.go, etc.) and returns a *cobra.Command.
	rootCmd.AddCommand(NewCreateCommand())
	rootCmd.AddCommand(NewListCommand())
	rootCmd.AddCommand(NewStopCommand())
	rootCmd.AddCommand(NewStartCommand())
	rootCmd.AddCommand(NewRemoveCommand())

	return rootCmd
}

// Execute runs the root command and handles exit codes.
// This is the main entry point called from main.go.
//
// It inspects errors returned by cobra commands and translates them
// into appropriate OS exit codes. CLIError types carry their own
// exit codes; other errors default to exit code 1.
func Execute(rootCmd *cobra.Command) {
	if err := rootCmd.Execute(); err != nil {
		// Check if the error is a CLIError with a specific exit code.
		// errors.As would also work here, but a type assertion is simpler
		// for this single-level check.
		if cliErr, ok := err.(*model.CLIError); ok {
			printError(cliErr.Message, cliErr.Err)
			os.Exit(int(cliErr.Code))
		}

		// Generic error — exit with code 1.
		printError(err.Error(), nil)
		os.Exit(int(model.ExitGeneralError))
	}
}

// printError outputs an error message in the appropriate format
// (JSON or text) based on the --json global flag.
func printError(message string, underlying error) {
	if jsonOutput {
		// JSON error format matches the CLI contracts specification.
		errObj := map[string]interface{}{
			"error": map[string]interface{}{
				"message": message,
			},
		}
		if underlying != nil {
			if errMap, ok := errObj["error"].(map[string]interface{}); ok {
				errMap["detail"] = underlying.Error()
			}
		}
		// json.MarshalIndent produces human-readable JSON with indentation.
		// We write to stderr for errors, even in JSON mode, because stdout
		// is reserved for successful command output.
		data, _ := json.MarshalIndent(errObj, "", "  ")
		fmt.Fprintln(os.Stderr, string(data))
	} else {
		// Text format: "Error: <message>" on stderr.
		if underlying != nil {
			fmt.Fprintf(os.Stderr, "Error: %s: %v\n", message, underlying)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", message)
		}
	}
}

// VerboseLog prints a message to stderr only when verbose mode is enabled.
// This is used throughout the CLI for debug/trace output that helps
// users understand what operations are being performed.
func VerboseLog(format string, args ...interface{}) {
	if verbose {
		fmt.Fprintf(os.Stderr, "[verbose] "+format+"\n", args...)
	}
}

// IsJSONOutput returns whether the --json flag is set.
// Subcommands use this to decide their output format.
func IsJSONOutput() bool {
	return jsonOutput
}
