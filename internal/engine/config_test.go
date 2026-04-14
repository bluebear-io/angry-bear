// config_test.go tests global config loading, merging, and repo preferences.
package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// globalConfigDefaults tests
// ---------------------------------------------------------------------------

func TestGlobalConfigDefaults(t *testing.T) {
	t.Parallel()

	defaults := globalConfigDefaults()

	t.Run("returns non-nil config", func(t *testing.T) {
		t.Parallel()
		if defaults == nil {
			t.Fatal("globalConfigDefaults() returned nil")
		}
	})

	t.Run("has default skill paths", func(t *testing.T) {
		t.Parallel()
		if len(defaults.SkillPaths) != 1 || defaults.SkillPaths[0] != ".claude/skills" {
			t.Errorf("SkillPaths = %v, want [.claude/skills]", defaults.SkillPaths)
		}
	})

	t.Run("has default state TTL", func(t *testing.T) {
		t.Parallel()
		if defaults.StateTTLHours != 24 {
			t.Errorf("StateTTLHours = %d, want 24", defaults.StateTTLHours)
		}
	})

	t.Run("has default agent wildcard", func(t *testing.T) {
		t.Parallel()
		if defaults.DefaultAgent != "*" {
			t.Errorf("DefaultAgent = %q, want \"*\"", defaults.DefaultAgent)
		}
	})

	t.Run("has default ignore patterns", func(t *testing.T) {
		t.Parallel()
		expected := map[string]bool{
			".git": true, "node_modules": true, "vendor": true,
			"dist": true, ".next": true, "__pycache__": true,
			".venv": true, "build": true, "target": true,
		}
		if len(defaults.IgnorePatterns) != len(expected) {
			t.Errorf("IgnorePatterns length = %d, want %d", len(defaults.IgnorePatterns), len(expected))
		}
		for _, p := range defaults.IgnorePatterns {
			if !expected[p] {
				t.Errorf("unexpected ignore pattern: %q", p)
			}
		}
	})

	t.Run("skill TTL minutes defaults to zero", func(t *testing.T) {
		t.Parallel()
		if defaults.SkillTTLMinutes != 0 {
			t.Errorf("SkillTTLMinutes = %d, want 0", defaults.SkillTTLMinutes)
		}
	})
}

// ---------------------------------------------------------------------------
// loadGlobalConfigFile tests
// ---------------------------------------------------------------------------

func TestLoadGlobalConfigFile(t *testing.T) {
	t.Parallel()

	t.Run("returns defaults when file does not exist", func(t *testing.T) {
		t.Parallel()
		defaults := &GlobalConfig{StateTTLHours: 42}
		got, err := loadGlobalConfigFile("/nonexistent/path/config.json", defaults)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != defaults {
			t.Errorf("expected defaults to be returned, got %+v", got)
		}
	})

	t.Run("returns nil when file does not exist and defaults is nil", func(t *testing.T) {
		t.Parallel()
		got, err := loadGlobalConfigFile("/nonexistent/path/config.json", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("parses valid config file", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		configPath := filepath.Join(tmp, "config.json")
		cfg := GlobalConfig{
			SkillPaths:      []string{"custom/skills"},
			StateTTLHours:   48,
			SkillTTLMinutes: 30,
			DefaultAgent:    "claude",
			IgnorePatterns:  []string{".git"},
		}
		writeGlobalConfig(t, configPath, cfg)

		got, err := loadGlobalConfigFile(configPath, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil config")
		}
		if got.StateTTLHours != 48 {
			t.Errorf("StateTTLHours = %d, want 48", got.StateTTLHours)
		}
		if got.DefaultAgent != "claude" {
			t.Errorf("DefaultAgent = %q, want \"claude\"", got.DefaultAgent)
		}
		if got.SkillTTLMinutes != 30 {
			t.Errorf("SkillTTLMinutes = %d, want 30", got.SkillTTLMinutes)
		}
	})

	t.Run("returns error on malformed JSON", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		configPath := filepath.Join(tmp, "config.json")
		if err := os.WriteFile(configPath, []byte("{bad json!!!"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := loadGlobalConfigFile(configPath, nil)
		if err == nil {
			t.Fatal("expected error on malformed JSON, got nil")
		}
	})

	t.Run("parses empty JSON object as zero-value config", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		configPath := filepath.Join(tmp, "config.json")
		if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := loadGlobalConfigFile(configPath, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil config for empty JSON object")
		}
		// All fields should be zero values.
		if got.StateTTLHours != 0 {
			t.Errorf("StateTTLHours = %d, want 0", got.StateTTLHours)
		}
		if got.DefaultAgent != "" {
			t.Errorf("DefaultAgent = %q, want empty", got.DefaultAgent)
		}
		if len(got.SkillPaths) != 0 {
			t.Errorf("SkillPaths = %v, want empty", got.SkillPaths)
		}
	})

	t.Run("ignores unknown JSON fields", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		configPath := filepath.Join(tmp, "config.json")
		content := `{"state_ttl_hours": 12, "unknown_field": "value", "another": 42}`
		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := loadGlobalConfigFile(configPath, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.StateTTLHours != 12 {
			t.Errorf("StateTTLHours = %d, want 12", got.StateTTLHours)
		}
	})
}

// ---------------------------------------------------------------------------
// LoadGlobalConfig tests (two-level merge: user + project)
// ---------------------------------------------------------------------------

func TestLoadGlobalConfig(t *testing.T) {
	t.Parallel()

	t.Run("returns defaults when no config files exist", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		projectRoot := filepath.Join(tmp, "project")
		mustMkdirAll(t, projectRoot)

		// Monkey-patch LoadGlobalDefaults by using LoadGlobalConfig with
		// a project that has no config — it should use built-in defaults.
		// We can't easily mock os.UserHomeDir, so we test LoadGlobalConfig
		// with a project root that has no .angry-bear/config.json.
		got, err := LoadGlobalConfig(projectRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil config")
		}
		// Should have built-in defaults (from globalConfigDefaults).
		if got.StateTTLHours != 24 {
			t.Errorf("StateTTLHours = %d, want 24 (default)", got.StateTTLHours)
		}
		if got.DefaultAgent != "*" {
			t.Errorf("DefaultAgent = %q, want \"*\" (default)", got.DefaultAgent)
		}
	})

	t.Run("project config overrides defaults for non-zero fields", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		projectRoot := filepath.Join(tmp, "project")

		// Write project-level config.json with overrides.
		projectCfg := GlobalConfig{
			StateTTLHours:  72,
			DefaultAgent:   "cursor",
			SkillPaths:     []string{"my/skills", "other/skills"},
			IgnorePatterns: []string{".git", "custom_dir"},
		}
		configDir := filepath.Join(projectRoot, ".angry-bear")
		mustMkdirAll(t, configDir)
		writeGlobalConfig(t, filepath.Join(configDir, "config.json"), projectCfg)

		got, err := LoadGlobalConfig(projectRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.StateTTLHours != 72 {
			t.Errorf("StateTTLHours = %d, want 72 (project override)", got.StateTTLHours)
		}
		if got.DefaultAgent != "cursor" {
			t.Errorf("DefaultAgent = %q, want \"cursor\" (project override)", got.DefaultAgent)
		}
		if len(got.SkillPaths) != 2 || got.SkillPaths[0] != "my/skills" {
			t.Errorf("SkillPaths = %v, want [my/skills other/skills]", got.SkillPaths)
		}
		if len(got.IgnorePatterns) != 2 {
			t.Errorf("IgnorePatterns = %v, want [.git custom_dir]", got.IgnorePatterns)
		}
	})

	t.Run("project zero-value fields preserve defaults", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		projectRoot := filepath.Join(tmp, "project")

		// Write project config with only DefaultAgent set (other fields are zero).
		configDir := filepath.Join(projectRoot, ".angry-bear")
		mustMkdirAll(t, configDir)
		content := `{"default_agent": "codex"}`
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := LoadGlobalConfig(projectRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// DefaultAgent should be overridden.
		if got.DefaultAgent != "codex" {
			t.Errorf("DefaultAgent = %q, want \"codex\"", got.DefaultAgent)
		}
		// StateTTLHours should remain at default since project has 0.
		if got.StateTTLHours != 24 {
			t.Errorf("StateTTLHours = %d, want 24 (preserved default)", got.StateTTLHours)
		}
		// SkillPaths should remain at default since project has empty slice.
		defaults := globalConfigDefaults()
		if len(got.SkillPaths) != len(defaults.SkillPaths) {
			t.Errorf("SkillPaths = %v, want %v (preserved default)", got.SkillPaths, defaults.SkillPaths)
		}
		// IgnorePatterns should remain at default.
		if len(got.IgnorePatterns) != len(defaults.IgnorePatterns) {
			t.Errorf("IgnorePatterns length = %d, want %d (preserved default)", len(got.IgnorePatterns), len(defaults.IgnorePatterns))
		}
	})

	t.Run("project SkillTTLMinutes positive overrides default zero", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		projectRoot := filepath.Join(tmp, "project")

		configDir := filepath.Join(projectRoot, ".angry-bear")
		mustMkdirAll(t, configDir)
		content := `{"skill_ttl_minutes": 15}`
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := LoadGlobalConfig(projectRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.SkillTTLMinutes != 15 {
			t.Errorf("SkillTTLMinutes = %d, want 15", got.SkillTTLMinutes)
		}
	})

	t.Run("project SkillTTLMinutes zero does not override default", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		projectRoot := filepath.Join(tmp, "project")

		configDir := filepath.Join(projectRoot, ".angry-bear")
		mustMkdirAll(t, configDir)
		// Explicitly set skill_ttl_minutes to 0 — should not change the default (also 0).
		content := `{"skill_ttl_minutes": 0, "state_ttl_hours": 10}`
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := LoadGlobalConfig(projectRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.SkillTTLMinutes != 0 {
			t.Errorf("SkillTTLMinutes = %d, want 0 (default preserved)", got.SkillTTLMinutes)
		}
		// StateTTLHours should be overridden.
		if got.StateTTLHours != 10 {
			t.Errorf("StateTTLHours = %d, want 10", got.StateTTLHours)
		}
	})

	t.Run("returns error on malformed project config JSON", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		projectRoot := filepath.Join(tmp, "project")

		configDir := filepath.Join(projectRoot, ".angry-bear")
		mustMkdirAll(t, configDir)
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("not json"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadGlobalConfig(projectRoot)
		if err == nil {
			t.Fatal("expected error on malformed project config, got nil")
		}
	})

	t.Run("all project fields override when set", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		projectRoot := filepath.Join(tmp, "project")

		projectCfg := GlobalConfig{
			SkillPaths:      []string{"a", "b"},
			StateTTLHours:   100,
			SkillTTLMinutes: 60,
			DefaultAgent:    "test-agent",
			IgnorePatterns:  []string{"only-this"},
		}
		configDir := filepath.Join(projectRoot, ".angry-bear")
		mustMkdirAll(t, configDir)
		writeGlobalConfig(t, filepath.Join(configDir, "config.json"), projectCfg)

		got, err := LoadGlobalConfig(projectRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.StateTTLHours != 100 {
			t.Errorf("StateTTLHours = %d, want 100", got.StateTTLHours)
		}
		if got.SkillTTLMinutes != 60 {
			t.Errorf("SkillTTLMinutes = %d, want 60", got.SkillTTLMinutes)
		}
		if got.DefaultAgent != "test-agent" {
			t.Errorf("DefaultAgent = %q, want \"test-agent\"", got.DefaultAgent)
		}
		if len(got.SkillPaths) != 2 || got.SkillPaths[0] != "a" {
			t.Errorf("SkillPaths = %v, want [a b]", got.SkillPaths)
		}
		if len(got.IgnorePatterns) != 1 || got.IgnorePatterns[0] != "only-this" {
			t.Errorf("IgnorePatterns = %v, want [only-this]", got.IgnorePatterns)
		}
	})
}

// ---------------------------------------------------------------------------
// RepoPreferences tests
// ---------------------------------------------------------------------------

func TestLoadRepoPreferences(t *testing.T) {
	t.Parallel()

	t.Run("returns empty prefs when file does not exist", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		repoDir := filepath.Join(tmp, "repo-config")
		mustMkdirAll(t, repoDir)

		prefs, err := LoadRepoPreferences(repoDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if prefs == nil {
			t.Fatal("expected non-nil prefs")
		}
		if prefs.PreferredPath != "" {
			t.Errorf("PreferredPath = %q, want empty", prefs.PreferredPath)
		}
	})

	t.Run("returns empty prefs when directory does not exist", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		nonexistent := filepath.Join(tmp, "does-not-exist")

		prefs, err := LoadRepoPreferences(nonexistent)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if prefs == nil {
			t.Fatal("expected non-nil prefs")
		}
		if prefs.PreferredPath != "" {
			t.Errorf("PreferredPath = %q, want empty", prefs.PreferredPath)
		}
	})

	t.Run("reads valid preferences file", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		repoDir := filepath.Join(tmp, "repo-config")
		mustMkdirAll(t, repoDir)

		prefs := &RepoPreferences{PreferredPath: "/home/user/projects/myrepo"}
		data, err := json.Marshal(prefs)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repoDir, "preferences.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := LoadRepoPreferences(repoDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.PreferredPath != "/home/user/projects/myrepo" {
			t.Errorf("PreferredPath = %q, want /home/user/projects/myrepo", got.PreferredPath)
		}
	})

	t.Run("returns error on malformed JSON", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		repoDir := filepath.Join(tmp, "repo-config")
		mustMkdirAll(t, repoDir)
		if err := os.WriteFile(filepath.Join(repoDir, "preferences.json"), []byte("{bad}"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadRepoPreferences(repoDir)
		if err == nil {
			t.Fatal("expected error on malformed JSON, got nil")
		}
	})
}

func TestSaveRepoPreferences(t *testing.T) {
	t.Parallel()

	t.Run("creates directory and writes file", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		repoDir := filepath.Join(tmp, "new", "nested", "repo-config")

		prefs := &RepoPreferences{PreferredPath: "/opt/projects/repo"}
		err := SaveRepoPreferences(repoDir, prefs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the file was written.
		data, err := os.ReadFile(filepath.Join(repoDir, "preferences.json"))
		if err != nil {
			t.Fatalf("failed to read written file: %v", err)
		}

		var got RepoPreferences
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("failed to parse written JSON: %v", err)
		}
		if got.PreferredPath != "/opt/projects/repo" {
			t.Errorf("PreferredPath = %q, want /opt/projects/repo", got.PreferredPath)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		repoDir := filepath.Join(tmp, "repo-config")
		mustMkdirAll(t, repoDir)

		// Write initial prefs.
		initial := &RepoPreferences{PreferredPath: "/old/path"}
		if err := SaveRepoPreferences(repoDir, initial); err != nil {
			t.Fatalf("first save failed: %v", err)
		}

		// Overwrite with new prefs.
		updated := &RepoPreferences{PreferredPath: "/new/path"}
		if err := SaveRepoPreferences(repoDir, updated); err != nil {
			t.Fatalf("second save failed: %v", err)
		}

		got, err := LoadRepoPreferences(repoDir)
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}
		if got.PreferredPath != "/new/path" {
			t.Errorf("PreferredPath = %q, want /new/path", got.PreferredPath)
		}
	})
}

func TestRepoPreferences_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		prefs RepoPreferences
	}{
		{
			name:  "typical path",
			prefs: RepoPreferences{PreferredPath: "/Users/dev/work/myproject"},
		},
		{
			name:  "empty path",
			prefs: RepoPreferences{PreferredPath: ""},
		},
		{
			name:  "path with spaces",
			prefs: RepoPreferences{PreferredPath: "/Users/my user/my project"},
		},
		{
			name:  "path with special characters",
			prefs: RepoPreferences{PreferredPath: "/opt/projects/my-repo_v2.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			repoDir := filepath.Join(tmp, "repo-config")

			if err := SaveRepoPreferences(repoDir, &tt.prefs); err != nil {
				t.Fatalf("save failed: %v", err)
			}

			got, err := LoadRepoPreferences(repoDir)
			if err != nil {
				t.Fatalf("load failed: %v", err)
			}

			if got.PreferredPath != tt.prefs.PreferredPath {
				t.Errorf("round-trip: PreferredPath = %q, want %q", got.PreferredPath, tt.prefs.PreferredPath)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// writeGlobalConfig writes a GlobalConfig to the given file path.
func writeGlobalConfig(t *testing.T, path string, cfg GlobalConfig) {
	t.Helper()
	dir := filepath.Dir(path)
	mustMkdirAll(t, dir)
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal global config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write global config to %s: %v", path, err)
	}
}

func TestLoadGlobalConfigFromDir_ReadsSkillTTL(t *testing.T) {
	dir := t.TempDir()
	cfg := GlobalConfig{
		SkillTTLMinutes: 5,
		StateTTLHours:   48,
		DefaultAgent:    "claude",
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)

	result, err := LoadGlobalConfigFromDir(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfigFromDir failed: %v", err)
	}
	if result.SkillTTLMinutes != 5 {
		t.Errorf("SkillTTLMinutes = %d, want 5", result.SkillTTLMinutes)
	}
	if result.StateTTLHours != 48 {
		t.Errorf("StateTTLHours = %d, want 48", result.StateTTLHours)
	}
	if result.DefaultAgent != "claude" {
		t.Errorf("DefaultAgent = %q, want claude", result.DefaultAgent)
	}
}

func TestLoadGlobalConfigFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := LoadGlobalConfigFromDir(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfigFromDir failed: %v", err)
	}
	// Should return defaults
	if result.SkillTTLMinutes != 0 {
		t.Errorf("SkillTTLMinutes = %d, want 0 (default)", result.SkillTTLMinutes)
	}
}

func TestLoadGlobalConfigFromDir_EmptyString(t *testing.T) {
	result, err := LoadGlobalConfigFromDir("")
	if err != nil {
		t.Fatalf("LoadGlobalConfigFromDir failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected defaults, got nil")
	}
}

func TestLoadGlobalConfigFromDir_DoesNotAppendAngryBear(t *testing.T) {
	// Regression: LoadGlobalConfig appended .angry-bear/config.json
	// causing double nesting. LoadGlobalConfigFromDir must NOT do this.
	dir := t.TempDir()

	// Write config at dir/config.json (correct)
	cfg := GlobalConfig{SkillTTLMinutes: 10}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)

	// Also write a WRONG file at dir/.angry-bear/config.json
	wrongDir := filepath.Join(dir, ".angry-bear")
	_ = os.MkdirAll(wrongDir, 0o755)
	wrongCfg := GlobalConfig{SkillTTLMinutes: 99}
	wrongData, _ := json.MarshalIndent(wrongCfg, "", "  ")
	_ = os.WriteFile(filepath.Join(wrongDir, "config.json"), wrongData, 0o644)

	result, err := LoadGlobalConfigFromDir(dir)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	// Should read 10 from dir/config.json, NOT 99 from dir/.angry-bear/config.json
	if result.SkillTTLMinutes != 10 {
		t.Errorf("SkillTTLMinutes = %d, want 10 (not 99 from wrong path)", result.SkillTTLMinutes)
	}
}
