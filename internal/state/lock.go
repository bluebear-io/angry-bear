// lock.go provides a thin wrapper around gofrs/flock for advisory file locking.
// It supports non-blocking exclusive and shared read locks for state file operations.
package state

import (
	"fmt"

	"github.com/gofrs/flock"
)

// FileLock provides advisory file locking for state file operations.
// Lock files are kept separate from data files (e.g., session.lock alongside session.json).
type FileLock struct {
	flock *flock.Flock
}

// NewFileLock creates a lock backed by the given lock file path.
// The lock file is created automatically by gofrs/flock on first acquisition.
func NewFileLock(path string) *FileLock {
	return &FileLock{flock: flock.New(path)}
}

// TryLock attempts to acquire an exclusive lock (non-blocking).
// Returns an error if the lock cannot be acquired (e.g., held by another process).
func (l *FileLock) TryLock() error {
	locked, err := l.flock.TryLock()
	if err != nil {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("state file is locked by another process")
	}
	return nil
}

// TryRLock attempts to acquire a shared read lock (non-blocking).
// Returns an error if the lock cannot be acquired (e.g., an exclusive lock is held).
func (l *FileLock) TryRLock() error {
	locked, err := l.flock.TryRLock()
	if err != nil {
		return fmt.Errorf("failed to acquire shared lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("state file is locked by another process")
	}
	return nil
}

// Unlock releases any held lock (exclusive or shared).
func (l *FileLock) Unlock() error {
	return l.flock.Unlock()
}
