// loaded_test.go tests CollectLoadedSkills which reads state directory files
// and builds a map of skill names to their load status (which agents loaded them).
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestCollectLoadedSkills_EmptyDir returns empty map for empty directory.
func TestCollectLoadedSkills_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	loaded := CollectLoadedSkills(dir)
	if len(loaded) != 0 {
		t.Errorf("expected empty map for empty dir, got %d entries", len(loaded))
	}
}

// TestCollectLoadedSkills_NonExistentDir returns empty map without error.
func TestCollectLoadedSkills_NonExistentDir(t *testing.T) {
	loaded := CollectLoadedSkills("/nonexistent/path/that/does/not/exist")
	if len(loaded) != 0 {
		t.Errorf("expected empty map for nonexistent dir, got %d entries", len(loaded))
	}
}

// TestCollectLoadedSkills_SingleSession reads a single session file
// and returns the correct skill-to-agent mapping.
func TestCollectLoadedSkills_SingleSession(t *testing.T) {
	dir := t.TempDir()

	ss := SessionState{
		SessionID:     "session-001",
		Agent:         "claude",
		InvokedSkills: []string{"git", "linear"},
	}
	data, _ := json.Marshal(ss)
	_ = os.WriteFile(filepath.Join(dir, "session-001.json"), data, 0o600)

	loaded := CollectLoadedSkills(dir)

	if len(loaded) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(loaded))
	}

	gitStatus := loaded["git"]
	if gitStatus == nil {
		t.Fatal("expected git skill in loaded map")
	}
	if len(gitStatus.Agents) != 1 || gitStatus.Agents[0] != "claude" {
		t.Errorf("git agents = %v, want [claude]", gitStatus.Agents)
	}

	linearStatus := loaded["linear"]
	if linearStatus == nil {
		t.Fatal("expected linear skill in loaded map")
	}
	if len(linearStatus.Agents) != 1 || linearStatus.Agents[0] != "claude" {
		t.Errorf("linear agents = %v, want [claude]", linearStatus.Agents)
	}
}

// TestCollectLoadedSkills_MultipleSessionsSameSkill merges agents from
// different sessions that loaded the same skill.
func TestCollectLoadedSkills_MultipleSessionsSameSkill(t *testing.T) {
	dir := t.TempDir()

	ss1 := SessionState{
		SessionID:     "session-001",
		Agent:         "claude",
		InvokedSkills: []string{"git"},
	}
	data1, _ := json.Marshal(ss1)
	_ = os.WriteFile(filepath.Join(dir, "session-001.json"), data1, 0o600)

	ss2 := SessionState{
		SessionID:     "session-002",
		Agent:         "cursor",
		InvokedSkills: []string{"git"},
	}
	data2, _ := json.Marshal(ss2)
	_ = os.WriteFile(filepath.Join(dir, "session-002.json"), data2, 0o600)

	loaded := CollectLoadedSkills(dir)

	gitStatus := loaded["git"]
	if gitStatus == nil {
		t.Fatal("expected git skill in loaded map")
	}
	agents := gitStatus.Agents
	sort.Strings(agents)
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents for git, got %d: %v", len(agents), agents)
	}
	if agents[0] != "claude" || agents[1] != "cursor" {
		t.Errorf("agents = %v, want [claude, cursor]", agents)
	}
}

// TestCollectLoadedSkills_DuplicateAgentNotRepeated verifies that the same
// agent is not listed twice for the same skill across multiple sessions.
func TestCollectLoadedSkills_DuplicateAgentNotRepeated(t *testing.T) {
	dir := t.TempDir()

	for _, id := range []string{"session-001", "session-002"} {
		ss := SessionState{
			SessionID:     id,
			Agent:         "claude",
			InvokedSkills: []string{"git"},
		}
		data, _ := json.Marshal(ss)
		_ = os.WriteFile(filepath.Join(dir, id+".json"), data, 0o600)
	}

	loaded := CollectLoadedSkills(dir)

	gitStatus := loaded["git"]
	if gitStatus == nil {
		t.Fatal("expected git skill in loaded map")
	}
	if len(gitStatus.Agents) != 1 {
		t.Errorf("expected 1 agent (deduped), got %d: %v", len(gitStatus.Agents), gitStatus.Agents)
	}
}

// TestCollectLoadedSkills_EmptyAgent uses "unknown" as the agent name
// when the session file has no agent field.
func TestCollectLoadedSkills_EmptyAgent(t *testing.T) {
	dir := t.TempDir()

	ss := SessionState{
		SessionID:     "session-old",
		Agent:         "",
		InvokedSkills: []string{"linear"},
	}
	data, _ := json.Marshal(ss)
	_ = os.WriteFile(filepath.Join(dir, "session-old.json"), data, 0o600)

	loaded := CollectLoadedSkills(dir)

	linearStatus := loaded["linear"]
	if linearStatus == nil {
		t.Fatal("expected linear skill in loaded map")
	}
	if len(linearStatus.Agents) != 1 || linearStatus.Agents[0] != "unknown" {
		t.Errorf("agents = %v, want [unknown] for empty agent", linearStatus.Agents)
	}
}

// TestCollectLoadedSkills_SkipsNonJSON verifies that non-JSON files and
// directories are ignored.
func TestCollectLoadedSkills_SkipsNonJSON(t *testing.T) {
	dir := t.TempDir()

	// Create a .lock file (should be skipped)
	_ = os.WriteFile(filepath.Join(dir, "session-001.lock"), []byte("lock"), 0o600)

	// Create a subdirectory (should be skipped)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	// Create a text file (should be skipped)
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0o600)

	loaded := CollectLoadedSkills(dir)
	if len(loaded) != 0 {
		t.Errorf("expected empty map when no JSON state files, got %d entries", len(loaded))
	}
}

// TestCollectLoadedSkills_SkipsCorruptJSON ignores files with invalid JSON
// without crashing.
func TestCollectLoadedSkills_SkipsCorruptJSON(t *testing.T) {
	dir := t.TempDir()

	// Corrupt JSON
	_ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not valid json"), 0o600)

	// Valid session
	ss := SessionState{
		SessionID:     "good",
		Agent:         "claude",
		InvokedSkills: []string{"git"},
	}
	data, _ := json.Marshal(ss)
	_ = os.WriteFile(filepath.Join(dir, "good.json"), data, 0o600)

	loaded := CollectLoadedSkills(dir)
	if len(loaded) != 1 {
		t.Errorf("expected 1 skill (from valid file), got %d", len(loaded))
	}
	if loaded["git"] == nil {
		t.Error("expected git skill from valid session file")
	}
}

// TestCollectLoadedSkills_NoInvokedSkills handles sessions with empty skill lists.
func TestCollectLoadedSkills_NoInvokedSkills(t *testing.T) {
	dir := t.TempDir()

	ss := SessionState{
		SessionID:     "session-empty",
		Agent:         "claude",
		InvokedSkills: []string{},
	}
	data, _ := json.Marshal(ss)
	_ = os.WriteFile(filepath.Join(dir, "session-empty.json"), data, 0o600)

	loaded := CollectLoadedSkills(dir)
	if len(loaded) != 0 {
		t.Errorf("expected empty map for session with no skills, got %d entries", len(loaded))
	}
}
