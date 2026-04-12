// status_test.go contains integration tests for the care-bare status command.
// Tests exercise the command against real temporary filesystems with controlled
// fixture data, verifying output for rules, sessions, skills, and agents.
package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Bear-Security/care-bare/internal/cli"
	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/state"
)

// runStatusInDir executes the status command with the working directory set
// to dir. It captures stdout and returns it along with any error.
func runStatusInDir(t *testing.T, dir string, extraArgs ...string) (string, error) {
	t.Helper()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get original working directory: %v", err)
	}
	err = os.Chdir(dir)
	if err != nil {
		t.Fatalf("failed to chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		err := os.Chdir(origDir)
		if err != nil {
			t.Logf("warning: failed to restore working directory: %v", err)
		}
	})

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(new(bytes.Buffer))

	args := []string{"status"}
	args = append(args, extraArgs...)
	cmd.SetArgs(args)

	execErr := cmd.Execute()
	return outBuf.String(), execErr
}

// TestStatus_DisplaysConfiguredRules verifies that status shows enforcement
// rules with their tool, path, skill, agent, and source information.
func TestStatus_DisplaysConfiguredRules(t *testing.T) {
	dir := t.TempDir()

	// Set up enforcement config with two rules.
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
		{Tool: "Write", Path: "handler/**", Skill: "handler-skill", Agent: "claude"},
	})

	output, err := runStatusInDir(t, dir)
	if err != nil {
		t.Fatalf("status command returned error: %v", err)
	}

	// Verify rules section header.
	if !strings.Contains(output, "=== Enforcement Rules ===") {
		t.Errorf("expected enforcement rules header, got: %s", output)
	}

	// Verify rule details.
	if !strings.Contains(output, "Tool: Edit") {
		t.Errorf("expected Tool: Edit in output, got: %s", output)
	}
	if !strings.Contains(output, "Skill: go-standards") {
		t.Errorf("expected Skill: go-standards in output, got: %s", output)
	}
	if !strings.Contains(output, "Skill: handler-skill") {
		t.Errorf("expected Skill: handler-skill in output, got: %s", output)
	}
	if !strings.Contains(output, "[1]") && !strings.Contains(output, "[2]") {
		t.Errorf("expected numbered rules in output, got: %s", output)
	}
}

// TestStatus_DisplaysActiveSessions verifies that status shows active sessions
// with their invoked skills.
func TestStatus_DisplaysActiveSessions(t *testing.T) {
	dir := t.TempDir()

	// Create .care-bare directory and enforcement config so project root resolves.
	writeEnforcementConfig(t, dir, []engine.Rule{})

	// Write session state files.
	writeStatusStateFile(t, dir, "session-abc", []string{"go-standards", "linear"})
	writeStatusStateFile(t, dir, "session-def", []string{})

	output, err := runStatusInDir(t, dir)
	if err != nil {
		t.Fatalf("status command returned error: %v", err)
	}

	// Verify sessions section.
	if !strings.Contains(output, "=== Active Sessions ===") {
		t.Errorf("expected active sessions header, got: %s", output)
	}
	if !strings.Contains(output, "session-abc") {
		t.Errorf("expected session-abc in output, got: %s", output)
	}
	if !strings.Contains(output, "go-standards, linear") {
		t.Errorf("expected invoked skills list in output, got: %s", output)
	}
	if !strings.Contains(output, "session-def") {
		t.Errorf("expected session-def in output, got: %s", output)
	}
	if !strings.Contains(output, "(none)") {
		t.Errorf("expected (none) for empty skills in output, got: %s", output)
	}
}

// TestStatus_DisplaysDiscoveredSkills verifies that status shows discovered
// skill definitions from configured skill paths.
func TestStatus_DisplaysDiscoveredSkills(t *testing.T) {
	dir := t.TempDir()

	// Create .care-bare with config pointing to .claude/skills.
	writeEnforcementConfig(t, dir, []engine.Rule{})

	// Create a skill file.
	skillDir := filepath.Join(dir, ".claude", "skills", "my-skill")
	err := os.MkdirAll(skillDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}
	skillContent := "---\nname: my-skill\ndescription: A test skill\n---\nSkill content here."
	err = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	output, err := runStatusInDir(t, dir)
	if err != nil {
		t.Fatalf("status command returned error: %v", err)
	}

	if !strings.Contains(output, "=== Discovered Skills ===") {
		t.Errorf("expected discovered skills header, got: %s", output)
	}
	if !strings.Contains(output, "my-skill") {
		t.Errorf("expected my-skill in output, got: %s", output)
	}
}

// TestStatus_WorksWithNoConfig verifies that status handles a project with
// no .care-bare directory gracefully, displaying empty/default state.
func TestStatus_WorksWithNoConfig(t *testing.T) {
	dir := t.TempDir()

	output, err := runStatusInDir(t, dir)
	if err != nil {
		t.Fatalf("status command returned error: %v", err)
	}

	// Should show "no rules", "no sessions", "no skills" messages without crashing.
	if !strings.Contains(output, "No enforcement rules configured.") {
		t.Errorf("expected no rules message, got: %s", output)
	}
	if !strings.Contains(output, "No state directory found.") {
		t.Errorf("expected no state directory message, got: %s", output)
	}
	if !strings.Contains(output, "No skills discovered.") {
		t.Errorf("expected no skills message, got: %s", output)
	}
}

// TestStatus_SessionFilter verifies that the --session flag filters output
// to only the specified session.
func TestStatus_SessionFilter(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})
	writeStatusStateFile(t, dir, "session-abc", []string{"go-standards"})
	writeStatusStateFile(t, dir, "session-def", []string{"linear"})

	output, err := runStatusInDir(t, dir, "--session", "session-abc")
	if err != nil {
		t.Fatalf("status command returned error: %v", err)
	}

	if !strings.Contains(output, "session-abc") {
		t.Errorf("expected session-abc in output, got: %s", output)
	}
	if strings.Contains(output, "session-def") {
		t.Errorf("session-def should be filtered out, got: %s", output)
	}
}

// TestStatus_DetectsAgents verifies that status correctly reports detected
// and undetected agents based on directory presence.
func TestStatus_DetectsAgents(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})

	// Create .claude/ directory but not .cursor/.
	err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .claude directory: %v", err)
	}

	output, err := runStatusInDir(t, dir)
	if err != nil {
		t.Fatalf("status command returned error: %v", err)
	}

	if !strings.Contains(output, "=== Agent Integrations ===") {
		t.Errorf("expected agent integrations header, got: %s", output)
	}
	if !strings.Contains(output, "claude: detected") {
		t.Errorf("expected claude detected, got: %s", output)
	}
	if !strings.Contains(output, "cursor: not detected") {
		t.Errorf("expected cursor not detected, got: %s", output)
	}
}

// TestStatus_CorruptStateFile verifies that status handles corrupt session
// state files gracefully by reporting them rather than crashing.
func TestStatus_CorruptStateFile(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})

	// Write a corrupt state file.
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.MkdirAll(stateDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}
	err = os.WriteFile(filepath.Join(stateDir, "corrupt-session.json"), []byte("{not valid json"), 0o600)
	if err != nil {
		t.Fatalf("failed to write corrupt state file: %v", err)
	}

	output, execErr := runStatusInDir(t, dir)
	if execErr != nil {
		t.Fatalf("status command returned error: %v", execErr)
	}

	if !strings.Contains(output, "corrupt-session") {
		t.Errorf("expected corrupt session mentioned in output, got: %s", output)
	}
	if !strings.Contains(output, "corrupt state file") {
		t.Errorf("expected corrupt state file message, got: %s", output)
	}
}

// writeStatusStateFile is a test helper that creates a session state file.
func writeStatusStateFile(t *testing.T, dir, sessionID string, skills []string) {
	t.Helper()
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.MkdirAll(stateDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}
	ss := state.SessionState{
		SessionID:     sessionID,
		CreatedAt:     "2025-01-15T10:30:00Z",
		InvokedSkills: skills,
	}
	data, err := json.Marshal(ss)
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}
	err = os.WriteFile(filepath.Join(stateDir, sessionID+".json"), data, 0o600)
	if err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}
}
