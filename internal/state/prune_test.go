// prune_test.go tests TTL-based cleanup of expired session state files.
// It uses os.Chtimes to manipulate file modification times for deterministic expiry testing.
package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPruneExpired_RemovesOldFiles verifies that state files older than the TTL are removed.
func TestPruneExpired_RemovesOldFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create an "old" state file and backdate its mtime.
	oldFile := filepath.Join(dir, "old-session.json")
	if err := os.WriteFile(oldFile, []byte(`{"session_id":"old-session"}`), 0600); err != nil {
		t.Fatalf("failed to create old state file: %v", err)
	}
	oldTime := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set mtime: %v", err)
	}

	ttl := 24 * time.Hour
	if err := PruneExpired(dir, ttl); err != nil {
		t.Fatalf("PruneExpired() returned error: %v", err)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old state file still exists after PruneExpired()")
	}
}

// TestPruneExpired_RemovesCorrespondingLockFiles verifies that when a state file is pruned,
// its corresponding .lock file is also removed.
func TestPruneExpired_RemovesCorrespondingLockFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	oldJSON := filepath.Join(dir, "old-session.json")
	oldLock := filepath.Join(dir, "old-session.lock")
	if err := os.WriteFile(oldJSON, []byte(`{"session_id":"old-session"}`), 0600); err != nil {
		t.Fatalf("failed to create state file: %v", err)
	}
	if err := os.WriteFile(oldLock, []byte{}, 0600); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	oldTime := time.Now().Add(-25 * time.Hour)
	os.Chtimes(oldJSON, oldTime, oldTime)
	os.Chtimes(oldLock, oldTime, oldTime)

	ttl := 24 * time.Hour
	if err := PruneExpired(dir, ttl); err != nil {
		t.Fatalf("PruneExpired() returned error: %v", err)
	}

	if _, err := os.Stat(oldJSON); !os.IsNotExist(err) {
		t.Error("old state file still exists")
	}
	if _, err := os.Stat(oldLock); !os.IsNotExist(err) {
		t.Error("old lock file still exists")
	}
}

// TestPruneExpired_PreservesRecentFiles verifies that state files within the TTL are not removed.
func TestPruneExpired_PreservesRecentFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	recentFile := filepath.Join(dir, "recent-session.json")
	if err := os.WriteFile(recentFile, []byte(`{"session_id":"recent-session"}`), 0600); err != nil {
		t.Fatalf("failed to create recent state file: %v", err)
	}
	// File was just created, mtime is now -- well within TTL.

	ttl := 24 * time.Hour
	if err := PruneExpired(dir, ttl); err != nil {
		t.Fatalf("PruneExpired() returned error: %v", err)
	}

	if _, err := os.Stat(recentFile); err != nil {
		t.Error("recent state file was incorrectly removed")
	}
}

// TestPruneExpired_EmptyDirectory verifies that PruneExpired handles an empty directory gracefully.
func TestPruneExpired_EmptyDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := PruneExpired(dir, 24*time.Hour); err != nil {
		t.Fatalf("PruneExpired() on empty dir returned error: %v", err)
	}
}

// TestPruneExpired_UsesMtime verifies that PruneExpired uses file mtime (not JSON parsing)
// by creating a file with recent JSON content but backdated mtime.
func TestPruneExpired_UsesMtime(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// The JSON contains a recent created_at, but the file mtime is old.
	recentJSON := `{"session_id":"mtime-test","created_at":"` + time.Now().UTC().Format(time.RFC3339) + `"}`
	filePath := filepath.Join(dir, "mtime-test.json")
	if err := os.WriteFile(filePath, []byte(recentJSON), 0600); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set mtime: %v", err)
	}

	ttl := 24 * time.Hour
	if err := PruneExpired(dir, ttl); err != nil {
		t.Fatalf("PruneExpired() returned error: %v", err)
	}

	// File should be removed based on mtime, not the JSON content.
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("file with old mtime was not removed (PruneExpired may be parsing JSON instead of using mtime)")
	}
}

// TestPruneExpired_SkipsNonMatchingFiles verifies that files not matching the expected
// naming pattern are left alone.
func TestPruneExpired_SkipsNonMatchingFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create a file that does NOT match the expected pattern.
	otherFile := filepath.Join(dir, "README.txt")
	if err := os.WriteFile(otherFile, []byte("hello"), 0600); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(otherFile, oldTime, oldTime)

	if err := PruneExpired(dir, 24*time.Hour); err != nil {
		t.Fatalf("PruneExpired() returned error: %v", err)
	}

	if _, err := os.Stat(otherFile); err != nil {
		t.Error("non-matching file was incorrectly removed")
	}
}

// TestPruneIfDue_SkipsWhenThrottled verifies that PruneIfDue skips pruning when
// less than 1 hour has passed since the last prune.
func TestPruneIfDue_SkipsWhenThrottled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Write a recent .last-prune timestamp.
	lastPrunePath := filepath.Join(dir, ".last-prune")
	recentTime := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
	if err := os.WriteFile(lastPrunePath, []byte(recentTime), 0600); err != nil {
		t.Fatalf("failed to write .last-prune: %v", err)
	}

	// Create an old file that would be pruned.
	oldFile := filepath.Join(dir, "old-session.json")
	if err := os.WriteFile(oldFile, []byte(`{}`), 0600); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(oldFile, oldTime, oldTime)

	if err := PruneIfDue(dir, 24*time.Hour); err != nil {
		t.Fatalf("PruneIfDue() returned error: %v", err)
	}

	// The old file should still exist because pruning was throttled.
	if _, err := os.Stat(oldFile); os.IsNotExist(err) {
		t.Error("old file was removed despite pruning being throttled")
	}
}

// TestPruneIfDue_RunsWhenDue verifies that PruneIfDue runs pruning when more than
// 1 hour has passed since the last prune.
func TestPruneIfDue_RunsWhenDue(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Write an old .last-prune timestamp.
	lastPrunePath := filepath.Join(dir, ".last-prune")
	oldPruneTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if err := os.WriteFile(lastPrunePath, []byte(oldPruneTime), 0600); err != nil {
		t.Fatalf("failed to write .last-prune: %v", err)
	}

	// Create an old file.
	oldFile := filepath.Join(dir, "old-session.json")
	if err := os.WriteFile(oldFile, []byte(`{}`), 0600); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(oldFile, oldTime, oldTime)

	if err := PruneIfDue(dir, 24*time.Hour); err != nil {
		t.Fatalf("PruneIfDue() returned error: %v", err)
	}

	// The old file should be removed because pruning was due.
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file was not removed despite pruning being due")
	}
}

// TestPruneIfDue_FirstRun verifies that PruneIfDue runs pruning when no .last-prune
// file exists (first time).
func TestPruneIfDue_FirstRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create an old file.
	oldFile := filepath.Join(dir, "old-session.json")
	if err := os.WriteFile(oldFile, []byte(`{}`), 0600); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(oldFile, oldTime, oldTime)

	if err := PruneIfDue(dir, 24*time.Hour); err != nil {
		t.Fatalf("PruneIfDue() returned error: %v", err)
	}

	// Should have pruned on first run.
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file was not removed on first prune run")
	}
}

// TestPruneExpired_UpdatesLastPrune verifies that PruneExpired updates the .last-prune
// timestamp after running.
func TestPruneExpired_UpdatesLastPrune(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	before := time.Now().UTC()

	if err := PruneExpired(dir, 24*time.Hour); err != nil {
		t.Fatalf("PruneExpired() returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".last-prune"))
	if err != nil {
		t.Fatalf("failed to read .last-prune: %v", err)
	}

	ts, err := time.Parse(time.RFC3339, string(data))
	if err != nil {
		t.Fatalf("failed to parse .last-prune timestamp %q: %v", string(data), err)
	}

	if ts.Before(before.Truncate(time.Second)) {
		t.Errorf(".last-prune timestamp %v is before test start %v", ts, before)
	}
}

// TestPruneExpired_SkipsLastPruneFile verifies that PruneExpired does not delete
// the .last-prune file itself, even if it's old.
func TestPruneExpired_SkipsLastPruneFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create an old .last-prune file.
	lastPrunePath := filepath.Join(dir, ".last-prune")
	if err := os.WriteFile(lastPrunePath, []byte("2020-01-01T00:00:00Z"), 0600); err != nil {
		t.Fatalf("failed to write .last-prune: %v", err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(lastPrunePath, oldTime, oldTime)

	if err := PruneExpired(dir, 24*time.Hour); err != nil {
		t.Fatalf("PruneExpired() returned error: %v", err)
	}

	// .last-prune should still exist (updated, not deleted).
	if _, err := os.Stat(lastPrunePath); os.IsNotExist(err) {
		t.Error(".last-prune was deleted instead of updated")
	}
}
