// claude.go implements the HookAdapter interface for Claude Code.
// It handles parsing Claude Code's PreToolUse hook JSON, formatting allow/deny
// responses, detecting skill invocations, and installing the angry-bear hook
// into .claude/settings.json.
package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// resolveCareBareCommand returns the path to the angry-bear binary for use
// in hook configurations. Uses binaryPath if non-empty, otherwise resolves the
// absolute path to the running binary. This ensures hooks work regardless
// of whether angry-bear is on PATH.
func resolveCareBareCommand(binaryPath string) string {
	if binaryPath != "" {
		return binaryPath
	}
	exe, err := os.Executable()
	if err != nil {
		return "angry-bear" // fallback to PATH-based lookup
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	return resolved
}

// ClaudeAdapter implements HookAdapter for Claude Code.
type ClaudeAdapter struct {
	HomeDir    string // Override home directory (empty = os.UserHomeDir)
	BinaryPath string // Override binary path (empty = auto-detect)
}

// ExitCodeForDeny returns 0. Claude reads the deny decision from stdout JSON.
func (a *ClaudeAdapter) ExitCodeForDeny() int { return 0 }

func (a *ClaudeAdapter) Name() string { return "claude" }

// ParseInput reads Claude Code hook JSON from stdin and returns a normalized HookInput.
// The expected JSON format contains: hook_event_name, session_id, tool_name, tool_input, cwd.
// tool_input is a nested object with tool-specific fields (e.g., file_path for Edit/Write,
// command for Bash, skill for Skill).
func (a *ClaudeAdapter) ParseInput(stdin io.Reader) (*HookInput, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	input := &HookInput{
		Agent:    "claude",
		RawInput: raw,
	}

	// Extract session_id
	if sid, ok := raw["session_id"].(string); ok {
		input.SessionID = sid
	}

	// Extract tool_name
	if tn, ok := raw["tool_name"].(string); ok {
		input.ToolName = tn
	}

	// Extract cwd
	if cwd, ok := raw["cwd"].(string); ok {
		input.Cwd = cwd
	}

	// Extract file path from tool_input. Different tools use different field names:
	// Edit/Write/Read use "file_path", Grep/Glob use "path".
	if toolInput, ok := raw["tool_input"].(map[string]any); ok {
		if fp, ok := toolInput["file_path"].(string); ok {
			input.FilePath = fp
		} else if p, ok := toolInput["path"].(string); ok {
			input.FilePath = p
		}
	}

	return input, nil
}

// FormatAllow returns empty bytes. Claude Code interprets exit 0 with no stdout as "allow".
func (a *ClaudeAdapter) FormatAllow() ([]byte, error) { return []byte{}, nil }

// FormatDeny returns JSON with hookSpecificOutput containing a deny decision.
// The format matches Claude Code's expected hook response structure.
func (a *ClaudeAdapter) FormatDeny(reason string) ([]byte, error) {
	response := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		},
	}
	return json.Marshal(response)
}

// ConfigPath returns the relative path to the agent's config directory marker.
// Used by init to detect if this agent is present in a project.
func (a *ClaudeAdapter) ConfigPath() string { return ".claude/settings.json" }

// GlobalConfigPath returns the absolute path to the global settings file.
// Uses a.HomeDir if set, otherwise resolves via os.UserHomeDir().
func (a *ClaudeAdapter) GlobalConfigPath() string {
	home := a.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return filepath.Join(".claude", "settings.json")
		}
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// InstallHook adds a angry-bear PreToolUse hook to the GLOBAL Claude Code settings.
// This ensures enforcement works in every project without per-project init.
// projectDir is ignored — hooks are always installed globally.
// This method is idempotent -- calling it twice does not duplicate the hook entry.
// It preserves all existing hooks and other settings keys.
func (a *ClaudeAdapter) InstallHook(projectDir string) error {
	settingsPath := a.GlobalConfigPath()
	claudeDir := filepath.Dir(settingsPath)

	// Ensure .claude directory exists
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
	}

	// Read existing settings or start with empty object
	settings := make(map[string]any)
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing existing settings.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading settings.json: %w", err)
	}

	// Navigate to hooks.PreToolUse, creating the path if missing
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}

	// Get existing PreToolUse array or create empty one
	var preToolUse []any
	if existing, ok := hooks["PreToolUse"].([]any); ok {
		preToolUse = existing
	}

	// Check if angry-bear hook already exists (idempotency check)
	if angryBearHookExists(preToolUse) {
		return nil
	}

	// Append the angry-bear hook entry using the absolute binary path
	binPath := resolveCareBareCommand(a.BinaryPath)
	angryBearEntry := map[string]any{
		"matcher": "*",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": binPath + " hook claude",
			},
		},
	}
	preToolUse = append(preToolUse, angryBearEntry)
	hooks["PreToolUse"] = preToolUse

	// Write back with 2-space indent for readability
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings.json: %w", err)
	}
	// Append trailing newline for clean file ending
	output = append(output, '\n')

	if err := os.WriteFile(settingsPath, output, 0o644); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}

	return nil
}

// angryBearHookExists checks if any hook entry in the PreToolUse array already
// contains the angry-bear hook command. Used for idempotency.
func angryBearHookExists(preToolUse []any) bool {
	for _, entry := range preToolUse {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := entryMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hooksList {
			hookMap, ok := hook.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hookMap["command"].(string); ok {
				if strings.Contains(cmd, "angry-bear hook") {
					return true
				}
			}
		}
	}
	return false
}

// UninstallHook removes all angry-bear hooks from Claude Code's global settings.
func (a *ClaudeAdapter) UninstallHook() error {
	settingsPath := a.GlobalConfigPath()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading settings.json: %w", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parsing settings.json: %w", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}

	preToolUse, ok := hooks["PreToolUse"].([]any)
	if !ok {
		return nil
	}

	// Filter out entries containing angry-bear hook commands
	var filtered []any
	for _, entry := range preToolUse {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			filtered = append(filtered, entry)
			continue
		}
		hooksList, ok := entryMap["hooks"].([]any)
		if !ok {
			filtered = append(filtered, entry)
			continue
		}
		hasCareBare := false
		for _, hook := range hooksList {
			hookMap, ok := hook.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hookMap["command"].(string); ok && strings.Contains(cmd, "angry-bear hook") {
				hasCareBare = true
				break
			}
		}
		if !hasCareBare {
			filtered = append(filtered, entry)
		}
	}

	hooks["PreToolUse"] = filtered
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings.json: %w", err)
	}
	output = append(output, '\n')
	return os.WriteFile(settingsPath, output, 0o644)
}

// DetectSkillInvocation checks if the parsed input represents a Skill tool invocation.
// If tool_name is "Skill", it extracts the skill name from tool_input.skill.
// Returns the skill name and true if found, or ("", false) otherwise.
func (a *ClaudeAdapter) DetectSkillInvocation(input *HookInput) (string, bool) {
	if input.ToolName != "Skill" {
		return "", false
	}

	// Extract skill name from RawInput -> tool_input -> skill
	toolInput, ok := input.RawInput["tool_input"].(map[string]any)
	if !ok {
		return "", false
	}
	skillName, ok := toolInput["skill"].(string)
	if !ok {
		return "", false
	}
	return skillName, true
}

// ScanProjects discovers all projects with Claude Code sessions by scanning
// ~/.claude/projects/. Each subdirectory is an encoded project path.
func (a *ClaudeAdapter) ScanProjects() ([]AgentProject, error) {
	home := a.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	return scanAgentProjectDir(projectsDir, "claude")
}

// scanAgentProjectDir scans an agent's projects directory and discovers
// project paths. First tries sessions-index.json for the real path,
// then falls back to greedy path decoding from the directory name.
func scanAgentProjectDir(dir, agent string) ([]AgentProject, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	seen := make(map[string]bool)
	var projects []AgentProject

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		// Strategy 1: Read sessions-index.json for the real path
		indexPath := filepath.Join(dir, e.Name(), "sessions-index.json")
		projectPath := readProjectPathFromIndex(indexPath)

		// Strategy 2: Greedy decode from directory name
		if projectPath == "" {
			projectPath = greedyDecodeDirName(e.Name())
		}

		if projectPath == "" {
			continue
		}

		// Deduplicate and verify path exists
		if seen[projectPath] {
			continue
		}
		if _, err := os.Stat(projectPath); err != nil {
			continue
		}
		seen[projectPath] = true

		projects = append(projects, AgentProject{
			Name:    filepath.Base(projectPath),
			Path:    projectPath,
			Agent:   agent,
			Encoded: e.Name(),
		})
	}
	return projects, nil
}

// greedyDecodeDirName decodes an encoded directory name to a real path.
// Handles both leading-dash (Claude: -Users-...) and no-dash (Cursor: Users-...) formats.
func greedyDecodeDirName(encoded string) string {
	// Simple decode first
	simple := "/" + strings.ReplaceAll(strings.TrimPrefix(encoded, "-"), "-", "/")
	if _, err := os.Stat(simple); err == nil {
		return simple
	}

	// Greedy: split into parts, try to match real dirs
	raw := strings.TrimPrefix(encoded, "-")
	parts := strings.Split(raw, "-")
	return greedyBuildPath("/", parts)
}

// greedyBuildPath recursively tries to build a real path from dash-separated parts.
func greedyBuildPath(prefix string, parts []string) string {
	if len(parts) == 0 {
		if _, err := os.Stat(prefix); err == nil {
			return prefix
		}
		return ""
	}
	// Try longest component first (more dashes joined = more specific)
	for i := len(parts); i >= 1; i-- {
		component := strings.Join(parts[:i], "-")
		// Try as-is, then with dots, then with underscores replacing dashes.
		// Dots are needed for usernames like "amir.shaked" which encode as "amir-shaked".
		for _, variant := range []string{
			component,
			strings.ReplaceAll(component, "-", "."),
			strings.ReplaceAll(component, "-", "_"),
		} {
			candidate := filepath.Join(prefix, variant)
			if i == len(parts) {
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			} else if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				if result := greedyBuildPath(candidate, parts[i:]); result != "" {
					return result
				}
			}
		}
	}
	return ""
}

// readProjectPathFromIndex reads the projectPath from sessions-index.json.
// Returns empty string if the file doesn't exist or can't be parsed.
func readProjectPathFromIndex(indexPath string) string {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return ""
	}

	var index struct {
		Entries []struct {
			ProjectPath string `json:"projectPath"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return ""
	}

	// Return the first non-empty projectPath
	for _, entry := range index.Entries {
		if entry.ProjectPath != "" {
			return entry.ProjectPath
		}
	}
	return ""
}
