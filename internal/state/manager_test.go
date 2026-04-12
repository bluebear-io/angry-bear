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
