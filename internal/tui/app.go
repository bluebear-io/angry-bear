// app.go defines the root Bubble Tea model for the care-bare TUI.
// It manages view transitions between the dashboard, rule editor, and tree picker,
// and holds shared state (config, skills, terminal dimensions).
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/scanner"
	"github.com/Blue-Bear-Security/care-bare/internal/state"
)

// loadedSkillsUpdatedMsg is pushed when the state directory changes.
type loadedSkillsUpdatedMsg struct {
	loaded map[string]*SkillStatus
}

// viewState identifies which view is currently active in the TUI.
type viewState int

const (
	viewDashboard  viewState = iota // Main skills+rules dashboard
	viewRuleEditor                  // Huh form for add/edit rule
	viewTreePicker                  // File browser for path selection
)

// --- Messages for inter-view communication ---

// openRuleEditorMsg is sent by the dashboard to open the rule editor.
type openRuleEditorMsg struct {
	skillName string
	ruleIndex int          // Index in config.Tools; -1 for new rule
	existing  *engine.Rule // nil for new rule
}

// ruleEditorDoneMsg is sent by the rule editor when the user submits or cancels.
type ruleEditorDoneMsg struct {
	rule      *engine.Rule // nil if cancelled
	ruleIndex int          // -1 if new rule
}

// openTreePickerMsg is sent by the rule editor to open the tree picker.
type openTreePickerMsg struct{}

// treePickerDoneMsg is sent by the tree picker when the user selects or cancels.
type treePickerDoneMsg struct {
	pattern string // Empty if cancelled
}

// saveResultMsg is sent after a config save attempt.
type saveResultMsg struct {
	err error
}

// App is the root Bubble Tea model that manages the TUI lifecycle.
type App struct {
	config       engine.Config    // Currently loaded enforcement config (mutable)
	configPath   string           // Path to the config file for saving
	stateDir     string           // Path to .care-bare/state/ for watching
	skills       []scanner.Skill  // Discovered skills from the scanner
	loadedSkills map[string]*SkillStatus // Skills loaded in active sessions, with agent info
	view         viewState        // Current active view
	dashboard    Dashboard        // Dashboard child model
	ruleEditor   RuleEditor       // Rule editor child model
	treePicker   TreePicker       // Tree picker child model
	statusMsg    string           // Transient status message ("Saved!", "Error: ...")
	width        int              // Terminal width
	height       int              // Terminal height
	styles       Styles           // Style definitions
}

// NewApp creates a new TUI application model with the given config, config path,
// discovered skills, and currently loaded skills from session state.
func NewApp(cfg engine.Config, configPath string, skills []scanner.Skill, loadedSkills map[string]*SkillStatus) App {
	styles := DefaultStyles()
	if loadedSkills == nil {
		loadedSkills = make(map[string]*SkillStatus)
	}
	stateDir := ""
	if configPath != "" {
		stateDir = filepath.Join(filepath.Dir(configPath), "state")
	}
	dashboard := NewDashboard(skills, cfg, styles, loadedSkills)
	return App{
		config:       cfg,
		configPath:   configPath,
		stateDir:     stateDir,
		skills:       skills,
		loadedSkills: loadedSkills,
		view:         viewDashboard,
		dashboard:    dashboard,
		styles:       styles,
	}
}

// Init returns the initial command — starts watching the state directory.
func (a App) Init() tea.Cmd {
	if a.stateDir != "" {
		return watchStateDir(a.stateDir)
	}
	return nil
}

// watchStateDir starts an fsnotify watcher on the state directory and sends
// loadedSkillsUpdatedMsg whenever state files change.
func watchStateDir(stateDir string) tea.Cmd {
	return func() tea.Msg {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}

		if err := watcher.Add(stateDir); err != nil {
			watcher.Close()
			return nil
		}

		// Wait for any file event
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 &&
					strings.HasSuffix(event.Name, ".json") {
					watcher.Close()
					return loadedSkillsUpdatedMsg{loaded: readLoadedSkills(stateDir)}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
}

// SkillStatus tracks which agents have loaded a skill.
type SkillStatus struct {
	Agents []string // e.g. ["claude", "cursor"]
}

// readLoadedSkills reads all session state files and returns loaded skills
// with their agent information.
func readLoadedSkills(stateDir string) map[string]*SkillStatus {
	loaded := make(map[string]*SkillStatus)
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return loaded
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(stateDir, e.Name()))
		if err != nil {
			continue
		}
		var ss state.SessionState
		if err := json.Unmarshal(data, &ss); err != nil {
			continue
		}
		agent := ss.Agent
		if agent == "" {
			agent = "unknown"
		}
		for _, skill := range ss.InvokedSkills {
			if loaded[skill] == nil {
				loaded[skill] = &SkillStatus{}
			}
			// Add agent if not already in list
			found := false
			for _, a := range loaded[skill].Agents {
				if a == agent {
					found = true
					break
				}
			}
			if !found {
				loaded[skill].Agents = append(loaded[skill].Agents, agent)
			}
		}
	}
	return loaded
}

// Update handles messages and routes them to the active view's child model.
// View transitions are triggered by custom message types.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedSkillsUpdatedMsg:
		// State files changed — update loaded skills and restart watcher
		a.loadedSkills = msg.loaded
		a.dashboard.loadedSkills = msg.loaded
		return a, watchStateDir(a.stateDir)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Propagate to child models.
		a.dashboard.width = msg.Width
		a.dashboard.height = msg.Height
		return a, nil

	case tea.KeyMsg:
		// ctrl+c always quits, from any view.
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		// Clear status message on any keypress.
		a.statusMsg = ""

	case openRuleEditorMsg:
		a.view = viewRuleEditor
		a.ruleEditor = NewRuleEditor(msg.skillName, msg.existing, msg.ruleIndex, a.styles)
		a.ruleEditor.SetExistingRules(a.config.Tools)
		// Set project root for path discovery (two levels up from .care-bare/skill_enforcement.json)
		if a.configPath != "" {
			a.ruleEditor.SetProjectRoot(filepath.Dir(filepath.Dir(a.configPath)))
		}
		return a, a.ruleEditor.Init()

	case ruleSubmittedMsg:
		// Single rule submitted (edit mode)
		if msg.rule != nil {
			if msg.ruleIndex >= 0 && msg.ruleIndex < len(a.config.Tools) {
				a.config.Tools[msg.ruleIndex] = *msg.rule
			} else {
				a.config.Tools = append(a.config.Tools, *msg.rule)
			}
			a.ruleEditor.SetExistingRules(a.config.Tools)
			return a, a.ruleEditor.SwitchToConfirm()
		}
		return a, nil

	case rulesSubmittedMsg:
		// Multiple rules submitted — add to config AND save to disk immediately
		for _, rule := range msg.rules {
			r := rule
			a.config.Tools = append(a.config.Tools, r)
		}
		a.ruleEditor.SetExistingRules(a.config.Tools)
		// Save to disk right away, then switch to confirm
		confirmCmd := a.ruleEditor.SwitchToConfirm()
		return a, tea.Batch(saveConfig(a.config, a.configPath), confirmCmd)

	case ruleEditorDoneMsg:
		// Editor is done (cancel or finished adding rules) — return to dashboard
		a.view = viewDashboard
		a.dashboard = NewDashboard(a.skills, a.config, a.styles, a.loadedSkills)
		a.dashboard.width = a.width
		a.dashboard.height = a.height
		a.statusMsg = "Rules updated (press s to save)"
		return a, nil

	case saveRequestMsg:
		// Save current config to disk.
		a.dashboard.config = a.config
		return a, saveConfig(a.config, a.configPath)

	case openTreePickerMsg:
		a.view = viewTreePicker
		root := "."
		if a.configPath != "" {
			// Use the directory two levels up from configPath (skip .care-bare/).
			root = filepath.Dir(filepath.Dir(a.configPath))
		}
		a.treePicker = NewTreePicker(root, a.styles)
		return a, a.treePicker.Init()

	case treePickerDoneMsg:
		a.view = viewRuleEditor
		if msg.pattern != "" {
			a.ruleEditor.SetPath(msg.pattern)
		}
		return a, nil

	case saveResultMsg:
		if msg.err != nil {
			a.statusMsg = fmt.Sprintf("Error saving: %v", msg.err)
		} else {
			a.statusMsg = "Saved!"
		}
		return a, nil
	}

	// Route to active view.
	var cmd tea.Cmd
	switch a.view {
	case viewDashboard:
		var newDashboard tea.Model
		newDashboard, cmd = a.dashboard.Update(msg)
		a.dashboard = newDashboard.(Dashboard)
		// Sync config back (dashboard may have deleted rules)
		a.config = a.dashboard.config
	case viewRuleEditor:
		var newEditor tea.Model
		newEditor, cmd = a.ruleEditor.Update(msg)
		a.ruleEditor = newEditor.(RuleEditor)
	case viewTreePicker:
		var newPicker tea.Model
		newPicker, cmd = a.treePicker.Update(msg)
		a.treePicker = newPicker.(TreePicker)
	}

	return a, cmd
}

// View renders the current state of the active view plus a persistent help bar.
func (a App) View() string {
	var content string

	title := a.styles.Header.Render("care-bare - Skill Enforcement Manager")

	switch a.view {
	case viewDashboard:
		content = a.dashboard.View()
	case viewRuleEditor:
		content = a.ruleEditor.View()
	case viewTreePicker:
		content = a.treePicker.View()
	}

	status := ""
	if a.statusMsg != "" {
		status = "\n" + a.styles.Success.Render(a.statusMsg)
	}

	help := a.helpBar()

	return title + "\n" + content + status + "\n" + help
}

// helpBar returns context-sensitive keybinding hints for the current view.
// Keys are highlighted to make them easy to spot.
func (a App) helpBar() string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	sepStyle := a.styles.Divider
	descStyle := a.styles.Help

	key := func(k, desc string) string {
		return keyStyle.Render(k) + descStyle.Render(" "+desc)
	}
	sep := sepStyle.Render(" | ")

	var text string
	switch a.view {
	case viewDashboard:
		if a.dashboard.focusPanel == 0 {
			text = key("↑↓", "navigate") + sep + key("→", "rules panel") + sep +
				key("enter/a", "add rules") + sep + key("s", "save") + sep + key("q", "quit")
		} else {
			text = key("↑↓", "navigate") + sep + key("←", "skills panel") + sep +
				key("t", "tool") + sep + key("p", "path") + sep + key("g", "agent") + sep +
				key("y", "dup") + sep + key("d", "del") + sep + key("s", "save") + sep + key("q", "quit")
		}
	case viewRuleEditor:
		text = "" // huh provides its own help
	case viewTreePicker:
		text = key("j/k", "navigate") + sep + key("enter", "select/open") + sep +
			key("backspace", "up dir") + sep + key("esc", "cancel")
	}
	return "\n" + text
}

// saveConfig writes the current config to disk as indented JSON.
func saveConfig(cfg engine.Config, path string) tea.Cmd {
	return func() tea.Msg {
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return saveResultMsg{err: err}
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return saveResultMsg{err: err}
		}
		return saveResultMsg{err: nil}
	}
}
