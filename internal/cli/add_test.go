// add_test.go contains integration tests for the angry-bear add command.
// Tests exercise the command against real temporary filesystems, verifying
// that rules are created correctly, cartesian products are generated, and
// deduplication works as expected.
package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluebear-io/angry-bear/internal/cli"
	"github.com/bluebear-io/angry-bear/internal/engine"
)

// runAddInDir executes the add command with the working directory set to dir
// and the --config flag pointing to the config file in dir. It captures stdout
// and returns it along with any error.
func runAddInDir(t *testing.T, dir string, extraArgs ...string) (string, error) {
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

	args := []string{"add", "--config", configPath}
	args = append(args, extraArgs...)
	cmd.SetArgs(args)

	execErr := cmd.Execute()
	return outBuf.String(), execErr
}

// readConfigFromDir reads and parses the skill_enforcement.json from a temp dir.
func readConfigFromDir(t *testing.T, dir string) engine.Config {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".angry-bear", "skill_enforcement.json"))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	var cfg engine.Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	return cfg
}

// TestAdd_CreatesRulesWithDefaults verifies that add with only a skill name
// creates a single rule with default tool (*), path (**), and agent (*).
func TestAdd_CreatesRulesWithDefaults(t *testing.T) {
	dir := t.TempDir()

	// Create .angry-bear directory so project root resolves.
	err := os.MkdirAll(filepath.Join(dir, ".angry-bear"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .angry-bear: %v", err)
	}

	output, err := runAddInDir(t, dir, "my-skill")
	if err != nil {
		t.Fatalf("add command returned error: %v", err)
	}

	if output != "Added 1 rules for skill \"my-skill\"\n" {
		t.Errorf("unexpected output: %s", output)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Tools))
	}

	r := cfg.Tools[0]
	if r.Skill != "my-skill" {
		t.Errorf("expected skill my-skill, got %s", r.Skill)
	}
	if r.Tool != "*" {
		t.Errorf("expected tool *, got %s", r.Tool)
	}
	if r.Path != "**" {
		t.Errorf("expected path **, got %s", r.Path)
	}
	if r.Agent != "*" {
		t.Errorf("expected agent *, got %s", r.Agent)
	}
}

// TestAdd_CartesianProduct verifies that comma-separated tools, paths, and agents
// generate the correct cartesian product of rules.
func TestAdd_CartesianProduct(t *testing.T) {
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, ".angry-bear"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .angry-bear: %v", err)
	}

	output, err := runAddInDir(t, dir, "go-standards",
		"--tool", "Edit,Write",
		"--path", "**/*.go,**/*.mod",
		"--agent", "claude,cursor",
	)
	if err != nil {
		t.Fatalf("add command returned error: %v", err)
	}

	// 2 tools x 2 paths x 2 agents = 8 rules
	if output != "Added 8 rules for skill \"go-standards\"\n" {
		t.Errorf("unexpected output: %s", output)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 8 {
		t.Fatalf("expected 8 rules, got %d", len(cfg.Tools))
	}

	// Verify all expected combinations exist.
	expected := map[string]bool{
		"Edit|**/*.go|go-standards|claude":   true,
		"Edit|**/*.go|go-standards|cursor":   true,
		"Edit|**/*.mod|go-standards|claude":  true,
		"Edit|**/*.mod|go-standards|cursor":  true,
		"Write|**/*.go|go-standards|claude":  true,
		"Write|**/*.go|go-standards|cursor":  true,
		"Write|**/*.mod|go-standards|claude": true,
		"Write|**/*.mod|go-standards|cursor": true,
	}

	for _, r := range cfg.Tools {
		key := r.Tool + "|" + r.Path + "|" + r.Skill + "|" + r.Agent
		if !expected[key] {
			t.Errorf("unexpected rule: %s", key)
		}
		delete(expected, key)
	}

	for key := range expected {
		t.Errorf("missing expected rule: %s", key)
	}
}

// TestAdd_NormalizesGlob verifies that relative paths are normalized with
// the **/ prefix via NormalizeGlob.
func TestAdd_NormalizesGlob(t *testing.T) {
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, ".angry-bear"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .angry-bear: %v", err)
	}

	_, err = runAddInDir(t, dir, "sst-architect",
		"--path", "stacks/*.ts",
	)
	if err != nil {
		t.Fatalf("add command returned error: %v", err)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cfg.Tools))
	}

	// NormalizeGlob preserves paths with slashes (specific directories).
	if cfg.Tools[0].Path != "stacks/*.ts" {
		t.Errorf("expected path stacks/*.ts, got %s", cfg.Tools[0].Path)
	}
}

// TestAdd_DeduplicatesRules verifies that running add twice with the same
// parameters does not create duplicate rules.
func TestAdd_DeduplicatesRules(t *testing.T) {
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, ".angry-bear"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .angry-bear: %v", err)
	}

	// First add.
	_, err = runAddInDir(t, dir, "go-standards", "--tool", "Edit", "--path", "**/*.go")
	if err != nil {
		t.Fatalf("first add returned error: %v", err)
	}

	// Second add with same parameters -- need to chdir again since cleanup restores.
	output, err := runAddInDir(t, dir, "go-standards", "--tool", "Edit", "--path", "**/*.go")
	if err != nil {
		t.Fatalf("second add returned error: %v", err)
	}

	if output != "No new rules added for skill \"go-standards\" (all already exist)\n" {
		t.Errorf("expected dedup message, got: %s", output)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 1 {
		t.Errorf("expected 1 rule after dedup, got %d", len(cfg.Tools))
	}
}

// TestAdd_AppendsToExistingConfig verifies that add appends rules to an
// existing config file without overwriting previous rules.
func TestAdd_AppendsToExistingConfig(t *testing.T) {
	dir := t.TempDir()

	// Write an initial config with one rule.
	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Bash", Path: "**", Skill: "existing-skill", Agent: "*"},
	})

	_, err := runAddInDir(t, dir, "new-skill", "--tool", "Edit")
	if err != nil {
		t.Fatalf("add command returned error: %v", err)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cfg.Tools))
	}

	// Verify both skills are present.
	skills := make(map[string]bool)
	for _, r := range cfg.Tools {
		skills[r.Skill] = true
	}
	if !skills["existing-skill"] {
		t.Error("expected existing-skill to be preserved")
	}
	if !skills["new-skill"] {
		t.Error("expected new-skill to be added")
	}
}

// TestAdd_RequiresSkillArgument verifies that add fails when no skill name
// is provided.
func TestAdd_RequiresSkillArgument(t *testing.T) {
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, ".angry-bear"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .angry-bear: %v", err)
	}

	_, execErr := runAddInDir(t, dir)
	if execErr == nil {
		t.Fatal("expected error for missing skill argument, got nil")
	}
}

// TestAdd_CreatesConfigDirectory verifies that add creates the .angry-bear
// directory if it does not exist.
func TestAdd_CreatesConfigDirectory(t *testing.T) {
	dir := t.TempDir()

	// Use --config flag to point to a path within the temp dir.
	configPath := filepath.Join(dir, ".angry-bear", "skill_enforcement.json")

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get original working directory: %v", err)
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
	cmd.SetArgs([]string{"add", "--config", configPath, "my-skill"})

	execErr := cmd.Execute()
	if execErr != nil {
		t.Fatalf("add command returned error: %v", execErr)
	}

	// Verify the config file was created.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("expected config file to be created")
	}
}

// TestAdd_MultiplePathsNormalized verifies that multiple comma-separated
// paths are each normalized independently.
func TestAdd_MultiplePathsNormalized(t *testing.T) {
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, ".angry-bear"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .angry-bear: %v", err)
	}

	_, err = runAddInDir(t, dir, "linear",
		"--tool", "Edit",
		"--path", "src/*.py,lib/*.ts",
	)
	if err != nil {
		t.Fatalf("add command returned error: %v", err)
	}

	cfg := readConfigFromDir(t, dir)
	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cfg.Tools))
	}

	paths := make(map[string]bool)
	for _, r := range cfg.Tools {
		paths[r.Path] = true
	}

	if !paths["src/*.py"] {
		t.Error("expected src/*.py")
	}
	if !paths["lib/*.ts"] {
		t.Error("expected lib/*.ts")
	}
}
