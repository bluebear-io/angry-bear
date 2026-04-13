// cursor_test.go contains comprehensive tests for the Cursor IDE adapter.
package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- ParseInput tests ---

func TestCursorParseInput_SessionID(t *testing.T) {
	fixture := loadFixture(t, "cursor_beforefileedit.json")
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.SessionID != "conv-abc-123" {
		t.Errorf("SessionID = %q, want %q", input.SessionID, "conv-abc-123")
	}
}

func TestCursorParseInput_UsesConversationIDNotSessionID(t *testing.T) {
	// Verify that conversation_id is used, not session_id
	jsonStr := `{"hook_event_name":"beforeFileEdit","conversation_id":"conv-from-cursor","session_id":"should-not-use","cursor_version":"0.48.1","workspace_roots":[]}`
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.SessionID != "conv-from-cursor" {
		t.Errorf("SessionID = %q, want %q (should use conversation_id, not session_id)", input.SessionID, "conv-from-cursor")
	}
}

func TestCursorParseInput_ToolName(t *testing.T) {
	fixture := loadFixture(t, "cursor_beforefileedit.json")
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.ToolName != "Edit" {
		t.Errorf("ToolName = %q, want %q (normalized from edit_file)", input.ToolName, "Edit")
	}
}

func TestCursorParseInput_FilePath(t *testing.T) {
	fixture := loadFixture(t, "cursor_beforefileedit.json")
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "internal/engine/types.go" {
		t.Errorf("FilePath = %q, want %q", input.FilePath, "internal/engine/types.go")
	}
}

func TestCursorParseInput_CwdFromWorkspaceRoots(t *testing.T) {
	fixture := loadFixture(t, "cursor_beforefileedit.json")
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Cwd != "/home/user/project" {
		t.Errorf("Cwd = %q, want %q", input.Cwd, "/home/user/project")
	}
}

func TestCursorParseInput_CwdUsesFirstWorkspaceRoot(t *testing.T) {
	// Multiple workspace roots -- should use the first one
	jsonStr := `{"hook_event_name":"beforeFileEdit","conversation_id":"x","cursor_version":"0.48.1","workspace_roots":["/home/user/project","/other/root"]}`
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Cwd != "/home/user/project" {
		t.Errorf("Cwd = %q, want %q (should use first workspace root)", input.Cwd, "/home/user/project")
	}
}

func TestCursorParseInput_EmptyWorkspaceRoots(t *testing.T) {
	jsonStr := `{"hook_event_name":"beforeFileEdit","conversation_id":"x","cursor_version":"0.48.1","workspace_roots":[]}`
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Cwd != "" {
		t.Errorf("Cwd = %q, want empty string for empty workspace_roots", input.Cwd)
	}
}

func TestCursorParseInput_SetsAgentCursor(t *testing.T) {
	fixture := loadFixture(t, "cursor_beforefileedit.json")
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Agent != "cursor" {
		t.Errorf("Agent = %q, want %q", input.Agent, "cursor")
	}
}

func TestCursorParseInput_ShellExecutionCommand(t *testing.T) {
	fixture := loadFixture(t, "cursor_beforeshellexecution.json")
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Command is a top-level field in Cursor, preserved in RawInput
	cmd, ok := input.RawInput["command"].(string)
	if !ok || cmd != "go test ./..." {
		t.Errorf("RawInput[command] = %q, want %q", cmd, "go test ./...")
	}
}

func TestCursorParseInput_PreToolUse(t *testing.T) {
	fixture := loadFixture(t, "cursor_pretooluse.json")
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.ToolName != "Read" {
		t.Errorf("ToolName = %q, want %q (normalized from read_file)", input.ToolName, "Read")
	}
	if input.FilePath != "README.md" {
		t.Errorf("FilePath = %q, want %q", input.FilePath, "README.md")
	}
	if input.SessionID != "conv-ghi-789" {
		t.Errorf("SessionID = %q, want %q", input.SessionID, "conv-ghi-789")
	}
}

func TestCursorParseInput_MissingOptionalFields(t *testing.T) {
	// Minimal valid Cursor JSON -- only required fields
	jsonStr := `{"hook_event_name":"beforeFileEdit","conversation_id":"x","cursor_version":"0.48.1"}`
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "" {
		t.Errorf("FilePath = %q, want empty", input.FilePath)
	}
	if input.ToolName != "Edit" {
		t.Errorf("ToolName = %q, want %q (derived from hook_event_name)", input.ToolName, "Edit")
	}
	if input.Cwd != "" {
		t.Errorf("Cwd = %q, want empty", input.Cwd)
	}
}

func TestCursorParseInput_InvalidJSON(t *testing.T) {
	adapter := &CursorAdapter{}

	_, err := adapter.ParseInput(strings.NewReader("not json at all"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestCursorParseInput_PreservesRawInput(t *testing.T) {
	fixture := loadFixture(t, "cursor_beforefileedit.json")
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(string(fixture)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify cursor_version is preserved in RawInput
	cv, ok := input.RawInput["cursor_version"].(string)
	if !ok || cv != "0.48.1" {
		t.Errorf("RawInput[cursor_version] = %q, want %q", cv, "0.48.1")
	}
	// Verify generation_id is preserved in RawInput
	gid, ok := input.RawInput["generation_id"].(string)
	if !ok || gid != "gen-456" {
		t.Errorf("RawInput[generation_id] = %q, want %q", gid, "gen-456")
	}
}

// --- FormatAllow / FormatDeny tests ---

func TestCursorFormatAllow_ReturnsContinueTrue(t *testing.T) {
	adapter := &CursorAdapter{}

	result, err := adapter.FormatAllow()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("FormatAllow returned invalid JSON: %v", err)
	}
	if parsed["continue"] != true {
		t.Errorf("continue = %v, want true", parsed["continue"])
	}
}

func TestCursorFormatDeny_ReturnsContinueFalse(t *testing.T) {
	adapter := &CursorAdapter{}

	result, err := adapter.FormatDeny("some reason")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("FormatDeny returned invalid JSON: %v", err)
	}
	if parsed["continue"] != false {
		t.Errorf("continue = %v, want false", parsed["continue"])
	}
}

func TestCursorFormatDeny_IncludesUserMessage(t *testing.T) {
	adapter := &CursorAdapter{}
	reason := "missing skill: linear"

	result, err := adapter.FormatDeny(reason)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("FormatDeny returned invalid JSON: %v", err)
	}
	if parsed["userMessage"] != reason {
		t.Errorf("userMessage = %v, want %q", parsed["userMessage"], reason)
	}
}

// --- DetectSkillInvocation tests ---

func TestCursorDetectSkillInvocation_AlwaysReturnsFalse(t *testing.T) {
	adapter := &CursorAdapter{}
	// Cursor does not have a native Skill tool, so this always returns false
	input := &HookInput{
		Agent:    "cursor",
		ToolName: "edit_file",
		FilePath: "some/file.go",
		RawInput: map[string]any{
			"tool_name": "edit_file",
		},
	}

	skillName, isSkill := adapter.DetectSkillInvocation(input)
	if isSkill {
		t.Fatal("expected isSkill=false for Cursor adapter")
	}
	if skillName != "" {
		t.Errorf("skillName = %q, want empty", skillName)
	}
}

func TestCursorDetectSkillInvocation_FalseEvenWithSkillLikeName(t *testing.T) {
	adapter := &CursorAdapter{}
	// Even if tool_name happened to be "Skill", Cursor doesn't have the concept
	input := &HookInput{
		Agent:    "cursor",
		ToolName: "Skill",
		RawInput: map[string]any{
			"tool_name": "Skill",
		},
	}

	skillName, isSkill := adapter.DetectSkillInvocation(input)
	if isSkill {
		t.Fatal("expected isSkill=false for Cursor adapter even with 'Skill' tool_name")
	}
	if skillName != "" {
		t.Errorf("skillName = %q, want empty", skillName)
	}
}

// --- InstallHook tests ---

func TestCursorInstallHook_CreatesHooksJsonWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	hooksPath := filepath.Join(cursorDir, "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("hooks.json not created: %v", err)
	}

	// Verify it contains the care-bare hook entry
	if !strings.Contains(string(data), "care-bare hook --agent cursor") {
		t.Errorf("hooks.json missing care-bare hook command:\n%s", data)
	}

	// Verify it contains version field
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("hooks.json is invalid JSON: %v", err)
	}
	if config["version"] != float64(1) {
		t.Errorf("version = %v, want 1", config["version"])
	}
}

func TestCursorInstallHook_RegistersAllRequiredHookTypes(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	hooksPath := filepath.Join(cursorDir, "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		t.Fatal("missing hooks object in config")
	}

	// All required hook types must be present
	requiredTypes := []string{"preToolUse", "beforeFileEdit", "beforeShellExecution", "beforeReadFile", "beforeMCPExecution"}
	for _, hookType := range requiredTypes {
		arr, ok := hooks[hookType].([]any)
		if !ok {
			t.Errorf("hook type %q not found or not an array", hookType)
			continue
		}
		if len(arr) == 0 {
			t.Errorf("hook type %q is empty", hookType)
		}
	}
}

func TestCursorInstallHook_PreservesExistingHooks(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}

	// Write existing hooks.json with an existing hook from another tool
	existing := `{
  "version": 1,
  "hooks": {
    "preToolUse": [
      {"command": "some-other-tool --check"}
    ],
    "beforeFileEdit": [
      {"command": "lint-on-save.sh"}
    ]
  }
}`
	hooksPath := filepath.Join(cursorDir, "hooks.json")
	if err := os.WriteFile(hooksPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to write existing hooks.json: %v", err)
	}

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	content := string(data)
	// Existing hooks must still be there
	if !strings.Contains(content, "some-other-tool --check") {
		t.Errorf("existing preToolUse hook was removed:\n%s", content)
	}
	if !strings.Contains(content, "lint-on-save.sh") {
		t.Errorf("existing beforeFileEdit hook was removed:\n%s", content)
	}
	// care-bare hook must be present
	if !strings.Contains(content, "care-bare hook --agent cursor") {
		t.Errorf("care-bare hook not added:\n%s", content)
	}
}

func TestCursorInstallHook_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}

	// Install twice
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("first InstallHook failed: %v", err)
	}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("second InstallHook failed: %v", err)
	}

	hooksPath := filepath.Join(cursorDir, "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	// Count occurrences of "care-bare hook" -- should be exactly 5 (one per hook type)
	count := strings.Count(string(data), "care-bare hook --agent cursor")
	if count != 5 {
		t.Errorf("care-bare hook appears %d times, want 5 (one per hook type):\n%s", count, data)
	}
}

func TestCursorInstallHook_CreatesCursorDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't pre-create .cursor dir -- InstallHook should handle it

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	hooksPath := filepath.Join(tmpDir, ".cursor", "hooks.json")
	if _, err := os.Stat(hooksPath); os.IsNotExist(err) {
		t.Fatal("hooks.json not created when .cursor dir was missing")
	}
}

func TestCursorInstallHook_CareBareIsPrepended(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}

	// Write existing hooks.json with an existing hook
	existing := `{
  "version": 1,
  "hooks": {
    "preToolUse": [
      {"command": "existing-tool"}
    ]
  }
}`
	hooksPath := filepath.Join(cursorDir, "hooks.json")
	if err := os.WriteFile(hooksPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to write existing hooks.json: %v", err)
	}

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bare"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hooks := config["hooks"].(map[string]any)
	preToolUse := hooks["preToolUse"].([]any)

	// care-bare should be the first entry (prepended)
	firstEntry := preToolUse[0].(map[string]any)
	if !strings.Contains(firstEntry["command"].(string), "care-bare hook") {
		t.Errorf("care-bare hook is not the first entry in preToolUse, first is: %v", firstEntry)
	}

	// existing-tool should be the second entry
	if len(preToolUse) < 2 {
		t.Fatalf("expected at least 2 entries in preToolUse, got %d", len(preToolUse))
	}
	secondEntry := preToolUse[1].(map[string]any)
	if secondEntry["command"] != "existing-tool" {
		t.Errorf("second entry = %v, want existing-tool", secondEntry["command"])
	}
}

// --- ConfigPath test ---

func TestCursorConfigPath(t *testing.T) {
	adapter := &CursorAdapter{}
	if adapter.ConfigPath() != ".cursor/hooks.json" {
		t.Errorf("ConfigPath() = %q, want %q", adapter.ConfigPath(), ".cursor/hooks.json")
	}
}
