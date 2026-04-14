// manager_test.go tests the StateManager for reading and writing session state files.
// It covers RecordSkill, HasSkill, GetInvokedSkills, Clean, and concurrency safety.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestRecordSkill_CreatesNewStateFile verifies that RecordSkill creates a new JSON state file
// when no state file exists for the given session.
func TestRecordSkill_CreatesNewStateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("session-001", "my-skill"); err != nil {
		t.Fatalf("RecordSkill() returned error: %v", err)
	}

	// Verify the file exists.
	statePath := filepath.Join(dir, "session-001.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("failed to unmarshal state file: %v", err)
	}

	if state.SessionID != "session-001" {
		t.Errorf("SessionID = %q, want %q", state.SessionID, "session-001")
	}
	if len(state.InvokedSkills) != 1 || state.InvokedSkills[0] != "my-skill" {
		t.Errorf("InvokedSkills = %v, want [my-skill]", state.InvokedSkills)
	}
}

// TestRecordSkill_AppendsSkill verifies that RecordSkill appends a new skill to an
// existing state file.
func TestRecordSkill_AppendsSkill(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("session-001", "skill-a"); err != nil {
		t.Fatalf("RecordSkill(skill-a) returned error: %v", err)
	}
	if err := mgr.RecordSkill("session-001", "skill-b"); err != nil {
		t.Fatalf("RecordSkill(skill-b) returned error: %v", err)
	}

	skills, err := mgr.GetInvokedSkills("session-001")
	if err != nil {
		t.Fatalf("GetInvokedSkills() returned error: %v", err)
	}

	if !skills["skill-a"] || !skills["skill-b"] {
		t.Errorf("skills = %v, want both skill-a and skill-b", skills)
	}
}

// TestRecordSkill_Idempotent verifies that recording the same skill twice does not
// create a duplicate entry.
func TestRecordSkill_Idempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("session-001", "my-skill"); err != nil {
		t.Fatalf("first RecordSkill() returned error: %v", err)
	}
	if err := mgr.RecordSkill("session-001", "my-skill"); err != nil {
		t.Fatalf("second RecordSkill() returned error: %v", err)
	}

	// Read the file and check the skill appears exactly once.
	data, err := os.ReadFile(filepath.Join(dir, "session-001.json"))
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(state.InvokedSkills) != 1 {
		t.Errorf("InvokedSkills has %d entries, want 1 (idempotent)", len(state.InvokedSkills))
	}
}

// TestRecordSkill_SetsCreatedAt verifies that a new state file gets a created_at timestamp.
func TestRecordSkill_SetsCreatedAt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	before := time.Now().UTC()
	if err := mgr.RecordSkill("session-001", "my-skill"); err != nil {
		t.Fatalf("RecordSkill() returned error: %v", err)
	}
	after := time.Now().UTC()

	data, err := os.ReadFile(filepath.Join(dir, "session-001.json"))
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	createdAt, err := time.Parse(time.RFC3339, state.CreatedAt)
	if err != nil {
		t.Fatalf("failed to parse created_at %q: %v", state.CreatedAt, err)
	}

	if createdAt.Before(before.Truncate(time.Second)) || createdAt.After(after.Add(time.Second)) {
		t.Errorf("created_at = %v, want between %v and %v", createdAt, before, after)
	}
}

// TestRecordSkill_PreservesCreatedAt verifies that appending a skill to an existing state
// file does not overwrite the original created_at timestamp.
func TestRecordSkill_PreservesCreatedAt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("session-001", "skill-a"); err != nil {
		t.Fatalf("RecordSkill(skill-a) returned error: %v", err)
	}

	// Read original created_at.
	data1, err := os.ReadFile(filepath.Join(dir, "session-001.json"))
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	var state1 SessionState
	if err := json.Unmarshal(data1, &state1); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	originalCreatedAt := state1.CreatedAt

	// Small delay to ensure time differs.
	time.Sleep(10 * time.Millisecond)

	if err := mgr.RecordSkill("session-001", "skill-b"); err != nil {
		t.Fatalf("RecordSkill(skill-b) returned error: %v", err)
	}

	data2, err := os.ReadFile(filepath.Join(dir, "session-001.json"))
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	var state2 SessionState
	if err := json.Unmarshal(data2, &state2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if state2.CreatedAt != originalCreatedAt {
		t.Errorf("created_at changed: %q -> %q, want preserved", originalCreatedAt, state2.CreatedAt)
	}
}

// TestRecordSkill_FilePermissions verifies that the state file is written with 0600 permissions.
func TestRecordSkill_FilePermissions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("session-001", "my-skill"); err != nil {
		t.Fatalf("RecordSkill() returned error: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "session-001.json"))
	if err != nil {
		t.Fatalf("failed to stat state file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

// TestHasSkill_True verifies that HasSkill returns true for a recorded skill.
func TestHasSkill_True(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("session-001", "my-skill"); err != nil {
		t.Fatalf("RecordSkill() returned error: %v", err)
	}

	if !mgr.HasSkill("session-001", "my-skill") {
		t.Error("HasSkill() = false, want true")
	}
}

// TestHasSkill_False verifies that HasSkill returns false for an unrecorded skill.
func TestHasSkill_False(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("session-001", "my-skill"); err != nil {
		t.Fatalf("RecordSkill() returned error: %v", err)
	}

	if mgr.HasSkill("session-001", "other-skill") {
		t.Error("HasSkill(other-skill) = true, want false")
	}
}

// TestHasSkill_NoStateFile verifies that HasSkill returns false when no state file exists.
func TestHasSkill_NoStateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if mgr.HasSkill("nonexistent", "my-skill") {
		t.Error("HasSkill() = true for nonexistent session, want false")
	}
}

// TestGetInvokedSkills_ReturnsAll verifies that GetInvokedSkills returns all recorded skills.
func TestGetInvokedSkills_ReturnsAll(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	skills := []string{"skill-a", "skill-b", "skill-c"}
	for _, s := range skills {
		if err := mgr.RecordSkill("session-001", s); err != nil {
			t.Fatalf("RecordSkill(%s) returned error: %v", s, err)
		}
	}

	result, err := mgr.GetInvokedSkills("session-001")
	if err != nil {
		t.Fatalf("GetInvokedSkills() returned error: %v", err)
	}

	for _, s := range skills {
		if !result[s] {
			t.Errorf("skill %q not found in result %v", s, result)
		}
	}

	if len(result) != len(skills) {
		t.Errorf("result has %d entries, want %d", len(result), len(skills))
	}
}

// TestGetInvokedSkills_MissingSession verifies that GetInvokedSkills returns an empty map
// for a session with no state file.
func TestGetInvokedSkills_MissingSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	result, err := mgr.GetInvokedSkills("nonexistent")
	if err != nil {
		t.Fatalf("GetInvokedSkills() returned error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("result = %v, want empty map", result)
	}
}

// TestClean_RemovesFiles verifies that Clean removes both the state and lock files.
func TestClean_RemovesFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("session-001", "my-skill"); err != nil {
		t.Fatalf("RecordSkill() returned error: %v", err)
	}

	// Create a lock file manually to simulate one that was left behind.
	lockPath := filepath.Join(dir, "session-001.lock")
	if err := os.WriteFile(lockPath, []byte{}, 0600); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	if err := mgr.Clean("session-001"); err != nil {
		t.Fatalf("Clean() returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "session-001.json")); !os.IsNotExist(err) {
		t.Error("state file still exists after Clean()")
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file still exists after Clean()")
	}
}

// TestClean_Idempotent verifies that Clean does not error when cleaning a missing session.
func TestClean_Idempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.Clean("nonexistent"); err != nil {
		t.Fatalf("Clean() returned error for nonexistent session: %v", err)
	}
}

// TestRecordSkill_InvalidSessionID verifies that RecordSkill rejects invalid session IDs.
func TestRecordSkill_InvalidSessionID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("../evil", "my-skill"); err == nil {
		t.Error("RecordSkill(../evil) returned nil, want error")
	}
}

// TestConcurrent_RecordSkill verifies that concurrent RecordSkill calls from multiple
// goroutines do not corrupt the state file. All skills should be recorded exactly once.
// Because TryLock is non-blocking, goroutines retry with a short backoff on lock contention.
func TestConcurrent_RecordSkill(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	const numGoroutines = 20
	var wg sync.WaitGroup
	errs := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			skillName := "skill-" + string(rune('A'+idx))
			// Retry with backoff since TryLock is non-blocking.
			var lastErr error
			for attempt := 0; attempt < 100; attempt++ {
				lastErr = mgr.RecordSkill("concurrent-session", skillName)
				if lastErr == nil {
					return
				}
				time.Sleep(time.Duration(attempt+1) * time.Millisecond)
			}
			errs <- lastErr
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("RecordSkill returned error after retries: %v", err)
	}

	// Verify all skills were recorded exactly once.
	skills, err := mgr.GetInvokedSkills("concurrent-session")
	if err != nil {
		t.Fatalf("GetInvokedSkills() returned error: %v", err)
	}

	if len(skills) != numGoroutines {
		t.Errorf("recorded %d skills, want %d", len(skills), numGoroutines)
	}

	for i := 0; i < numGoroutines; i++ {
		skillName := "skill-" + string(rune('A'+i))
		if !skills[skillName] {
			t.Errorf("skill %q was not recorded", skillName)
		}
	}
}

// TestConcurrent_RecordSkill_And_HasSkill verifies that concurrent RecordSkill and
// HasSkill calls do not deadlock. All operations should complete within a timeout.
func TestConcurrent_RecordSkill_And_HasSkill(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	done := make(chan struct{})

	go func() {
		var wg sync.WaitGroup
		const numGoroutines = 10

		// Half the goroutines do RecordSkill (with retry), half do HasSkill.
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				skillName := "skill-" + string(rune('A'+idx))
				for attempt := 0; attempt < 50; attempt++ {
					if err := mgr.RecordSkill("mixed-session", skillName); err == nil {
						return
					}
					time.Sleep(time.Duration(attempt+1) * time.Millisecond)
				}
			}(i)

			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				skillName := "skill-" + string(rune('A'+idx))
				_ = mgr.HasSkill("mixed-session", skillName)
			}(i)
		}

		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success -- no deadlock.
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent RecordSkill + HasSkill timed out (possible deadlock)")
	}
}

// --- RecordSkillWithAgent tests ---

func TestRecordSkillWithAgent_SetsAgent(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkillWithAgent("sess1", "git", "cursor"); err != nil {
		t.Fatalf("RecordSkillWithAgent failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sess1.json"))
	if err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if state.Agent != "cursor" {
		t.Errorf("agent = %q, want %q", state.Agent, "cursor")
	}
}

// --- Skill timestamp tests ---

func TestRecordSkill_StoresTimestamp(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(dir)

	before := time.Now().UTC()
	err := mgr.RecordSkillWithAgent("sess1", "git", "claude")
	if err != nil {
		t.Fatalf("RecordSkillWithAgent failed: %v", err)
	}
	after := time.Now().UTC()

	data, err := os.ReadFile(filepath.Join(dir, "sess1.json"))
	if err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	var state SessionState
	err = json.Unmarshal(data, &state)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	ts, ok := state.SkillTimestamps["git"]
	if !ok {
		t.Fatal("no timestamp for skill 'git'")
	}

	loadedAt, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t.Fatalf("invalid timestamp %q: %v", ts, err)
	}

	if loadedAt.Before(before.Truncate(time.Second)) || loadedAt.After(after.Add(time.Second)) {
		t.Errorf("timestamp = %v, want between %v and %v", loadedAt, before, after)
	}
}

func TestRecordSkill_RefreshesTimestamp(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(dir)

	err := mgr.RecordSkillWithAgent("sess1", "git", "claude")
	if err != nil {
		t.Fatalf("first record failed: %v", err)
	}

	// Read first timestamp, then backdate it to ensure refresh is visible
	stPath := filepath.Join(dir, "sess1.json")
	data1, _ := os.ReadFile(stPath)
	var state1 SessionState
	_ = json.Unmarshal(data1, &state1)
	state1.SkillTimestamps["git"] = time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	backdated, _ := json.Marshal(state1)
	_ = os.WriteFile(stPath, backdated, 0600)

	// Re-record same skill — should update timestamp to now
	err = mgr.RecordSkillWithAgent("sess1", "git", "claude")
	if err != nil {
		t.Fatalf("second record failed: %v", err)
	}

	data2, _ := os.ReadFile(stPath)
	var state2 SessionState
	_ = json.Unmarshal(data2, &state2)
	ts2 := state2.SkillTimestamps["git"]

	refreshedAt, _ := time.Parse(time.RFC3339, ts2)
	if time.Since(refreshedAt) > 5*time.Second {
		t.Errorf("timestamp not refreshed to now, got %q", ts2)
	}

	// Should still have exactly 1 entry in the list
	if len(state2.InvokedSkills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(state2.InvokedSkills))
	}
}

// --- GetFreshSkills tests ---

func TestGetFreshSkills_ZeroTTLReturnsAll(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(dir)

	_ = mgr.RecordSkillWithAgent("sess1", "git", "claude")
	_ = mgr.RecordSkillWithAgent("sess1", "linear", "claude")

	skills, err := mgr.GetFreshSkills("sess1", 0)
	if err != nil {
		t.Fatalf("GetFreshSkills failed: %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
}

func TestGetFreshSkills_ExpiredSkillsExcluded(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(dir)

	// Record a skill, then manually backdate its timestamp
	_ = mgr.RecordSkillWithAgent("sess1", "old-skill", "claude")
	_ = mgr.RecordSkillWithAgent("sess1", "fresh-skill", "claude")

	// Backdate old-skill to 2 hours ago
	stPath := filepath.Join(dir, "sess1.json")
	data, _ := os.ReadFile(stPath)
	var state SessionState
	_ = json.Unmarshal(data, &state)
	state.SkillTimestamps["old-skill"] = time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	updated, _ := json.Marshal(state)
	_ = os.WriteFile(stPath, updated, 0600)

	// TTL of 1 hour — old-skill should be expired
	skills, err := mgr.GetFreshSkills("sess1", 1*time.Hour)
	if err != nil {
		t.Fatalf("GetFreshSkills failed: %v", err)
	}

	if skills["old-skill"] {
		t.Error("old-skill should be expired but was returned as fresh")
	}
	if !skills["fresh-skill"] {
		t.Error("fresh-skill should be fresh but was not returned")
	}
}

func TestGetFreshSkills_NoTimestampTreatedAsFresh(t *testing.T) {
	dir := t.TempDir()

	// Write a state file WITHOUT timestamps (old format)
	state := SessionState{
		SessionID:     "sess1",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		InvokedSkills: []string{"legacy-skill"},
	}
	data, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(dir, "sess1.json"), data, 0600)

	mgr := NewStateManager(dir)
	skills, err := mgr.GetFreshSkills("sess1", 30*time.Minute)
	if err != nil {
		t.Fatalf("GetFreshSkills failed: %v", err)
	}

	if !skills["legacy-skill"] {
		t.Error("legacy skill without timestamp should be treated as fresh")
	}
}

func TestGetFreshSkills_MissingSession(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(dir)

	skills, err := mgr.GetFreshSkills("nonexistent", 30*time.Minute)
	if err != nil {
		t.Fatalf("GetFreshSkills failed: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected empty map, got %v", skills)
	}
}

func TestRecordSkillWithAgent_UpdatesAgent(t *testing.T) {
	dir := t.TempDir()
	mgr := NewStateManager(dir)

	// First call sets agent to claude
	_ = mgr.RecordSkillWithAgent("sess1", "git", "claude")
	// Second call updates to cursor
	_ = mgr.RecordSkillWithAgent("sess1", "linear", "cursor")

	data, _ := os.ReadFile(filepath.Join(dir, "sess1.json"))
	var state SessionState
	_ = json.Unmarshal(data, &state)

	if state.Agent != "cursor" {
		t.Errorf("agent = %q, want %q (last writer wins)", state.Agent, "cursor")
	}
	if len(state.InvokedSkills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(state.InvokedSkills))
	}
}

// TestClean_InvalidSessionID verifies that Clean rejects invalid session IDs.
func TestClean_InvalidSessionID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.Clean("../evil"); err == nil {
		t.Error("Clean(../evil) returned nil, want error for path traversal")
	}
	if err := mgr.Clean(""); err == nil {
		t.Error("Clean(\"\") returned nil, want error for empty session ID")
	}
}

// TestClean_RemovesStateFileOnly verifies that Clean works when only a state file
// exists (no lock file).
func TestClean_RemovesStateFileOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if err := mgr.RecordSkill("sess-clean", "my-skill"); err != nil {
		t.Fatalf("RecordSkill() returned error: %v", err)
	}

	// No lock file exists (normal case after lock release).
	if err := mgr.Clean("sess-clean"); err != nil {
		t.Fatalf("Clean() returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "sess-clean.json")); !os.IsNotExist(err) {
		t.Error("state file still exists after Clean()")
	}
}

// TestClean_RemovesLockFileOnly verifies that Clean works when only a lock file
// exists (state file was already removed or never created).
func TestClean_RemovesLockFileOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	// Create only a lock file (no state file).
	lockPath := filepath.Join(dir, "orphan-lock.lock")
	if err := os.WriteFile(lockPath, []byte{}, 0o600); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	if err := mgr.Clean("orphan-lock"); err != nil {
		t.Fatalf("Clean() returned error: %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file still exists after Clean()")
	}
}

// TestGetFreshSkills_UnparseableTimestampTreatedAsFresh verifies that skills
// with unparseable timestamps are treated as fresh (fail open).
func TestGetFreshSkills_UnparseableTimestampTreatedAsFresh(t *testing.T) {
	dir := t.TempDir()

	state := SessionState{
		SessionID:     "sess-bad-ts",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		InvokedSkills: []string{"broken-ts-skill"},
		SkillTimestamps: map[string]string{
			"broken-ts-skill": "not-a-valid-time",
		},
	}
	data, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(dir, "sess-bad-ts.json"), data, 0o600)

	mgr := NewStateManager(dir)
	skills, err := mgr.GetFreshSkills("sess-bad-ts", 30*time.Minute)
	if err != nil {
		t.Fatalf("GetFreshSkills failed: %v", err)
	}

	if !skills["broken-ts-skill"] {
		t.Error("skill with unparseable timestamp should be treated as fresh")
	}
}

// TestGetFreshSkills_ExpiredSkillFiltered verifies that skills with timestamps
// older than the TTL are filtered out.
func TestGetFreshSkills_ExpiredSkillFiltered(t *testing.T) {
	dir := t.TempDir()

	oldTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	state := SessionState{
		SessionID:     "sess-expired",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		InvokedSkills: []string{"expired-skill"},
		SkillTimestamps: map[string]string{
			"expired-skill": oldTime,
		},
	}
	data, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(dir, "sess-expired.json"), data, 0o600)

	mgr := NewStateManager(dir)
	skills, err := mgr.GetFreshSkills("sess-expired", 30*time.Minute)
	if err != nil {
		t.Fatalf("GetFreshSkills failed: %v", err)
	}

	if skills["expired-skill"] {
		t.Error("expired skill should NOT be in fresh skills map")
	}
	if len(skills) != 0 {
		t.Errorf("expected empty map, got %v", skills)
	}
}

// TestGetFreshSkills_ZeroTTLDelegatesToGetInvokedSkills verifies that a zero
// TTL returns all skills regardless of timestamps.
func TestGetFreshSkills_ZeroTTLReturnsAllSkills(t *testing.T) {
	dir := t.TempDir()

	oldTime := time.Now().UTC().Add(-100 * time.Hour).Format(time.RFC3339)
	state := SessionState{
		SessionID:     "sess-no-ttl",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		InvokedSkills: []string{"ancient-skill"},
		SkillTimestamps: map[string]string{
			"ancient-skill": oldTime,
		},
	}
	data, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(dir, "sess-no-ttl.json"), data, 0o600)

	mgr := NewStateManager(dir)
	skills, err := mgr.GetFreshSkills("sess-no-ttl", 0)
	if err != nil {
		t.Fatalf("GetFreshSkills failed: %v", err)
	}

	if !skills["ancient-skill"] {
		t.Error("with zero TTL, all skills should be returned regardless of age")
	}
}

// TestGetFreshSkills_InvalidSessionID verifies that GetFreshSkills rejects
// invalid session IDs.
func TestGetFreshSkills_InvalidSessionID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	_, err := mgr.GetFreshSkills("../evil", 30*time.Minute)
	if err == nil {
		t.Error("expected error for path traversal session ID")
	}
}

func TestMarkSkillExpired_PreventsRelogging(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)
	sid := "test-expire-dedup"

	err := mgr.RecordSkill(sid, "linear")
	if err != nil {
		t.Fatalf("RecordSkill: %v", err)
	}

	if mgr.IsSkillExpired(sid, "linear") {
		t.Error("expected linear to NOT be expired before marking")
	}

	err = mgr.MarkSkillExpired(sid, "linear")
	if err != nil {
		t.Fatalf("MarkSkillExpired: %v", err)
	}

	if !mgr.IsSkillExpired(sid, "linear") {
		t.Error("expected linear to be expired after marking")
	}

	// Idempotent — marking again is a no-op.
	err = mgr.MarkSkillExpired(sid, "linear")
	if err != nil {
		t.Fatalf("MarkSkillExpired (second call): %v", err)
	}
}

func TestIsSkillExpired_MissingSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)

	if mgr.IsSkillExpired("nonexistent", "linear") {
		t.Error("expected false for nonexistent session")
	}
}

func TestMarkSkillExpired_ClearedOnReload(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewStateManager(dir)
	sid := "test-expire-reload"

	err := mgr.RecordSkill(sid, "linear")
	if err != nil {
		t.Fatalf("RecordSkill: %v", err)
	}
	err = mgr.MarkSkillExpired(sid, "linear")
	if err != nil {
		t.Fatalf("MarkSkillExpired: %v", err)
	}

	// Reload clears the expired flag.
	err = mgr.RecordSkill(sid, "linear")
	if err != nil {
		t.Fatalf("RecordSkill (reload): %v", err)
	}
	if mgr.IsSkillExpired(sid, "linear") {
		t.Error("expected linear to NOT be expired after reload")
	}
}
