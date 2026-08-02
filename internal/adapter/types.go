// Package adapter provides pluggable adapters for AI coding agent hook integration.
package adapter

// HookInput is the normalized, adapter-agnostic hook input.
type HookInput struct {
	SessionID string
	ToolName  string
	FilePath  string
	Command   string // Shell command string for command-running tools (e.g. Bash).
	Agent     string
	Cwd       string
	RawInput  map[string]any
}
