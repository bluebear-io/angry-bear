// hook_test.go contains integration tests for the care-bare hook command.
// These tests exercise the full stdin-to-stdout flow by setting up real
// config files and state files, piping JSON through the hook command via
// cmd.SetIn(), and verifying stdout output and exit behavior.
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

// ---------------------------------------------------------------------------
// Test helper functions
// ---------------------------------------------------------------------------

// writeEnforcementConfig writes a .care-bare/skill_enforcement.json file
// with the given rules and version 1 into the specified directory.
func writeEnforcementConfig(t *testing.T, dir string, rules []engine.Rule) {
	t.Helper()
	cfg := engine.Config{
		Version: 1,
		Tools:   rules,
	}
	configDir := filepath.Join(dir, ".care-bare")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create .care-bare directory: %v", err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "skill_enforcement.json"), data, 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
}

// writeStateFile writes a .care-bare/state/{sessionID}.json file with the
// given invoked skills into the specified directory.
func writeStateFile(t *testing.T, dir string, sessionID string, skills []string) {
	t.Helper()
	stateDir := filepath.Join(dir, ".care-bare", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}
	ss := state.SessionState{
		SessionID:     sessionID,
		CreatedAt:     "2024-01-01T00:00:00Z",
		InvokedSkills: skills,
	}
	data, err := json.Marshal(ss)
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, sessionID+".json"), data, 0o600); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}
}

// claudeStdin returns a valid Claude Code PreToolUse JSON string with the
// given session ID, tool name, and file path. The cwd field is set to dir
// so that project root resolution finds the temp directory.
func claudeStdin(sessionID, toolName, filePath, cwd string) string {
	input := map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      sessionID,
		"tool_name":       toolName,
		"cwd":             cwd,
		"tool_input": map[string]any{
			"file_path": filePath,
		},
	}
	data, _ := json.Marshal(input)
	return string(data)
}

// claudeSkillStdin returns a Claude Code PreToolUse JSON string for a Skill
// tool invocation with the given session ID and skill name.
func claudeSkillStdin(sessionID, skillName, cwd string) string {
	input := map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      sessionID,
		"tool_name":       "Skill",
		"cwd":             cwd,
		"tool_input": map[string]any{
			"skill": skillName,
		},
	}
	data, _ := json.Marshal(input)
	return string(data)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestHook_BlocksWhenSkillNotInvoked verifies that the hook blocks a tool
// invocation when the required skill has not been loaded in the session.
func TestHook_BlocksWhenSkillNotInvoked(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
	})

	stdin := claudeStdin("sess1", "Edit", filepath.Join(dir, "internal", "foo.go"), dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v (stderr: %s)", execErr, errBuf.String())
	}

	output := outBuf.String()
	if output == "" {
		t.Fatal("expected deny JSON output, got empty stdout")
	}

	// Verify it's a deny response.
	var response map[string]any
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("failed to parse deny JSON: %v, output: %s", err, output)
	}

	hookOutput, ok := response["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("missing hookSpecificOutput in response: %s", output)
	}
	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("expected permissionDecision=deny, got %v", hookOutput["permissionDecision"])
	}
	reason, _ := hookOutput["permissionDecisionReason"].(string)
	if !strings.Contains(reason, "go-standards") {
		t.Errorf("expected reason to mention go-standards, got: %s", reason)
	}
}

// TestHook_AllowsWhenSkillInvoked verifies that the hook allows a tool
// invocation when the required skill has been loaded in the session.
func TestHook_AllowsWhenSkillInvoked(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
	})
	writeStateFile(t, dir, "sess1", []string{"go-standards"})

	stdin := claudeStdin("sess1", "Edit", filepath.Join(dir, "internal", "foo.go"), dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v (stderr: %s)", execErr, errBuf.String())
	}

	if outBuf.String() != "" {
		t.Errorf("expected empty stdout (allow), got: %s", outBuf.String())
	}
}

// TestHook_RecordsSkillInvocation verifies that when a Skill tool invocation
// is detected, the hook records it in the state file and allows the operation.
func TestHook_RecordsSkillInvocation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create .care-bare so project root resolution finds it.
	if err := os.MkdirAll(filepath.Join(dir, ".care-bare"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdin := claudeSkillStdin("sess1", "go-standards", dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v (stderr: %s)", execErr, errBuf.String())
	}

	// Verify allow (empty stdout).
	if outBuf.String() != "" {
		t.Errorf("expected empty stdout (allow), got: %s", outBuf.String())
	}

	// Verify state file was created with the skill recorded.
	stateDir := filepath.Join(dir, ".care-bare", "state")
	mgr := state.NewStateManager(stateDir)
	skills, err := mgr.GetInvokedSkills("sess1")
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	if !skills["go-standards"] {
		t.Errorf("expected go-standards to be recorded, got: %v", skills)
	}
}

// TestHook_AllowsWhenNoConfig verifies that the hook allows all operations
// when no enforcement config exists (fail-open).
func TestHook_AllowsWhenNoConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create .care-bare dir but no skill_enforcement.json.
	if err := os.MkdirAll(filepath.Join(dir, ".care-bare"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdin := claudeStdin("sess1", "Edit", filepath.Join(dir, "internal", "foo.go"), dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v (stderr: %s)", execErr, errBuf.String())
	}

	if outBuf.String() != "" {
		t.Errorf("expected empty stdout (allow), got: %s", outBuf.String())
	}
}

// TestHook_ExitsZeroOnAllow verifies that the hook exits with code 0 and
// empty stdout when allowing an operation.
func TestHook_ExitsZeroOnAllow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".care-bare"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdin := claudeStdin("sess1", "Read", filepath.Join(dir, "README.md"), dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error (expected exit 0): %v", execErr)
	}

	if outBuf.String() != "" {
		t.Errorf("expected empty stdout for allow, got: %s", outBuf.String())
	}
}

// TestHook_ExitsZeroOnDeny verifies that the hook exits with code 0 even
// when blocking, with deny JSON in stdout. Claude Code reads the decision
// from the JSON body, not the exit code.
func TestHook_ExitsZeroOnDeny(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Write", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
	})

	stdin := claudeStdin("sess1", "Write", filepath.Join(dir, "src", "main.go"), dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	// Exit 0 even for deny.
	if execErr != nil {
		t.Fatalf("hook command returned error (expected exit 0 even for deny): %v", execErr)
	}

	output := outBuf.String()
	if output == "" {
		t.Fatal("expected deny JSON output, got empty stdout")
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("failed to parse deny JSON: %v", err)
	}

	hookOutput, ok := response["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("missing hookSpecificOutput in response: %s", output)
	}
	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("expected permissionDecision=deny, got %v", hookOutput["permissionDecision"])
	}
}

// TestHook_OversizedStdin verifies that the hook rejects input larger than
// the 5MB limit without hanging or running out of memory.
func TestHook_OversizedStdin(t *testing.T) {
	t.Parallel()

	// Create 6MB of data (exceeds 5MB limit).
	oversizedData := strings.Repeat("x", 6*1024*1024)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(oversizedData))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	// Should return non-zero exit (error).
	if execErr == nil {
		t.Fatal("expected error for oversized stdin, got nil")
	}
}

// TestHook_AgentFlag verifies that the --agent flag works for explicit
// adapter selection and behaves identically to auto-detect for Claude input.
func TestHook_AgentFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
	})
	writeStateFile(t, dir, "sess1", []string{"go-standards"})

	stdin := claudeStdin("sess1", "Edit", filepath.Join(dir, "foo.go"), dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v (stderr: %s)", execErr, errBuf.String())
	}

	if outBuf.String() != "" {
		t.Errorf("expected empty stdout (allow), got: %s", outBuf.String())
	}
}

// TestHook_AutoDetectsAgent verifies that the hook correctly auto-detects
// the Claude adapter when --agent is not provided, based on the
// hook_event_name field in the JSON input.
func TestHook_AutoDetectsAgent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
	})

	stdin := claudeStdin("sess1", "Edit", filepath.Join(dir, "foo.go"), dir)

	// No --agent flag.
	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v (stderr: %s)", execErr, errBuf.String())
	}

	// Should deny (skill not loaded) -- verifying auto-detect worked.
	output := outBuf.String()
	if output == "" {
		t.Fatal("expected deny JSON (skill not loaded), got empty stdout")
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("failed to parse deny JSON: %v, output: %s", err, output)
	}
	hookOutput, ok := response["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %s", output)
	}
	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("expected deny, got %v", hookOutput["permissionDecision"])
	}
}

// TestHook_MultipleRulesMultipleSkills verifies that when multiple rules
// require different skills, all missing skills are reported in the deny response.
func TestHook_MultipleRulesMultipleSkills(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
		{Tool: "Edit", Path: "**/*.go", Skill: "testing-standards", Agent: "*"},
	})

	stdin := claudeStdin("sess1", "Edit", filepath.Join(dir, "main.go"), dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v", execErr)
	}

	output := outBuf.String()
	if output == "" {
		t.Fatal("expected deny JSON, got empty stdout")
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("failed to parse deny JSON: %v", err)
	}
	hookOutput := response["hookSpecificOutput"].(map[string]any)
	reason := hookOutput["permissionDecisionReason"].(string)
	if !strings.Contains(reason, "go-standards") {
		t.Errorf("expected reason to mention go-standards, got: %s", reason)
	}
	if !strings.Contains(reason, "testing-standards") {
		t.Errorf("expected reason to mention testing-standards, got: %s", reason)
	}
}

// TestHook_PartialSkillsLoaded verifies that if only some required skills
// are loaded, only the missing ones cause a block.
func TestHook_PartialSkillsLoaded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
		{Tool: "Edit", Path: "**/*.go", Skill: "testing-standards", Agent: "*"},
	})
	writeStateFile(t, dir, "sess1", []string{"go-standards"})

	stdin := claudeStdin("sess1", "Edit", filepath.Join(dir, "main.go"), dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v", execErr)
	}

	output := outBuf.String()
	if output == "" {
		t.Fatal("expected deny JSON (testing-standards missing), got empty stdout")
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("failed to parse deny JSON: %v", err)
	}
	hookOutput := response["hookSpecificOutput"].(map[string]any)
	reason := hookOutput["permissionDecisionReason"].(string)
	if strings.Contains(reason, "go-standards") {
		t.Errorf("reason should NOT mention go-standards (already loaded), got: %s", reason)
	}
	if !strings.Contains(reason, "testing-standards") {
		t.Errorf("expected reason to mention testing-standards, got: %s", reason)
	}
}

// TestHook_InvalidAdapterName verifies that an invalid --agent flag value
// causes a non-zero exit (infrastructure error).
func TestHook_InvalidAdapterName(t *testing.T) {
	t.Parallel()

	stdin := `{"hook_event_name":"PreToolUse","session_id":"s1","tool_name":"Edit","cwd":"/tmp","tool_input":{}}`

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "nonexistent"})

	execErr := cmd.Execute()
	if execErr == nil {
		t.Fatal("expected error for unknown adapter, got nil")
	}
}

// TestHook_MalformedStdinJSON verifies that malformed JSON stdin causes
// a non-zero exit (infrastructure error).
func TestHook_MalformedStdinJSON(t *testing.T) {
	t.Parallel()

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader("{not valid json"))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// TestHook_EmptyStdin verifies that empty stdin causes a non-zero exit
// (infrastructure error from the adapter auto-detect or parse).
func TestHook_EmptyStdin(t *testing.T) {
	t.Parallel()

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	// Empty stdin should cause a parse error.
	if execErr == nil {
		t.Fatal("expected error for empty stdin, got nil")
	}
}

// TestHook_SkillRecordingCreatesStateDir verifies that the hook creates
// the .care-bare/state/ directory if it doesn't exist when recording skills.
func TestHook_SkillRecordingCreatesStateDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Only create .care-bare, not .care-bare/state.
	if err := os.MkdirAll(filepath.Join(dir, ".care-bare"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdin := claudeSkillStdin("sess1", "my-skill", dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v", execErr)
	}

	// Verify the state directory was created.
	stateDir := filepath.Join(dir, ".care-bare", "state")
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("state directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("state path is not a directory")
	}

	// Verify the skill was recorded.
	mgr := state.NewStateManager(stateDir)
	skills, err := mgr.GetInvokedSkills("sess1")
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	if !skills["my-skill"] {
		t.Errorf("expected my-skill to be recorded, got: %v", skills)
	}
}

// TestHook_NonMatchingRuleAllows verifies that when rules exist but none
// match the current tool/path combination, the operation is allowed.
func TestHook_NonMatchingRuleAllows(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Bash", Path: "**/*.sh", Skill: "bash-skill", Agent: "*"},
	})

	// Using Edit tool on a .go file -- should not match the Bash/*.sh rule.
	stdin := claudeStdin("sess1", "Edit", filepath.Join(dir, "main.go"), dir)

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs([]string{"hook", "--agent", "claude"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("hook command returned error: %v", execErr)
	}

	if outBuf.String() != "" {
		t.Errorf("expected empty stdout (allow, non-matching rule), got: %s", outBuf.String())
	}
}
