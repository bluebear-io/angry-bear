// constants.go defines shared constants and option lists used across the TUI.
// Centralizing these avoids duplication between rule_editor.go, tree_picker.go,
// and dashboard.go.
package tui

// DefaultIgnoreSet defines directory names hidden by default in file browsers
// and path tree pickers. Used by both rule_editor.go and tree_picker.go.
var DefaultIgnoreSet = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	".next":        true,
	"__pycache__":  true,
	".venv":        true,
	"build":        true,
	"target":       true,
	".care-bear":   true,
	"bin":          true,
}

// ToolOptions lists the available tool names for rule configuration.
var ToolOptions = []string{"Edit", "Write", "Bash", "Read", "Glob", "Grep", "Agent", "*"}

// AgentOptions lists the available agent names for rule configuration.
var AgentOptions = []string{"claude", "cursor", "*"}
