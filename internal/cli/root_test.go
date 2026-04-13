// root_test.go validates that the Cobra command tree is wired correctly,
// all subcommands are registered, and the version command outputs version
// information.
package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNewRootCommand_CreatedWithoutError verifies that the root command
// is created without error and is not nil.
func TestNewRootCommand_CreatedWithoutError(t *testing.T) {
	cmd := NewRootCommand()
	if cmd == nil {
		t.Fatal("NewRootCommand() returned nil")
	}
}

// TestNewRootCommand_HasAllSubcommands verifies that all expected
// subcommands are registered on the root command.
func TestNewRootCommand_HasAllSubcommands(t *testing.T) {
	cmd := NewRootCommand()

	expectedSubcommands := []string{"hook", "status", "clean", "doctor", "version", "add", "rules", "rm"}
	registeredNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		registeredNames[sub.Name()] = true
	}

	for _, name := range expectedSubcommands {
		if !registeredNames[name] {
			t.Errorf("expected subcommand %q not found in root command", name)
		}
	}
}

// TestVersionCommand_OutputsVersionString verifies that the version
// subcommand outputs a string containing version information.
func TestVersionCommand_OutputsVersionString(t *testing.T) {
	SetVersionInfo("1.2.3", "abc123", "2024-01-01")

	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version command returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "1.2.3") {
		t.Errorf("version output does not contain version string, got: %s", output)
	}
	if !strings.Contains(output, "abc123") {
		t.Errorf("version output does not contain commit hash, got: %s", output)
	}
}
