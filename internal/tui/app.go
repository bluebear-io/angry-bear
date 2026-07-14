// app.go defines the root Bubble Tea model for the angry-bear TUI.
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

	"github.com/bluebear-io/angry-bear/internal/engine"
	"github.com/bluebear-io/angry-bear/internal/scanner"
	"github.com/bluebear-io/angry-bear/internal/state"
)

// loadedSkillsUpdatedMsg is pushed when the state directory changes.
type loadedSkillsUpdatedMsg struct {
	loaded map[string]*state.SkillStatus
}

// eventsUpdatedMsg is pushed when events.log changes.
type eventsUpdatedMsg struct{}

// switchProjectMsg triggers returning to the project picker.
type switchProjectMsg struct{}

// viewState identifies which view is currently active in the TUI.
type viewState int

const (
	viewDashboard  viewState = iota // Main skills+rules dashboard
	viewRuleEditor                  // Huh form for add/edit rule
	viewTreePicker                  // File browser for path selection
	viewSettings                    // Config.json settings editor
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

// hookHealthUpdatedMsg is sent when agent config files change.
type hookHealthUpdatedMsg struct {
	health map[string]bool
}

// openSettingsMsg triggers the settings view.'

type openSettingsMsg struct{}

// saveResultMsg is sent after a config save attempt.
type saveResultMsg struct {
	err error
}

// App is the root Bubble Tea model that manages the TUI lifecycle.
type App struct {
	config          engine.Config                 // Currently loaded enforcement config (mutable)
	globalConfig    *engine.GlobalConfig          // Global config (skill_ttl, state_ttl, etc.)
	configPath      string                        // Path to the config file for saving
	projectRoot     string                        // Actual project root (for path tree in rule editor)
	repoConfigDir   string                        // Path to ~/.angry-bear/repos/{hash}-{slug}/ (empty if no repo)
	availablePaths  []string                      // All local checkout paths for this repo
	stateDir        string                        // Path to .angry-bear/state/ for watching
	skills          []scanner.Skill               // Discovered skills from the scanner
	loadedSkills    map[string]*state.SkillStatus // Skills loaded in active sessions, with agent info
	switchRequested bool
	hookHealth      map[string]bool        // agent name → hook installed
	hookHealthFn    func() map[string]bool // provided by CLI layer (uses adapter registry)                          // True when user pressed P to switch projects
	view            viewState              // Current active view
	dashboard       Dashboard              // Dashboard child model
	ruleEditor      RuleEditor             // Rule editor child model
	treePicker      TreePicker             // Tree picker child model
	settings        Settings               // Settings editor child model
	statusMsg       string                 // Transient status message ("Saved!", "Error: ...")
	savePrompt      bool                   // True when showing save destination prompt
	width           int                    // Terminal width
	height          int                    // Terminal height
	styles          Styles                 // Style definitions
}

// SwitchRequested returns true if the user requested switching projects.
func (a App) SwitchRequested() bool {
	return a.switchRequested
}

// SetHookHealthFn sets the function used to check hook health.
// Called by the CLI layer to inject adapter-registry-based health checks.
func (a *App) SetHookHealthFn(fn func() map[string]bool) {
	a.hookHealthFn = fn
	a.hookHealth = fn()
}

// NewApp creates a new TUI application model with the given config, config path,
// discovered skills, and currently loaded skills from session state.
// repoConfigDir is the path to ~/.angry-bear/repos/{hash}-{slug}/ (may be empty).
// availablePaths lists all local checkout directories for this repo (may be nil).
func NewApp(
	cfg engine.Config,
	ruleSources []string,
	configPath string,
	projectRoot string,
	skills []scanner.Skill,
	loadedSkills map[string]*state.SkillStatus,
	globalCfg *engine.GlobalConfig,
	repoConfigDir string,
	availablePaths []string,
) App {
	styles := DefaultStyles()
	if loadedSkills == nil {
		loadedSkills = make(map[string]*state.SkillStatus)
	}
	if globalCfg == nil {
		globalCfg = &engine.GlobalConfig{
			SkillPaths:    []string{".claude/skills"},
			StateTTLHours: 24,
			DefaultAgent:  "*",
		}
	}
	stateDir := ""
	if repoConfigDir != "" {
		stateDir = filepath.Join(repoConfigDir, "state")
	} else if configPath != "" {
		stateDir = filepath.Join(filepath.Dir(configPath), "state")
	}
	dashboard := NewDashboard(skills, cfg, styles, loadedSkills)
	dashboard.ruleSources = ruleSources
	// hookHealthFn is set after creation by the CLI layer
	return App{
		config:         cfg,
		globalConfig:   globalCfg,
		configPath:     configPath,
		projectRoot:    projectRoot,
		repoConfigDir:  repoConfigDir,
		availablePaths: availablePaths,
		stateDir:       stateDir,
		skills:         skills,
		loadedSkills:   loadedSkills,
		view:           viewDashboard,
		dashboard:      dashboard,
		styles:         styles,
		hookHealth:     make(map[string]bool),
	}
}

// LoadEvents loads the event log from disk. Must be called before Init()
// because Init() runs on a value receiver and mutations don't persist.
func (a *App) LoadEvents(projectRoot string) {
	a.dashboard.LoadEventLog(projectRoot)
}

// Init returns the initial command — starts watchers.
// because Init runs on a value receiver and mutations don't persist.
func (a App) Init() tea.Cmd {
	var cmds []tea.Cmd
	// Watch agent config files for hook health changes
	if a.hookHealthFn != nil {
		cmds = append(cmds, watchAgentConfigs(a.hookHealthFn))
	}
	if a.stateDir != "" {
		cmds = append(cmds, watchStateDir(a.stateDir))
		// Watch global events.log for real-time updates
		home, _ := os.UserHomeDir()
		eventsLog := filepath.Join(home, ".angry-bear", "events.log")
		cmds = append(cmds, watchEventsLog(eventsLog))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
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
					return loadedSkillsUpdatedMsg{loaded: state.CollectLoadedSkills(stateDir)}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
}

// watchEventsLog watches the events.log file and sends eventsUpdatedMsg on changes.
func watchEventsLog(logPath string) tea.Cmd {
	return func() tea.Msg {
		// Watch the parent directory since the file may not exist yet
		dir := filepath.Dir(logPath)
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}
		if err := watcher.Add(dir); err != nil {
			watcher.Close()
			return nil
		}
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 &&
					strings.HasSuffix(event.Name, "events.log") {
					watcher.Close()
					return eventsUpdatedMsg{}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
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

	case hookHealthUpdatedMsg:
		a.hookHealth = msg.health
		return a, watchAgentConfigs(a.hookHealthFn)

	case eventsUpdatedMsg:
		// Events log changed — reload, auto-scroll to latest, restart watcher
		a.dashboard.LoadEventLog("")
		// Auto-scroll to the newest event
		if len(a.dashboard.eventLines) > 0 {
			a.dashboard.logScroll.Cursor = len(a.dashboard.eventLines) - 1
		}
		home, _ := os.UserHomeDir()
		eventsLog := filepath.Join(home, ".angry-bear", "events.log")
		return a, watchEventsLog(eventsLog)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Propagate to child models.
		a.dashboard.width = msg.Width
		a.dashboard.height = msg.Height
		return a, nil

	case switchProjectMsg:
		a.switchRequested = true
		return a, tea.Quit

	case tea.KeyMsg:
		// ctrl+c always quits, from any view.
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		// Handle save destination prompt.
		if a.savePrompt {
			switch msg.String() {
			case "r":
				a.savePrompt = false
				a.statusMsg = ""
				return a, func() tea.Msg { return saveToRepoMsg{} }
			case "m":
				a.savePrompt = false
				a.statusMsg = ""
				return a, func() tea.Msg { return saveToMachineMsg{} }
			case "esc":
				a.savePrompt = false
				a.statusMsg = ""
				return a, nil
			}
			return a, nil
		}
		// Clear status message on any keypress.
		a.statusMsg = ""

	case openRuleEditorMsg:
		a.view = viewRuleEditor
		a.ruleEditor = NewRuleEditor(msg.skillName, msg.existing, msg.ruleIndex, a.styles)
		a.ruleEditor.height = a.height
		a.ruleEditor.SetExistingRules(a.config.Tools)
		if a.projectRoot != "" {
			a.ruleEditor.SetProjectRoot(a.projectRoot)
		}
		return a, a.ruleEditor.Init()

	case ruleSubmittedMsg:
		// Single rule submitted (edit mode) — update config and prompt to save.
		if msg.rule != nil {
			if msg.ruleIndex >= 0 && msg.ruleIndex < len(a.config.Tools) {
				a.config.Tools[msg.ruleIndex] = *msg.rule
			} else {
				a.config.Tools = append(a.config.Tools, *msg.rule)
			}
			a.view = viewDashboard
			a.dashboard = NewDashboard(a.skills, a.config, a.styles, a.loadedSkills)
			a.dashboard.LoadEventLog("")
			a.dashboard.width = a.width
			a.dashboard.height = a.height
			a.savePrompt = true
			a.statusMsg = "Save to: [r]epo (shared via git) or [m]achine (local only)?"
			return a, nil
		}
		return a, nil

	case rulesSubmittedMsg:
		// Multiple rules submitted — replace existing rules for this skill, then prompt to save.
		if len(msg.rules) > 0 {
			skillName := msg.rules[0].Skill
			var kept []engine.Rule
			for _, r := range a.config.Tools {
				if r.Skill != skillName {
					kept = append(kept, r)
				}
			}
			a.config.Tools = append(kept, msg.rules...)
		}
		a.view = viewDashboard
		a.dashboard = NewDashboard(a.skills, a.config, a.styles, a.loadedSkills)
		a.dashboard.LoadEventLog("")
		a.dashboard.width = a.width
		a.dashboard.height = a.height
		a.savePrompt = true
		a.statusMsg = "Save to: [r]epo (shared via git) or [m]achine (local only)?"
		return a, nil

	case ruleEditorDoneMsg:
		// Editor is done (cancel or finished adding rules) — return to dashboard
		a.view = viewDashboard
		a.dashboard = NewDashboard(a.skills, a.config, a.styles, a.loadedSkills)
		a.dashboard.LoadEventLog("")
		a.dashboard.width = a.width
		a.dashboard.height = a.height
		a.statusMsg = "Rules updated (press s to save)"
		return a, nil

	case saveRequestMsg:
		// Show save destination prompt.
		a.savePrompt = true
		a.statusMsg = "Save to: [r]epo (shared via git) or [m]achine (local only)?"
		return a, nil

	case saveToRepoMsg:
		a.savePrompt = false
		repoPath := filepath.Join(a.projectRoot, ".angry-bear", "skill_enforcement.json")
		a.dashboard.config = a.config
		// Mark all rules as repo-sourced.
		a.dashboard.ruleSources = make([]string, len(a.config.Tools))
		for i := range a.dashboard.ruleSources {
			a.dashboard.ruleSources[i] = engine.SourceRepo
		}
		return a, saveConfig(a.config, repoPath)

	case saveToMachineMsg:
		a.savePrompt = false
		a.dashboard.config = a.config
		// Mark all rules as machine-sourced.
		a.dashboard.ruleSources = make([]string, len(a.config.Tools))
		for i := range a.dashboard.ruleSources {
			a.dashboard.ruleSources[i] = engine.SourceMachine
		}
		return a, saveConfig(a.config, a.configPath)

	case openTreePickerMsg:
		a.view = viewTreePicker
		root := "."
		if a.projectRoot != "" {
			root = a.projectRoot
		}
		a.treePicker = NewTreePicker(root, a.styles)
		return a, a.treePicker.Init()

	case treePickerDoneMsg:
		a.view = viewRuleEditor
		return a, nil

	case openSettingsMsg:
		a.view = viewSettings
		a.settings = NewSettings(a.globalConfig, a.styles, a.projectRoot, a.availablePaths)
		a.settings.width = a.width
		a.settings.height = a.height
		return a, a.settings.Init()

	case settingsDoneMsg:
		a.view = viewDashboard
		var cmds []tea.Cmd

		// Handle preferred path change.
		if msg.preferredPath != "" && msg.preferredPath != a.projectRoot && a.repoConfigDir != "" {
			cmds = append(cmds, savePreferredPath(a.repoConfigDir, msg.preferredPath))
		}

		if msg.config != nil {
			// Preserve fields not editable in settings
			msg.config.SkillPaths = a.globalConfig.SkillPaths
			msg.config.IgnorePatterns = a.globalConfig.IgnorePatterns
			a.globalConfig = msg.config
			cmds = append(cmds, saveGlobalConfig(a.globalConfig, msg.configLevel, a.configPath))
		}

		if len(cmds) == 0 {
			return a, nil
		}
		return a, tea.Batch(cmds...)

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
	case viewSettings:
		var newSettings tea.Model
		newSettings, cmd = a.settings.Update(msg)
		a.settings = newSettings.(Settings)
	}

	return a, cmd
}

// View renders the current state of the active view plus a persistent help bar.
func (a App) View() string {
	var content string

	// Show project name and path in header
	projectLabel := ""
	if a.projectRoot != "" {
		name := filepath.Base(a.projectRoot)
		projectLabel = "  " + a.styles.Description.Render(name+" — "+a.projectRoot)
	}
	// Hook health badges per agent
	hookBadges := a.renderHookBadges()
	title := a.styles.Header.Render("angry-bear") + projectLabel + "  " + hookBadges

	switch a.view {
	case viewDashboard:
		content = a.dashboard.View()
	case viewRuleEditor:
		content = a.ruleEditor.View()
	case viewTreePicker:
		content = a.treePicker.View()
	case viewSettings:
		content = a.settings.View()
	}

	status := ""
	if a.statusMsg != "" && !a.savePrompt {
		status = "\n" + a.styles.Success.Render(a.statusMsg)
	}

	help := a.helpBar()
	result := title + "\n" + content + status + "\n" + help

	// Overlay save dialog on top of the screen.
	if a.savePrompt {
		result = a.renderSaveDialog(result)
	}

	return result
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
		switch a.dashboard.focusPanel {
		case 0:
			text = key("↑↓", "navigate") + sep + key("tab/→", "rules") + sep +
				key("enter/a", "add rules") + sep + key("c", "settings") + sep + key("s", "save") + sep + key("P", "switch project") + sep + key("q", "quit")
		case 1:
			text = key("↑↓", "navigate") + sep + key("tab", "logs") + sep + key("←", "skills") + sep +
				key("t", "tool") + sep + key("p", "path") + sep + key("g", "agent") + sep +
				key("d", "del") + sep + key("c", "settings") + sep + key("s", "save") + sep + key("P", "switch project") + sep + key("q", "quit")
		case 2:
			text = key("↑↓", "scroll") + sep + key("PgUp/Dn", "page") + sep + key("Home/End", "top/bottom") + sep +
				key("f", "filter") + sep + key("F", "clear filters") + sep + key("K", "clear logs") + sep + key("enter", "jump") + sep +
				key("c", "settings") + sep + key("s", "save") + sep + key("P", "switch project") + sep + key("q", "quit")
		}
	case viewRuleEditor:
		text = "" // huh provides its own help
	case viewTreePicker:
		text = key("j/k", "navigate") + sep + key("enter", "select/open") + sep +
			key("backspace", "up dir") + sep + key("esc", "cancel")
	case viewSettings:
		text = key("↑↓", "navigate") + sep + key("←→", "cycle") + sep + key("enter", "edit") + sep +
			key("g", "global") + sep + key("p", "project") + sep + key("esc/q", "save & exit")
	}
	return "\n" + text
}

// saveGlobalConfig writes the global config (config.json) to disk.
// When level is "global", it writes to ~/.angry-bear/config.json.
// When level is "project", it writes alongside the enforcement config file.
func saveGlobalConfig(cfg *engine.GlobalConfig, level string, enforcementConfigPath string) tea.Cmd {
	return func() tea.Msg {
		var configPath string
		if level == "global" {
			home, err := os.UserHomeDir()
			if err != nil {
				return saveResultMsg{err: fmt.Errorf("getting home dir: %w", err)}
			}
			dir := filepath.Join(home, ".angry-bear")
			err = os.MkdirAll(dir, 0o755)
			if err != nil {
				return saveResultMsg{err: fmt.Errorf("creating global config dir: %w", err)}
			}
			configPath = filepath.Join(dir, "config.json")
		} else {
			// Project level: config.json sits alongside skill_enforcement.json
			dir := filepath.Dir(enforcementConfigPath)
			configPath = filepath.Join(dir, "config.json")
		}

		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return saveResultMsg{err: err}
		}
		err = os.WriteFile(configPath, data, 0o644)
		if err != nil {
			return saveResultMsg{err: err}
		}
		return saveResultMsg{err: nil}
	}
}

// savePreferredPath writes the preferred checkout path to preferences.json.
func savePreferredPath(repoConfigDir string, path string) tea.Cmd {
	return func() tea.Msg {
		prefs := &engine.RepoPreferences{PreferredPath: path}
		err := engine.SaveRepoPreferences(repoConfigDir, prefs)
		if err != nil {
			return saveResultMsg{err: fmt.Errorf("saving preferred path: %w", err)}
		}
		return saveResultMsg{err: nil}
	}
}

// saveConfig writes the current config to disk as indented JSON.
// Deduplicates rules before writing to prevent duplicate entries.
func saveConfig(cfg engine.Config, path string) tea.Cmd {
	return func() tea.Msg {
		engine.DeduplicateRules(&cfg)
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

// renderSaveDialog overlays a centered dialog box on top of the existing screen content.
func (a App) renderSaveDialog(background string) string {
	lines := strings.Split(background, "\n")

	dialogWidth := 48
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FBBF24")).
		Padding(1, 2).
		Width(dialogWidth)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24"))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#34D399"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))

	dialogContent := titleStyle.Render("Save rules to:") + "\n\n" +
		"  " + keyStyle.Render("r") + descStyle.Render("  Repo — shared via git (.angry-bear/)") + "\n" +
		"  " + keyStyle.Render("m") + descStyle.Render("  Machine — local only (~/.angry-bear/)") + "\n\n" +
		"  " + keyStyle.Render("esc") + descStyle.Render("  Cancel")

	box := boxStyle.Render(dialogContent)
	boxLines := strings.Split(box, "\n")

	// Center the dialog vertically and horizontally
	startRow := (len(lines) - len(boxLines)) / 2
	if startRow < 2 {
		startRow = 2
	}
	startCol := (a.width - lipgloss.Width(box)) / 2
	if startCol < 0 {
		startCol = 0
	}

	pad := strings.Repeat(" ", startCol)
	for i, bLine := range boxLines {
		row := startRow + i
		if row < len(lines) {
			lines[row] = pad + bLine
		}
	}

	return strings.Join(lines, "\n")
}

// renderHookBadges renders colored badges showing hook health per agent.
func (a App) renderHookBadges() string {
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#1F2937")).Background(lipgloss.Color("#34D399")).Padding(0, 1)
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#1F2937")).Background(lipgloss.Color("#EF4444")).Padding(0, 1)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

	// Refresh hook health if map is empty but the function is available.
	// This handles the case where Bubble Tea's value-receiver model copy
	// loses the initial hookHealth set by SetHookHealthFn.
	health := a.hookHealth
	if len(health) == 0 && a.hookHealthFn != nil {
		health = a.hookHealthFn()
	}

	var badges []string
	for agent, healthy := range health {
		if healthy {
			badges = append(badges, green.Render(agent+" ✓"))
		} else {
			badges = append(badges, red.Render(agent+" ✗"))
		}
	}

	if len(badges) == 0 {
		return dim.Render("no agents")
	}
	return strings.Join(badges, " ")
}

// watchAgentConfigs watches Claude and Cursor config files for changes
// and sends hookHealthUpdatedMsg when they're modified.
func watchAgentConfigs(healthFn func() map[string]bool) tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}

		// Watch the directories containing the config files
		paths := []string{
			filepath.Join(home, ".claude"),
			filepath.Join(home, ".cursor"),
		}
		for _, p := range paths {
			_ = watcher.Add(p)
		}

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					base := filepath.Base(event.Name)
					if base == "settings.json" || base == "hooks.json" {
						watcher.Close()
						return hookHealthUpdatedMsg{health: healthFn()}
					}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
}
