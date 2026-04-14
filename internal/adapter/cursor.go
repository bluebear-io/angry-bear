// cursor.go implements the HookAdapter interface for Cursor IDE.
// Cursor hooks use a different JSON format from Claude Code: session identifiers
// use conversation_id, tool fields are top-level (not nested in tool_input),
// and allow/deny responses use {"continue": bool, "userMessage": "..."} format.
package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// cursorToolMap maps Cursor-native tool names and hook events to canonical
// angry-bear tool names. This is the adapter's core job — normalize agent-specific
// names so enforcement rules use a single vocabulary across all agents.
var cursorToolMap = map[string]string{
	// Cursor tool_name values
	"edit_file":            "Edit",
	"write_file":           "Write",
	"read_file":            "Read",
	"create_file":          "Write",
	"delete_file":          "Write",
	"list_dir":             "Glob",
	"search_files":         "Grep",
	"codebase_search":      "Grep",
	"grep_search":          "Grep",
	"run_terminal_command": "Bash",
	"terminal":             "Bash",

	// Cursor hook_event_name values (fallback when tool_name is empty)
	"beforeFileEdit":       "Edit",
	"beforeShellExecution": "Bash",
	"beforeReadFile":       "Read",
	"beforeMCPExecution":   "Agent",
	"preToolUse":           "", // generic, keep original
}

// CursorAdapter implements HookAdapter for Cursor IDE.
type CursorAdapter struct {
	HomeDir    string // Override home directory (empty = os.UserHomeDir)
	BinaryPath string // Override binary path (empty = auto-detect)
}

// ExitCodeForDeny returns 2. Cursor blocks operations on non-zero exit codes.
func (a *CursorAdapter) ExitCodeForDeny() int { return 2 }

func (a *CursorAdapter) Name() string { return "cursor" }

// ParseInput reads Cursor hook JSON from stdin and returns a normalized HookInput.
// The expected JSON format contains: hook_event_name, conversation_id, cursor_version,
// workspace_roots, and top-level tool fields (tool_name, file_path, command).
// Unlike Claude Code, tool-specific fields are top-level rather than nested in tool_input.
func (a *CursorAdapter) ParseInput(stdin io.Reader) (*HookInput, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	input := &HookInput{
		Agent:    "cursor",
		RawInput: raw,
	}

	// Extract conversation_id as SessionID (Cursor uses conversation_id, not session_id)
	if cid, ok := raw["conversation_id"].(string); ok {
		input.SessionID = cid
	}

	// Extract and normalize tool_name to canonical angry-bear name.
	// Cursor uses agent-specific names like "edit_file", "write_file" etc.
	if tn, ok := raw["tool_name"].(string); ok {
		if canonical, exists := cursorToolMap[tn]; exists && canonical != "" {
			input.ToolName = canonical
		} else {
			input.ToolName = tn
		}
	}

	// Fallback: if tool_name is empty, derive from hook_event_name
	if input.ToolName == "" {
		if event, ok := raw["hook_event_name"].(string); ok {
			if canonical, exists := cursorToolMap[event]; exists && canonical != "" {
				input.ToolName = canonical
			}
		}
	}

	// Extract file_path — check top-level first, then tool_input (preToolUse format)
	if fp, ok := raw["file_path"].(string); ok {
		input.FilePath = fp
	} else if toolInput, ok := raw["tool_input"].(map[string]any); ok {
		if fp, ok := toolInput["file_path"].(string); ok {
			input.FilePath = fp
		} else if p, ok := toolInput["path"].(string); ok {
			input.FilePath = p
		}
	}

	// Extract session_id (preToolUse format uses session_id alongside conversation_id)
	if input.SessionID == "" {
		if sid, ok := raw["session_id"].(string); ok {
			input.SessionID = sid
		}
	}

	// Extract Cwd from workspace_roots array (use first entry)
	if roots, ok := raw["workspace_roots"].([]any); ok && len(roots) > 0 {
		if first, ok := roots[0].(string); ok {
			input.Cwd = first
		}
	}

	return input, nil
}

// FormatAllow returns the JSON for allowing a tool call in Cursor format.
// Cursor expects {"continue": true} to pass through a hook.
func (a *CursorAdapter) FormatAllow() ([]byte, error) {
	response := map[string]any{
		"continue": true,
	}
	return json.Marshal(response)
}

// FormatDeny returns the JSON for blocking a tool call in Cursor format.
func (a *CursorAdapter) FormatDeny(reason string) ([]byte, error) {
	response := map[string]any{
		"continue":     false,
		"permission":   "deny",
		"userMessage":  reason,
		"agentMessage": reason,
	}
	return json.Marshal(response)
}

// ConfigPath returns the relative path to detect if Cursor is present in a project.
func (a *CursorAdapter) ConfigPath() string { return ".cursor/hooks.json" }

// GlobalConfigPath returns the absolute path to the global Cursor hooks file.
// Uses a.HomeDir if set, otherwise resolves via os.UserHomeDir().
func (a *CursorAdapter) GlobalConfigPath() string {
	home := a.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return filepath.Join(".cursor", "hooks.json")
		}
	}
	return filepath.Join(home, ".cursor", "hooks.json")
}

// InstallHook adds angry-bear hooks to the GLOBAL Cursor hooks config.
// This ensures enforcement works in every project without per-project init.
// projectDir is ignored — hooks are always installed globally.
// This method is idempotent -- calling it twice does not duplicate the hook entry.
// It preserves all existing hooks and other config keys (e.g., version).
func (a *CursorAdapter) InstallHook(projectDir string) error {
	hooksPath := a.GlobalConfigPath()
	cursorDir := filepath.Dir(hooksPath)

	// Ensure .cursor directory exists
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		return fmt.Errorf("creating .cursor directory: %w", err)
	}

	// Read existing hooks.json or start with default structure
	config := map[string]any{
		"version": float64(1),
		"hooks":   map[string]any{},
	}
	data, err := os.ReadFile(hooksPath)
	if err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parsing existing hooks.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading hooks.json: %w", err)
	}

	// Navigate to hooks, creating if missing
	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		config["hooks"] = hooks
	}

	// Check if angry-bear hook already exists in any hook type (idempotency check)
	if cursorCareBareHookExists(hooks) {
		return nil
	}

	// Only preToolUse can block operations in Cursor (exit code 2).
	// Other before* hooks are observe-only — they cannot prevent actions.
	hookTypes := []string{
		"preToolUse",
	}

	// Cursor requires relative binary name (not absolute path) for hook execution.
	binPath := "angry-bear"
	angryBearEntry := map[string]any{
		"command": binPath + " hook cursor",
	}

	// Prepend angry-bear hook to each hook type, preserving existing entries
	for _, hookType := range hookTypes {
		var existing []any
		if arr, ok := hooks[hookType].([]any); ok {
			existing = arr
		}
		// Prepend angry-bear entry so it runs before other hooks
		hooks[hookType] = append([]any{angryBearEntry}, existing...)
	}

	// Write back with 2-space indent for readability
	output, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling hooks.json: %w", err)
	}
	// Append trailing newline for clean file ending
	output = append(output, '\n')

	if err := os.WriteFile(hooksPath, output, 0o644); err != nil {
		return fmt.Errorf("writing hooks.json: %w", err)
	}

	return nil
}

// cursorCareBareHookExists checks if any hook array in the hooks map already
// contains a angry-bear hook command. Used for idempotency.
func cursorCareBareHookExists(hooks map[string]any) bool {
	for _, hookList := range hooks {
		arr, ok := hookList.([]any)
		if !ok {
			continue
		}
		for _, entry := range arr {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := entryMap["command"].(string); ok {
				if strings.Contains(cmd, "angry-bear hook") {
					return true
				}
			}
		}
	}
	return false
}

// UninstallHook removes all angry-bear hooks from Cursor's global hooks config.
func (a *CursorAdapter) UninstallHook() error {
	hooksPath := a.GlobalConfigPath()
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading hooks.json: %w", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing hooks.json: %w", err)
	}

	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		return nil
	}

	// For each hook type, filter out angry-bear entries
	for hookType, hookList := range hooks {
		arr, ok := hookList.([]any)
		if !ok {
			continue
		}
		var filtered []any
		for _, entry := range arr {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				filtered = append(filtered, entry)
				continue
			}
			if cmd, ok := entryMap["command"].(string); ok && strings.Contains(cmd, "angry-bear hook") {
				continue // skip angry-bear entries
			}
			filtered = append(filtered, entry)
		}
		hooks[hookType] = filtered
	}

	output, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling hooks.json: %w", err)
	}
	output = append(output, '\n')
	return os.WriteFile(hooksPath, output, 0o644)
}

// DetectSkillInvocation checks if the input represents a skill invocation.
// Cursor does not have a native Skill tool like Claude Code, so this always
// returns ("", false).
func (a *CursorAdapter) DetectSkillInvocation(input *HookInput) (string, bool) {
	return "", false
}

// ScanProjects discovers all projects with Cursor sessions by scanning
// ~/.cursor/projects/. Each subdirectory is an encoded project path.
func (a *CursorAdapter) ScanProjects() ([]AgentProject, error) {
	home := a.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}

	projectsDir := filepath.Join(home, ".cursor", "projects")

	// Use the shared scanner that tries sessions-index.json then greedy decode
	return scanAgentProjectDir(projectsDir, "cursor")
}
