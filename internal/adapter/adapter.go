// adapter.go defines the HookAdapter interface that each AI agent adapter must implement.
// All agent-specific logic lives in adapters. To add a new agent (e.g., Codex),
// implement this interface — no other code needs to know about agent-specific details.
package adapter

import "io"

// AgentProject represents a project discovered from an agent's project directory.
type AgentProject struct {
	Name    string // Human-readable name (last path component)
	Path    string // Absolute path to the project on disk
	Agent   string // Which agent owns this project
	Encoded string // Encoded directory name from the agent's config
}

// HookAdapter is the interface each AI agent adapter must implement.
// Every piece of agent-specific logic belongs here — hook formats, tool name
// normalization, project discovery, config file locations, everything.
type HookAdapter interface {
	// Name returns the adapter identifier (e.g., "claude", "cursor").
	Name() string
	// ParseInput reads JSON from the agent's stdin and returns a normalized HookInput.
	ParseInput(stdin io.Reader) (*HookInput, error)
	// FormatAllow returns the bytes to write to stdout when allowing a tool call.
	FormatAllow() ([]byte, error)
	// FormatDeny returns the bytes to write to stdout when blocking a tool call.
	FormatDeny(reason string) ([]byte, error)
	// ConfigPath returns the relative path to the agent's hook config file.
	// Used to detect if this agent is present in a project.
	ConfigPath() string
	// InstallHook modifies the agent's global config to add a care-bear hook.
	InstallHook(projectDir string) error
	// GlobalConfigPath returns the absolute path to the agent's global hook config file.
	GlobalConfigPath() string
	// UninstallHook removes all care-bear hooks from the agent's global config.
	UninstallHook() error
	// ExitCodeForDeny returns the process exit code for a deny response.
	// Claude Code uses 0 (reads deny from stdout JSON), Cursor uses 2.
	ExitCodeForDeny() int
	// DetectSkillInvocation checks if the input represents a skill being invoked.
	DetectSkillInvocation(input *HookInput) (skillName string, isSkill bool)
	// ScanProjects discovers all projects that have sessions with this agent.
	// Each adapter knows where its agent stores project data
	// (e.g., ~/.claude/projects/, ~/.cursor/projects/).
	ScanProjects() ([]AgentProject, error)
}
