// parser_test.go contains unit tests for the ParseSkillFile function,
// which extracts skill metadata from YAML frontmatter in skill files.
package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseSkillFile validates frontmatter extraction and fallback behavior
// across multiple skill file formats and edge cases.
func TestParseSkillFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		dir         string // Parent directory name (used for fallback)
		filename    string // File name within the directory
		content     string
		wantName    string
		wantDesc    string
		wantErr     bool
		wantSource  bool // If true, assert Source is set to the file path
	}{
		{
			name:     "extracts name from YAML frontmatter",
			dir:      "some-skill",
			filename: "SKILL.md",
			content: `---
name: go-coding-standards
description: Enforces Go coding standards for the project
---

# Go Coding Standards

Use when writing Go code...
`,
			wantName: "go-coding-standards",
			wantDesc: "Enforces Go coding standards for the project",
			wantErr:  false,
		},
		{
			name:     "extracts description from YAML frontmatter",
			dir:      "my-skill",
			filename: "SKILL.md",
			content: `---
name: testing-workflow
description: Manages testing workflow and TDD practices
---

# Testing
`,
			wantName: "testing-workflow",
			wantDesc: "Manages testing workflow and TDD practices",
			wantErr:  false,
		},
		{
			name:     "handles missing frontmatter uses directory name for SKILL.md",
			dir:      "plain-skill",
			filename: "SKILL.md",
			content: `# Just a plain skill

Some description in body text.
`,
			wantName: "plain-skill",
			wantDesc: "",
			wantErr:  false,
		},
		{
			name:     "handles missing frontmatter uses file stem for mdc",
			dir:      "rules",
			filename: "security-check.mdc",
			content: `# Security Check

Some rules here.
`,
			wantName: "security-check",
			wantDesc: "",
			wantErr:  false,
		},
		{
			name:     "handles malformed YAML falls back to directory name",
			dir:      "broken-skill",
			filename: "SKILL.md",
			content: `---
name: [invalid yaml
description: {broken
---

# Broken
`,
			wantName: "broken-skill",
			wantDesc: "",
			wantErr:  false,
		},
		{
			name:     "sets Source to the file path",
			dir:      "source-skill",
			filename: "SKILL.md",
			content: `---
name: source-test
description: Tests source path
---
`,
			wantName:   "source-test",
			wantDesc:   "Tests source path",
			wantErr:    false,
			wantSource: true,
		},
		{
			name:     "empty file falls back to directory name",
			dir:      "empty-skill",
			filename: "SKILL.md",
			content:  "",
			wantName: "empty-skill",
			wantDesc: "",
			wantErr:  false,
		},
		{
			name:     "frontmatter with no name key falls back to directory name",
			dir:      "noname-skill",
			filename: "SKILL.md",
			content: `---
description: Has description but no name
---

# No Name
`,
			wantName: "noname-skill",
			wantDesc: "Has description but no name",
			wantErr:  false,
		},
		{
			name:     "frontmatter with empty name value falls back to directory name",
			dir:      "emptyname-skill",
			filename: "SKILL.md",
			content: `---
name: ""
description: Empty name value
---
`,
			wantName: "emptyname-skill",
			wantDesc: "Empty name value",
			wantErr:  false,
		},
		{
			name:     "single delimiter treated as no frontmatter",
			dir:      "single-delim",
			filename: "SKILL.md",
			content: `---
name: wont-be-parsed
description: Only one delimiter
`,
			wantName: "single-delim",
			wantDesc: "",
			wantErr:  false,
		},
		{
			name:     "BOM at file start is stripped before checking frontmatter",
			dir:      "bom-skill",
			filename: "SKILL.md",
			content: "\xef\xbb\xbf---\nname: bom-skill-name\ndescription: Has BOM\n---\n",
			wantName: "bom-skill-name",
			wantDesc: "Has BOM",
			wantErr:  false,
		},
		{
			name:     "mdc file with frontmatter extracts name",
			dir:      "cursor-rules",
			filename: "review.mdc",
			content: `---
name: code-review
description: Code review rules
---

# Code Review
`,
			wantName: "code-review",
			wantDesc: "Code review rules",
			wantErr:  false,
		},
		{
			name:     "mdc file without frontmatter uses file stem",
			dir:      "cursor-rules",
			filename: "linting.mdc",
			content: `# Linting Rules

Apply linting rules.
`,
			wantName: "linting",
			wantDesc: "",
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create temp directory structure
			tmpDir := t.TempDir()
			skillDir := filepath.Join(tmpDir, tc.dir)
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				t.Fatalf("failed to create skill dir: %v", err)
			}

			filePath := filepath.Join(skillDir, tc.filename)
			if err := os.WriteFile(filePath, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("failed to write fixture file: %v", err)
			}

			skill, err := ParseSkillFile(filePath)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if skill == nil {
				t.Fatal("expected non-nil skill")
			}
			if skill.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", skill.Name, tc.wantName)
			}
			if skill.Description != tc.wantDesc {
				t.Errorf("Description = %q, want %q", skill.Description, tc.wantDesc)
			}
			if tc.wantSource && skill.Source != filePath {
				t.Errorf("Source = %q, want %q", skill.Source, filePath)
			}
			if !tc.wantSource && skill.Source == "" {
				t.Error("Source should not be empty")
			}
		})
	}
}

// TestParseSkillFileNonexistent verifies that parsing a nonexistent file returns an error.
func TestParseSkillFileNonexistent(t *testing.T) {
	t.Parallel()

	_, err := ParseSkillFile("/tmp/nonexistent-skill-file-12345/SKILL.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}
