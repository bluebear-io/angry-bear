// registry_test.go contains tests for the adapter registry and auto-detection logic.
package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryGet_Claude(t *testing.T) {
	reg := NewRegistry()

	adapter, err := reg.Get("claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "claude")
	}
}

func TestRegistryGet_Unknown(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Get("unknown")
	if err == nil {
		t.Fatal("expected error for unknown adapter, got nil")
	}
}

func TestAutoDetect_Claude(t *testing.T) {
	reg := NewRegistry()
	input := []byte(`{"hook_event_name":"PreToolUse","session_id":"x","tool_name":"Edit"}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "claude")
	}
}

func TestAutoDetect_CursorVersionDetected(t *testing.T) {
	reg := NewRegistry()
	// JSON with cursor_version should detect as cursor (even with hook_event_name)
	input := []byte(`{"hook_event_name":"PreToolUse","cursor_version":"0.50","tool_name":"Edit"}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "cursor")
	}
}

func TestAutoDetect_UnrecognizableInput(t *testing.T) {
	reg := NewRegistry()
	input := []byte(`{"some":"random","fields":"here"}`)

	_, err := reg.AutoDetect(input)
	if err == nil {
		t.Fatal("expected error for unrecognizable input, got nil")
	}
}

func TestAutoDetect_InvalidJSON(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.AutoDetect([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestRegistryGet_Cursor(t *testing.T) {
	reg := NewRegistry()

	adapter, err := reg.Get("cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "cursor")
	}
}

func TestAutoDetect_Cursor_BeforeFileEdit(t *testing.T) {
	reg := NewRegistry()
	input := []byte(`{"hook_event_name":"beforeFileEdit","conversation_id":"x","cursor_version":"0.48.1"}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "cursor")
	}
}

func TestAutoDetect_CursorPreferredOverClaude(t *testing.T) {
	reg := NewRegistry()
	// Both hook_event_name and cursor_version present -- cursor_version takes priority
	input := []byte(`{"hook_event_name":"preToolUse","conversation_id":"x","cursor_version":"0.48.1"}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q (cursor_version should take priority over hook_event_name)", adapter.Name(), "cursor")
	}
}

func TestRegistryNames(t *testing.T) {
	reg := NewRegistry()
	names := reg.Names()

	if len(names) < 2 {
		t.Fatalf("expected at least 2 adapters, got %d", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["claude"] {
		t.Error("missing 'claude' in Names()")
	}
	if !nameSet["cursor"] {
		t.Error("missing 'cursor' in Names()")
	}
}

func TestRegistryNames_AreSorted(t *testing.T) {
	reg := NewRegistry()
	names := reg.Names()

	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("Names() not sorted: %q comes after %q", names[i], names[i-1])
		}
	}
}

// --- SetRegistryDefaults tests ---

func TestSetRegistryDefaults_AppliedToNewRegistry(t *testing.T) {
	// Save original values and restore after test
	origHome := registryDefaultHomeDir
	origBin := registryDefaultBinaryPath
	t.Cleanup(func() {
		SetRegistryDefaults(origHome, origBin)
	})

	SetRegistryDefaults("/test/home", "/test/bin/angry-bear")

	reg := NewRegistry()

	// Verify claude adapter got the defaults
	claudeAdapter, err := reg.Get("claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ca, ok := claudeAdapter.(*ClaudeAdapter)
	if !ok {
		t.Fatal("claude adapter is not *ClaudeAdapter")
	}
	if ca.HomeDir != "/test/home" {
		t.Errorf("ClaudeAdapter.HomeDir = %q, want %q", ca.HomeDir, "/test/home")
	}
	if ca.BinaryPath != "/test/bin/angry-bear" {
		t.Errorf("ClaudeAdapter.BinaryPath = %q, want %q", ca.BinaryPath, "/test/bin/angry-bear")
	}

	// Verify cursor adapter got the defaults
	cursorAdapter, err := reg.Get("cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cua, ok := cursorAdapter.(*CursorAdapter)
	if !ok {
		t.Fatal("cursor adapter is not *CursorAdapter")
	}
	if cua.HomeDir != "/test/home" {
		t.Errorf("CursorAdapter.HomeDir = %q, want %q", cua.HomeDir, "/test/home")
	}
	if cua.BinaryPath != "/test/bin/angry-bear" {
		t.Errorf("CursorAdapter.BinaryPath = %q, want %q", cua.BinaryPath, "/test/bin/angry-bear")
	}
}

func TestRegistryBinaryPath(t *testing.T) {
	origHome := registryDefaultHomeDir
	origBin := registryDefaultBinaryPath
	t.Cleanup(func() {
		SetRegistryDefaults(origHome, origBin)
	})

	SetRegistryDefaults("", "/my/special/binary")
	if got := RegistryBinaryPath(); got != "/my/special/binary" {
		t.Errorf("RegistryBinaryPath() = %q, want %q", got, "/my/special/binary")
	}
}

func TestSetRegistryDefaults_ClearOverrides(t *testing.T) {
	origHome := registryDefaultHomeDir
	origBin := registryDefaultBinaryPath
	t.Cleanup(func() {
		SetRegistryDefaults(origHome, origBin)
	})

	SetRegistryDefaults("/some/home", "/some/bin")
	SetRegistryDefaults("", "") // Clear

	if registryDefaultHomeDir != "" {
		t.Errorf("registryDefaultHomeDir = %q, want empty after clear", registryDefaultHomeDir)
	}
	if registryDefaultBinaryPath != "" {
		t.Errorf("registryDefaultBinaryPath = %q, want empty after clear", registryDefaultBinaryPath)
	}
}

// --- AutoDetect additional tests ---

func TestAutoDetect_EmptyJSON(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.AutoDetect([]byte("{}"))
	if err == nil {
		t.Fatal("expected error for empty JSON object, got nil")
	}
}

func TestAutoDetect_CursorWithAllFields(t *testing.T) {
	reg := NewRegistry()
	// Full Cursor payload with both hook_event_name and cursor_version
	input := []byte(`{
		"hook_event_name": "beforeFileEdit",
		"conversation_id": "conv-123",
		"generation_id": "gen-456",
		"cursor_version": "0.48.1",
		"workspace_roots": ["/home/user/project"],
		"tool_name": "edit_file",
		"file_path": "main.go"
	}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "cursor")
	}
}

func TestAutoDetect_ClaudeWithAllFields(t *testing.T) {
	reg := NewRegistry()
	input := []byte(`{
		"hook_event_name": "PreToolUse",
		"session_id": "sess-abc",
		"tool_name": "Edit",
		"tool_input": {"file_path": "main.go"},
		"cwd": "/home/user/project"
	}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "claude")
	}
}

// --- ScanAllProjects tests ---

func TestScanAllProjects_EmptyProjectDirs(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := registryDefaultHomeDir
	origBin := registryDefaultBinaryPath
	t.Cleanup(func() {
		SetRegistryDefaults(origHome, origBin)
	})
	SetRegistryDefaults(tmpDir, "angry-bear")

	// Create empty projects dirs for both agents
	for _, agent := range []string{".claude", ".cursor"} {
		projDir := filepath.Join(tmpDir, agent, "projects")
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			t.Fatalf("failed to create %s/projects: %v", agent, err)
		}
	}

	reg := NewRegistry()
	projects, err := reg.ScanAllProjects()
	if err != nil {
		t.Fatalf("ScanAllProjects failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0", len(projects))
	}
}

func TestScanAllProjects_MergesAgentsForSameProject(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := registryDefaultHomeDir
	origBin := registryDefaultBinaryPath
	t.Cleanup(func() {
		SetRegistryDefaults(origHome, origBin)
	})
	SetRegistryDefaults(tmpDir, "angry-bear")

	// Create a real project directory
	projectPath := filepath.Join(tmpDir, "shared-project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create both Claude and Cursor project entries pointing to the same directory
	for _, agent := range []string{".claude", ".cursor"} {
		encodedDir := filepath.Join(tmpDir, agent, "projects", "encoded-shared")
		if err := os.MkdirAll(encodedDir, 0o755); err != nil {
			t.Fatalf("failed to create encoded dir for %s: %v", agent, err)
		}
		indexJSON := `{"entries":[{"projectPath":"` + projectPath + `"}]}`
		if err := os.WriteFile(filepath.Join(encodedDir, "sessions-index.json"), []byte(indexJSON), 0o644); err != nil {
			t.Fatalf("failed to write sessions-index.json for %s: %v", agent, err)
		}
	}

	reg := NewRegistry()
	projects, err := reg.ScanAllProjects()
	if err != nil {
		t.Fatalf("ScanAllProjects failed: %v", err)
	}

	// Should be 1 merged project with both agents
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1 (same path should be merged)", len(projects))
	}

	p := projects[0]
	if len(p.Agents) != 2 {
		t.Errorf("Agents = %v, want 2 agents", p.Agents)
	}
	if p.Path != projectPath {
		t.Errorf("Path = %q, want %q", p.Path, projectPath)
	}
}

func TestScanAllProjects_SortsResultsByName(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := registryDefaultHomeDir
	origBin := registryDefaultBinaryPath
	t.Cleanup(func() {
		SetRegistryDefaults(origHome, origBin)
	})
	SetRegistryDefaults(tmpDir, "angry-bear")

	// Create two project directories
	projectA := filepath.Join(tmpDir, "alpha-project")
	projectZ := filepath.Join(tmpDir, "zulu-project")
	for _, p := range []string{projectA, projectZ} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("failed to create project dir %s: %v", p, err)
		}
	}

	// Create encoded dirs under .claude/projects/
	claudeProjectsDir := filepath.Join(tmpDir, ".claude", "projects")
	for _, data := range []struct {
		encoded string
		path    string
	}{
		{"encoded-zulu", projectZ},
		{"encoded-alpha", projectA},
	} {
		encodedDir := filepath.Join(claudeProjectsDir, data.encoded)
		if err := os.MkdirAll(encodedDir, 0o755); err != nil {
			t.Fatalf("failed to create encoded dir: %v", err)
		}
		indexJSON := `{"entries":[{"projectPath":"` + data.path + `"}]}`
		if err := os.WriteFile(filepath.Join(encodedDir, "sessions-index.json"), []byte(indexJSON), 0o644); err != nil {
			t.Fatalf("failed to write sessions-index.json: %v", err)
		}
	}

	// Also create empty cursor projects dir so cursor adapter doesn't error
	if err := os.MkdirAll(filepath.Join(tmpDir, ".cursor", "projects"), 0o755); err != nil {
		t.Fatalf("failed to create cursor projects dir: %v", err)
	}

	reg := NewRegistry()
	projects, err := reg.ScanAllProjects()
	if err != nil {
		t.Fatalf("ScanAllProjects failed: %v", err)
	}

	if len(projects) < 2 {
		t.Fatalf("got %d projects, want at least 2", len(projects))
	}

	// Verify sorted order
	for i := 1; i < len(projects); i++ {
		if projects[i].Name < projects[i-1].Name {
			t.Errorf("projects not sorted: %q comes after %q", projects[i].Name, projects[i-1].Name)
		}
	}
}

func TestScanAllProjects_MissingProjectDirs(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := registryDefaultHomeDir
	origBin := registryDefaultBinaryPath
	t.Cleanup(func() {
		SetRegistryDefaults(origHome, origBin)
	})
	SetRegistryDefaults(tmpDir, "angry-bear")

	// Don't create any .claude or .cursor dirs -- should handle gracefully
	reg := NewRegistry()
	projects, err := reg.ScanAllProjects()
	if err != nil {
		t.Fatalf("ScanAllProjects failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0 for missing project dirs", len(projects))
	}
}

func TestScanAllProjects_AgentsSorted(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := registryDefaultHomeDir
	origBin := registryDefaultBinaryPath
	t.Cleanup(func() {
		SetRegistryDefaults(origHome, origBin)
	})
	SetRegistryDefaults(tmpDir, "angry-bear")

	// Create a project visible to both agents
	projectPath := filepath.Join(tmpDir, "multi-agent-project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	for _, agent := range []string{".cursor", ".claude"} {
		encodedDir := filepath.Join(tmpDir, agent, "projects", "encoded-multi")
		if err := os.MkdirAll(encodedDir, 0o755); err != nil {
			t.Fatalf("failed to create encoded dir for %s: %v", agent, err)
		}
		indexJSON := `{"entries":[{"projectPath":"` + projectPath + `"}]}`
		if err := os.WriteFile(filepath.Join(encodedDir, "sessions-index.json"), []byte(indexJSON), 0o644); err != nil {
			t.Fatalf("failed to write sessions-index.json for %s: %v", agent, err)
		}
	}

	reg := NewRegistry()
	projects, err := reg.ScanAllProjects()
	if err != nil {
		t.Fatalf("ScanAllProjects failed: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}

	agents := projects[0].Agents
	if len(agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(agents))
	}
	// Agents should be sorted alphabetically
	if agents[0] > agents[1] {
		t.Errorf("Agents not sorted: %v", agents)
	}
}
