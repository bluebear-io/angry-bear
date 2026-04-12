// lock_test.go tests the FileLock wrapper around gofrs/flock for advisory file locking.
// It verifies exclusive and shared lock acquisition, non-blocking behavior, and unlock semantics.
package state

import (
	"path/filepath"
	"testing"
)

// TestFileLock_TryLock_Success verifies that an exclusive lock can be acquired on a new lock file.
func TestFileLock_TryLock_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")
	fl := NewFileLock(lockPath)

	if err := fl.TryLock(); err != nil {
		t.Fatalf("TryLock() returned error: %v", err)
	}
	defer fl.Unlock()
}

// TestFileLock_TryRLock_Success verifies that a shared read lock can be acquired.
func TestFileLock_TryRLock_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")
	fl := NewFileLock(lockPath)

	if err := fl.TryRLock(); err != nil {
		t.Fatalf("TryRLock() returned error: %v", err)
	}
	defer fl.Unlock()
}

// TestFileLock_TryLock_FailsWhenExclusiveHeld verifies that a second exclusive lock
// acquisition fails (non-blocking) when the lock is already held.
func TestFileLock_TryLock_FailsWhenExclusiveHeld(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	// Acquire the first exclusive lock.
	fl1 := NewFileLock(lockPath)
	if err := fl1.TryLock(); err != nil {
		t.Fatalf("first TryLock() returned error: %v", err)
	}
	defer fl1.Unlock()

	// Attempt a second exclusive lock on the same file (should fail non-blocking).
	fl2 := NewFileLock(lockPath)
	err := fl2.TryLock()
	if err == nil {
		fl2.Unlock()
		t.Fatal("second TryLock() returned nil, want error because lock is already held")
	}
}

// TestFileLock_Unlock_ReleasesLock verifies that after unlocking, another lock can be acquired.
func TestFileLock_Unlock_ReleasesLock(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	// Acquire and release.
	fl1 := NewFileLock(lockPath)
	if err := fl1.TryLock(); err != nil {
		t.Fatalf("TryLock() returned error: %v", err)
	}
	if err := fl1.Unlock(); err != nil {
		t.Fatalf("Unlock() returned error: %v", err)
	}

	// Now a new lock should succeed.
	fl2 := NewFileLock(lockPath)
	if err := fl2.TryLock(); err != nil {
		t.Fatalf("TryLock() after unlock returned error: %v", err)
	}
	defer fl2.Unlock()
}

// TestFileLock_CreatesLockFile verifies that the lock file is created separate from data files.
func TestFileLock_CreatesLockFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lockPath := filepath.Join(dir, "session.lock")
	fl := NewFileLock(lockPath)

	if err := fl.TryLock(); err != nil {
		t.Fatalf("TryLock() returned error: %v", err)
	}
	defer fl.Unlock()

	// The .lock file should exist (created by flock).
	// We just verify the lock path is what we expect -- the data file (session.json) is separate.
	if fl.flock.Path() != lockPath {
		t.Errorf("lock file path = %q, want %q", fl.flock.Path(), lockPath)
	}
}
