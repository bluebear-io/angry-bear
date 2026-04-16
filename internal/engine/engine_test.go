// engine_test.go contains comprehensive table-driven tests for the angry-bear
// enforcement engine. Tests cover config loading, rule matching, glob
// normalization, file path normalization, and project root resolution.
package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NormalizeGlob tests
// ---------------------------------------------------------------------------

func TestNormalizeGlob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{
			name:    "empty string unchanged",
			pattern: "",
			want:    "",
		},
		{
			name:    "wildcard star unchanged",
			pattern: "*",
			want:    "*",
		},
		{
			name:    "absolute path unchanged",
			pattern: "/usr/local/bin/*.go",
			want:    "/usr/local/bin/*.go",
		},
		{
			name:    "already prefixed with doublestar unchanged",
			pattern: "**/handler/**",
			want:    "**/handler/**",
		},
		{
			name:    "relative path with slash preserved as-is",
			pattern: "handler/**",
			want:    "handler/**",
		},
		{
			name:    "extension pattern gets doublestar prefix",
			pattern: "*.go",
			want:    "**/*.go",
		},
		{
			name:    "nested relative path preserved as-is",
			pattern: "internal/engine/*.go",
			want:    "internal/engine/*.go",
		},
		{
			name:    "double star without slash unchanged",
			pattern: "**",
			want:    "**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeGlob(tt.pattern)
			if got != tt.want {
				t.Errorf("NormalizeGlob(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NormalizeFilePath tests
// ---------------------------------------------------------------------------

func TestNormalizeFilePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filePath    string
		projectRoot string
		want        string
	}{
		{
			name:        "backslashes converted to forward slashes",
			filePath:    `internal\engine\types.go`,
			projectRoot: "/project",
			want:        "internal/engine/types.go",
		},
		{
			name:        "dot-dot segments cleaned",
			filePath:    "internal/engine/../engine/types.go",
			projectRoot: "/project",
			want:        "internal/engine/types.go",
		},
		{
			name:        "project root prefix stripped from absolute path",
			filePath:    "/project/internal/engine/types.go",
			projectRoot: "/project",
			want:        "internal/engine/types.go",
		},
		{
			name:        "relative path returned unchanged",
			filePath:    "internal/engine/types.go",
			projectRoot: "/project",
			want:        "internal/engine/types.go",
		},
		{
			name:        "project root with trailing slash handled",
			filePath:    "/project/internal/engine/types.go",
			projectRoot: "/project/",
			want:        "internal/engine/types.go",
		},
		{
			name:        "empty file path returns empty",
			filePath:    "",
			projectRoot: "/project",
			want:        "",
		},
	}

	// Windows-specific test
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			name        string
			filePath    string
			projectRoot string
			want        string
		}{
			name:        "Windows-style absolute paths",
			filePath:    `C:\Users\dev\project\main.go`,
			projectRoot: `C:\Users\dev\project`,
			want:        "main.go",
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeFilePath(tt.filePath, tt.projectRoot)
			if got != tt.want {
				t.Errorf("NormalizeFilePath(%q, %q) = %q, want %q", tt.filePath, tt.projectRoot, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MatchPath tests
// ---------------------------------------------------------------------------

func TestMatchPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pattern  string
		filePath string
		want     bool
		wantErr  bool
	}{
		{
			name:     "exact file match",
			pattern:  "internal/engine/types.go",
			filePath: "internal/engine/types.go",
			want:     true,
		},
		{
			name:     "doublestar matches nested path",
			pattern:  "**/engine/*.go",
			filePath: "internal/engine/types.go",
			want:     true,
		},
		{
			name:     "extension glob matches",
			pattern:  "**/*.go",
			filePath: "internal/engine/types.go",
			want:     true,
		},
		{
			name:     "no match returns false",
			pattern:  "**/*.py",
			filePath: "internal/engine/types.go",
			want:     false,
		},
		{
			name:     "star matches all",
			pattern:  "*",
			filePath: "anything.go",
			want:     true,
		},
		{
			name:     "empty pattern matches empty path",
			pattern:  "",
			filePath: "",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := MatchPath(tt.pattern, tt.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("MatchPath(%q, %q) error = %v, wantErr %v", tt.pattern, tt.filePath, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MatchPath(%q, %q) = %v, want %v", tt.pattern, tt.filePath, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ResolveProjectRoot tests
// ---------------------------------------------------------------------------

func TestResolveProjectRoot(t *testing.T) {
	t.Parallel()

	t.Run("finds nearest .git directory", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		root := filepath.Join(tmp, "project")
		sub := filepath.Join(root, "a", "b", "c")
		mustMkdirAll(t, filepath.Join(root, ".git"))
		mustMkdirAll(t, sub)

		got := ResolveProjectRoot(sub)
		if got != root {
			t.Errorf("ResolveProjectRoot(%q) = %q, want %q", sub, got, root)
		}
	})

	t.Run("falls back to nearest git directory", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		root := filepath.Join(tmp, "project")
		sub := filepath.Join(root, "a", "b")
		mustMkdirAll(t, filepath.Join(root, ".git"))
		mustMkdirAll(t, sub)

		got := ResolveProjectRoot(sub)
		if got != root {
			t.Errorf("ResolveProjectRoot(%q) = %q, want %q", sub, got, root)
		}
	})

	t.Run("angry-bear takes priority over git", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		root := filepath.Join(tmp, "project")
		sub := filepath.Join(root, "a", "b")
		mustMkdirAll(t, filepath.Join(root, ".git"))
		mustMkdirAll(t, filepath.Join(root, ".git"))
		mustMkdirAll(t, sub)

		got := ResolveProjectRoot(sub)
		if got != root {
			t.Errorf("ResolveProjectRoot(%q) = %q, want %q", sub, got, root)
		}
	})

	t.Run("falls back to startDir when neither found", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		sub := filepath.Join(tmp, "bare", "dir")
		mustMkdirAll(t, sub)

		got := ResolveProjectRoot(sub)
		if got != sub {
			t.Errorf("ResolveProjectRoot(%q) = %q, want %q", sub, got, sub)
		}
	})

	t.Run("works from subdirectory", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		root := filepath.Join(tmp, "project")
		deep := filepath.Join(root, "a", "b", "c", "d", "e")
		mustMkdirAll(t, filepath.Join(root, ".git"))
		mustMkdirAll(t, deep)

		got := ResolveProjectRoot(deep)
		if got != root {
			t.Errorf("ResolveProjectRoot(%q) = %q, want %q", deep, got, root)
		}
	})
}

// ---------------------------------------------------------------------------
// LoadConfig tests
// ---------------------------------------------------------------------------

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	t.Run("returns empty config when no files exist", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		mustMkdirAll(t, fakeHome)
		startDir := filepath.Join(tmp, "project")
		mustMkdirAll(t, startDir)

		rules, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if len(rules) != 0 {
			t.Errorf("expected 0 rules, got %d", len(rules))
		}
	})

	t.Run("reads user-level config", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		startDir := filepath.Join(tmp, "project")
		mustMkdirAll(t, startDir)

		writeConfig(t, fakeHome, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Edit", Path: "*.go", Skill: "go-coding-standards", Agent: "claude"},
			},
		})

		rules, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if len(rules) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(rules))
		}
		if rules[0].Rule.Skill != "go-coding-standards" {
			t.Errorf("expected skill 'go-coding-standards', got %q", rules[0].Rule.Skill)
		}
		if rules[0].Source == "" {
			t.Error("expected Source to be set")
		}
	})

	t.Run("reads project-level config", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		mustMkdirAll(t, fakeHome)
		startDir := filepath.Join(tmp, "project")

		writeConfig(t, startDir, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Bash", Skill: "backend-standards"},
			},
		})

		rules, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if len(rules) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(rules))
		}
		if rules[0].Rule.Skill != "backend-standards" {
			t.Errorf("expected skill 'backend-standards', got %q", rules[0].Rule.Skill)
		}
	})

	t.Run("merges rules from both levels", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		startDir := filepath.Join(tmp, "project")

		writeConfig(t, fakeHome, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Edit", Skill: "user-skill"},
			},
		})
		writeConfig(t, startDir, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Bash", Skill: "project-skill"},
			},
		})

		rules, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if len(rules) != 2 {
			t.Fatalf("expected 2 rules, got %d", len(rules))
		}

		skills := map[string]bool{}
		for _, r := range rules {
			skills[r.Rule.Skill] = true
		}
		if !skills["user-skill"] || !skills["project-skill"] {
			t.Errorf("expected both user-skill and project-skill, got %v", skills)
		}
	})

	t.Run("walks up directories collecting project-level configs", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		mustMkdirAll(t, fakeHome)

		// Create nested project structure
		root := filepath.Join(tmp, "project")
		sub := filepath.Join(root, "a", "b")
		mustMkdirAll(t, sub)

		// Config at root level
		writeConfig(t, root, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Edit", Skill: "root-skill"},
			},
		})

		// Config at intermediate level
		writeConfig(t, filepath.Join(root, "a"), Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Bash", Skill: "mid-skill"},
			},
		})

		rules, err := LoadConfig(sub, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}

		skills := map[string]bool{}
		for _, r := range rules {
			skills[r.Rule.Skill] = true
		}
		if !skills["root-skill"] {
			t.Error("expected root-skill from walk-up")
		}
		if !skills["mid-skill"] {
			t.Error("expected mid-skill from walk-up")
		}
	})

	t.Run("returns error on malformed JSON", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		mustMkdirAll(t, fakeHome)
		startDir := filepath.Join(tmp, "project")

		// Write malformed JSON
		dir := filepath.Join(startDir, ".angry-bear")
		mustMkdirAll(t, dir)
		err := os.WriteFile(filepath.Join(dir, "skill_enforcement.json"), []byte("{bad json"), 0o644)
		if err != nil {
			t.Fatal(err)
		}

		_, err = LoadConfig(startDir, WithHomeDir(fakeHome))
		if err == nil {
			t.Fatal("expected error on malformed JSON, got nil")
		}
	})

	t.Run("returns error on unsupported config version", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		mustMkdirAll(t, fakeHome)
		startDir := filepath.Join(tmp, "project")

		writeConfig(t, startDir, Config{
			Version: 2,
			Tools: []Rule{
				{Tool: "Edit", Skill: "some-skill"},
			},
		})

		_, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err == nil {
			t.Fatal("expected error on version 2, got nil")
		}
	})

	t.Run("accepts version 1 configs", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		mustMkdirAll(t, fakeHome)
		startDir := filepath.Join(tmp, "project")

		writeConfig(t, startDir, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Edit", Skill: "valid-skill"},
			},
		})

		rules, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if len(rules) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(rules))
		}
	})

	t.Run("tracks Source field on each MatchedRule", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		startDir := filepath.Join(tmp, "project")

		writeConfig(t, fakeHome, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Edit", Skill: "user-skill"},
			},
		})
		writeConfig(t, startDir, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Bash", Skill: "project-skill"},
			},
		})

		rules, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}

		for _, r := range rules {
			if r.Source == "" {
				t.Errorf("rule %q has empty Source", r.Rule.Skill)
			}
		}
	})

	t.Run("uses os.Stat before reading does not error on missing", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		mustMkdirAll(t, fakeHome)
		startDir := filepath.Join(tmp, "project")
		mustMkdirAll(t, startDir)

		// No config files anywhere -- should not error
		rules, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if len(rules) != 0 {
			t.Errorf("expected 0 rules, got %d", len(rules))
		}
	})

	t.Run("stops walking at home dir", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")

		// Put a config above the home dir -- should NOT be picked up
		writeConfig(t, tmp, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Edit", Skill: "above-home-skill"},
			},
		})

		startDir := filepath.Join(fakeHome, "project")
		mustMkdirAll(t, startDir)

		rules, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}

		for _, r := range rules {
			if r.Rule.Skill == "above-home-skill" {
				t.Error("should not have loaded config above home dir")
			}
		}
	})

	t.Run("normalizes glob paths on loaded rules", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		fakeHome := filepath.Join(tmp, "home")
		mustMkdirAll(t, fakeHome)
		startDir := filepath.Join(tmp, "project")

		writeConfig(t, startDir, Config{
			Version: 1,
			Tools: []Rule{
				{Tool: "Edit", Path: "*.go", Skill: "go-skill"},
			},
		})

		rules, err := LoadConfig(startDir, WithHomeDir(fakeHome))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if len(rules) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(rules))
		}
		// *.go should have been normalized to **/*.go
		if rules[0].Rule.Path != "**/*.go" {
			t.Errorf("expected path '**/*.go', got %q", rules[0].Rule.Path)
		}
	})
}

// ---------------------------------------------------------------------------
// ShouldBlock tests
// ---------------------------------------------------------------------------

func TestShouldBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rules         []MatchedRule
		toolName      string
		filePath      string
		agent         string
		invokedSkills map[string]bool
		wantBlocked   bool
		wantMissing   []string
		wantReasonHas string
	}{
		{
			name:        "no rules returns not blocked",
			rules:       nil,
			toolName:    "Edit",
			filePath:    "main.go",
			agent:       "claude",
			wantBlocked: false,
		},
		{
			name: "blocks when required skill not invoked",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Skill: "go-standards"}, Source: "/project/.angry-bear/skill_enforcement.json"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"go-standards"},
		},
		{
			name: "allows when required skill already invoked",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Skill: "go-standards"}, Source: "/project/.angry-bear/skill_enforcement.json"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{"go-standards": true},
			wantBlocked:   false,
		},
		{
			name: "matches tool name exactly",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Skill: "edit-skill"}, Source: "src1"},
				{Rule: Rule{Tool: "Bash", Skill: "bash-skill"}, Source: "src2"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"edit-skill"},
		},
		{
			name: "matches tool wildcard star",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "*", Skill: "universal-skill"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"universal-skill"},
		},
		{
			name: "matches empty tool field",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "", Skill: "all-tool-skill"}, Source: "src"},
			},
			toolName:      "Bash",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"all-tool-skill"},
		},
		{
			name: "matches agent name exactly",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Agent: "claude", Skill: "claude-skill"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "claude",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"claude-skill"},
		},
		{
			name: "matches agent wildcard star",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Agent: "*", Skill: "any-agent-skill"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "cursor",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"any-agent-skill"},
		},
		{
			name: "matches empty agent field",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Agent: "", Skill: "no-agent-skill"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "codex",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"no-agent-skill"},
		},
		{
			name: "skips rule when agent does not match",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Agent: "claude", Skill: "claude-only"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "cursor",
			invokedSkills: map[string]bool{},
			wantBlocked:   false,
		},
		{
			name: "matches path with doublestar glob",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Path: "**/engine/*.go", Skill: "engine-skill"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "internal/engine/types.go",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"engine-skill"},
		},
		{
			name: "matches wildcard path star",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Path: "*", Skill: "all-path-skill"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "anything.go",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"all-path-skill"},
		},
		{
			name: "matches empty path for all files",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Path: "", Skill: "no-path-skill"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "any/file.go",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"no-path-skill"},
		},
		{
			name: "skips path match when filePath is empty",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Path: "**/*.go", Skill: "go-skill"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   false,
		},
		{
			name: "doublestar path matches when filePath is empty",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Bash", Path: "**", Skill: "bash-skill"}, Source: "src"},
			},
			toolName:      "Bash",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"bash-skill"},
		},
		{
			name: "deduplicates matched rules by skill name",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Skill: "same-skill"}, Source: "src1"},
				{Rule: Rule{Tool: "Edit", Skill: "same-skill"}, Source: "src2"},
				{Rule: Rule{Tool: "Edit", Skill: "same-skill"}, Source: "src3"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"same-skill"},
		},
		{
			name: "returns all missing skills in BlockResult",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Skill: "skill-a"}, Source: "src"},
				{Rule: Rule{Tool: "Edit", Skill: "skill-b"}, Source: "src"},
				{Rule: Rule{Tool: "Edit", Skill: "skill-c"}, Source: "src"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{"skill-b": true},
			wantBlocked:   true,
			wantMissing:   []string{"skill-a", "skill-c"},
		},
		{
			name: "returns specific reason with skill names and sources",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Skill: "linear"}, Source: "/home/.angry-bear/skill_enforcement.json"},
				{Rule: Rule{Tool: "Edit", Skill: "go-standards"}, Source: "/project/.angry-bear/skill_enforcement.json"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"linear", "go-standards"},
			wantReasonHas: "angry-bear",
		},
		{
			name: "handles multiple rules requiring different skills for same file",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Path: "**/*.go", Skill: "go-skill"}, Source: "src1"},
				{Rule: Rule{Tool: "Edit", Path: "**/*.go", Skill: "lint-skill"}, Source: "src2"},
			},
			toolName:      "Edit",
			filePath:      "main.go",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"go-skill", "lint-skill"},
		},
		{
			name: "handles rules from multiple config sources",
			rules: []MatchedRule{
				{Rule: Rule{Tool: "Edit", Skill: "user-skill"}, Source: "/home/.angry-bear/skill_enforcement.json"},
				{Rule: Rule{Tool: "Edit", Skill: "project-skill"}, Source: "/project/.angry-bear/skill_enforcement.json"},
			},
			toolName:      "Edit",
			filePath:      "",
			agent:         "",
			invokedSkills: map[string]bool{},
			wantBlocked:   true,
			wantMissing:   []string{"user-skill", "project-skill"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ShouldBlock(tt.rules, tt.toolName, tt.filePath, tt.agent, tt.invokedSkills)

			if got.Blocked != tt.wantBlocked {
				t.Errorf("Blocked = %v, want %v", got.Blocked, tt.wantBlocked)
			}

			if tt.wantMissing != nil {
				if len(got.Missing) != len(tt.wantMissing) {
					t.Errorf("Missing = %v, want %v", got.Missing, tt.wantMissing)
				} else {
					gotSet := map[string]bool{}
					for _, s := range got.Missing {
						gotSet[s] = true
					}
					for _, want := range tt.wantMissing {
						if !gotSet[want] {
							t.Errorf("Missing does not contain %q, got %v", want, got.Missing)
						}
					}
				}
			}

			if tt.wantReasonHas != "" {
				if got.Reason == "" {
					t.Error("expected non-empty Reason")
				} else if !strings.Contains(got.Reason, tt.wantReasonHas) {
					t.Errorf("Reason %q does not contain %q", got.Reason, tt.wantReasonHas)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// mustMkdirAll creates a directory tree, failing the test if it cannot.
func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("failed to create directory %q: %v", path, err)
	}
}

// writeConfig writes a skill_enforcement.json config file into
// dir/.angry-bear/skill_enforcement.json.
func writeConfig(t *testing.T, dir string, cfg Config) {
	t.Helper()
	configDir := filepath.Join(dir, ".angry-bear")
	mustMkdirAll(t, configDir)
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "skill_enforcement.json"), data, 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NormalizeGlob edge cases
// ---------------------------------------------------------------------------

func TestNormalizeGlob_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{
			name:    "single filename gets doublestar prefix",
			pattern: "Makefile",
			want:    "**/Makefile",
		},
		{
			name:    "dot-prefixed file gets doublestar prefix",
			pattern: ".gitignore",
			want:    "**/.gitignore",
		},
		{
			name:    "complex extension pattern",
			pattern: "*.test.ts",
			want:    "**/*.test.ts",
		},
		{
			name:    "directory-only pattern preserved",
			pattern: "src/",
			want:    "src/",
		},
		{
			name:    "doublestar alone at start with extra path",
			pattern: "**/src/*.go",
			want:    "**/src/*.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeGlob(tt.pattern)
			if got != tt.want {
				t.Errorf("NormalizeGlob(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NormalizeFilePath edge cases
// ---------------------------------------------------------------------------

func TestNormalizeFilePath_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filePath    string
		projectRoot string
		want        string
	}{
		{
			name:        "empty project root with absolute path",
			filePath:    "/some/absolute/path.go",
			projectRoot: "",
			want:        "some/absolute/path.go",
		},
		{
			name:        "absolute path not under project root",
			filePath:    "/other/project/file.go",
			projectRoot: "/my/project",
			want:        "other/project/file.go",
		},
		{
			name:        "path with multiple dot-dot segments",
			filePath:    "a/b/c/../../d/file.go",
			projectRoot: "/project",
			want:        "a/d/file.go",
		},
		{
			name:        "project root equals file directory",
			filePath:    "/project/file.go",
			projectRoot: "/project",
			want:        "file.go",
		},
		{
			name:        "mixed separators in both path and root",
			filePath:    `internal\engine\glob.go`,
			projectRoot: `/project`,
			want:        "internal/engine/glob.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeFilePath(tt.filePath, tt.projectRoot)
			if got != tt.want {
				t.Errorf("NormalizeFilePath(%q, %q) = %q, want %q", tt.filePath, tt.projectRoot, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// loadConfigFile edge case tests
// ---------------------------------------------------------------------------

func TestLoadConfigFile_VersionZero(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".angry-bear")
	mustMkdirAll(t, dir)
	configPath := filepath.Join(dir, "skill_enforcement.json")

	// version 0 should fail (only version 1 is supported).
	content := `{"version": 0, "tools": []}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadConfigFile(configPath)
	if err == nil {
		t.Fatal("expected error for version 0, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported config version") {
		t.Errorf("error = %q, want to contain 'unsupported config version'", err.Error())
	}
}

func TestLoadConfigFile_EmptyToolsList(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".angry-bear")
	mustMkdirAll(t, dir)
	configPath := filepath.Join(dir, "skill_enforcement.json")

	content := `{"version": 1, "tools": []}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := loadConfigFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestLoadConfigFile_MultipleRules(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".angry-bear")
	mustMkdirAll(t, dir)
	configPath := filepath.Join(dir, "skill_enforcement.json")

	cfg := Config{
		Version: 1,
		Tools: []Rule{
			{Tool: "Edit", Path: "*.go", Skill: "go-standards"},
			{Tool: "Write", Path: "*.py", Skill: "python-standards"},
			{Tool: "*", Path: "stacks/**", Skill: "sst-architect"},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := loadConfigFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	// Verify path normalization happened.
	if rules[0].Rule.Path != "**/*.go" {
		t.Errorf("rules[0].Path = %q, want **/*.go", rules[0].Rule.Path)
	}
	if rules[1].Rule.Path != "**/*.py" {
		t.Errorf("rules[1].Path = %q, want **/*.py", rules[1].Rule.Path)
	}
	// stacks/** has a slash so it's preserved as-is (specific path).
	if rules[2].Rule.Path != "stacks/**" {
		t.Errorf("rules[2].Path = %q, want stacks/**", rules[2].Rule.Path)
	}

	// Verify source is set to machine (loaded via LoadConfig fallback).
	for i, r := range rules {
		if r.Source != SourceMachine {
			t.Errorf("rules[%d].Source = %q, want %q", i, r.Source, SourceMachine)
		}
	}
}

// ---------------------------------------------------------------------------
// ShouldBlock edge cases
// ---------------------------------------------------------------------------

func TestShouldBlock_NilInvokedSkills(t *testing.T) {
	t.Parallel()
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Skill: "some-skill"}, Source: "src"},
	}

	// nil invokedSkills should still trigger block.
	got := ShouldBlock(rules, "Edit", "", "", nil)
	if !got.Blocked {
		t.Error("expected Blocked=true with nil invokedSkills")
	}
	if len(got.Missing) != 1 || got.Missing[0] != "some-skill" {
		t.Errorf("Missing = %v, want [some-skill]", got.Missing)
	}
}

func TestShouldBlock_ReasonContainsLoadInstructions(t *testing.T) {
	t.Parallel()
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Skill: "my-skill"}, Source: "src"},
	}

	got := ShouldBlock(rules, "Edit", "", "", nil)
	if !got.Blocked {
		t.Fatal("expected Blocked=true")
	}
	// Reason should include the slash-command instruction.
	if !strings.Contains(got.Reason, "/my-skill") {
		t.Errorf("Reason %q does not contain /my-skill load instruction", got.Reason)
	}
	// Reason should include the SKILL.md path.
	if !strings.Contains(got.Reason, ".claude/skills/my-skill/SKILL.md") {
		t.Errorf("Reason %q does not contain SKILL.md path", got.Reason)
	}
}

func TestShouldBlock_GlobPatternError(t *testing.T) {
	t.Parallel()
	// A pattern with an unclosed bracket is invalid for doublestar.
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Path: "[unclosed", Skill: "bad-pattern-skill"}, Source: "src"},
	}

	got := ShouldBlock(rules, "Edit", "somefile.go", "", nil)
	// The rule should be skipped (not crash), so result should be not blocked.
	if got.Blocked {
		t.Error("expected Blocked=false when glob pattern is invalid (rule skipped)")
	}
}

// --- MatchedSkills tests ---

func TestMatchedSkills_FindsMatching(t *testing.T) {
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Path: "**/stacks/**", Skill: "sst-architect", Agent: "*"}, Source: "test"},
		{Rule: Rule{Tool: "Write", Path: "**/*.go", Skill: "go-coding", Agent: "claude"}, Source: "test"},
	}

	matched := MatchedSkills(rules, "Edit", "stacks/api.ts", "*")
	if len(matched) != 1 || matched[0] != "sst-architect" {
		t.Errorf("expected [sst-architect], got %v", matched)
	}
}

func TestMatchedSkills_NoMatch(t *testing.T) {
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Path: "**/stacks/**", Skill: "sst-architect", Agent: "*"}, Source: "test"},
	}

	matched := MatchedSkills(rules, "Edit", "handler/main.go", "*")
	if len(matched) != 0 {
		t.Errorf("expected no matches, got %v", matched)
	}
}

func TestMatchedSkills_MultipleSkills(t *testing.T) {
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Path: "**", Skill: "linear", Agent: "*"}, Source: "test"},
		{Rule: Rule{Tool: "Edit", Path: "**/stacks/**", Skill: "sst-architect", Agent: "*"}, Source: "test"},
	}

	matched := MatchedSkills(rules, "Edit", "stacks/api.ts", "*")
	if len(matched) != 2 {
		t.Errorf("expected 2 matches, got %v", matched)
	}
}

func TestMatchedSkills_AgentFilter(t *testing.T) {
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Path: "**", Skill: "eval", Agent: "cursor"}, Source: "test"},
	}

	// Claude should not match cursor-only rules
	matched := MatchedSkills(rules, "Edit", "test.go", "claude")
	if len(matched) != 0 {
		t.Errorf("expected no matches for claude, got %v", matched)
	}

	// Cursor should match
	matched = MatchedSkills(rules, "Edit", "test.go", "cursor")
	if len(matched) != 1 {
		t.Errorf("expected 1 match for cursor, got %v", matched)
	}
}

func TestMatchedSkills_Deduplicates(t *testing.T) {
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Path: "**", Skill: "linear", Agent: "*"}, Source: "a"},
		{Rule: Rule{Tool: "Write", Path: "**", Skill: "linear", Agent: "*"}, Source: "b"},
	}

	matched := MatchedSkills(rules, "Edit", "test.go", "*")
	if len(matched) != 1 {
		t.Errorf("expected 1 deduplicated match, got %v", matched)
	}
}

func TestMatchedSkills_EmptyFilePath(t *testing.T) {
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Path: "**/*.go", Skill: "go-skill", Agent: "*"}, Source: "test"},
	}

	// When filePath is empty but rule requires a specific path, it should not match.
	matched := MatchedSkills(rules, "Edit", "", "*")
	if len(matched) != 0 {
		t.Errorf("expected no matches with empty filePath, got %v", matched)
	}
}

func TestMatchedSkills_DoublestarMatchesEmptyFilePath(t *testing.T) {
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Bash", Path: "**", Skill: "bash-skill", Agent: "*"}, Source: "test"},
	}

	// ** means "all files" so it should match even when filePath is empty (e.g. Bash tool).
	matched := MatchedSkills(rules, "Bash", "", "*")
	if len(matched) != 1 || matched[0] != "bash-skill" {
		t.Errorf("expected [bash-skill] with ** path and empty filePath, got %v", matched)
	}
}

func TestMatchedSkills_GlobPatternError(t *testing.T) {
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Path: "[invalid", Skill: "bad-skill", Agent: "*"}, Source: "test"},
	}

	// Invalid glob pattern should cause the rule to be skipped (not crash).
	matched := MatchedSkills(rules, "Edit", "file.go", "*")
	if len(matched) != 0 {
		t.Errorf("expected no matches with invalid glob, got %v", matched)
	}
}

func TestMatchedSkills_EmptyRules(t *testing.T) {
	matched := MatchedSkills(nil, "Edit", "file.go", "claude")
	if len(matched) != 0 {
		t.Errorf("expected no matches with nil rules, got %v", matched)
	}

	matched = MatchedSkills([]MatchedRule{}, "Edit", "file.go", "claude")
	if len(matched) != 0 {
		t.Errorf("expected no matches with empty rules, got %v", matched)
	}
}

func TestMatchedSkills_WildcardPathAndEmptyPath(t *testing.T) {
	rules := []MatchedRule{
		{Rule: Rule{Tool: "Edit", Path: "*", Skill: "star-skill", Agent: "*"}, Source: "test"},
		{Rule: Rule{Tool: "Edit", Path: "", Skill: "empty-skill", Agent: "*"}, Source: "test"},
	}

	matched := MatchedSkills(rules, "Edit", "any-file.go", "*")
	if len(matched) != 2 {
		t.Errorf("expected 2 matches, got %v", matched)
	}
}

// --- LoadConfigFromDir tests ---

func TestLoadConfigFromDir_LoadsRules(t *testing.T) {
	dir := t.TempDir()
	config := `{"version": 1, "tools": [{"tool": "Edit", "path": "**/*.go", "skill": "go-coding", "agent": "*"}]}`
	if err := os.WriteFile(filepath.Join(dir, "skill_enforcement.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadConfigFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Rule.Skill != "go-coding" {
		t.Errorf("expected skill go-coding, got %s", rules[0].Rule.Skill)
	}
}

func TestLoadLegacyConfigFile_VersionZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "skill_enforcement.json")

	// Legacy config without version field (version defaults to 0).
	content := `{"tools": [{"tool": "Edit", "path": "**/*.go", "skill": "go-skill", "agent": "claude"}]}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := loadLegacyConfigFile(configPath, SourceRepo)
	if err != nil {
		t.Fatalf("expected no error for version 0 legacy config, got: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Rule.Skill != "go-skill" {
		t.Errorf("expected skill go-skill, got %s", rules[0].Rule.Skill)
	}
	if rules[0].Source != SourceRepo {
		t.Errorf("expected source %s, got %s", SourceRepo, rules[0].Source)
	}
}

func TestLoadLegacyConfigFile_VersionOne(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "skill_enforcement.json")

	content := `{"version": 1, "tools": [{"tool": "Write", "path": "**", "skill": "test-skill", "agent": "*"}]}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := loadLegacyConfigFile(configPath, SourceMachine)
	if err != nil {
		t.Fatalf("expected no error for version 1 legacy config, got: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
}

func TestLoadLegacyConfigFile_UnsupportedVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "skill_enforcement.json")

	content := `{"version": 99, "tools": []}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadLegacyConfigFile(configPath, SourceRepo)
	if err == nil {
		t.Fatal("expected error for version 99, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported config version") {
		t.Errorf("error = %q, want to contain 'unsupported config version'", err.Error())
	}
}

func TestLoadLegacyConfigFile_Missing(t *testing.T) {
	t.Parallel()
	rules, err := loadLegacyConfigFile("/nonexistent/path/config.json", SourceRepo)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if rules != nil {
		t.Errorf("expected nil rules for missing file, got %d", len(rules))
	}
}

func TestLoadMergedConfig_BluebearBackwardCompat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create .bluebear config WITHOUT version field (legacy format).
	bluebearDir := filepath.Join(dir, ".bluebear")
	mustMkdirAll(t, bluebearDir)
	content := `{"tools": [{"tool": "Edit", "path": "**/*.py", "skill": "python-standards", "agent": "claude"}]}`
	if err := os.WriteFile(filepath.Join(bluebearDir, "skill_enforcement.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadMergedConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadMergedConfig failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule from .bluebear, got %d", len(rules))
	}
	if rules[0].Rule.Skill != "python-standards" {
		t.Errorf("expected skill python-standards, got %s", rules[0].Rule.Skill)
	}
	if rules[0].Source != SourceRepo {
		t.Errorf("expected source %s, got %s", SourceRepo, rules[0].Source)
	}
}

func TestMigrateBluebearRules(t *testing.T) {
	t.Parallel()

	t.Run("migrates rules from .bluebear to .angry-bear", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		bluebearDir := filepath.Join(dir, ".bluebear")
		mustMkdirAll(t, bluebearDir)
		content := `{"tools": [{"tool": "Edit", "path": "**/*.go", "skill": "go-skill", "agent": "claude"}]}`
		if err := os.WriteFile(filepath.Join(bluebearDir, "skill_enforcement.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		migrated := MigrateBluebearRules(dir)
		if !migrated {
			t.Fatal("expected migration to occur")
		}

		angryPath := filepath.Join(dir, ".angry-bear", "skill_enforcement.json")
		data, err := os.ReadFile(angryPath)
		if err != nil {
			t.Fatalf("failed to read migrated config: %v", err)
		}
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("failed to parse migrated config: %v", err)
		}
		if cfg.Version != 1 {
			t.Errorf("expected version 1, got %d", cfg.Version)
		}
		if len(cfg.Tools) != 1 || cfg.Tools[0].Skill != "go-skill" {
			t.Errorf("unexpected rules: %+v", cfg.Tools)
		}
		if _, err := os.Stat(filepath.Join(bluebearDir, "skill_enforcement.json")); err != nil {
			t.Error("expected .bluebear config to still exist")
		}
	})

	t.Run("skips when .angry-bear already exists", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		mustMkdirAll(t, filepath.Join(dir, ".bluebear"))
		if err := os.WriteFile(filepath.Join(dir, ".bluebear", "skill_enforcement.json"), []byte(`{"tools":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		mustMkdirAll(t, filepath.Join(dir, ".angry-bear"))
		if err := os.WriteFile(filepath.Join(dir, ".angry-bear", "skill_enforcement.json"), []byte(`{"version":1,"tools":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}

		if MigrateBluebearRules(dir) {
			t.Error("expected no migration when .angry-bear already exists")
		}
	})

	t.Run("skips when .bluebear does not exist", func(t *testing.T) {
		t.Parallel()
		if MigrateBluebearRules(t.TempDir()) {
			t.Error("expected no migration without .bluebear")
		}
	})
}

func TestLoadMergedConfig_MigratesBluebearOnFirstLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	mustMkdirAll(t, filepath.Join(dir, ".bluebear"))
	content := `{"tools": [{"tool": "Write", "path": "**", "skill": "linear", "agent": "claude"},{"tool": "Edit", "path": "**/*.py", "skill": "python-standards", "agent": "*"}]}`
	if err := os.WriteFile(filepath.Join(dir, ".bluebear", "skill_enforcement.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadMergedConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadMergedConfig failed: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules after migration, got %d", len(rules))
	}
	if _, err := os.Stat(filepath.Join(dir, ".angry-bear", "skill_enforcement.json")); err != nil {
		t.Error("expected .angry-bear config to be created by migration")
	}
	for _, r := range rules {
		if r.Source != SourceRepo {
			t.Errorf("expected source %s, got %s", SourceRepo, r.Source)
		}
	}
}

func TestLoadConfigFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	rules, err := LoadConfigFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}
