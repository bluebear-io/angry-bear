// scanner_test.go contains unit tests for the ScanSkills function,
// which discovers skill definitions by walking directory paths.
package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFixture is a test helper that creates a file at the given path
// with the given content, creating parent directories as needed.
func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}

// validClaudeSkill returns valid SKILL.md content with the given name and description.
func validClaudeSkill(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n"
}

// validCursorRule returns valid .mdc content with the given name and description.
func validCursorRule(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n"
}

// TestScanSkillsFindsClaudeSkills verifies that ScanSkills discovers SKILL.md files.
func TestScanSkillsFindsClaudeSkills(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	writeFixture(t,
		filepath.Join(tmpDir, "skills", "go-standards", "SKILL.md"),
		validClaudeSkill("go-standards", "Go coding standards"),
	)
	writeFixture(t,
		filepath.Join(tmpDir, "skills", "linear-workflow", "SKILL.md"),
		validClaudeSkill("linear-workflow", "Linear workflow integration"),
	)

	skills, err := ScanSkills([]string{filepath.Join(tmpDir, "skills")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Should be sorted alphabetically
	if skills[0].Name != "go-standards" {
		t.Errorf("skills[0].Name = %q, want %q", skills[0].Name, "go-standards")
	}
	if skills[1].Name != "linear-workflow" {
		t.Errorf("skills[1].Name = %q, want %q", skills[1].Name, "linear-workflow")
	}

	// Agent should be set to "claude"
	for _, s := range skills {
		if s.Agent != "claude" {
			t.Errorf("skill %q: Agent = %q, want %q", s.Name, s.Agent, "claude")
		}
	}
}

// TestScanSkillsFindsCursorRules verifies that ScanSkills discovers .mdc files.
func TestScanSkillsFindsCursorRules(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	writeFixture(t,
		filepath.Join(tmpDir, "rules", "security.mdc"),
		validCursorRule("security", "Security rules"),
	)
	writeFixture(t,
		filepath.Join(tmpDir, "rules", "testing.mdc"),
		validCursorRule("testing", "Testing rules"),
	)

	skills, err := ScanSkills([]string{filepath.Join(tmpDir, "rules")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	if skills[0].Name != "security" {
		t.Errorf("skills[0].Name = %q, want %q", skills[0].Name, "security")
	}
	if skills[1].Name != "testing" {
		t.Errorf("skills[1].Name = %q, want %q", skills[1].Name, "testing")
	}

	for _, s := range skills {
		if s.Agent != "cursor" {
			t.Errorf("skill %q: Agent = %q, want %q", s.Name, s.Agent, "cursor")
		}
	}
}

// TestScanSkillsEmptyDirectory verifies that ScanSkills returns an empty list
// for a directory that exists but contains no skill files.
func TestScanSkillsEmptyDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	skills, err := ScanSkills([]string{tmpDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

// TestScanSkillsNonexistentPath verifies that ScanSkills returns an empty list
// (not an error) for paths that do not exist.
func TestScanSkillsNonexistentPath(t *testing.T) {
	t.Parallel()

	skills, err := ScanSkills([]string{"/tmp/does-not-exist-care-bear-test-99999"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

// TestScanSkillsDeduplicatesByName verifies that when the same skill name
// appears in multiple paths, only the first discovered instance is kept.
func TestScanSkillsDeduplicatesByName(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create same-named skill in two different paths
	path1 := filepath.Join(tmpDir, "path1")
	path2 := filepath.Join(tmpDir, "path2")

	writeFixture(t,
		filepath.Join(path1, "my-skill", "SKILL.md"),
		validClaudeSkill("my-skill", "First version"),
	)
	writeFixture(t,
		filepath.Join(path2, "my-skill", "SKILL.md"),
		validClaudeSkill("my-skill", "Second version"),
	)

	skills, err := ScanSkills([]string{path1, path2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (deduplicated), got %d", len(skills))
	}

	if skills[0].Description != "First version" {
		t.Errorf("expected first-discovered skill, got Description = %q", skills[0].Description)
	}
}

// TestScanSkillsSortsAlphabetically verifies that results are sorted by name.
func TestScanSkillsSortsAlphabetically(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create skills in non-alphabetical order
	writeFixture(t,
		filepath.Join(tmpDir, "zebra", "SKILL.md"),
		validClaudeSkill("zebra", "Last alphabetically"),
	)
	writeFixture(t,
		filepath.Join(tmpDir, "alpha", "SKILL.md"),
		validClaudeSkill("alpha", "First alphabetically"),
	)
	writeFixture(t,
		filepath.Join(tmpDir, "middle", "SKILL.md"),
		validClaudeSkill("middle", "Middle alphabetically"),
	)

	skills, err := ScanSkills([]string{tmpDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(skills))
	}

	expected := []string{"alpha", "middle", "zebra"}
	for i, want := range expected {
		if skills[i].Name != want {
			t.Errorf("skills[%d].Name = %q, want %q", i, skills[i].Name, want)
		}
	}
}

// TestScanSkillsMultiplePaths verifies that ScanSkills scans all provided paths
// and merges results from Claude and Cursor skill sources.
func TestScanSkillsMultiplePaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	claudePath := filepath.Join(tmpDir, "claude-skills")
	cursorPath := filepath.Join(tmpDir, "cursor-rules")

	writeFixture(t,
		filepath.Join(claudePath, "go-standards", "SKILL.md"),
		validClaudeSkill("go-standards", "Go coding standards"),
	)
	writeFixture(t,
		filepath.Join(cursorPath, "security.mdc"),
		validCursorRule("security", "Security rules"),
	)

	skills, err := ScanSkills([]string{claudePath, cursorPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Verify mixed agents are present (sorted alphabetically: go-standards, security)
	if skills[0].Name != "go-standards" || skills[0].Agent != "claude" {
		t.Errorf("skills[0] = {Name: %q, Agent: %q}, want {go-standards, claude}", skills[0].Name, skills[0].Agent)
	}
	if skills[1].Name != "security" || skills[1].Agent != "cursor" {
		t.Errorf("skills[1] = {Name: %q, Agent: %q}, want {security, cursor}", skills[1].Name, skills[1].Agent)
	}
}

// TestScanSkillsFallsBackToDirectoryName verifies that skills without frontmatter
// use the parent directory name as the skill name.
func TestScanSkillsFallsBackToDirectoryName(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	writeFixture(t,
		filepath.Join(tmpDir, "plain-skill", "SKILL.md"),
		"# Just a plain skill\n\nNo frontmatter here.\n",
	)

	skills, err := ScanSkills([]string{tmpDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "plain-skill" {
		t.Errorf("Name = %q, want %q (directory name fallback)", skills[0].Name, "plain-skill")
	}
}

// TestScanSkillsNilPaths verifies that ScanSkills handles nil paths gracefully.
func TestScanSkillsNilPaths(t *testing.T) {
	t.Parallel()

	skills, err := ScanSkills(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

// TestScanSkillsEmptyPaths verifies that ScanSkills handles empty path slice gracefully.
func TestScanSkillsEmptyPaths(t *testing.T) {
	t.Parallel()

	skills, err := ScanSkills([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

// TestScanSkillsMdcFallsBackToFileStem verifies that .mdc files without frontmatter
// use the file stem (name without extension) as the skill name.
func TestScanSkillsMdcFallsBackToFileStem(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	writeFixture(t,
		filepath.Join(tmpDir, "linting.mdc"),
		"# Linting Rules\n\nNo frontmatter here.\n",
	)

	skills, err := ScanSkills([]string{tmpDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "linting" {
		t.Errorf("Name = %q, want %q (file stem fallback)", skills[0].Name, "linting")
	}
}
