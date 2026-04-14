// claude_test.go contains comprehensive tests for the Claude Code adapter.
package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadFixture reads a JSON fixture file from the testdata directory.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	// Navigate from internal/adapter/ up to repo root, then into test/testdata/
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "testdata", name))
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", name, err)
	}
	return data
}

// --- ParseInput tests ---

func TestClaudeParseInput_SessionID(t *testing.T) {
	fixture := loadFixture(t, "claude_pretooluse_edit.json")
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.SessionID != "sess-abc-123" {
		t.Errorf("SessionID = %q, want %q", input.SessionID, "sess-abc-123")
	}
}

func TestClaudeParseInput_ToolName(t *testing.T) {
	fixture := loadFixture(t, "claude_pretooluse_edit.json")
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.ToolName != "Edit" {
		t.Errorf("ToolName = %q, want %q", input.ToolName, "Edit")
	}
}

func TestClaudeParseInput_EditFilePath(t *testing.T) {
	fixture := loadFixture(t, "claude_pretooluse_edit.json")
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "internal/engine/types.go" {
		t.Errorf("FilePath = %q, want %q", input.FilePath, "internal/engine/types.go")
	}
}

func TestClaudeParseInput_WriteFilePath(t *testing.T) {
	fixture := loadFixture(t, "claude_pretooluse_write.json")
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "new_file.go" {
		t.Errorf("FilePath = %q, want %q", input.FilePath, "new_file.go")
	}
}

func TestClaudeParseInput_BashNoFilePath(t *testing.T) {
	fixture := loadFixture(t, "claude_pretooluse_bash.json")
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "" {
		t.Errorf("FilePath = %q, want empty string for Bash tool", input.FilePath)
	}
	// Verify command is preserved in RawInput
	toolInput, ok := input.RawInput["tool_input"].(map[string]any)
	if !ok {
		t.Fatal("RawInput[tool_input] is not a map")
	}
	cmd, ok := toolInput["command"].(string)
	if !ok || cmd != "go test ./..." {
		t.Errorf("command = %q, want %q", cmd, "go test ./...")
	}
}

func TestClaudeParseInput_Cwd(t *testing.T) {
	fixture := loadFixture(t, "claude_pretooluse_edit.json")
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Cwd != "/home/user/project" {
		t.Errorf("Cwd = %q, want %q", input.Cwd, "/home/user/project")
	}
}

func TestClaudeParseInput_MissingOptionalFields(t *testing.T) {
	// Minimal valid JSON -- only required fields
	jsonStr := `{"hook_event_name":"PreToolUse","session_id":"x","tool_name":"Read"}`
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "" {
		t.Errorf("FilePath = %q, want empty", input.FilePath)
	}
	if input.Cwd != "" {
		t.Errorf("Cwd = %q, want empty", input.Cwd)
	}
}

func TestClaudeParseInput_InvalidJSON(t *testing.T) {
	adapter := &ClaudeAdapter{}

	_, err := adapter.ParseInput(strings.NewReader("not json at all"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestClaudeParseInput_SetsAgentClaude(t *testing.T) {
	fixture := loadFixture(t, "claude_pretooluse_edit.json")
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Agent != "claude" {
		t.Errorf("Agent = %q, want %q", input.Agent, "claude")
	}
}

// --- DetectSkillInvocation tests ---

func TestClaudeDetectSkillInvocation_SkillTool(t *testing.T) {
	fixture := loadFixture(t, "claude_pretooluse_skill.json")
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skillName, isSkill := adapter.DetectSkillInvocation(input)
	if !isSkill {
		t.Fatal("expected isSkill=true for Skill tool")
	}
	if skillName != "linear" {
		t.Errorf("skillName = %q, want %q", skillName, "linear")
	}
}

func TestClaudeDetectSkillInvocation_NonSkillTool(t *testing.T) {
	fixture := loadFixture(t, "claude_pretooluse_edit.json")
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skillName, isSkill := adapter.DetectSkillInvocation(input)
	if isSkill {
		t.Fatal("expected isSkill=false for Edit tool")
	}
	if skillName != "" {
		t.Errorf("skillName = %q, want empty", skillName)
	}
}

// --- FormatAllow / FormatDeny tests ---

func TestClaudeFormatAllow_ReturnsEmpty(t *testing.T) {
	adapter := &ClaudeAdapter{}

	result, err := adapter.FormatAllow()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("FormatAllow returned %d bytes, want 0", len(result))
	}
}

func TestClaudeFormatDeny_ReturnsDenyDecision(t *testing.T) {
	adapter := &ClaudeAdapter{}

	result, err := adapter.FormatDeny("some reason")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("FormatDeny returned invalid JSON: %v", err)
	}

	hookOutput, ok := parsed["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatal("missing hookSpecificOutput in response")
	}
	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision = %v, want %q", hookOutput["permissionDecision"], "deny")
	}
}

func TestClaudeFormatDeny_IncludesReason(t *testing.T) {
	adapter := &ClaudeAdapter{}
	reason := "missing skill: linear"

	result, err := adapter.FormatDeny(reason)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("FormatDeny returned invalid JSON: %v", err)
	}

	hookOutput := parsed["hookSpecificOutput"].(map[string]any)
	if hookOutput["permissionDecisionReason"] != reason {
		t.Errorf("permissionDecisionReason = %v, want %q", hookOutput["permissionDecisionReason"], reason)
	}
}

func TestClaudeFormatDeny_IncludesHookEventName(t *testing.T) {
	adapter := &ClaudeAdapter{}

	result, err := adapter.FormatDeny("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("FormatDeny returned invalid JSON: %v", err)
	}

	hookOutput := parsed["hookSpecificOutput"].(map[string]any)
	if hookOutput["hookEventName"] != "PreToolUse" {
		t.Errorf("hookEventName = %v, want %q", hookOutput["hookEventName"], "PreToolUse")
	}
}

// --- InstallHook tests ---

func TestClaudeInstallHook_CreatesSettingsWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "angry-bear"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	// Verify it contains the angry-bear hook entry
	if !strings.Contains(string(data), "angry-bear hook claude") {
		t.Errorf("settings.json missing angry-bear hook command:\n%s", data)
	}
}

func TestClaudeInstallHook_PreservesExistingHooks(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	// Write existing settings with a Bash hook
	existing := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {"type": "command", "command": "some-existing-hook.sh"}
        ]
      }
    ]
  },
  "enabledPlugins": {
    "linear@claude-plugins-official": true
  }
}`
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to write existing settings: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "angry-bear"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	content := string(data)
	// Existing Bash hook must still be there
	if !strings.Contains(content, "some-existing-hook.sh") {
		t.Errorf("existing Bash hook was removed:\n%s", content)
	}
	// angry-bear hook must be present
	if !strings.Contains(content, "angry-bear hook claude") {
		t.Errorf("angry-bear hook not added:\n%s", content)
	}
	// enabledPlugins must be preserved
	if !strings.Contains(content, "linear@claude-plugins-official") {
		t.Errorf("enabledPlugins was removed:\n%s", content)
	}
}

func TestClaudeInstallHook_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "angry-bear"}

	// Install twice
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("first InstallHook failed: %v", err)
	}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("second InstallHook failed: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	// Count occurrences of "angry-bear hook" -- should be exactly 1
	count := strings.Count(string(data), "angry-bear hook claude")
	if count != 1 {
		t.Errorf("angry-bear hook appears %d times, want 1:\n%s", count, data)
	}
}

func TestClaudeInstallHook_CreatesClaudeDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't pre-create .claude dir -- InstallHook should handle it

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "angry-bear"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Fatal("settings.json not created when .claude dir was missing")
	}
}

// --- InstallHook JSON structure tests ---

func TestClaudeInstallHook_CorrectJSONStructure(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "/usr/local/bin/angry-bear"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is invalid JSON: %v", err)
	}

	// Navigate to hooks -> PreToolUse -> [0]
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("missing 'hooks' key in settings")
	}
	preToolUse, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatal("missing 'PreToolUse' key in hooks")
	}
	if len(preToolUse) != 1 {
		t.Fatalf("PreToolUse has %d entries, want 1", len(preToolUse))
	}

	entry, ok := preToolUse[0].(map[string]any)
	if !ok {
		t.Fatal("PreToolUse[0] is not a map")
	}

	// Verify matcher is "*"
	if entry["matcher"] != "*" {
		t.Errorf("matcher = %v, want %q", entry["matcher"], "*")
	}

	// Verify hooks array inside the entry
	hooksList, ok := entry["hooks"].([]any)
	if !ok {
		t.Fatal("entry missing 'hooks' array")
	}
	if len(hooksList) != 1 {
		t.Fatalf("hooks array has %d entries, want 1", len(hooksList))
	}

	hookEntry, ok := hooksList[0].(map[string]any)
	if !ok {
		t.Fatal("hooks[0] is not a map")
	}
	if hookEntry["type"] != "command" {
		t.Errorf("type = %v, want %q", hookEntry["type"], "command")
	}
	if hookEntry["command"] != "/usr/local/bin/angry-bear hook claude" {
		t.Errorf("command = %v, want %q", hookEntry["command"], "/usr/local/bin/angry-bear hook claude")
	}
}

func TestClaudeInstallHook_MalformedExistingJSON(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	// Write malformed JSON
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{not valid json!"), 0o644); err != nil {
		t.Fatalf("failed to write malformed settings: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "angry-bear"}
	err := adapter.InstallHook(tmpDir)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing existing settings.json") {
		t.Errorf("error = %q, want it to mention parsing failure", err.Error())
	}
}

func TestClaudeInstallHook_TrailingNewline(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "angry-bear"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	if !strings.HasSuffix(string(data), "\n") {
		t.Error("settings.json does not end with trailing newline")
	}
}

// --- GlobalConfigPath tests ---

func TestClaudeGlobalConfigPath_WithHomeDir(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{HomeDir: "/custom/home"}
	got := adapter.GlobalConfigPath()
	want := filepath.Join("/custom/home", ".claude", "settings.json")
	if got != want {
		t.Errorf("GlobalConfigPath() = %q, want %q", got, want)
	}
}

// --- ScanProjects tests ---

func TestClaudeScanProjects_EmptyProjectsDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create the projects dir but leave it empty
	projectsDir := filepath.Join(tmpDir, ".claude", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("failed to create projects dir: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	projects, err := adapter.ScanProjects()
	if err != nil {
		t.Fatalf("ScanProjects failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0", len(projects))
	}
}

func TestClaudeScanProjects_MissingProjectsDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	// Don't create .claude/projects/ at all

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	projects, err := adapter.ScanProjects()
	if err != nil {
		t.Fatalf("ScanProjects failed: %v", err)
	}
	if projects != nil {
		t.Errorf("got %v, want nil for missing projects dir", projects)
	}
}

// --- ExitCodeForDeny tests ---

func TestClaudeExitCodeForDeny_ReturnsZero(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	// Claude Code reads deny decision from stdout JSON, so the exit code is 0
	// even when denying. This is fundamental to the Claude hook protocol.
	got := adapter.ExitCodeForDeny()
	if got != 0 {
		t.Errorf("ExitCodeForDeny() = %d, want 0 (Claude reads deny from stdout JSON, not exit code)", got)
	}
}

// --- UninstallHook tests ---

func TestClaudeUninstallHook_RemovesCareBareEntry(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	// Write settings with both a angry-bear hook and an unrelated hook
	existing := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {"type": "command", "command": "some-linter.sh"}
        ]
      },
      {
        "matcher": "*",
        "hooks": [
          {"type": "command", "command": "/usr/local/bin/angry-bear hook claude"}
        ]
      }
    ]
  },
  "enabledPlugins": {
    "linear@claude-plugins-official": true
  }
}`
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to write settings: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	if err := adapter.UninstallHook(); err != nil {
		t.Fatalf("UninstallHook failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings after uninstall: %v", err)
	}

	content := string(data)
	// angry-bear hook must be removed
	if strings.Contains(content, "angry-bear hook") {
		t.Errorf("angry-bear hook still present after UninstallHook:\n%s", content)
	}
	// Unrelated linter hook must be preserved
	if !strings.Contains(content, "some-linter.sh") {
		t.Errorf("unrelated hook was removed by UninstallHook:\n%s", content)
	}
	// Other config keys must be preserved
	if !strings.Contains(content, "linear@claude-plugins-official") {
		t.Errorf("enabledPlugins was removed by UninstallHook:\n%s", content)
	}
}

func TestClaudeUninstallHook_NoSettingsFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	// No .claude/settings.json exists -- should not error

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	err := adapter.UninstallHook()
	if err != nil {
		t.Fatalf("UninstallHook should succeed when settings.json does not exist, got: %v", err)
	}
}

func TestClaudeUninstallHook_NoHooksSection(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	// Settings file with no hooks section at all
	existing := `{"enabledPlugins": {"some-plugin": true}}`
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to write settings: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	err := adapter.UninstallHook()
	if err != nil {
		t.Fatalf("UninstallHook should succeed with no hooks section, got: %v", err)
	}
}

func TestClaudeUninstallHook_MalformedJSON(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{broken json!!"), 0o644); err != nil {
		t.Fatalf("failed to write malformed settings: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	err := adapter.UninstallHook()
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing settings.json") {
		t.Errorf("error = %q, want it to mention parsing failure", err.Error())
	}
}

func TestClaudeUninstallHook_RoundTrip(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Install hook first, then uninstall -- verify clean removal
	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "angry-bear"}
	if err := adapter.InstallHook(""); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	// Verify hook was installed
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not found after install: %v", err)
	}
	if !strings.Contains(string(data), "angry-bear hook") {
		t.Fatal("angry-bear hook not found after install")
	}

	// Uninstall
	if err := adapter.UninstallHook(); err != nil {
		t.Fatalf("UninstallHook failed: %v", err)
	}

	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not found after uninstall: %v", err)
	}
	if strings.Contains(string(data), "angry-bear hook") {
		t.Errorf("angry-bear hook still present after uninstall:\n%s", data)
	}

	// Verify the file is still valid JSON
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON after uninstall: %v", err)
	}
}

func TestClaudeUninstallHook_TrailingNewline(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "angry-bear"}
	if err := adapter.InstallHook(""); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}
	if err := adapter.UninstallHook(); err != nil {
		t.Fatalf("UninstallHook failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("settings.json does not end with trailing newline after uninstall")
	}
}

// --- GlobalConfigPath additional tests ---

func TestClaudeGlobalConfigPath_DefaultFallback(t *testing.T) {
	t.Parallel()
	// When HomeDir is empty and os.UserHomeDir() works, the path should include
	// the home directory. We can only test the format here since os.UserHomeDir()
	// is system-dependent.
	adapter := &ClaudeAdapter{}
	got := adapter.GlobalConfigPath()

	// The path must end with .claude/settings.json regardless of prefix
	wantSuffix := filepath.Join(".claude", "settings.json")
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("GlobalConfigPath() = %q, want suffix %q", got, wantSuffix)
	}

	// The path should be absolute (either from os.UserHomeDir or the fallback)
	if got == wantSuffix {
		// This means os.UserHomeDir() failed, which is unlikely in a test
		// but still valid behavior (returns relative path as fallback)
		return
	}
	if !filepath.IsAbs(got) {
		t.Errorf("GlobalConfigPath() = %q, want absolute path", got)
	}
}

func TestClaudeScanProjects_WithSessionsIndex(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a real project directory that ScanProjects can discover
	projectPath := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create encoded project dir under .claude/projects/
	encodedDir := filepath.Join(tmpDir, ".claude", "projects", "encoded-project")
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatalf("failed to create encoded dir: %v", err)
	}

	// Write sessions-index.json pointing to the real project
	indexJSON := `{"entries":[{"projectPath":"` + projectPath + `"}]}`
	indexPath := filepath.Join(encodedDir, "sessions-index.json")
	if err := os.WriteFile(indexPath, []byte(indexJSON), 0o644); err != nil {
		t.Fatalf("failed to write sessions-index.json: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	projects, err := adapter.ScanProjects()
	if err != nil {
		t.Fatalf("ScanProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if projects[0].Path != projectPath {
		t.Errorf("Path = %q, want %q", projects[0].Path, projectPath)
	}
	if projects[0].Agent != "claude" {
		t.Errorf("Agent = %q, want %q", projects[0].Agent, "claude")
	}
	if projects[0].Name != "myproject" {
		t.Errorf("Name = %q, want %q", projects[0].Name, "myproject")
	}
	if projects[0].Encoded != "encoded-project" {
		t.Errorf("Encoded = %q, want %q", projects[0].Encoded, "encoded-project")
	}
}

func TestClaudeScanProjects_SkipsNonexistentProjectPaths(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create an encoded project dir with sessions-index pointing to a nonexistent path
	encodedDir := filepath.Join(tmpDir, ".claude", "projects", "gone-project")
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatalf("failed to create encoded dir: %v", err)
	}

	indexJSON := `{"entries":[{"projectPath":"/nonexistent/path/that/does/not/exist"}]}`
	if err := os.WriteFile(filepath.Join(encodedDir, "sessions-index.json"), []byte(indexJSON), 0o644); err != nil {
		t.Fatalf("failed to write sessions-index.json: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	projects, err := adapter.ScanProjects()
	if err != nil {
		t.Fatalf("ScanProjects failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0 (nonexistent path should be skipped)", len(projects))
	}
}

func TestClaudeScanProjects_DeduplicatesSamePath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a real project directory
	projectPath := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create two encoded dirs both pointing to the same project
	for _, name := range []string{"encoded-a", "encoded-b"} {
		encodedDir := filepath.Join(tmpDir, ".claude", "projects", name)
		if err := os.MkdirAll(encodedDir, 0o755); err != nil {
			t.Fatalf("failed to create encoded dir: %v", err)
		}
		indexJSON := `{"entries":[{"projectPath":"` + projectPath + `"}]}`
		if err := os.WriteFile(filepath.Join(encodedDir, "sessions-index.json"), []byte(indexJSON), 0o644); err != nil {
			t.Fatalf("failed to write sessions-index.json: %v", err)
		}
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	projects, err := adapter.ScanProjects()
	if err != nil {
		t.Fatalf("ScanProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("got %d projects, want 1 (duplicates should be merged)", len(projects))
	}
}

func TestClaudeScanProjects_GreedyDecodeWithoutIndex(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a real project directory
	projectPath := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create an encoded project dir WITHOUT sessions-index.json.
	// The scanner should fall back to greedy decode.
	// Encoded name: all path components joined by "-", leading "-"
	pathWithoutSlash := projectPath[1:] // strip leading "/"
	encodedName := "-" + strings.ReplaceAll(pathWithoutSlash, "/", "-")
	encodedDir := filepath.Join(tmpDir, ".claude", "projects", encodedName)
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatalf("failed to create encoded dir: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	projects, err := adapter.ScanProjects()
	if err != nil {
		t.Fatalf("ScanProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1 (greedy decode should find it)", len(projects))
	}
	if projects[0].Path != projectPath {
		t.Errorf("Path = %q, want %q", projects[0].Path, projectPath)
	}
}

func TestClaudeScanProjects_SkipsNonDirEntries(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create the projects dir with a file (not a directory)
	projectsDir := filepath.Join(tmpDir, ".claude", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("failed to create projects dir: %v", err)
	}
	// Create a regular file that should be ignored
	if err := os.WriteFile(filepath.Join(projectsDir, "not-a-dir.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	adapter := &ClaudeAdapter{HomeDir: tmpDir}
	projects, err := adapter.ScanProjects()
	if err != nil {
		t.Fatalf("ScanProjects failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0 (files should be skipped)", len(projects))
	}
}

// --- DetectSkillInvocation edge cases ---

func TestClaudeDetectSkillInvocation_MissingToolInput(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}
	input := &HookInput{
		Agent:    "claude",
		ToolName: "Skill",
		RawInput: map[string]any{
			// No tool_input key at all
		},
	}

	skillName, isSkill := adapter.DetectSkillInvocation(input)
	if isSkill {
		t.Fatal("expected isSkill=false when tool_input is missing")
	}
	if skillName != "" {
		t.Errorf("skillName = %q, want empty", skillName)
	}
}

func TestClaudeDetectSkillInvocation_MissingSkillField(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}
	input := &HookInput{
		Agent:    "claude",
		ToolName: "Skill",
		RawInput: map[string]any{
			"tool_input": map[string]any{
				// tool_input present but no "skill" key
				"args": "some-args",
			},
		},
	}

	skillName, isSkill := adapter.DetectSkillInvocation(input)
	if isSkill {
		t.Fatal("expected isSkill=false when skill field is missing from tool_input")
	}
	if skillName != "" {
		t.Errorf("skillName = %q, want empty", skillName)
	}
}

func TestClaudeDetectSkillInvocation_SkillFieldIsNotString(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}
	input := &HookInput{
		Agent:    "claude",
		ToolName: "Skill",
		RawInput: map[string]any{
			"tool_input": map[string]any{
				"skill": 42, // not a string
			},
		},
	}

	skillName, isSkill := adapter.DetectSkillInvocation(input)
	if isSkill {
		t.Fatal("expected isSkill=false when skill field is not a string")
	}
	if skillName != "" {
		t.Errorf("skillName = %q, want empty", skillName)
	}
}

func TestClaudeDetectSkillInvocation_ToolInputIsNotMap(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}
	input := &HookInput{
		Agent:    "claude",
		ToolName: "Skill",
		RawInput: map[string]any{
			"tool_input": "not a map",
		},
	}

	skillName, isSkill := adapter.DetectSkillInvocation(input)
	if isSkill {
		t.Fatal("expected isSkill=false when tool_input is not a map")
	}
	if skillName != "" {
		t.Errorf("skillName = %q, want empty", skillName)
	}
}

// --- ParseInput edge cases ---

func TestClaudeParseInput_GrepPathField(t *testing.T) {
	t.Parallel()
	// Grep/Glob use "path" instead of "file_path"
	jsonStr := `{"hook_event_name":"PreToolUse","session_id":"x","tool_name":"Grep","tool_input":{"path":"src/","pattern":"TODO"}}`
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "src/" {
		t.Errorf("FilePath = %q, want %q (should extract from tool_input.path)", input.FilePath, "src/")
	}
}

func TestClaudeParseInput_EmptyJSON(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	input, err := adapter.ParseInput(strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", input.SessionID)
	}
	if input.ToolName != "" {
		t.Errorf("ToolName = %q, want empty", input.ToolName)
	}
	if input.Agent != "claude" {
		t.Errorf("Agent = %q, want %q", input.Agent, "claude")
	}
}

// --- resolveCareBareCommand tests ---

func TestResolveCareBareCommand_WithExplicitPath(t *testing.T) {
	t.Parallel()
	result := resolveCareBareCommand("/explicit/path/angry-bear")
	if result != "/explicit/path/angry-bear" {
		t.Errorf("resolveCareBareCommand = %q, want %q", result, "/explicit/path/angry-bear")
	}
}

func TestResolveCareBareCommand_EmptyFallsBackToExecutable(t *testing.T) {
	t.Parallel()
	// When empty string is passed, it should resolve to os.Executable or "angry-bear"
	result := resolveCareBareCommand("")
	if result == "" {
		t.Error("resolveCareBareCommand returned empty string")
	}
	// Should return either an absolute path or "angry-bear" fallback
	if result != "angry-bear" && !filepath.IsAbs(result) {
		t.Errorf("resolveCareBareCommand = %q, expected absolute path or 'angry-bear'", result)
	}
}

// --- readProjectPathFromIndex tests ---

func TestReadProjectPathFromIndex_ValidIndex(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "sessions-index.json")

	indexJSON := `{"entries":[{"projectPath":"/home/user/myproject","sessionId":"s1"}]}`
	if err := os.WriteFile(indexPath, []byte(indexJSON), 0o644); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	got := readProjectPathFromIndex(indexPath)
	if got != "/home/user/myproject" {
		t.Errorf("readProjectPathFromIndex = %q, want %q", got, "/home/user/myproject")
	}
}

func TestReadProjectPathFromIndex_MissingFile(t *testing.T) {
	t.Parallel()
	got := readProjectPathFromIndex("/nonexistent/sessions-index.json")
	if got != "" {
		t.Errorf("readProjectPathFromIndex = %q, want empty for missing file", got)
	}
}

func TestReadProjectPathFromIndex_MalformedJSON(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "sessions-index.json")
	if err := os.WriteFile(indexPath, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	got := readProjectPathFromIndex(indexPath)
	if got != "" {
		t.Errorf("readProjectPathFromIndex = %q, want empty for malformed JSON", got)
	}
}

func TestReadProjectPathFromIndex_EmptyEntries(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "sessions-index.json")
	if err := os.WriteFile(indexPath, []byte(`{"entries":[]}`), 0o644); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	got := readProjectPathFromIndex(indexPath)
	if got != "" {
		t.Errorf("readProjectPathFromIndex = %q, want empty for no entries", got)
	}
}

func TestReadProjectPathFromIndex_EmptyProjectPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "sessions-index.json")
	if err := os.WriteFile(indexPath, []byte(`{"entries":[{"projectPath":""}]}`), 0o644); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	got := readProjectPathFromIndex(indexPath)
	if got != "" {
		t.Errorf("readProjectPathFromIndex = %q, want empty for empty projectPath", got)
	}
}

func TestReadProjectPathFromIndex_MultipleEntries_ReturnsFirst(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "sessions-index.json")
	indexJSON := `{"entries":[{"projectPath":""},{"projectPath":"/second/path"}]}`
	if err := os.WriteFile(indexPath, []byte(indexJSON), 0o644); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	got := readProjectPathFromIndex(indexPath)
	if got != "/second/path" {
		t.Errorf("readProjectPathFromIndex = %q, want %q (should skip empty, return first non-empty)", got, "/second/path")
	}
}

// --- careBareHookExists tests ---

func TestCareBareHookExists_EmptyArray(t *testing.T) {
	t.Parallel()
	if careBareHookExists(nil) {
		t.Error("expected false for nil array")
	}
	if careBareHookExists([]any{}) {
		t.Error("expected false for empty array")
	}
}

func TestCareBareHookExists_MalformedEntries(t *testing.T) {
	t.Parallel()
	// Array with non-map entries -- should not panic
	entries := []any{
		"a string, not a map",
		42,
		nil,
	}
	if careBareHookExists(entries) {
		t.Error("expected false for array with non-map entries")
	}
}

func TestCareBareHookExists_EntryWithoutHooksList(t *testing.T) {
	t.Parallel()
	entries := []any{
		map[string]any{
			"matcher": "*",
			// "hooks" key missing
		},
	}
	if careBareHookExists(entries) {
		t.Error("expected false when entry has no hooks list")
	}
}

func TestCareBareHookExists_HooksListWithNonMapEntries(t *testing.T) {
	t.Parallel()
	entries := []any{
		map[string]any{
			"matcher": "*",
			"hooks":   []any{"not a map", 42},
		},
	}
	if careBareHookExists(entries) {
		t.Error("expected false when hooks contain non-map entries")
	}
}

func TestCareBareHookExists_FindsExistingHook(t *testing.T) {
	t.Parallel()
	entries := []any{
		map[string]any{
			"matcher": "*",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "/usr/bin/angry-bear hook claude",
				},
			},
		},
	}
	if !careBareHookExists(entries) {
		t.Error("expected true when angry-bear hook exists")
	}
}

func TestCareBareHookExists_IgnoresUnrelatedHooks(t *testing.T) {
	t.Parallel()
	entries := []any{
		map[string]any{
			"matcher": "Bash",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "some-other-tool --check",
				},
			},
		},
	}
	if careBareHookExists(entries) {
		t.Error("expected false when no angry-bear hook is present")
	}
}

// --- greedyDecodeDirName tests ---

func TestGreedyDecodeDirName_SimpleDecodeSuccess(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a directory structure that matches the "simple decode" path.
	// Simple decode: strip leading dash, replace all remaining dashes with /.
	// For an encoded name "-a-b-c", simple decode produces "/a/b/c".
	targetDir := filepath.Join(tmpDir, "a", "b", "c")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	// Encoded name "-{tmpDir without leading /}-a-b-c", but since tmpDir varies,
	// let's construct the encoded name from tmpDir.
	// Simple decode: "/" + replace("-", "/") of trimmed string.
	// So we need the encoded form that, after TrimPrefix("-") and ReplaceAll("-","/"),
	// gives us the tmpDir path + "/a/b/c".
	// Easier: build the encoded name as the path with "/" replaced by "-" and leading "-".
	pathWithoutLeadingSlash := targetDir[1:] // remove leading "/"
	encoded := "-" + strings.ReplaceAll(pathWithoutLeadingSlash, "/", "-")

	result := greedyDecodeDirName(encoded)
	if result != targetDir {
		t.Errorf("greedyDecodeDirName(%q) = %q, want %q", encoded, result, targetDir)
	}
}

func TestGreedyDecodeDirName_GreedyFallback(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a hyphenated directory: the simple decode won't find it
	// because simple decode splits ALL dashes into path separators.
	// Create: tmpDir/my-project (hyphenated dir name)
	targetDir := filepath.Join(tmpDir, "my-project")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	// For the encoded name, we need the greedy path to discover it.
	// The encoded form would be all path components joined by "-".
	// tmpDir is something like /tmp/TestXYZ/001. Encoded:
	// Strip leading /, replace / with -, prepend -: -tmp-TestXYZ-001-my-project
	pathWithoutLeadingSlash := targetDir[1:]
	encoded := "-" + strings.ReplaceAll(pathWithoutLeadingSlash, "/", "-")

	// Simple decode will fail because it replaces ALL "-" with "/"
	// which would split "my-project" into "my/project" which doesn't exist.
	// So greedyDecodeDirName should fall through to greedy mode.
	result := greedyDecodeDirName(encoded)
	if result != targetDir {
		t.Errorf("greedyDecodeDirName(%q) = %q, want %q", encoded, result, targetDir)
	}
}

func TestGreedyDecodeDirName_NoLeadingDash(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Cursor format: no leading dash (e.g., "Users-orr-project")
	targetDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	// Encoded without leading dash -- path components from tmpDir + "sub"
	pathWithoutLeadingSlash := targetDir[1:]
	encoded := strings.ReplaceAll(pathWithoutLeadingSlash, "/", "-")

	result := greedyDecodeDirName(encoded)
	if result != targetDir {
		t.Errorf("greedyDecodeDirName(%q) = %q, want %q", encoded, result, targetDir)
	}
}

func TestGreedyDecodeDirName_NonexistentPath(t *testing.T) {
	t.Parallel()
	result := greedyDecodeDirName("-nonexistent-path-that-does-not-exist-anywhere")
	if result != "" {
		t.Errorf("greedyDecodeDirName = %q, want empty for nonexistent path", result)
	}
}

func TestGreedyBuildPath_EmptyParts(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// With no parts, should return prefix if it exists
	result := greedyBuildPath(tmpDir, []string{})
	if result != tmpDir {
		t.Errorf("greedyBuildPath with empty parts = %q, want %q", result, tmpDir)
	}
}

func TestGreedyBuildPath_NonexistentPath(t *testing.T) {
	t.Parallel()
	result := greedyBuildPath("/nonexistent", []string{"abc", "def"})
	if result != "" {
		t.Errorf("greedyBuildPath for nonexistent = %q, want empty", result)
	}
}

func TestGreedyBuildPath_HyphenatedDirectoryName(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a directory with a hyphen in its name: "my-project"
	targetDir := filepath.Join(tmpDir, "my-project")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	// Parts ["my", "project"] should be joined to "my-project" and matched
	result := greedyBuildPath(tmpDir, []string{"my", "project"})
	if result != targetDir {
		t.Errorf("greedyBuildPath = %q, want %q (should join hyphenated parts)", result, targetDir)
	}
}

func TestGreedyBuildPath_NestedHyphenatedDirs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create nested path: work/my-project/src
	targetDir := filepath.Join(tmpDir, "work", "my-project", "src")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	result := greedyBuildPath(tmpDir, []string{"work", "my", "project", "src"})
	if result != targetDir {
		t.Errorf("greedyBuildPath = %q, want %q", result, targetDir)
	}
}

func TestGreedyBuildPath_RealDashPreferredOverDot(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Both my-project (real dashes) and my.project (dots) exist.
	// The as-is variant (real dashes) should win because it's tried first.
	dashDir := filepath.Join(tmpDir, "my-project")
	dotDir := filepath.Join(tmpDir, "my.project")
	if err := os.MkdirAll(dashDir, 0o755); err != nil {
		t.Fatalf("failed to create dash dir: %v", err)
	}
	if err := os.MkdirAll(dotDir, 0o755); err != nil {
		t.Fatalf("failed to create dot dir: %v", err)
	}

	result := greedyBuildPath(tmpDir, []string{"my", "project"})
	if result != dashDir {
		t.Errorf("greedyBuildPath = %q, want %q (real dashes should be preferred)", result, dashDir)
	}
}

func TestGreedyBuildPath_DottedDirectoryName(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create path with dots: Users/amir.shaked/Development/blueden
	targetDir := filepath.Join(tmpDir, "Users", "amir.shaked", "Development", "blueden")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	// Encoded as: Users-amir-shaked-Development-blueden
	result := greedyBuildPath(tmpDir, []string{"Users", "amir", "shaked", "Development", "blueden"})
	if result != targetDir {
		t.Errorf("greedyBuildPath = %q, want %q", result, targetDir)
	}
}

func TestGreedyDecodeDirName_DottedUsername(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Simulate /Users/amir.shaked/dev/blueden
	targetDir := filepath.Join(tmpDir, "amir.shaked", "dev", "blueden")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	// Encoded directory name (without leading tmpDir prefix — test greedyBuildPath directly)
	result := greedyBuildPath(tmpDir, []string{"amir", "shaked", "dev", "blueden"})
	if result != targetDir {
		t.Errorf("greedyBuildPath = %q, want %q", result, targetDir)
	}
}

// --- ConfigPath test ---

func TestClaudeConfigPath(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}
	if adapter.ConfigPath() != ".claude/settings.json" {
		t.Errorf("ConfigPath() = %q, want %q", adapter.ConfigPath(), ".claude/settings.json")
	}
}

// --- Name test ---

func TestClaudeName(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}
	if adapter.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "claude")
	}
}
