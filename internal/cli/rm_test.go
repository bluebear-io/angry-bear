// rm_test.go contains integration tests for the angry-bear rm command.
// Tests exercise the command against real temporary filesystems, verifying
// that rules are removed correctly with full and partial matching.
package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Bear-Security/angry-bear/internal/cli"
	"github.com/Blue-Bear-Security/angry-bear/internal/engine"
)

// runRmInDir executes the rm command with the working directory set to dir
// and the --config flag pointing to the config file in dir. It captures stdout
// and returns it along with any error.
func runRmInDir(t *testing.T, dir string, extraArgs ...string) (string, error) {
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

	configPath := filepath.Join(dir, ".angry-bear", "skill_enforcement.json")

	cmd := cli.NewRootCommand()
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(new(bytes.Buffer))

	args := []string{"rm", "--config", configPath}
	args = append(args, extraArgs...)
	cmd.SetArgs(args)

	execErr := cmd.Execute()
	return outBuf.String(), execErr
}

// TestRm_RemovesAllRulesForSkill verifies that rm removes all rules matching
// the given skill name.
func TestRm_RemovesAllRulesForSkill(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
		{Tool: "Edit", Path: "**", Skill: "linear", Agent: "*"},
	})

	output, err := runRmInDir(t, dir, "go-standards")
	if err != nil {
		t.Fatalf("rm command returned error: %v", err)
	}

	if !strings.Contains(output, "Removed 2 rules for skill \"go-standards\"") {
		t.Errorf("expected removal message, got: %s", output)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 remaining rule, got %d", len(cfg.Tools))
	}
	if cfg.Tools[0].Skill != "linear" {
		t.Errorf("expected remaining skill linear, got %s", cfg.Tools[0].Skill)
	}
}

// TestRm_WithToolFilter verifies that rm with --tool only removes rules
// matching both the skill and the specified tool.
func TestRm_WithToolFilter(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
		{Tool: "Bash", Path: "**", Skill: "go-standards", Agent: "*"},
	})

	output, err := runRmInDir(t, dir, "go-standards", "--tool", "Edit")
	if err != nil {
		t.Fatalf("rm command returned error: %v", err)
	}

	if !strings.Contains(output, "Removed 1 rules for skill \"go-standards\"") {
		t.Errorf("expected removal of 1 rule, got: %s", output)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 remaining rules, got %d", len(cfg.Tools))
	}

	// Verify the Edit rule was removed.
	for _, r := range cfg.Tools {
		if r.Tool == "Edit" && r.Skill == "go-standards" {
			t.Error("expected Edit rule to be removed")
		}
	}
}

// TestRm_WithPathFilter verifies that rm with --path only removes rules
// matching both the skill and the specified path.
func TestRm_WithPathFilter(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Edit", Path: "**/*.ts", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**", Skill: "linear", Agent: "*"},
	})

	output, err := runRmInDir(t, dir, "go-standards", "--path", "**/*.go")
	if err != nil {
		t.Fatalf("rm command returned error: %v", err)
	}

	if !strings.Contains(output, "Removed 1 rules for skill \"go-standards\"") {
		t.Errorf("expected removal of 1 rule, got: %s", output)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 remaining rules, got %d", len(cfg.Tools))
	}
}

// TestRm_WithToolAndPathFilter verifies that rm with both --tool and --path
// only removes rules matching all three criteria.
func TestRm_WithToolAndPathFilter(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Edit", Path: "**/*.ts", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
	})

	output, err := runRmInDir(t, dir, "go-standards", "--tool", "Edit", "--path", "**/*.go")
	if err != nil {
		t.Fatalf("rm command returned error: %v", err)
	}

	if !strings.Contains(output, "Removed 1 rules for skill \"go-standards\"") {
		t.Errorf("expected removal of 1 rule, got: %s", output)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 remaining rules, got %d", len(cfg.Tools))
	}

	// Verify only the Edit+**/*.go rule was removed.
	for _, r := range cfg.Tools {
		if r.Tool == "Edit" && r.Path == "**/*.go" && r.Skill == "go-standards" {
			t.Error("expected Edit+**/*.go rule to be removed")
		}
	}
}

// TestRm_NoMatchingRules verifies that rm with a non-existent skill shows
// the "no matching rules" message without error.
func TestRm_NoMatchingRules(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
	})

	output, err := runRmInDir(t, dir, "nonexistent")
	if err != nil {
		t.Fatalf("rm command returned error: %v", err)
	}

	if !strings.Contains(output, "No matching rules found for skill \"nonexistent\"") {
		t.Errorf("expected no-match message, got: %s", output)
	}

	// Verify no rules were removed.
	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 1 {
		t.Errorf("expected 1 rule unchanged, got %d", len(cfg.Tools))
	}
}

// TestRm_RequiresSkillArgument verifies that rm fails when no skill name
// is provided.
func TestRm_RequiresSkillArgument(t *testing.T) {
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, ".angry-bear"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .angry-bear: %v", err)
	}

	_, execErr := runRmInDir(t, dir)
	if execErr == nil {
		t.Fatal("expected error for missing skill argument, got nil")
	}
}

// TestRm_EmptyConfig verifies that rm handles a config file with no rules
// gracefully.
func TestRm_EmptyConfig(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})

	output, err := runRmInDir(t, dir, "go-standards")
	if err != nil {
		t.Fatalf("rm command returned error: %v", err)
	}

	if !strings.Contains(output, "No matching rules found") {
		t.Errorf("expected no-match message, got: %s", output)
	}
}

// TestRm_NoConfigFile verifies that rm handles a missing config file
// gracefully by reporting no matching rules.
func TestRm_NoConfigFile(t *testing.T) {
	dir := t.TempDir()

	// Create .angry-bear directory but no config file.
	err := os.MkdirAll(filepath.Join(dir, ".angry-bear"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .angry-bear: %v", err)
	}

	output, execErr := runRmInDir(t, dir, "go-standards")
	if execErr != nil {
		t.Fatalf("rm command returned error: %v", execErr)
	}

	if !strings.Contains(output, "No matching rules found") {
		t.Errorf("expected no-match message, got: %s", output)
	}
}

// TestRm_PathNormalization verifies that --path values are normalized via
// NormalizeGlob before matching against stored rules.
func TestRm_PathNormalization(t *testing.T) {
	dir := t.TempDir()

	// Stored rules have paths with slashes (preserved as-is by NormalizeGlob).
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "src/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**", Skill: "linear", Agent: "*"},
	})

	// Path in --path flag matches stored path directly.
	output, err := runRmInDir(t, dir, "go-standards", "--path", "src/*.go")
	if err != nil {
		t.Fatalf("rm command returned error: %v", err)
	}

	if !strings.Contains(output, "Removed 1 rules") {
		t.Errorf("expected removal with normalized path, got: %s", output)
	}
}

// TestRm_RepoFlag verifies that rm --repo removes rules from the repo-level
// config directory ({project}/.angry-bear/) instead of the machine config.
func TestRm_RepoFlag(t *testing.T) {
	dir := t.TempDir()

	// Write repo-level config.
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**", Skill: "linear", Agent: "*"},
	})

	configPath := filepath.Join(dir, ".angry-bear", "skill_enforcement.json")

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	err = os.Chdir(dir)
	if err != nil {
		t.Fatalf("failed to chdir: %v", err)
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
	cmd.SetArgs([]string{"rm", "--config", configPath, "go-standards"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("rm --repo returned error: %v", execErr)
	}

	output := outBuf.String()
	if !strings.Contains(output, "Removed 1 rules") {
		t.Errorf("expected removal message, got: %s", output)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 remaining rule, got %d", len(cfg.Tools))
	}
	if cfg.Tools[0].Skill != "linear" {
		t.Errorf("expected remaining skill linear, got %s", cfg.Tools[0].Skill)
	}
}

// TestRm_MultipleToolFilter verifies that --tool with comma-separated values
// removes rules matching any of the specified tools.
func TestRm_MultipleToolFilter(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
		{Tool: "Bash", Path: "**", Skill: "go-standards", Agent: "*"},
	})

	output, err := runRmInDir(t, dir, "go-standards", "--tool", "Edit,Write")
	if err != nil {
		t.Fatalf("rm command returned error: %v", err)
	}

	if !strings.Contains(output, "Removed 2 rules") {
		t.Errorf("expected removal of 2 rules, got: %s", output)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 remaining rule, got %d", len(cfg.Tools))
	}
	if cfg.Tools[0].Tool != "Bash" {
		t.Errorf("expected remaining tool Bash, got %s", cfg.Tools[0].Tool)
	}
}
