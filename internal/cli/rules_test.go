// rules_test.go contains integration tests for the angry-bear rules command.
// Tests exercise the command against real temporary filesystems, verifying
// table output, JSON output, skill filtering, and empty-state behavior.
package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluebear-io/angry-bear/internal/cli"
	"github.com/bluebear-io/angry-bear/internal/engine"
)

// runRulesInDir executes the rules command with the working directory set
// to dir and the --config flag pointing to the config file in dir.
// It captures stdout and returns it along with any error.
func runRulesInDir(t *testing.T, dir string, extraArgs ...string) (string, error) {
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

	args := []string{"rules", "--config", configPath}
	args = append(args, extraArgs...)
	cmd.SetArgs(args)

	execErr := cmd.Execute()
	return outBuf.String(), execErr
}

// TestRules_DisplaysAllRules verifies that rules shows all configured rules
// in table format with correct numbering and field values.
func TestRules_DisplaysAllRules(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**", Skill: "linear", Agent: "*"},
	})

	output, err := runRulesInDir(t, dir)
	if err != nil {
		t.Fatalf("rules command returned error: %v", err)
	}

	if !strings.Contains(output, "Enforcement Rules") {
		t.Errorf("expected header, got: %s", output)
	}
	if !strings.Contains(output, "[1]") {
		t.Errorf("expected [1] numbering, got: %s", output)
	}
	if !strings.Contains(output, "[2]") {
		t.Errorf("expected [2] numbering, got: %s", output)
	}
	if !strings.Contains(output, "go-standards") {
		t.Errorf("expected go-standards in output, got: %s", output)
	}
	if !strings.Contains(output, "linear") {
		t.Errorf("expected linear in output, got: %s", output)
	}
	if !strings.Contains(output, "2 rules") {
		t.Errorf("expected '2 rules' summary, got: %s", output)
	}
}

// TestRules_FiltersbySkill verifies that --skill filters rules to only the
// specified skill.
func TestRules_FiltersbySkill(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**", Skill: "linear", Agent: "*"},
		{Tool: "Bash", Path: "**/*.go", Skill: "go-standards", Agent: "*"},
	})

	output, err := runRulesInDir(t, dir, "--skill", "go-standards")
	if err != nil {
		t.Fatalf("rules command returned error: %v", err)
	}

	if !strings.Contains(output, "go-standards") {
		t.Errorf("expected go-standards in output, got: %s", output)
	}
	if strings.Contains(output, "linear") {
		t.Errorf("linear should be filtered out, got: %s", output)
	}
	if !strings.Contains(output, "2 rules") {
		t.Errorf("expected '2 rules' summary, got: %s", output)
	}
}

// TestRules_SkillFilterNoMatch verifies that --skill with a non-existent skill
// shows the "no rules found" message.
func TestRules_SkillFilterNoMatch(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
	})

	output, err := runRulesInDir(t, dir, "--skill", "nonexistent")
	if err != nil {
		t.Fatalf("rules command returned error: %v", err)
	}

	if !strings.Contains(output, "No rules found for skill \"nonexistent\"") {
		t.Errorf("expected 'no rules found' message, got: %s", output)
	}
}

// TestRules_JSONOutput verifies that --json produces valid JSON output
// with the correct structure and rule data.
func TestRules_JSONOutput(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**", Skill: "linear", Agent: "*"},
	})

	output, err := runRulesInDir(t, dir, "--json")
	if err != nil {
		t.Fatalf("rules command returned error: %v", err)
	}

	// Verify valid JSON.
	var result struct {
		Source string        `json:"source"`
		Rules  []engine.Rule `json:"rules"`
	}
	err = json.Unmarshal([]byte(output), &result)
	if err != nil {
		t.Fatalf("invalid JSON output: %v\nOutput: %s", err, output)
	}

	if len(result.Rules) != 2 {
		t.Fatalf("expected 2 rules in JSON, got %d", len(result.Rules))
	}

	if result.Rules[0].Skill != "go-standards" {
		t.Errorf("expected first rule skill go-standards, got %s", result.Rules[0].Skill)
	}
	if result.Rules[1].Skill != "linear" {
		t.Errorf("expected second rule skill linear, got %s", result.Rules[1].Skill)
	}

	if result.Source == "" {
		t.Error("expected non-empty source in JSON output")
	}
}

// TestRules_JSONOutputWithSkillFilter verifies that --json combined with
// --skill filters correctly in JSON mode.
func TestRules_JSONOutputWithSkillFilter(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-standards", Agent: "claude"},
		{Tool: "Write", Path: "**", Skill: "linear", Agent: "*"},
	})

	output, err := runRulesInDir(t, dir, "--json", "--skill", "go-standards")
	if err != nil {
		t.Fatalf("rules command returned error: %v", err)
	}

	var result struct {
		Source string        `json:"source"`
		Rules  []engine.Rule `json:"rules"`
	}
	err = json.Unmarshal([]byte(output), &result)
	if err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if len(result.Rules) != 1 {
		t.Fatalf("expected 1 rule in filtered JSON, got %d", len(result.Rules))
	}
	if result.Rules[0].Skill != "go-standards" {
		t.Errorf("expected skill go-standards, got %s", result.Rules[0].Skill)
	}
}

// TestRules_EmptyConfig verifies that rules handles a config file with no
// rules gracefully.
func TestRules_EmptyConfig(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})

	output, err := runRulesInDir(t, dir)
	if err != nil {
		t.Fatalf("rules command returned error: %v", err)
	}

	if !strings.Contains(output, "No enforcement rules configured.") {
		t.Errorf("expected empty rules message, got: %s", output)
	}
}

// TestRules_NoConfigFile verifies that rules handles a missing config file
// gracefully by showing no rules.
func TestRules_NoConfigFile(t *testing.T) {
	dir := t.TempDir()

	// Create .angry-bear directory but no config file.
	err := os.MkdirAll(filepath.Join(dir, ".angry-bear"), 0o755)
	if err != nil {
		t.Fatalf("failed to create .angry-bear: %v", err)
	}

	output, execErr := runRulesInDir(t, dir)
	if execErr != nil {
		t.Fatalf("rules command returned error: %v", execErr)
	}

	if !strings.Contains(output, "No enforcement rules configured.") {
		t.Errorf("expected empty rules message, got: %s", output)
	}
}

// TestRules_JSONEmptyRulesIsArray verifies that --json with no rules produces
// an empty JSON array (not null).
func TestRules_JSONEmptyRulesIsArray(t *testing.T) {
	dir := t.TempDir()

	writeEnforcementConfig(t, dir, []engine.Rule{})

	output, err := runRulesInDir(t, dir, "--json")
	if err != nil {
		t.Fatalf("rules command returned error: %v", err)
	}

	var result struct {
		Rules []engine.Rule `json:"rules"`
	}
	err = json.Unmarshal([]byte(output), &result)
	if err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	// Verify rules is an empty array, not null.
	if result.Rules == nil {
		t.Error("expected empty array [], got null")
	}
	if len(result.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(result.Rules))
	}
}
