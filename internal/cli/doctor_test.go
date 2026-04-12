// doctor_test.go contains integration tests for the care-bare doctor command.
// Tests exercise the command against controlled temporary environments to verify
// each diagnostic check (config, hooks, state dir, binary, skill paths).
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
)

// runDoctorInDir executes the doctor command with the working directory set
// to dir. It captures stdout and returns it along with any error.
func runDoctorInDir(t *testing.T, dir string) (string, error) {
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
	cmd.SetArgs([]string{"doctor"})

	execErr := cmd.Execute()
	return outBuf.String(), execErr
}

// setupHealthyProject creates a fully valid care-bare project in the temp dir
// with valid configs, state directory, a detected agent with hooks, and skills.
func setupHealthyProject(t *testing.T, dir string) {
	t.Helper()

	// Create .care-bare/ with valid skill_enforcement.json.
	careBareDir := filepath.Join(dir, ".care-bare")
	err := os.MkdirAll(careBareDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create .care-bare directory: %v", err)
	}
	enforcementCfg := engine.Config{
		Version: 1,
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
		},
	}
	data, err := json.MarshalIndent(enforcementCfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal enforcement config: %v", err)
	}
	err = os.WriteFile(filepath.Join(careBareDir, "skill_enforcement.json"), data, 0o644)
	if err != nil {
		t.Fatalf("failed to write enforcement config: %v", err)
	}

	// Create valid config.json.
	globalCfg := engine.GlobalConfig{
		SkillPaths:    []string{".claude/skills"},
		StateTTLHours: 24,
		DefaultAgent:  "*",
	}
	data, err = json.MarshalIndent(globalCfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal global config: %v", err)
	}
	err = os.WriteFile(filepath.Join(careBareDir, "config.json"), data, 0o644)
	if err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	// Create .care-bare/state/ directory.
	err = os.MkdirAll(filepath.Join(careBareDir, "state"), 0o755)
	if err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}

	// Create .claude/ with settings.json containing care-bare hook.
	claudeDir := filepath.Join(dir, ".claude")
	err = os.MkdirAll(claudeDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create .claude directory: %v", err)
	}
	settings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "*",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "care-bare hook --agent claude",
						},
					},
				},
			},
		},
	}
	data, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}
	err = os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0o644)
	if err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}

	// Create .claude/skills/ with a skill file.
	skillDir := filepath.Join(claudeDir, "skills", "go-standards")
	err = os.MkdirAll(skillDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}
	skillContent := "---\nname: go-standards\ndescription: Go coding standards\n---\nSkill content."
	err = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}
}

// TestDoctor_AllPassWhenHealthy verifies that doctor reports all checks as
// PASS when the project is fully configured. The binary-on-PATH check may
// fail (care-bare is not installed globally in tests), so we allow that.
func TestDoctor_AllPassWhenHealthy(t *testing.T) {
	dir := t.TempDir()
	setupHealthyProject(t, dir)

	output, _ := runDoctorInDir(t, dir)

	// Verify header.
	if !strings.Contains(output, "care-bare doctor") {
		t.Errorf("expected doctor header, got: %s", output)
	}

	// Verify config checks pass.
	if !strings.Contains(output, "[PASS] Config validity: skill_enforcement.json") {
		t.Errorf("expected enforcement config PASS, got: %s", output)
	}
	if !strings.Contains(output, "[PASS] Config validity: config.json") {
		t.Errorf("expected global config PASS, got: %s", output)
	}

	// Verify hook check passes.
	if !strings.Contains(output, "[PASS] Hook installed: claude") {
		t.Errorf("expected claude hook PASS, got: %s", output)
	}

	// Verify state directory check passes.
	if !strings.Contains(output, "[PASS] State directory") {
		t.Errorf("expected state directory PASS, got: %s", output)
	}

	// Verify skill path check passes.
	if !strings.Contains(output, "[PASS] Skill path:") {
		t.Errorf("expected skill path PASS, got: %s", output)
	}

	// Verify summary line exists.
	if !strings.Contains(output, "Result:") {
		t.Errorf("expected result summary, got: %s", output)
	}
}

// TestDoctor_FailsWhenHookNotInstalled verifies that doctor reports FAIL
// when an agent is detected but the hook entry is missing from its config.
func TestDoctor_FailsWhenHookNotInstalled(t *testing.T) {
	dir := t.TempDir()
	setupHealthyProject(t, dir)

	// Overwrite settings.json without the care-bare hook.
	settings := map[string]any{
		"hooks": map[string]any{},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}
	err = os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), data, 0o644)
	if err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}

	output, execErr := runDoctorInDir(t, dir)

	// Doctor should return an error (exit 1).
	if execErr == nil {
		t.Fatal("expected doctor to return error when hook not installed, got nil")
	}

	if !strings.Contains(output, "[FAIL] Hook installed: claude") {
		t.Errorf("expected hook FAIL, got: %s", output)
	}
	if !strings.Contains(output, "care-bare init") {
		t.Errorf("expected fix hint mentioning 'care-bare init', got: %s", output)
	}
}

// TestDoctor_FailsWhenConfigHasJSONErrors verifies that doctor reports FAIL
// when skill_enforcement.json contains malformed JSON.
func TestDoctor_FailsWhenConfigHasJSONErrors(t *testing.T) {
	dir := t.TempDir()
	setupHealthyProject(t, dir)

	// Overwrite skill_enforcement.json with malformed JSON.
	err := os.WriteFile(
		filepath.Join(dir, ".care-bare", "skill_enforcement.json"),
		[]byte("{not valid json"),
		0o644,
	)
	if err != nil {
		t.Fatalf("failed to write malformed config: %v", err)
	}

	output, execErr := runDoctorInDir(t, dir)

	if execErr == nil {
		t.Fatal("expected doctor to return error for invalid JSON, got nil")
	}

	if !strings.Contains(output, "[FAIL] Config validity: skill_enforcement.json") {
		t.Errorf("expected config FAIL, got: %s", output)
	}
	if !strings.Contains(output, "invalid JSON") {
		t.Errorf("expected JSON error detail, got: %s", output)
	}
}

// TestDoctor_FailsWhenStateDirectoryNotWritable verifies that doctor reports
// FAIL when the state directory exists but is not writable.
func TestDoctor_FailsWhenStateDirectoryNotWritable(t *testing.T) {
	dir := t.TempDir()
	setupHealthyProject(t, dir)

	// Make state directory read-only.
	stateDir := filepath.Join(dir, ".care-bare", "state")
	err := os.Chmod(stateDir, 0o444)
	if err != nil {
		t.Fatalf("failed to make state directory read-only: %v", err)
	}
	t.Cleanup(func() {
		// Restore permissions so TempDir cleanup can proceed.
		os.Chmod(stateDir, 0o755)
	})

	output, execErr := runDoctorInDir(t, dir)

	if execErr == nil {
		t.Fatal("expected doctor to return error for read-only state dir, got nil")
	}

	if !strings.Contains(output, "[FAIL] State directory") {
		t.Errorf("expected state directory FAIL, got: %s", output)
	}
	if !strings.Contains(output, "not writable") {
		t.Errorf("expected 'not writable' detail, got: %s", output)
	}
}

// TestDoctor_FailsWhenSkillPathsDoNotExist verifies that doctor reports FAIL
// when configured skill paths point to nonexistent directories.
func TestDoctor_FailsWhenSkillPathsDoNotExist(t *testing.T) {
	dir := t.TempDir()
	setupHealthyProject(t, dir)

	// Overwrite config.json with a skill path that does not exist.
	cfg := engine.GlobalConfig{
		SkillPaths:    []string{"nonexistent/skills"},
		StateTTLHours: 24,
		DefaultAgent:  "*",
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	err = os.WriteFile(filepath.Join(dir, ".care-bare", "config.json"), data, 0o644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	output, execErr := runDoctorInDir(t, dir)

	if execErr == nil {
		t.Fatal("expected doctor to return error for missing skill paths, got nil")
	}

	if !strings.Contains(output, "[FAIL] Skill path:") {
		t.Errorf("expected skill path FAIL, got: %s", output)
	}
	if !strings.Contains(output, "does not exist") {
		t.Errorf("expected 'does not exist' detail, got: %s", output)
	}
}

// TestDoctor_FailsWhenSkillPathExistsButEmpty verifies that doctor reports
// FAIL when a skill path exists but contains no skill files.
func TestDoctor_FailsWhenSkillPathExistsButEmpty(t *testing.T) {
	dir := t.TempDir()
	setupHealthyProject(t, dir)

	// Remove the skill file but keep the directory.
	skillFile := filepath.Join(dir, ".claude", "skills", "go-standards", "SKILL.md")
	err := os.Remove(skillFile)
	if err != nil {
		t.Fatalf("failed to remove skill file: %v", err)
	}

	output, execErr := runDoctorInDir(t, dir)

	if execErr == nil {
		t.Fatal("expected doctor to return error for empty skill path, got nil")
	}

	if !strings.Contains(output, "[FAIL] Skill path:") {
		t.Errorf("expected skill path FAIL, got: %s", output)
	}
	if !strings.Contains(output, "no skill files") {
		t.Errorf("expected 'no skill files' detail, got: %s", output)
	}
}

// TestDoctor_MissingStateDirectory verifies that doctor reports FAIL when
// the .care-bare/state/ directory does not exist.
func TestDoctor_MissingStateDirectory(t *testing.T) {
	dir := t.TempDir()
	setupHealthyProject(t, dir)

	// Remove the state directory.
	err := os.RemoveAll(filepath.Join(dir, ".care-bare", "state"))
	if err != nil {
		t.Fatalf("failed to remove state directory: %v", err)
	}

	output, execErr := runDoctorInDir(t, dir)

	if execErr == nil {
		t.Fatal("expected doctor to return error for missing state dir, got nil")
	}

	if !strings.Contains(output, "[FAIL] State directory") {
		t.Errorf("expected state directory FAIL, got: %s", output)
	}
	if !strings.Contains(output, "does not exist") {
		t.Errorf("expected 'does not exist' detail, got: %s", output)
	}
}

// TestDoctor_UnsupportedConfigVersion verifies that doctor reports FAIL
// when skill_enforcement.json has an unsupported version.
func TestDoctor_UnsupportedConfigVersion(t *testing.T) {
	dir := t.TempDir()
	setupHealthyProject(t, dir)

	// Overwrite enforcement config with version 99.
	cfg := map[string]any{
		"version": 99,
		"tools":   []any{},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	err = os.WriteFile(filepath.Join(dir, ".care-bare", "skill_enforcement.json"), data, 0o644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	output, execErr := runDoctorInDir(t, dir)

	if execErr == nil {
		t.Fatal("expected doctor to return error for unsupported version, got nil")
	}

	if !strings.Contains(output, "[FAIL] Config validity: skill_enforcement.json") {
		t.Errorf("expected config FAIL, got: %s", output)
	}
	if !strings.Contains(output, "unsupported version") {
		t.Errorf("expected unsupported version detail, got: %s", output)
	}
}

// TestDoctor_NoConfigFilesIsAcceptable verifies that doctor does not fail
// when no config files exist (defaults are used).
func TestDoctor_NoConfigFilesIsAcceptable(t *testing.T) {
	dir := t.TempDir()

	// Create only the state directory (minimal setup).
	err := os.MkdirAll(filepath.Join(dir, ".care-bare", "state"), 0o755)
	if err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}

	output, _ := runDoctorInDir(t, dir)

	// Config checks should pass (files not present = acceptable).
	if !strings.Contains(output, "[PASS] Config validity: skill_enforcement.json") {
		t.Errorf("expected enforcement config PASS when missing, got: %s", output)
	}
	if !strings.Contains(output, "[PASS] Config validity: config.json") {
		t.Errorf("expected global config PASS when missing, got: %s", output)
	}
}
