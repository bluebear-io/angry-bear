// manager.go provides the StateManager for reading and writing session state files.
// It handles concurrent access via file locks and atomic writes to ensure consistency.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/natefinch/atomic"
)

// StateManager handles reading and writing session state files.
// State files are stored as JSON in the configured state directory,
// one file per session identified by session ID.
type StateManager struct {
	stateDir string
}

// NewStateManager creates a StateManager that operates on the given directory.
// The directory must already exist; NewStateManager does not create it.
func NewStateManager(stateDir string) *StateManager {
	return &StateManager{stateDir: stateDir}
}

// statePath returns the path to the JSON state file for the given session.
func (m *StateManager) statePath(sessionID string) string {
	return filepath.Join(m.stateDir, sessionID+".json")
}

// lockPath returns the path to the advisory lock file for the given session.
func (m *StateManager) lockPath(sessionID string) string {
	return filepath.Join(m.stateDir, sessionID+".lock")
}

// RecordSkill adds a skill to the session's invoked skills list.
// Creates the state file if it doesn't exist. Idempotent -- recording the same
// skill twice has no effect. Uses an exclusive file lock for concurrency safety
// and atomic writes to prevent partial/corrupt state files.
func (m *StateManager) RecordSkill(sessionID, skillName string) error {
	return m.RecordSkillWithAgent(sessionID, skillName, "")
}

// RecordSkillWithAgent adds a skill to the session's invoked skills list and
// sets the agent for this session. The agent is recorded once per session.
func (m *StateManager) RecordSkillWithAgent(sessionID, skillName, agent string) error {
	if err := ValidateSessionID(sessionID); err != nil {
		return err
	}

	lock := NewFileLock(m.lockPath(sessionID))
	if err := lock.TryLock(); err != nil {
		return err
	}
	defer lock.Unlock()

	stPath := m.statePath(sessionID)

	// Read existing state or create a new one.
	state, err := readStateFile(stPath)
	if err != nil {
		return err
	}

	if state == nil {
		state = &SessionState{
			SessionID:     sessionID,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
			InvokedSkills: []string{},
		}
	}

	// Set agent if provided. Always update — the most recent caller wins.
	if agent != "" {
		state.Agent = agent
	}

	// Initialize timestamps map if nil (backward compat with old state files).
	if state.SkillTimestamps == nil {
		state.SkillTimestamps = make(map[string]string)
	}

	// Always update the timestamp for this skill (refreshes TTL on reload).
	state.SkillTimestamps[skillName] = time.Now().UTC().Format(time.RFC3339)

	// Check if skill is already recorded (idempotent for the list).
	for _, s := range state.InvokedSkills {
		if s == skillName {
			return writeStateFile(stPath, state)
		}
	}

	// Append the new skill.
	state.InvokedSkills = append(state.InvokedSkills, skillName)

	return writeStateFile(stPath, state)
}

// HasSkill checks if a skill has been invoked in the given session.
// Returns false if the state file doesn't exist or the session ID is invalid.
func (m *StateManager) HasSkill(sessionID, skillName string) bool {
	if err := ValidateSessionID(sessionID); err != nil {
		return false
	}

	lock := NewFileLock(m.lockPath(sessionID))
	if err := lock.TryRLock(); err != nil {
		return false
	}
	defer lock.Unlock()

	state, err := readStateFile(m.statePath(sessionID))
	if err != nil || state == nil {
		return false
	}

	for _, s := range state.InvokedSkills {
		if s == skillName {
			return true
		}
	}
	return false
}

// GetInvokedSkills returns all invoked skills for a session as a map.
// Returns an empty map if no state file exists. Each skill name maps to true.
func (m *StateManager) GetInvokedSkills(sessionID string) (map[string]bool, error) {
	if err := ValidateSessionID(sessionID); err != nil {
		return nil, err
	}

	lock := NewFileLock(m.lockPath(sessionID))
	if err := lock.TryRLock(); err != nil {
		return nil, err
	}
	defer lock.Unlock()

	state, err := readStateFile(m.statePath(sessionID))
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	if state == nil {
		return result, nil
	}

	for _, s := range state.InvokedSkills {
		result[s] = true
	}
	return result, nil
}

// GetFreshSkills returns invoked skills that are still within the given TTL.
// If ttl is zero, all skills are returned (no expiry). Skills without a recorded
// timestamp (from older state files) are always considered fresh.
func (m *StateManager) GetFreshSkills(sessionID string, ttl time.Duration) (map[string]bool, error) {
	if ttl == 0 {
		return m.GetInvokedSkills(sessionID)
	}

	err := ValidateSessionID(sessionID)
	if err != nil {
		return nil, err
	}

	lock := NewFileLock(m.lockPath(sessionID))
	err = lock.TryRLock()
	if err != nil {
		return nil, err
	}
	defer lock.Unlock()

	st, err := readStateFile(m.statePath(sessionID))
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	if st == nil {
		return result, nil
	}

	now := time.Now().UTC()
	for _, skill := range st.InvokedSkills {
		ts, hasTS := st.SkillTimestamps[skill]
		if !hasTS {
			// No timestamp recorded (old state file) — treat as fresh.
			result[skill] = true
			continue
		}
		loadedAt, parseErr := time.Parse(time.RFC3339, ts)
		if parseErr != nil {
			// Unparseable timestamp — treat as fresh to fail open.
			result[skill] = true
			continue
		}
		if now.Sub(loadedAt) <= ttl {
			result[skill] = true
		}
	}
	return result, nil
}

// Clean removes the state file and lock file for a specific session.
// Idempotent -- no error is returned if the files don't exist.
func (m *StateManager) Clean(sessionID string) error {
	if err := ValidateSessionID(sessionID); err != nil {
		return err
	}

	// Remove the state file; ignore "not exist" errors.
	if err := os.Remove(m.statePath(sessionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Remove the lock file; ignore "not exist" errors.
	if err := os.Remove(m.lockPath(sessionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// readStateFile reads and unmarshals a session state JSON file.
// Returns (nil, nil) if the file does not exist.
// Returns an error if the file exists but cannot be read or parsed.
func readStateFile(path string) (*SessionState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// writeStateFile marshals and atomically writes a session state to disk with 0600 permissions.
// Uses natefinch/atomic to ensure no partial writes on crash.
func writeStateFile(path string, state *SessionState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	reader := bytes.NewReader(data)
	if err := atomic.WriteFile(path, reader); err != nil {
		return err
	}

	// Ensure 0600 permissions (atomic.WriteFile may use default umask).
	return os.Chmod(path, 0o600)
}
