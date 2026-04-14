// cursor_test.go contains comprehensive tests for the Cursor IDE adapter.
package adapter

import (
	"encoding/json"
	"fmt"
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

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bear"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	hooksPath := filepath.Join(cursorDir, "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("hooks.json not created: %v", err)
	}

	// Verify it contains the care-bear hook entry
	if !strings.Contains(string(data), "care-bear hook cursor") {
		t.Errorf("hooks.json missing care-bear hook command:\n%s", data)
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

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bear"}
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
	requiredTypes := []string{"preToolUse"}
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

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bear"}
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
	// care-bear hook must be present
	if !strings.Contains(content, "care-bear hook cursor") {
		t.Errorf("care-bear hook not added:\n%s", content)
	}
}

func TestCursorInstallHook_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bear"}

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

	// Count occurrences of "care-bear hook" -- should be exactly 5 (one per hook type)
	count := strings.Count(string(data), "care-bear hook cursor")
	if count != 1 {
		t.Errorf("care-bear hook appears %d times, want 1 (preToolUse only):\n%s", count, data)
	}
}

func TestCursorInstallHook_CreatesCursorDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't pre-create .cursor dir -- InstallHook should handle it

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bear"}
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

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bear"}
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

	// care-bear should be the first entry (prepended)
	firstEntry := preToolUse[0].(map[string]any)
	if !strings.Contains(firstEntry["command"].(string), "care-bear hook") {
		t.Errorf("care-bear hook is not the first entry in preToolUse, first is: %v", firstEntry)
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

// --- InstallHook JSON structure tests ---

func TestCursorInstallHook_CorrectJSONStructure(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "/usr/local/bin/care-bear"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	hooksPath := filepath.Join(tmpDir, ".cursor", "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("hooks.json is invalid JSON: %v", err)
	}

	// Verify version field
	if config["version"] != float64(1) {
		t.Errorf("version = %v, want 1", config["version"])
	}

	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		t.Fatal("missing 'hooks' key in config")
	}

	hookTypes := []string{"preToolUse"}
	// Verify each hook type has the correct command
	for _, hookType := range hookTypes {
		arr, ok := hooks[hookType].([]any)
		if !ok {
			t.Errorf("hook type %q not found or not an array", hookType)
			continue
		}
		if len(arr) != 1 {
			t.Errorf("hook type %q has %d entries, want 1", hookType, len(arr))
			continue
		}
		entry, ok := arr[0].(map[string]any)
		if !ok {
			t.Errorf("hook type %q entry is not a map", hookType)
			continue
		}
		wantCmd := "care-bear hook cursor"
		if entry["command"] != wantCmd {
			t.Errorf("hook type %q command = %v, want %q", hookType, entry["command"], wantCmd)
		}
	}
}

func TestCursorInstallHook_MalformedExistingJSON(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}

	hooksPath := filepath.Join(cursorDir, "hooks.json")
	if err := os.WriteFile(hooksPath, []byte("{broken json!!!"), 0o644); err != nil {
		t.Fatalf("failed to write malformed hooks.json: %v", err)
	}

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bear"}
	err := adapter.InstallHook(tmpDir)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing existing hooks.json") {
		t.Errorf("error = %q, want it to mention parsing failure", err.Error())
	}
}

func TestCursorInstallHook_TrailingNewline(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	adapter := &CursorAdapter{HomeDir: tmpDir, BinaryPath: "care-bear"}
	if err := adapter.InstallHook(tmpDir); err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	if !strings.HasSuffix(string(data), "\n") {
		t.Error("hooks.json does not end with trailing newline")
	}
}

// --- Tool name normalization ---

func TestCursorParseInput_ToolNameNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		toolName string
		want     string
	}{
		{"edit_file maps to Edit", "edit_file", "Edit"},
		{"write_file maps to Write", "write_file", "Write"},
		{"read_file maps to Read", "read_file", "Read"},
		{"create_file maps to Write", "create_file", "Write"},
		{"delete_file maps to Write", "delete_file", "Write"},
		{"list_dir maps to Glob", "list_dir", "Glob"},
		{"search_files maps to Grep", "search_files", "Grep"},
		{"codebase_search maps to Grep", "codebase_search", "Grep"},
		{"grep_search maps to Grep", "grep_search", "Grep"},
		{"run_terminal_command maps to Bash", "run_terminal_command", "Bash"},
		{"terminal maps to Bash", "terminal", "Bash"},
		{"unknown_tool preserved as-is", "unknown_tool", "unknown_tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			jsonStr := `{"hook_event_name":"preToolUse","conversation_id":"x","cursor_version":"0.48.1","tool_name":"` + tt.toolName + `"}`
			adapter := &CursorAdapter{}

			input, err := adapter.ParseInput(strings.NewReader(jsonStr))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if input.ToolName != tt.want {
				t.Errorf("ToolName = %q, want %q", input.ToolName, tt.want)
			}
		})
	}
}

func TestCursorParseInput_HookEventNameFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		eventName string
		want      string
	}{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// No tool_name field -- should fallback to hook_event_name mapping
			jsonStr := `{"hook_event_name":"` + tt.eventName + `","conversation_id":"x","cursor_version":"0.48.1"}`
			adapter := &CursorAdapter{}

			input, err := adapter.ParseInput(strings.NewReader(jsonStr))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if input.ToolName != tt.want {
				t.Errorf("ToolName = %q, want %q (fallback from hook_event_name)", input.ToolName, tt.want)
			}
		})
	}
}

// --- ParseInput edge cases ---

func TestCursorParseInput_FallbackToSessionIDWhenNoConversationID(t *testing.T) {
	t.Parallel()
	// When conversation_id is absent, should fall back to session_id
	jsonStr := `{"hook_event_name":"preToolUse","session_id":"fallback-session","cursor_version":"0.48.1"}`
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.SessionID != "fallback-session" {
		t.Errorf("SessionID = %q, want %q (should fallback to session_id)", input.SessionID, "fallback-session")
	}
}

func TestCursorParseInput_FilePathFromToolInput(t *testing.T) {
	t.Parallel()
	// When file_path is in tool_input (preToolUse format)
	jsonStr := `{"hook_event_name":"preToolUse","conversation_id":"x","cursor_version":"0.48.1","tool_input":{"file_path":"nested/path.go"}}`
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "nested/path.go" {
		t.Errorf("FilePath = %q, want %q (should extract from tool_input.file_path)", input.FilePath, "nested/path.go")
	}
}

func TestCursorParseInput_PathFromToolInput(t *testing.T) {
	t.Parallel()
	// When "path" is in tool_input (Grep/Glob style)
	jsonStr := `{"hook_event_name":"preToolUse","conversation_id":"x","cursor_version":"0.48.1","tool_input":{"path":"src/"}}`
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "src/" {
		t.Errorf("FilePath = %q, want %q (should extract from tool_input.path)", input.FilePath, "src/")
	}
}

func TestCursorParseInput_TopLevelFilePathPriority(t *testing.T) {
	t.Parallel()
	// Top-level file_path should take priority over tool_input.file_path
	jsonStr := `{"hook_event_name":"preToolUse","conversation_id":"x","cursor_version":"0.48.1","file_path":"top-level.go","tool_input":{"file_path":"nested.go"}}`
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.FilePath != "top-level.go" {
		t.Errorf("FilePath = %q, want %q (top-level should take priority)", input.FilePath, "top-level.go")
	}
}

func TestCursorParseInput_EmptyJSON(t *testing.T) {
	t.Parallel()
	adapter := &CursorAdapter{}

	input, err := adapter.ParseInput(strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Agent != "cursor" {
		t.Errorf("Agent = %q, want %q", input.Agent, "cursor")
	}
	if input.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", input.SessionID)
	}
	if input.ToolName != "" {
		t.Errorf("ToolName = %q, want empty", input.ToolName)
	}
}

// --- FormatDeny additional fields ---

func TestCursorFormatDeny_IncludesAllFields(t *testing.T) {
	t.Parallel()
	adapter := &CursorAdapter{}
	reason := "blocked by policy"

	result, err := adapter.FormatDeny(reason)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("FormatDeny returned invalid JSON: %v", err)
	}

	// Check all expected fields
	if parsed["continue"] != false {
		t.Errorf("continue = %v, want false", parsed["continue"])
	}
	if parsed["permission"] != "deny" {
		t.Errorf("permission = %v, want %q", parsed["permission"], "deny")
	}
	if parsed["userMessage"] != reason {
		t.Errorf("userMessage = %v, want %q", parsed["userMessage"], reason)
	}
	if parsed["agentMessage"] != reason {
		t.Errorf("agentMessage = %v, want %q", parsed["agentMessage"], reason)
	}
}

func TestCursorFormatAllow_NoExtraFields(t *testing.T) {
	t.Parallel()
	adapter := &CursorAdapter{}

	result, err := adapter.FormatAllow()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("FormatAllow returned invalid JSON: %v", err)
	}
	// Should only have "continue" field
	if len(parsed) != 1 {
		t.Errorf("FormatAllow has %d fields, want 1 (only 'continue')", len(parsed))
	}
}

// --- cursorCareBareHookExists tests ---

func TestCursorCareBareHookExists_EmptyHooksMap(t *testing.T) {
	t.Parallel()
	if cursorCareBareHookExists(map[string]any{}) {
		t.Error("expected false for empty hooks map")
	}
}

func TestCursorCareBareHookExists_MalformedEntries(t *testing.T) {
	t.Parallel()
	hooks := map[string]any{
		"preToolUse": "not an array",
		"beforeFileEdit": []any{
			"not a map",
			42,
			map[string]any{
				"type": "webhook",
			},
		},
	}
	if cursorCareBareHookExists(hooks) {
		t.Error("expected false for malformed entries")
	}
}

func TestCursorCareBareHookExists_FindsExisting(t *testing.T) {
	t.Parallel()
	hooks := map[string]any{
		"preToolUse": []any{
			map[string]any{
				"command": "some-other-tool",
			},
			map[string]any{
				"command": "/usr/bin/care-bear hook cursor",
			},
		},
	}
	if !cursorCareBareHookExists(hooks) {
		t.Error("expected true when care-bear hook exists in any hook type")
	}
}

// --- GlobalConfigPath tests ---

func TestCursorGlobalConfigPath_WithHomeDir(t *testing.T) {
	t.Parallel()
	adapter := &CursorAdapter{HomeDir: "/custom/home"}
	got := adapter.GlobalConfigPath()
	want := filepath.Join("/custom/home", ".cursor", "hooks.json")
	if got != want {
		t.Errorf("GlobalConfigPath() = %q, want %q", got, want)
	}
}

// --- ScanProjects tests ---

func TestCursorScanProjects_EmptyProjectsDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	projectsDir := filepath.Join(tmpDir, ".cursor", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("failed to create projects dir: %v", err)
	}

	adapter := &CursorAdapter{HomeDir: tmpDir}
	projects, err := adapter.ScanProjects()
	if err != nil {
		t.Fatalf("ScanProjects failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0", len(projects))
	}
}

func TestCursorScanProjects_MissingProjectsDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	adapter := &CursorAdapter{HomeDir: tmpDir}
	projects, err := adapter.ScanProjects()
	if err != nil {
		t.Fatalf("ScanProjects failed: %v", err)
	}
	if projects != nil {
		t.Errorf("got %v, want nil for missing projects dir", projects)
	}
}

func TestCursorScanProjects_WithSessionsIndex(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a real project directory
	projectPath := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create encoded project dir under .cursor/projects/
	encodedDir := filepath.Join(tmpDir, ".cursor", "projects", "encoded-cursor-project")
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatalf("failed to create encoded dir: %v", err)
	}

	indexJSON := `{"entries":[{"projectPath":"` + projectPath + `"}]}`
	if err := os.WriteFile(filepath.Join(encodedDir, "sessions-index.json"), []byte(indexJSON), 0o644); err != nil {
		t.Fatalf("failed to write sessions-index.json: %v", err)
	}

	adapter := &CursorAdapter{HomeDir: tmpDir}
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
	if projects[0].Agent != "cursor" {
		t.Errorf("Agent = %q, want %q", projects[0].Agent, "cursor")
	}
}

// --- ConfigPath test ---

func TestCursorConfigPath(t *testing.T) {
	t.Parallel()
	adapter := &CursorAdapter{}
	if adapter.ConfigPath() != ".cursor/hooks.json" {
		t.Errorf("ConfigPath() = %q, want %q", adapter.ConfigPath(), ".cursor/hooks.json")
	}
}

// --- Name test ---

func TestCursorName(t *testing.T) {
	t.Parallel()
	adapter := &CursorAdapter{}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "cursor")
	}
}

// TestCursorHookCommand_MatchesBluebearFormat verifies the Cursor hook command
// uses the same format as the working bluebear hook (no --flag syntax).
func TestCursorHookCommand_MatchesBluebearFormat(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := &CursorAdapter{HomeDir: tmpDir}
	err := adapter.InstallHook("")
	if err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("failed to read hooks.json: %v", err)
	}

	// The command should NOT contain "--agent" flag syntax.
	// Cursor hooks work with simple positional args like "bluebear hook cursor".
	if strings.Contains(string(data), "--agent") {
		t.Error("Cursor hook command should not use --agent flag; use positional arg like 'care-bear hook cursor'")
	}
	if !strings.Contains(string(data), "care-bear hook cursor") {
		t.Errorf("expected 'care-bear hook cursor' in hooks.json, got: %s", string(data))
	}
}

// TestCursorInstallHook_OnlyPreToolUse verifies that InstallHook only adds
// care-bear to preToolUse. Other hook types should not be modified.
// This is critical: adding entries to multiple hook types caused Cursor
// to stop firing hooks entirely.
func TestCursorInstallHook_OnlyPreToolUse(t *testing.T) {
	tmpDir := t.TempDir()

	// Pre-populate with existing hooks
	cursorDir := filepath.Join(tmpDir, ".cursor")
	_ = os.MkdirAll(cursorDir, 0o755)
	existing := map[string]any{
		"version": float64(1),
		"hooks": map[string]any{
			"preToolUse":   []any{map[string]any{"command": "other-tool"}},
			"sessionStart": []any{map[string]any{"command": "logger start"}},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	_ = os.WriteFile(filepath.Join(cursorDir, "hooks.json"), data, 0o644)

	adapter := &CursorAdapter{HomeDir: tmpDir}
	err := adapter.InstallHook("")
	if err != nil {
		t.Fatalf("InstallHook failed: %v", err)
	}

	result, _ := os.ReadFile(filepath.Join(cursorDir, "hooks.json"))
	var config map[string]any
	_ = json.Unmarshal(result, &config)
	hooks := config["hooks"].(map[string]any)

	// preToolUse should have care-bear prepended
	preToolUse := hooks["preToolUse"].([]any)
	firstCmd := preToolUse[0].(map[string]any)["command"].(string)
	if !strings.Contains(firstCmd, "care-bear") {
		t.Errorf("preToolUse first entry should be care-bear, got: %s", firstCmd)
	}

	// beforeFileEdit should NOT have care-bear (only preToolUse gets care-bear)
	if arr, ok := hooks["beforeFileEdit"].([]any); ok {
		for _, e := range arr {
			if m, ok := e.(map[string]any); ok {
				if cmd, ok := m["command"].(string); ok && strings.Contains(cmd, "care-bear") {
					t.Errorf("beforeFileEdit should NOT have care-bear, got: %s", cmd)
				}
			}
		}
	}

	// sessionStart should be untouched
	sessionStart := hooks["sessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Errorf("sessionStart should have 1 entry, got %d", len(sessionStart))
	}
}

// TestCursorInstallHook_WorksWithManyExistingHooks verifies that care-bear
// installs correctly and the file remains valid when other tools have added
// hundreds of hook entries across many hook types.
func TestCursorInstallHook_WorksWithManyExistingHooks(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	_ = os.MkdirAll(cursorDir, 0o755)

	// Simulate a hooks.json with 1000+ entries from other tools
	hooks := map[string]any{}
	hookTypes := []string{
		"preToolUse", "beforeFileEdit", "beforeShellExecution",
		"afterFileEdit", "afterShellExecution", "sessionStart",
		"sessionEnd", "beforeSubmitPrompt", "postToolUse",
		"afterAgentResponse",
	}
	for _, ht := range hookTypes {
		var entries []any
		for i := 0; i < 100; i++ {
			entries = append(entries, map[string]any{
				"command": fmt.Sprintf("tool-%d hook %s", i, ht),
			})
		}
		hooks[ht] = entries
	}
	config := map[string]any{"version": float64(1), "hooks": hooks}
	data, _ := json.MarshalIndent(config, "", "  ")
	_ = os.WriteFile(filepath.Join(cursorDir, "hooks.json"), data, 0o644)

	// Install care-bear
	adapter := &CursorAdapter{HomeDir: tmpDir}
	err := adapter.InstallHook("")
	if err != nil {
		t.Fatalf("InstallHook failed with 1000 existing hooks: %v", err)
	}

	// Read result
	result, _ := os.ReadFile(filepath.Join(cursorDir, "hooks.json"))
	var resultConfig map[string]any
	_ = json.Unmarshal(result, &resultConfig)
	resultHooks := resultConfig["hooks"].(map[string]any)

	// preToolUse should have care-bear as FIRST entry + 100 existing
	preToolUse := resultHooks["preToolUse"].([]any)
	if len(preToolUse) != 101 {
		t.Errorf("preToolUse should have 101 entries (care-bear + 100 existing), got %d", len(preToolUse))
	}
	firstCmd := preToolUse[0].(map[string]any)["command"].(string)
	if !strings.Contains(firstCmd, "care-bear") {
		t.Errorf("care-bear should be first in preToolUse, got: %s", firstCmd)
	}

	// Other hook types should be untouched (100 entries each)
	for _, ht := range hookTypes[1:] {
		arr := resultHooks[ht].([]any)
		if len(arr) != 100 {
			t.Errorf("%s should have 100 entries (untouched), got %d", ht, len(arr))
		}
		for _, e := range arr {
			cmd := e.(map[string]any)["command"].(string)
			if strings.Contains(cmd, "care-bear") {
				t.Errorf("%s should NOT have care-bear, got: %s", ht, cmd)
			}
		}
	}

	// Idempotent — install again should not duplicate
	err = adapter.InstallHook("")
	if err != nil {
		t.Fatalf("second InstallHook failed: %v", err)
	}
	result2, _ := os.ReadFile(filepath.Join(cursorDir, "hooks.json"))
	var resultConfig2 map[string]any
	_ = json.Unmarshal(result2, &resultConfig2)
	preToolUse2 := resultConfig2["hooks"].(map[string]any)["preToolUse"].([]any)
	if len(preToolUse2) != 101 {
		t.Errorf("after second install, preToolUse should still have 101 entries, got %d", len(preToolUse2))
	}
}
