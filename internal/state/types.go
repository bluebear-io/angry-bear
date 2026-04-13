// Package state provides file-system based session state management for care-bare.
// It tracks which skills have been invoked per session using JSON files on disk.
package state

// SessionState represents the persisted state for a single session.
type SessionState struct {
	SessionID       string            `json:"session_id"`
	Agent           string            `json:"agent,omitempty"`
	CreatedAt       string            `json:"created_at"`
	InvokedSkills   []string          `json:"invoked_skills"`
	SkillTimestamps map[string]string `json:"skill_timestamps,omitempty"`
}
