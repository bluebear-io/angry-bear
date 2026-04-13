// clean_test.go contains integration tests for the care-bare clean command.
// Tests exercise the command against real temporary filesystems with controlled
// state files, verifying TTL-based pruning, --all cleanup, and --session cleanup.
package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Blue-Bear-Security/care-bare/internal/cli"
	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/state"
)

// runCleanInDir executes the clean command with the working directory set
// to dir. It captures stdout and returns it along with any error.
func runCleanInDir(t *testing.T, dir string, extraArgs ...string) (string, error) {
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

	args := []string{"clean"}
	args = append(args, extraArgs...)
	cmd.SetArgs(args)

	execErr := cmd.Execute()
	return outBuf.String(), execErr
}

// writeCleanStateFile creates a session state file in the given dir.
func writeCleanStateFile(t *testing.T, dir, sessionID string, skills []string) {
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

// writeDefaultConfig writes a default config.json with the given TTL hours.
func writeDefaultConfig(t *testing.T, dir string, ttlHours int) {
	t.Helper()
	configDir := filepath.Join(dir, ".care-bare")
	err := os.MkdirAll(configDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create .care-bare directory: %v", err)
	}
	cfg := engine.GlobalConfig{
		SkillPaths:    []string{".claude/skills"},
		StateTTLHours: ttlHours,
		DefaultAgent:  "*",
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	err = os.WriteFile(filepath.Join(configDir, "config.json"), data, 0o644)
	if err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}
}

// TestClean_RemovesExpiredStateFiles verifies that clean (no flags) removes
// session files older than the TTL while preserving fresh ones.
func TestClean_RemovesExpiredStateFiles(t *testing.T) {
	dir := t.TempDir()

	// Write config with 24-hour TTL.
	writeDefaultConfig(t, dir, 24)
	writeEnforcementConfig(t, dir, []engine.Rule{})

	// Create two session files.
	writeCleanStateFile(t, dir, "old-session", []string{"skill-a"})
	writeCleanStateFile(t, dir, "new-session", []string{"skill-b"})

	// Backdate the old session file beyond the 24-hour TTL.
	stateDir := filepath.Join(dir, ".care-bare", "state")
	oldTime := time.Now().Add(-48 * time.Hour)
	err := os.Chtimes(filepath.Join(stateDir, "old-session.json"), oldTime, oldTime)
	if err != nil {
		t.Fatalf("failed to backdate state file: %v", err)
	}

	output, execErr := runCleanInDir(t, dir)
	if execErr != nil {
		t.Fatalf("clean command returned error: %v", execErr)
	}

	if !strings.Contains(output, "Pruned expired sessions") {
		t.Errorf("expected prune message, got: %s", output)
	}

	// Verify old session was removed.
	if _, err := os.Stat(filepath.Join(stateDir, "old-session.json")); !os.IsNotExist(err) {
		t.Error("expected old-session.json to be removed")
	}

	// Verify new session was preserved.
	if _, err := os.Stat(filepath.Join(stateDir, "new-session.json")); err != nil {
		t.Errorf("expected new-session.json to be preserved, got error: %v", err)
	}
}

// TestClean_AllRemovesAllStateFiles verifies that clean --all removes all
// session state files regardless of TTL.
func TestClean_AllRemovesAllStateFiles(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})
	writeCleanStateFile(t, dir, "session-1", []string{"skill-a"})
	writeCleanStateFile(t, dir, "session-2", []string{"skill-b"})
	writeCleanStateFile(t, dir, "session-3", []string{})

	output, execErr := runCleanInDir(t, dir, "--all")
	if execErr != nil {
		t.Fatalf("clean --all returned error: %v", execErr)
	}

	if !strings.Contains(output, "Cleaned 3 sessions") {
		t.Errorf("expected 'Cleaned 3 sessions', got: %s", output)
	}

	// Verify all session files are gone.
	stateDir := filepath.Join(dir, ".care-bare", "state")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("failed to read state directory: %v", err)
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			t.Errorf("expected all session files to be removed, found: %s", entry.Name())
		}
	}
}

// TestClean_SessionRemovesSpecificSession verifies that clean --session removes
// only the specified session's state and lock files.
func TestClean_SessionRemovesSpecificSession(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})
	writeCleanStateFile(t, dir, "session-keep", []string{"skill-a"})
	writeCleanStateFile(t, dir, "session-remove", []string{"skill-b"})

	// Also create a lock file for the session being removed.
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.WriteFile(filepath.Join(stateDir, "session-remove.lock"), []byte{}, 0o600)
	if err != nil {
		t.Fatalf("failed to write lock file: %v", err)
	}

	output, execErr := runCleanInDir(t, dir, "--session", "session-remove")
	if execErr != nil {
		t.Fatalf("clean --session returned error: %v", execErr)
	}

	if !strings.Contains(output, "Cleaned session: session-remove") {
		t.Errorf("expected clean confirmation, got: %s", output)
	}

	// Verify the targeted session was removed.
	if _, err := os.Stat(filepath.Join(stateDir, "session-remove.json")); !os.IsNotExist(err) {
		t.Error("expected session-remove.json to be removed")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "session-remove.lock")); !os.IsNotExist(err) {
		t.Error("expected session-remove.lock to be removed")
	}

	// Verify the other session is preserved.
	if _, err := os.Stat(filepath.Join(stateDir, "session-keep.json")); err != nil {
		t.Errorf("expected session-keep.json to be preserved, got error: %v", err)
	}
}

// TestClean_HandlesEmptyStateDirectory verifies that clean handles an empty
// state directory without error.
func TestClean_HandlesEmptyStateDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create the .care-bare/state/ directory but no session files.
	writeEnforcementConfig(t, dir, []engine.Rule{})
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.MkdirAll(stateDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}

	output, execErr := runCleanInDir(t, dir)
	if execErr != nil {
		t.Fatalf("clean command returned error: %v", execErr)
	}

	if !strings.Contains(output, "Pruned expired sessions") {
		t.Errorf("expected prune message, got: %s", output)
	}
}

// TestClean_HandlesMissingStateDirectory verifies that clean prints a message
// and returns without error when the state directory does not exist.
func TestClean_HandlesMissingStateDirectory(t *testing.T) {
	dir := t.TempDir()

	output, execErr := runCleanInDir(t, dir)
	if execErr != nil {
		t.Fatalf("clean command returned error: %v", execErr)
	}

	if !strings.Contains(output, "No state directory found") {
		t.Errorf("expected missing state directory message, got: %s", output)
	}
}

// TestClean_MutuallyExclusiveFlags verifies that providing both --all and
// --session results in an error.
func TestClean_MutuallyExclusiveFlags(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.MkdirAll(stateDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}

	_, execErr := runCleanInDir(t, dir, "--all", "--session", "some-session")
	if execErr == nil {
		t.Fatal("expected error for mutually exclusive flags, got nil")
	}

	if !strings.Contains(execErr.Error(), "mutually exclusive") {
		t.Errorf("expected mutually exclusive error, got: %v", execErr)
	}
}

// TestClean_InvalidSessionID verifies that clean --session rejects invalid
// session IDs with a clear error.
func TestClean_InvalidSessionID(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.MkdirAll(stateDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}

	_, execErr := runCleanInDir(t, dir, "--session", "../../../etc/passwd")
	if execErr == nil {
		t.Fatal("expected error for invalid session ID, got nil")
	}

	if !strings.Contains(execErr.Error(), "invalid session ID") {
		t.Errorf("expected invalid session ID error, got: %v", execErr)
	}
}

// TestClean_AllSkipsDirectoriesAndHiddenFiles verifies that clean --all
// skips subdirectories and hidden files in the state directory, only
// removing JSON session files.
func TestClean_AllSkipsDirectoriesAndHiddenFiles(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})
	writeCleanStateFile(t, dir, "session-real", []string{"skill-a"})

	// Add a subdirectory and a hidden file to the state dir.
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.Mkdir(filepath.Join(stateDir, "subdir"), 0o755)
	if err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}
	err = os.WriteFile(filepath.Join(stateDir, ".hidden"), []byte("hidden"), 0o600)
	if err != nil {
		t.Fatalf("failed to create hidden file: %v", err)
	}
	err = os.WriteFile(filepath.Join(stateDir, "not-json.txt"), []byte("text"), 0o600)
	if err != nil {
		t.Fatalf("failed to create non-json file: %v", err)
	}

	output, execErr := runCleanInDir(t, dir, "--all")
	if execErr != nil {
		t.Fatalf("clean --all returned error: %v", execErr)
	}

	// Only the real session should be cleaned (subdir, hidden, non-json are skipped).
	if !strings.Contains(output, "Cleaned 1 sessions") {
		t.Errorf("expected 'Cleaned 1 sessions', got: %s", output)
	}

	// Verify the subdirectory and hidden file are still there.
	if _, err := os.Stat(filepath.Join(stateDir, "subdir")); err != nil {
		t.Error("subdirectory should not be removed by clean --all")
	}
	if _, err := os.Stat(filepath.Join(stateDir, ".hidden")); err != nil {
		t.Error("hidden file should not be removed by clean --all")
	}
}

// TestClean_MalformedGlobalConfig verifies that clean handles a malformed
// config.json by returning an error.
func TestClean_MalformedGlobalConfig(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.MkdirAll(stateDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}

	// Write malformed config.json.
	err = os.WriteFile(filepath.Join(dir, ".care-bare", "config.json"), []byte("{bad json"), 0o644)
	if err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	_, execErr := runCleanInDir(t, dir)
	if execErr == nil {
		t.Fatal("expected error for malformed config.json, got nil")
	}
}

// TestClean_AllWithNoSessions verifies that clean --all with no session files
// reports 0 sessions cleaned.
func TestClean_AllWithNoSessions(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.MkdirAll(stateDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}

	output, execErr := runCleanInDir(t, dir, "--all")
	if execErr != nil {
		t.Fatalf("clean --all returned error: %v", execErr)
	}

	if !strings.Contains(output, "Cleaned 0 sessions") {
		t.Errorf("expected 'Cleaned 0 sessions', got: %s", output)
	}
}
