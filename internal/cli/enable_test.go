// enable_test.go contains tests for angry-bear enable and angry-bear disable commands.
// These tests verify that hook installation and removal work through the CLI layer.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluebear-io/angry-bear/internal/adapter"
)

// TestRunEnable_InstallsHooksForDetectedAgents verifies that the enable command
// installs hooks for agents whose config directories exist on the machine.
func TestRunEnable_InstallsHooksForDetectedAgents(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create the .claude directory so the claude adapter is detected
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	// Set registry defaults so adapters use our temp home
	adapter.SetRegistryDefaults(tmpDir, "angry-bear")
	defer adapter.SetRegistryDefaults("", "")

	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"enable"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("enable command returned error: %v", err)
	}

	output := buf.String()
	// Should report "Enforcement is active."
	if !strings.Contains(output, "Enforcement is active.") {
		t.Errorf("enable output missing 'Enforcement is active.', got: %s", output)
	}

	// Verify hook was actually installed in settings.json
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created after enable: %v", err)
	}
	if !strings.Contains(string(data), "angry-bear hook") {
		t.Errorf("settings.json missing angry-bear hook after enable:\n%s", data)
	}
}

// TestRunEnable_ReportsSkippedAgents verifies that agents whose config directories
// do not exist are reported as skipped or silently handled.
func TestRunEnable_ReportsSkippedAgents(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Don't create any agent config dirs
	adapter.SetRegistryDefaults(tmpDir, "angry-bear")
	defer adapter.SetRegistryDefaults("", "")

	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"enable"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("enable command returned error: %v", err)
	}

	output := buf.String()
	// With no detected agents, should say hooks already installed or no agents found
	if !strings.Contains(output, "Enforcement is active.") {
		t.Errorf("enable output missing 'Enforcement is active.', got: %s", output)
	}
}

// TestRunDisable_RemovesHooks verifies that the disable command removes angry-bear
// hooks from agent configs.
func TestRunDisable_RemovesHooks(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create .claude directory and install a hook
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	hookSettings := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {"type": "command", "command": "angry-bear hook claude"}
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(hookSettings), 0o644); err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}

	adapter.SetRegistryDefaults(tmpDir, "angry-bear")
	defer adapter.SetRegistryDefaults("", "")

	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"disable"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("disable command returned error: %v", err)
	}

	output := buf.String()
	// Should report hooks removed
	if !strings.Contains(output, "Hooks removed for claude") {
		t.Errorf("disable output missing removal confirmation, got: %s", output)
	}
	if !strings.Contains(output, "Enforcement is disabled.") {
		t.Errorf("disable output missing 'Enforcement is disabled.', got: %s", output)
	}

	// Verify hook was actually removed from settings.json
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not found after disable: %v", err)
	}
	if strings.Contains(string(data), "angry-bear hook") {
		t.Errorf("angry-bear hook still present after disable:\n%s", data)
	}
}

// TestRunDisable_NoHooksToRemove verifies the disable command handles the case
// where no hooks are installed.
func TestRunDisable_NoHooksToRemove(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Don't create any agent directories
	adapter.SetRegistryDefaults(tmpDir, "angry-bear")
	defer adapter.SetRegistryDefaults("", "")

	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"disable"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("disable command returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No hooks found to remove.") {
		t.Errorf("disable output should say no hooks found, got: %s", output)
	}
}

// TestRunDisable_BothAgents verifies that disable removes hooks from both
// Claude and Cursor when both are present.
func TestRunDisable_BothAgents(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create both agent directories with hooks
	claudeDir := filepath.Join(tmpDir, ".claude")
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}

	// Write Claude settings with hook
	claudeSettings := `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "*", "hooks": [{"type": "command", "command": "angry-bear hook claude"}]}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(claudeSettings), 0o644); err != nil {
		t.Fatalf("failed to write claude settings: %v", err)
	}

	// Write Cursor hooks with hook
	cursorHooks := `{
  "version": 1,
  "hooks": {
    "preToolUse": [
      {"command": "angry-bear hook cursor"}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(cursorDir, "hooks.json"), []byte(cursorHooks), 0o644); err != nil {
		t.Fatalf("failed to write cursor hooks: %v", err)
	}

	adapter.SetRegistryDefaults(tmpDir, "angry-bear")
	defer adapter.SetRegistryDefaults("", "")

	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"disable"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("disable command returned error: %v", err)
	}

	output := buf.String()
	// Both agents should be reported as removed
	if !strings.Contains(output, "Hooks removed for claude") {
		t.Errorf("disable output missing claude removal, got: %s", output)
	}
	if !strings.Contains(output, "Hooks removed for cursor") {
		t.Errorf("disable output missing cursor removal, got: %s", output)
	}
}
