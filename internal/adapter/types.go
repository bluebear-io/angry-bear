// Package adapter provides pluggable adapters for AI coding agent hook integration.
package adapter

// HookInput is the normalized, adapter-agnostic hook input.
type HookInput struct {
	SessionID string
	ToolName  string
	FilePath  string
	Agent     string
	Cwd       string
	RawInput  map[string]any
}
