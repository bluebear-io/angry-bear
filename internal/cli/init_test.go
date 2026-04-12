// init_test.go contains integration tests for the care-bare init command.
// These tests exercise the init command against real (temporary) filesystems,
// verifying directory creation, file contents, hook installation, gitignore
// modifications, and idempotency behavior.
package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Bear-Security/care-bare/internal/adapter"
	"github.com/Blue-Bear-Security/care-bare/internal/cli"
	"github.com/Blue-Bear-Security/care-bare/internal/engine"
)

// ---------------------------------------------------------------------------
// Test helper
// ---------------------------------------------------------------------------

// runInitInDir executes the init command with the working directory set to dir.
// It captures stdout and stderr and returns them along with any error.
func runInitInDir(t *testing.T, dir string) (stdout, stderr string, err error) {
	t.Helper()

	// Set a stable binary path and home dir for hook installation during tests.
	oldBinPath := adapter.BinaryPath
	adapter.BinaryPath = "care-bare"
	t.Cleanup(func() { adapter.BinaryPath = oldBinPath })

	oldHome := adapter.TestHomeDir
	adapter.TestHomeDir = dir
	t.Cleanup(func() { adapter.TestHomeDir = oldHome })

	// Change to the target directory so os.Getwd() returns it.
	origDir, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("failed to get original working directory: %v", wdErr)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("warning: failed to restore working directory: %v", err)
		}
	})

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"init"})

	execErr := cmd.Execute()
	return outBuf.String(), errBuf.String(), execErr
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestInit_CreatesDirectoryStructure verifies that init creates the
// .care-bare/ and .care-bare/state/ directories.
func TestInit_CreatesDirectoryStructure(t *testing.T) {
	dir := t.TempDir()

	stdout, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify .care-bare/ exists and is a directory.
	info, statErr := os.Stat(filepath.Join(dir, ".care-bare"))
	if statErr != nil {
		t.Fatalf(".care-bare directory was not created: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatal(".care-bare is not a directory")
	}

	// Verify .care-bare/state/ exists and is a directory.
	info, statErr = os.Stat(filepath.Join(dir, ".care-bare", "state"))
	if statErr != nil {
		t.Fatalf(".care-bare/state directory was not created: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatal(".care-bare/state is not a directory")
	}

	// Verify stdout mentions initialized.
	if !strings.Contains(stdout, "care-bare initialized") {
		t.Errorf("expected stdout to mention initialization, got: %s", stdout)
	}
}

// TestInit_CreatesDefaultSkillEnforcement verifies that init writes a correct
// default skill_enforcement.json with version 1 and empty tools array.
func TestInit_CreatesDefaultSkillEnforcement(t *testing.T) {
	dir := t.TempDir()

	_, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	enforcementPath := filepath.Join(dir, ".care-bare", "skill_enforcement.json")
	data, readErr := os.ReadFile(enforcementPath)
	if readErr != nil {
		t.Fatalf("skill_enforcement.json was not created: %v", readErr)
	}

	var cfg engine.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse skill_enforcement.json: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}

	if cfg.Tools == nil {
		t.Fatal("expected tools to be an empty array, got nil")
	}
	if len(cfg.Tools) != 0 {
		t.Errorf("expected tools to be empty, got %d rules", len(cfg.Tools))
	}

	// Also verify the raw JSON has "tools": [] (not null).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to parse raw JSON: %v", err)
	}
	if string(raw["tools"]) != "[]" {
		t.Errorf("expected raw tools to be '[]', got %s", string(raw["tools"]))
	}
}

// TestInit_CreatesDefaultConfig verifies that init writes a correct
// default config.json with sensible defaults.
func TestInit_CreatesDefaultConfig(t *testing.T) {
	dir := t.TempDir()

	_, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	configPath := filepath.Join(dir, ".care-bare", "config.json")
	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("config.json was not created: %v", readErr)
	}

	var cfg engine.GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse config.json: %v", err)
	}

	// Verify skill_paths contains ".claude/skills".
	found := false
	for _, sp := range cfg.SkillPaths {
		if sp == ".claude/skills" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected skill_paths to contain '.claude/skills', got %v", cfg.SkillPaths)
	}

	if cfg.StateTTLHours != 24 {
		t.Errorf("expected state_ttl_hours 24, got %d", cfg.StateTTLHours)
	}

	if cfg.DefaultAgent != "*" {
		t.Errorf("expected default_agent '*', got %s", cfg.DefaultAgent)
	}

	// Verify ignore_patterns contains at least ".git" and "node_modules".
	if len(cfg.IgnorePatterns) == 0 {
		t.Fatal("expected ignore_patterns to be non-empty")
	}
	hasGit := false
	hasNodeModules := false
	for _, p := range cfg.IgnorePatterns {
		if p == ".git" {
			hasGit = true
		}
		if p == "node_modules" {
			hasNodeModules = true
		}
	}
	if !hasGit {
		t.Errorf("expected ignore_patterns to contain '.git', got %v", cfg.IgnorePatterns)
	}
	if !hasNodeModules {
		t.Errorf("expected ignore_patterns to contain 'node_modules', got %v", cfg.IgnorePatterns)
	}
}

// TestInit_DetectsClaudeAndInstallsHook verifies that when .claude/ exists,
// init detects it and installs the care-bare hook in .claude/settings.json.
func TestInit_DetectsClaudeAndInstallsHook(t *testing.T) {
	dir := t.TempDir()

	// Pre-create .claude/ directory to simulate an existing Claude project.
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatalf("failed to create .claude directory: %v", err)
	}

	stdout, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify stdout mentions claude detection.
	if !strings.Contains(stdout, "claude") {
		t.Errorf("expected stdout to mention claude, got: %s", stdout)
	}

	// Verify .claude/settings.json was created with the hook.
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	data, readErr := os.ReadFile(settingsPath)
	if readErr != nil {
		t.Fatalf("settings.json was not created: %v", readErr)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	// Navigate to hooks.PreToolUse.
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("missing 'hooks' key in settings.json: %s", string(data))
	}

	preToolUse, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatalf("missing 'PreToolUse' in hooks: %s", string(data))
	}

	if len(preToolUse) == 0 {
		t.Fatal("PreToolUse array is empty")
	}

	// Verify the entry contains the care-bare hook command.
	foundHook := false
	for _, entry := range preToolUse {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := entryMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hooksList {
			hookMap, ok := hook.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hookMap["command"].(string); ok {
				if cmd == "care-bare hook --agent claude" {
					foundHook = true
				}
			}
		}
	}
	if !foundHook {
		t.Errorf("expected care-bare hook command in settings.json, got: %s", string(data))
	}
}

// TestInit_DetectsCursorAndInstallsHook verifies that when .cursor/ exists,
// init detects it and installs the care-bare hook in .cursor/hooks.json.
func TestInit_DetectsCursorAndInstallsHook(t *testing.T) {
	dir := t.TempDir()

	// Pre-create .cursor/ directory.
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatalf("failed to create .cursor directory: %v", err)
	}

	stdout, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify stdout mentions cursor detection.
	if !strings.Contains(stdout, "cursor") {
		t.Errorf("expected stdout to mention cursor, got: %s", stdout)
	}

	// Verify .cursor/hooks.json was created with the hook.
	hooksPath := filepath.Join(dir, ".cursor", "hooks.json")
	data, readErr := os.ReadFile(hooksPath)
	if readErr != nil {
		t.Fatalf("hooks.json was not created: %v", readErr)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse hooks.json: %v", err)
	}

	// Verify hooks contain care-bare command.
	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("missing 'hooks' key in hooks.json: %s", string(data))
	}

	// Check that at least one hook type contains care-bare.
	foundHook := false
	for _, hookList := range hooks {
		arr, ok := hookList.([]any)
		if !ok {
			continue
		}
		for _, entry := range arr {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := entryMap["command"].(string); ok {
				if strings.Contains(cmd, "care-bare hook --agent cursor") {
					foundHook = true
				}
			}
		}
	}
	if !foundHook {
		t.Errorf("expected care-bare hook command in hooks.json, got: %s", string(data))
	}
}

// TestInit_AddsGitignoreEntry verifies that init adds .care-bare/state/
// to .gitignore when no .gitignore exists.
func TestInit_AddsGitignoreEntry(t *testing.T) {
	dir := t.TempDir()

	_, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	data, readErr := os.ReadFile(gitignorePath)
	if readErr != nil {
		t.Fatalf(".gitignore was not created: %v", readErr)
	}

	content := string(data)
	if !strings.Contains(content, ".care-bare/state/") {
		t.Errorf("expected .gitignore to contain '.care-bare/state/', got:\n%s", content)
	}

	// Verify the comment is present.
	if !strings.Contains(content, "# care-bare state") {
		t.Errorf("expected .gitignore to contain comment, got:\n%s", content)
	}
}

// TestInit_IsIdempotent verifies that running init twice does not duplicate
// hooks, config entries, or gitignore lines, and does not overwrite existing configs.
func TestInit_IsIdempotent(t *testing.T) {
	dir := t.TempDir()

	// Pre-create .claude/ directory.
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatalf("failed to create .claude directory: %v", err)
	}

	// First run.
	_, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Read configs after first run.
	enforcementData1, _ := os.ReadFile(filepath.Join(dir, ".care-bare", "skill_enforcement.json"))
	configData1, _ := os.ReadFile(filepath.Join(dir, ".care-bare", "config.json"))

	// Second run.
	stdout, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	// Verify no error.
	if !strings.Contains(stdout, "care-bare initialized") {
		t.Errorf("expected success message on second run, got: %s", stdout)
	}

	// Verify skill_enforcement.json was preserved (not overwritten).
	enforcementData2, _ := os.ReadFile(filepath.Join(dir, ".care-bare", "skill_enforcement.json"))
	if string(enforcementData1) != string(enforcementData2) {
		t.Error("skill_enforcement.json was modified on second run")
	}

	// Verify config.json was preserved.
	configData2, _ := os.ReadFile(filepath.Join(dir, ".care-bare", "config.json"))
	if string(configData1) != string(configData2) {
		t.Error("config.json was modified on second run")
	}

	// Verify settings.json has care-bare hook exactly once.
	settingsData, _ := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	hookCount := strings.Count(string(settingsData), "care-bare hook")
	if hookCount != 1 {
		t.Errorf("expected care-bare hook to appear once, found %d times in: %s", hookCount, string(settingsData))
	}

	// Verify .gitignore has .care-bare/state/ exactly once.
	gitignoreData, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	stateCount := strings.Count(string(gitignoreData), ".care-bare/state/")
	if stateCount != 1 {
		t.Errorf("expected .care-bare/state/ once in .gitignore, found %d times", stateCount)
	}

	// Verify stdout mentions preserved files on second run.
	if !strings.Contains(stdout, "Preserved") {
		t.Errorf("expected second run to mention preserved files, got: %s", stdout)
	}
}

// TestInit_PreservesExistingHooks verifies that when .claude/settings.json
// already has hooks, init adds the care-bare hook without removing existing ones.
func TestInit_PreservesExistingHooks(t *testing.T) {
	dir := t.TempDir()

	// Pre-create .claude/ directory with an existing settings.json containing hooks.
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude directory: %v", err)
	}

	existingSettings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "my-lint.sh",
						},
					},
				},
			},
		},
	}
	existingData, _ := json.MarshalIndent(existingSettings, "", "  ")
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), existingData, 0o644); err != nil {
		t.Fatalf("failed to write existing settings.json: %v", err)
	}

	_, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Read the updated settings.json.
	data, readErr := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if readErr != nil {
		t.Fatalf("failed to read settings.json: %v", readErr)
	}

	content := string(data)

	// Verify the existing hook is still present.
	if !strings.Contains(content, "my-lint.sh") {
		t.Errorf("existing hook was removed, settings.json: %s", content)
	}

	// Verify the care-bare hook was added.
	if !strings.Contains(content, "care-bare hook --agent claude") {
		t.Errorf("care-bare hook was not added, settings.json: %s", content)
	}
}

// TestInit_PreservesExistingConfigs verifies that init does not overwrite
// user-edited config files on re-init.
func TestInit_PreservesExistingConfigs(t *testing.T) {
	dir := t.TempDir()

	// Run init once to create defaults.
	_, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Modify skill_enforcement.json with a custom rule.
	customConfig := engine.Config{
		Version: 1,
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
		},
	}
	customData, _ := json.MarshalIndent(customConfig, "", "  ")
	enforcementPath := filepath.Join(dir, ".care-bare", "skill_enforcement.json")
	if err := os.WriteFile(enforcementPath, customData, 0o644); err != nil {
		t.Fatalf("failed to write custom config: %v", err)
	}

	// Run init again.
	_, _, err = runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	// Verify the custom rule is preserved.
	data, _ := os.ReadFile(enforcementPath)
	var cfg engine.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse config after re-init: %v", err)
	}

	if len(cfg.Tools) != 1 {
		t.Errorf("expected 1 rule after re-init, got %d", len(cfg.Tools))
	}
	if len(cfg.Tools) > 0 && cfg.Tools[0].Skill != "go-standards" {
		t.Errorf("expected go-standards rule, got %s", cfg.Tools[0].Skill)
	}
}

// TestInit_GitignoreAppendsToExisting verifies that init appends to an
// existing .gitignore without removing existing entries.
func TestInit_GitignoreAppendsToExisting(t *testing.T) {
	dir := t.TempDir()

	// Create a .gitignore with existing content.
	existingContent := "node_modules/\n.env\n"
	gitignorePath := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(existingContent), 0o644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	_, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	data, _ := os.ReadFile(gitignorePath)
	content := string(data)

	// Verify existing entries are preserved.
	if !strings.Contains(content, "node_modules/") {
		t.Error("existing node_modules/ entry was removed")
	}
	if !strings.Contains(content, ".env") {
		t.Error("existing .env entry was removed")
	}

	// Verify care-bare entry was added.
	if !strings.Contains(content, ".care-bare/state/") {
		t.Error("care-bare/state/ was not added to .gitignore")
	}
}

// TestInit_GitignoreSkipsIfAlreadyPresent verifies that init does not
// duplicate the .care-bare/state/ entry in .gitignore.
func TestInit_GitignoreSkipsIfAlreadyPresent(t *testing.T) {
	dir := t.TempDir()

	// Create a .gitignore that already has the entry.
	existingContent := "node_modules/\n.care-bare/state/\n"
	gitignorePath := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(existingContent), 0o644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	_, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	data, _ := os.ReadFile(gitignorePath)
	content := string(data)

	count := strings.Count(content, ".care-bare/state/")
	if count != 1 {
		t.Errorf("expected .care-bare/state/ exactly once, found %d times in:\n%s", count, content)
	}
}

// TestInit_NoAgentsDetectedMessage verifies that when no agent directories
// exist, the summary includes a message about no agents detected.
func TestInit_NoAgentsDetectedMessage(t *testing.T) {
	dir := t.TempDir()

	stdout, _, err := runInitInDir(t, dir)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if !strings.Contains(stdout, "No AI agents detected") {
		t.Errorf("expected 'No AI agents detected' message, got: %s", stdout)
	}
}
