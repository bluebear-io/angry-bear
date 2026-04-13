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

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	// Verify it contains the care-bare hook entry
	if !strings.Contains(string(data), "care-bare hook --agent claude") {
		t.Errorf("settings.json missing care-bare hook command:\n%s", data)
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

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}
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
	// care-bare hook must be present
	if !strings.Contains(content, "care-bare hook --agent claude") {
		t.Errorf("care-bare hook not added:\n%s", content)
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

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}

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

	// Count occurrences of "care-bare hook" -- should be exactly 1
	count := strings.Count(string(data), "care-bare hook --agent claude")
	if count != 1 {
		t.Errorf("care-bare hook appears %d times, want 1:\n%s", count, data)
	}
}

func TestClaudeInstallHook_CreatesClaudeDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't pre-create .claude dir -- InstallHook should handle it

	adapter := &ClaudeAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Fatal("settings.json not created when .claude dir was missing")
	}
}

// --- ConfigPath test ---

func TestClaudeConfigPath(t *testing.T) {
	adapter := &ClaudeAdapter{}
	if adapter.ConfigPath() != ".claude/settings.json" {
		t.Errorf("ConfigPath() = %q, want %q", adapter.ConfigPath(), ".claude/settings.json")
	}
}
