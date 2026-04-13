// init_internal_test.go contains internal tests for init_cmd.go functions.
// Lives in package cli to access unexported variables like execLookPath.
package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Bear-Security/care-bare/internal/adapter"
)

// TestInit_CareBareNotOnPATH verifies that when care-bare is not on PATH,
// the init output includes setup instructions.
func TestInit_CareBareNotOnPATH(t *testing.T) {
	dir := t.TempDir()

	// Override execLookPath to simulate care-bare not being on PATH.
	origLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	t.Cleanup(func() { execLookPath = origLookPath })

	// Set test overrides for adapters.
	adapter.SetRegistryDefaults(dir, "care-bare")
	t.Cleanup(func() { adapter.SetRegistryDefaults("", "") })

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cmd := NewRootCommand()
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"init"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("init failed: %v", execErr)
	}

	output := outBuf.String()
	if !strings.Contains(output, "care-bare is not on your PATH") {
		t.Errorf("expected PATH not found message, got: %s", output)
	}
	if !strings.Contains(output, "make install") {
		t.Errorf("expected 'make install' instruction, got: %s", output)
	}
}

// TestInit_CareBareOnPATH verifies that when care-bare IS on PATH,
// the init output does NOT include PATH setup instructions.
func TestInit_CareBareOnPATH(t *testing.T) {
	dir := t.TempDir()

	// Override execLookPath to simulate care-bare being on PATH.
	origLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		return "/usr/local/bin/care-bare", nil
	}
	t.Cleanup(func() { execLookPath = origLookPath })

	adapter.SetRegistryDefaults(dir, "care-bare")
	t.Cleanup(func() { adapter.SetRegistryDefaults("", "") })

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cmd := NewRootCommand()
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"init"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("init failed: %v", execErr)
	}

	output := outBuf.String()
	if strings.Contains(output, "care-bare is not on your PATH") {
		t.Errorf("should NOT show PATH message when binary is found, got: %s", output)
	}
}

// TestInit_HookInstallationFailure verifies that init returns an error
// when hook installation fails for a detected agent.
func TestInit_HookInstallationFailure(t *testing.T) {
	dir := t.TempDir()

	origLookPath := execLookPath
	execLookPath = exec.LookPath
	t.Cleanup(func() { execLookPath = origLookPath })

	adapter.SetRegistryDefaults(dir, "care-bare")
	t.Cleanup(func() { adapter.SetRegistryDefaults("", "") })

	// Create .claude/ directory to trigger detection.
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Make settings.json a directory to cause hook installation to fail.
	if err := os.Mkdir(filepath.Join(claudeDir, "settings.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	cmd := NewRootCommand()
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"init"})

	execErr := cmd.Execute()
	if execErr == nil {
		t.Fatal("expected error for hook installation failure, got nil")
	}

	output := outBuf.String()
	if !strings.Contains(output, "failed") {
		t.Errorf("expected failure message, got: %s", output)
	}
}
