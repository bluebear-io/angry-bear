package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Blue-Bear-Security/care-bear/internal/engine"
	"github.com/Blue-Bear-Security/care-bear/internal/scanner"
	"github.com/Blue-Bear-Security/care-bear/internal/state"
)

func testSkills() []scanner.Skill {
	return []scanner.Skill{
		{Name: "go-coding", Description: "Go coding standards"},
		{Name: "linear", Description: "Manage Linear tickets"},
		{Name: "testing", Description: "Testing strategy"},
	}
}

func testConfig() engine.Config {
	return engine.Config{
		Version: 1,
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "claude"},
			{Tool: "Write", Path: "**/*.ts", Skill: "linear", Agent: "*"},
		},
	}
}

func TestNewAppInitialization(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	if len(app.config.Tools) != 2 {
		t.Errorf("expected 2 rules, got %d", len(app.config.Tools))
	}
	if app.view != viewDashboard {
		t.Errorf("expected dashboard view")
	}
}

func TestDashboardRendersSkillNames(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.width, app.height = 120, 40
	app.dashboard.width, app.dashboard.height = 120, 40
	output := app.View()
	for _, s := range testSkills() {
		if !strings.Contains(output, s.Name) {
			t.Errorf("missing skill %q in output", s.Name)
		}
	}
}

func TestDashboardNavigation(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = m.(App)
	if app.dashboard.skillScroll.Cursor != 1 {
		t.Errorf("expected cursor=1, got %d", app.dashboard.skillScroll.Cursor)
	}
}

func TestDashboardPanelSwitch(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = m.(App)
	if app.dashboard.focusPanel != 1 {
		t.Errorf("expected right panel after tab")
	}
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	app = m.(App)
	if app.dashboard.focusPanel != 0 {
		t.Errorf("expected left panel after shift+tab")
	}
}

func TestDashboardQuit(t *testing.T) {
	app := NewApp(engine.Config{Version: 1}, nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestDashboardSave(t *testing.T) {
	tmpFile := t.TempDir() + "/config.json"
	app := NewApp(testConfig(), nil, tmpFile, "/tmp", testSkills(), nil, nil, "", nil)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("expected command from 's'")
	}
	reqMsg := cmd()
	_, cmd = app.Update(reqMsg)
	if cmd == nil {
		t.Fatal("expected save command")
	}
	result := cmd().(saveResultMsg)
	if result.err != nil {
		t.Fatalf("save failed: %v", result.err)
	}
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if !strings.Contains(string(data), "go-coding") {
		t.Errorf("missing rules in saved file")
	}
}

func TestDashboardDeleteRule(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.dashboard.focusPanel = 1
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = m.(App)
	if len(app.config.Tools) != 1 {
		t.Errorf("expected 1 rule after delete, got %d", len(app.config.Tools))
	}
}

func TestRuleEditorCancel(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.view = viewRuleEditor
	m, _ := app.Update(ruleEditorDoneMsg{rule: nil, ruleIndex: -1})
	app = m.(App)
	if app.view != viewDashboard {
		t.Errorf("expected dashboard after cancel")
	}
}

func TestRuleEditorSubmit(t *testing.T) {
	app := NewApp(engine.Config{Version: 1}, nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.view = viewRuleEditor
	rule := engine.Rule{Tool: "Edit", Path: "**/*.go", Skill: "linear", Agent: "*"}
	m, _ := app.Update(rulesSubmittedMsg{rules: []engine.Rule{rule}})
	app = m.(App)
	if len(app.config.Tools) != 1 {
		t.Errorf("expected 1 rule, got %d", len(app.config.Tools))
	}
}

func TestLoadedSkillsShown(t *testing.T) {
	loaded := map[string]*state.SkillStatus{"linear": {Agents: []string{"claude"}}}
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), loaded, nil, "", nil)
	app.width, app.height = 120, 40
	app.dashboard.width, app.dashboard.height = 120, 40
	output := app.View()
	// Loaded skills should have the green dot indicator
	if !strings.Contains(output, "claude") {
		t.Errorf("expected loaded indicator in output:\n%s", output)
	}
}

func TestWindowSizeMsg(t *testing.T) {
	app := NewApp(engine.Config{Version: 1}, nil, "/tmp/test.json", "/tmp", nil, nil, nil, "", nil)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app = m.(App)
	if app.width != 120 || app.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", app.width, app.height)
	}
}

func TestHelpBarContent(t *testing.T) {
	app := NewApp(engine.Config{Version: 1}, nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.width, app.height = 120, 40
	app.dashboard.width, app.dashboard.height = 120, 40
	output := app.View()
	if !strings.Contains(output, "navigate") || !strings.Contains(output, "save") || !strings.Contains(output, "quit") {
		t.Errorf("help bar missing text:\n%s", output)
	}
}

// --- Three-panel navigation tests ---

func TestThreePanelTabCycle(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	// Start at panel 0 (skills)
	if app.dashboard.focusPanel != 0 {
		t.Fatalf("expected panel 0, got %d", app.dashboard.focusPanel)
	}

	// Tab → panel 1
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = m.(App)
	if app.dashboard.focusPanel != 1 {
		t.Errorf("after tab: expected panel 1, got %d", app.dashboard.focusPanel)
	}

	// Tab → panel 2
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = m.(App)
	if app.dashboard.focusPanel != 2 {
		t.Errorf("after tab: expected panel 2, got %d", app.dashboard.focusPanel)
	}

	// Tab → panel 0 (wrap)
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = m.(App)
	if app.dashboard.focusPanel != 0 {
		t.Errorf("after tab: expected panel 0 (wrap), got %d", app.dashboard.focusPanel)
	}
}

func TestShiftTabReverse(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	// Shift+Tab from panel 0 → panel 2
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	app = m.(App)
	if app.dashboard.focusPanel != 2 {
		t.Errorf("expected panel 2, got %d", app.dashboard.focusPanel)
	}
}

func TestLogPanelNavigation(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.dashboard.eventLines = []string{
		"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | linear",
		"2026-04-13T00:00:01Z | proj | claude | abc12 | SKILL-LOAD | | LOAD | linear",
		"2026-04-13T00:00:02Z | proj | claude | abc12 | Edit | test.go | ALLOW | linear",
	}
	app.dashboard.focusPanel = 2

	// Down
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = m.(App)
	if app.dashboard.logScroll.Cursor != 1 {
		t.Errorf("expected logCursor=1, got %d", app.dashboard.logScroll.Cursor)
	}

	// Down again
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = m.(App)
	if app.dashboard.logScroll.Cursor != 2 {
		t.Errorf("expected logCursor=2, got %d", app.dashboard.logScroll.Cursor)
	}

	// Up
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyUp})
	app = m.(App)
	if app.dashboard.logScroll.Cursor != 1 {
		t.Errorf("expected logCursor=1, got %d", app.dashboard.logScroll.Cursor)
	}
}

func TestMultiColumnFilter(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.dashboard.eventLines = []string{
		"2026-04-13T00:00:00Z | blueden | claude | abc12 | Edit | test.go | BLOCK | git",
		"2026-04-13T00:00:01Z | baloo   | cursor | def45 | Edit | main.go | BLOCK | review",
	}
	app.dashboard.focusPanel = 2

	// Press f — enters filter mode on ACTION column
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app = m.(App)
	if !app.dashboard.filterMode {
		t.Error("expected filter mode active")
	}
	if app.dashboard.filterCursor != filterAction {
		t.Errorf("expected cursor on ACTION, got %d", app.dashboard.filterCursor)
	}

	// Press f again — moves to PROJECT column
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app = m.(App)
	if app.dashboard.filterCursor != filterProject {
		t.Errorf("expected cursor on PROJECT, got %d", app.dashboard.filterCursor)
	}

	// Press down — cycles to first project value
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = m.(App)
	projectFilter := app.dashboard.logFilters[filterProject]
	if projectFilter == "" {
		t.Error("expected project filter to be set after down")
	}

	// Press F (shift) — clears all filters
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	app = m.(App)
	if len(app.dashboard.logFilters) != 0 {
		t.Errorf("expected empty filters after F, got %v", app.dashboard.logFilters)
	}
}

func TestJumpToLogEntry(t *testing.T) {
	skills := testSkills() // go-coding, linear, testing
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", skills, nil, nil, "", nil)
	app.dashboard.eventLines = []string{
		"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | linear",
	}
	app.dashboard.focusPanel = 2
	app.dashboard.logScroll.Cursor = 0

	// Press enter — should jump to "linear" skill
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = m.(App)

	if app.dashboard.focusPanel != 1 {
		t.Errorf("expected focusPanel=1 (rules), got %d", app.dashboard.focusPanel)
	}
	if app.dashboard.skillScroll.Cursor != 1 { // linear is second skill (go-coding, linear, testing)
		t.Errorf("expected skillCursor=1 (linear), got %d", app.dashboard.skillScroll.Cursor)
	}
}

func TestPadToHeight(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		height    int
		wantLines int
	}{
		{
			name:      "shorter content gets padded",
			content:   "line1\nline2",
			height:    5,
			wantLines: 5,
		},
		{
			name:      "exact height unchanged",
			content:   "line1\nline2\nline3",
			height:    3,
			wantLines: 3,
		},
		{
			name:      "longer content gets truncated",
			content:   "line1\nline2\nline3\nline4\nline5",
			height:    3,
			wantLines: 3,
		},
		{
			name:      "trailing newline handled",
			content:   "line1\nline2\n",
			height:    4,
			wantLines: 4,
		},
		{
			name:      "empty content padded",
			content:   "",
			height:    3,
			wantLines: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padToHeight(tt.content, tt.height)
			lines := strings.Split(result, "\n")
			if len(lines) != tt.wantLines {
				t.Errorf("padToHeight(%q, %d) = %d lines; want %d lines\nresult: %q",
					tt.content, tt.height, len(lines), tt.wantLines, result)
			}
		})
	}
}

func TestSwitchProjectKey(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)

	// Press P -- should trigger switchProjectMsg which sets switchRequested.
	m, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	if cmd == nil {
		t.Fatal("expected command from P")
	}
	// Execute the command to get the message, then update app with it.
	msg := cmd()
	m, _ = m.(App).Update(msg)
	if !m.(App).SwitchRequested() {
		t.Error("expected SwitchRequested() to be true after P")
	}
}

// --- Settings view tests ---

func TestSettingsOpenAndClose(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, cfg, "", nil)

	// Press c to open settings
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	app = m.(App)

	// Should receive openSettingsMsg and switch to settings view
	cmd := func() tea.Msg { return openSettingsMsg{} }
	m, _ = app.Update(cmd())
	app = m.(App)
	if app.view != viewSettings {
		t.Errorf("expected settings view, got %d", app.view)
	}

	// Press esc to exit settings
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = m.(App)
	// Should get settingsDoneMsg
	settingsCmd := func() tea.Msg {
		return settingsDoneMsg{config: cfg, configLevel: "project"}
	}
	m, _ = app.Update(settingsCmd())
	app = m.(App)
	if app.view != viewDashboard {
		t.Errorf("expected dashboard view after settings exit, got %d", app.view)
	}
}

func TestSettingsPreservesValues(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 30,
		StateTTLHours:   48,
		DefaultAgent:    "claude",
		SkillPaths:      []string{".claude/skills"},
		IgnorePatterns:  []string{".git"},
	}
	settings := NewSettings(cfg, DefaultStyles(), "/tmp", nil)

	// Build config from settings and verify values preserved
	result := settings.buildConfig()
	if result.SkillTTLMinutes != 30 {
		t.Errorf("SkillTTLMinutes = %d, want 30", result.SkillTTLMinutes)
	}
	if result.StateTTLHours != 48 {
		t.Errorf("StateTTLHours = %d, want 48", result.StateTTLHours)
	}
	if result.DefaultAgent != "claude" {
		t.Errorf("DefaultAgent = %q, want %q", result.DefaultAgent, "claude")
	}
}

func TestSettingsRendersContent(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	settings := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	output := settings.View()

	if !strings.Contains(output, "Skill TTL") {
		t.Error("settings view should show Skill TTL")
	}
	if !strings.Contains(output, "Session TTL") {
		t.Error("settings view should show Session TTL")
	}
	if !strings.Contains(output, "60") {
		t.Error("settings view should show value 60")
	}
}

// --- App View tests for each view state ---

func TestApp_ViewSettings(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, cfg, "", nil)
	app.width, app.height = 120, 40
	app.view = viewSettings
	app.settings = NewSettings(cfg, DefaultStyles(), "/tmp", nil)

	output := app.View()
	if !strings.Contains(output, "SETTINGS") {
		t.Error("expected SETTINGS in view when in settings mode")
	}
	if !strings.Contains(output, "care-bear") {
		t.Error("expected care-bear header in app view")
	}
}

func TestApp_ViewRuleEditor(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.width, app.height = 120, 40
	app.view = viewRuleEditor
	app.ruleEditor = RuleEditor{
		skillName: "test-skill",
		phase:     phaseEdit,
		styles:    DefaultStyles(),
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", label: "Edit"},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**", label: "** (all files)"},
		},
		agentItems: []listItem{
			{typ: itemCheckbox, value: "claude", label: "claude"},
		},
	}

	output := app.View()
	if !strings.Contains(output, "test-skill") {
		t.Error("expected skill name in rule editor view")
	}
}

func TestApp_ViewTreePicker(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(dir+"/test.go", []byte(""), 0o644)

	app := NewApp(testConfig(), nil, "/tmp/test.json", dir, testSkills(), nil, nil, "", nil)
	app.width, app.height = 120, 40
	app.view = viewTreePicker
	app.treePicker = NewTreePicker(dir, DefaultStyles())

	output := app.View()
	if !strings.Contains(output, "Select Path") {
		t.Error("expected Select Path header in tree picker view")
	}
}

// --- App: statusMsg display ---

func TestApp_StatusMsgDisplayed(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.width, app.height = 120, 40
	app.dashboard.width, app.dashboard.height = 120, 40
	app.statusMsg = "Saved!"

	output := app.View()
	if !strings.Contains(output, "Saved!") {
		t.Error("expected status message in view")
	}
}

// --- App: saveResultMsg handling ---

func TestApp_SaveResultSuccess(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	m, _ := app.Update(saveResultMsg{err: nil})
	app = m.(App)
	if app.statusMsg != "Saved!" {
		t.Errorf("statusMsg = %q, want %q", app.statusMsg, "Saved!")
	}
}

func TestApp_SaveResultError(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	m, _ := app.Update(saveResultMsg{err: os.ErrPermission})
	app = m.(App)
	if !strings.Contains(app.statusMsg, "Error") {
		t.Errorf("statusMsg = %q, want error message", app.statusMsg)
	}
}

// --- App: ctrl+c from any view ---

func TestApp_CtrlCQuits(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit command from ctrl+c")
	}
}

// --- App: key clears status message ---

func TestApp_KeyClearsStatusMsg(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.statusMsg = "Saved!"

	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = m.(App)
	if app.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty (should be cleared on keypress)", app.statusMsg)
	}
}

// --- App: helpBar for each panel and view ---

func TestApp_HelpBarRulesPanel(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.width, app.height = 120, 40
	app.dashboard.width, app.dashboard.height = 120, 40
	app.dashboard.focusPanel = 1

	output := app.View()
	if !strings.Contains(output, "tool") || !strings.Contains(output, "path") {
		t.Error("help bar should show rule editing keys when on rules panel")
	}
}

func TestApp_HelpBarLogPanel(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.width, app.height = 120, 40
	app.dashboard.width, app.dashboard.height = 120, 40
	app.dashboard.focusPanel = 2

	output := app.View()
	if !strings.Contains(output, "filter") {
		t.Error("help bar should show filter key when on log panel")
	}
}

func TestApp_HelpBarSettingsView(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, cfg, "", nil)
	app.width, app.height = 120, 40
	app.view = viewSettings
	app.settings = NewSettings(cfg, DefaultStyles(), "/tmp", nil)

	output := app.View()
	if !strings.Contains(output, "global") || !strings.Contains(output, "project") {
		t.Error("settings help bar should show g/p level keys")
	}
}

// --- App: openTreePickerMsg ---

func TestApp_OpenTreePicker(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(testConfig(), nil, "/tmp/test.json", dir, testSkills(), nil, nil, "", nil)

	m, _ := app.Update(openTreePickerMsg{})
	app = m.(App)
	if app.view != viewTreePicker {
		t.Errorf("view = %d, want %d (viewTreePicker)", app.view, viewTreePicker)
	}
}

func TestApp_TreePickerDone(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.view = viewTreePicker

	m, _ := app.Update(treePickerDoneMsg{pattern: "src/**"})
	app = m.(App)
	if app.view != viewRuleEditor {
		t.Errorf("view = %d, want %d (viewRuleEditor)", app.view, viewRuleEditor)
	}
}

// --- App: ruleSubmittedMsg for edit mode ---

func TestApp_RuleSubmittedEditExisting(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.view = viewRuleEditor
	app.ruleEditor = RuleEditor{
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "*"}},
	}

	// Submit a rule to replace existing at index 0
	rule := engine.Rule{Tool: "Bash", Path: "scripts/**", Skill: "go-coding", Agent: "*"}
	m, _ := app.Update(ruleSubmittedMsg{rule: &rule, ruleIndex: 0})
	app = m.(App)
	if app.config.Tools[0].Tool != "Bash" {
		t.Errorf("tool = %q, want %q after edit submission", app.config.Tools[0].Tool, "Bash")
	}
}

func TestApp_RuleSubmittedNilRule(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.view = viewRuleEditor
	before := len(app.config.Tools)

	m, _ := app.Update(ruleSubmittedMsg{rule: nil, ruleIndex: -1})
	app = m.(App)
	if len(app.config.Tools) != before {
		t.Error("nil rule submission should not change tools")
	}
}

// --- App: settingsDoneMsg with path change ---

func TestApp_SettingsDoneWithPathChange(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/path/a", testSkills(), nil, cfg, "/tmp/repo-config", []string{"/path/a", "/path/b"})
	app.view = viewSettings

	m, cmds := app.Update(settingsDoneMsg{
		config:        cfg,
		configLevel:   "project",
		preferredPath: "/path/b",
	})
	app = m.(App)
	if app.view != viewDashboard {
		t.Errorf("view = %d, want %d (viewDashboard)", app.view, viewDashboard)
	}
	if cmds == nil {
		t.Error("expected commands for saving config and preferred path")
	}
}

func TestApp_SettingsDoneNilConfig(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.view = viewSettings

	m, cmd := app.Update(settingsDoneMsg{config: nil, configLevel: "project"})
	app = m.(App)
	if app.view != viewDashboard {
		t.Errorf("view = %d, want %d (viewDashboard)", app.view, viewDashboard)
	}
	if cmd != nil {
		t.Error("expected no commands for nil config")
	}
}

// --- saveGlobalConfig tests ---

func TestSaveGlobalConfig_GlobalLevel(t *testing.T) {
	// Override HOME so saveGlobalConfig writes to a temp directory.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 90,
		StateTTLHours:   48,
		DefaultAgent:    "claude",
	}

	cmd := saveGlobalConfig(cfg, "global", "")
	msg := cmd()
	result := msg.(saveResultMsg)
	if result.err != nil {
		t.Fatalf("saveGlobalConfig failed: %v", result.err)
	}

	// Verify the file was written.
	configPath := tmpHome + "/.care-bear/config.json"
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if !strings.Contains(string(data), "90") {
		t.Error("expected SkillTTLMinutes=90 in saved config")
	}
	if !strings.Contains(string(data), "claude") {
		t.Error("expected DefaultAgent=claude in saved config")
	}
}

func TestSaveGlobalConfig_ProjectLevel(t *testing.T) {
	tmpDir := t.TempDir()
	enforcementPath := tmpDir + "/skill_enforcement.json"
	// Create the enforcement file so the directory exists.
	_ = os.WriteFile(enforcementPath, []byte("{}"), 0o644)

	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 30,
		StateTTLHours:   12,
		DefaultAgent:    "*",
	}

	cmd := saveGlobalConfig(cfg, "project", enforcementPath)
	msg := cmd()
	result := msg.(saveResultMsg)
	if result.err != nil {
		t.Fatalf("saveGlobalConfig project level failed: %v", result.err)
	}

	// Verify config.json sits alongside skill_enforcement.json.
	configPath := tmpDir + "/config.json"
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("project config file not written: %v", err)
	}
	if !strings.Contains(string(data), "30") {
		t.Error("expected SkillTTLMinutes=30 in saved project config")
	}
}

// --- savePreferredPath tests ---

func TestSavePreferredPath_WritesPreferencesJSON(t *testing.T) {
	repoConfigDir := t.TempDir()

	cmd := savePreferredPath(repoConfigDir, "/new/preferred/path")
	msg := cmd()
	result := msg.(saveResultMsg)
	if result.err != nil {
		t.Fatalf("savePreferredPath failed: %v", result.err)
	}

	// Verify preferences.json was written.
	prefs, err := engine.LoadRepoPreferences(repoConfigDir)
	if err != nil {
		t.Fatalf("failed to load saved preferences: %v", err)
	}
	if prefs.PreferredPath != "/new/preferred/path" {
		t.Errorf("PreferredPath = %q, want %q", prefs.PreferredPath, "/new/preferred/path")
	}
}

// --- saveConfig tests ---

func TestSaveConfig_WritesValidJSON(t *testing.T) {
	tmpFile := t.TempDir() + "/test_config.json"

	cfg := engine.Config{
		Version: 1,
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "*"},
		},
	}

	cmd := saveConfig(cfg, tmpFile)
	msg := cmd()
	result := msg.(saveResultMsg)
	if result.err != nil {
		t.Fatalf("saveConfig failed: %v", result.err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if !strings.Contains(string(data), "go-coding") {
		t.Error("expected go-coding skill in saved config")
	}
	if !strings.Contains(string(data), "Edit") {
		t.Error("expected Edit tool in saved config")
	}
}

func TestSaveConfig_InvalidPathFails(t *testing.T) {
	cfg := engine.Config{Version: 1}
	cmd := saveConfig(cfg, "/nonexistent/deeply/nested/dir/config.json")
	msg := cmd()
	result := msg.(saveResultMsg)
	if result.err == nil {
		t.Error("expected error when writing to nonexistent directory")
	}
}

// --- App: loadedSkillsUpdatedMsg handling ---

func TestApp_LoadedSkillsUpdatedMsg(t *testing.T) {
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)

	newLoaded := map[string]*state.SkillStatus{
		"git":    {Agents: []string{"claude"}},
		"linear": {Agents: []string{"cursor"}},
	}

	m, _ := app.Update(loadedSkillsUpdatedMsg{loaded: newLoaded})
	app = m.(App)

	if len(app.loadedSkills) != 2 {
		t.Errorf("expected 2 loaded skills, got %d", len(app.loadedSkills))
	}
	if app.dashboard.loadedSkills["git"] == nil {
		t.Error("expected git in dashboard's loadedSkills")
	}
}

// --- App: eventsUpdatedMsg handling ---

func TestApp_EventsUpdatedMsg(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create events.log
	eventsDir := tmpHome + "/.care-bear"
	_ = os.MkdirAll(eventsDir, 0o755)
	_ = os.WriteFile(eventsDir+"/events.log",
		[]byte("2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | git\n"),
		0o644)

	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)

	m, _ := app.Update(eventsUpdatedMsg{})
	app = m.(App)

	// After receiving eventsUpdatedMsg, the dashboard should have reloaded events.
	// The reload reads from ~/.care-bear/events.log.
	if len(app.dashboard.eventLines) != 1 {
		t.Errorf("expected 1 event line after reload, got %d", len(app.dashboard.eventLines))
	}
}

// --- App: settings done with global config saves global ---

func TestApp_SettingsDoneGlobalSave(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	app := NewApp(testConfig(), nil, "/tmp/test.json", "/tmp", testSkills(), nil, cfg, "", nil)
	app.view = viewSettings

	updatedCfg := &engine.GlobalConfig{
		SkillTTLMinutes: 120,
		StateTTLHours:   72,
		DefaultAgent:    "claude",
	}

	m, cmd := app.Update(settingsDoneMsg{
		config:      updatedCfg,
		configLevel: "global",
	})
	app = m.(App)
	if app.view != viewDashboard {
		t.Errorf("view = %d, want %d (viewDashboard)", app.view, viewDashboard)
	}
	if cmd == nil {
		t.Error("expected save command for global config")
	}
	// Verify the global config was updated in app state
	if app.globalConfig.SkillTTLMinutes != 120 {
		t.Errorf("SkillTTLMinutes = %d, want 120", app.globalConfig.SkillTTLMinutes)
	}
}

// --- App: rulesSubmittedMsg adds multiple rules and stays in editor for confirm ---

func TestApp_RulesSubmittedMultipleRules(t *testing.T) {
	app := NewApp(engine.Config{Version: 1}, nil, "/tmp/test.json", "/tmp", testSkills(), nil, nil, "", nil)
	app.view = viewRuleEditor
	app.ruleEditor = RuleEditor{
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "*"}},
	}

	rules := []engine.Rule{
		{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "claude"},
		{Tool: "Write", Path: "**/*.go", Skill: "go-coding", Agent: "claude"},
		{Tool: "Bash", Path: "scripts/**", Skill: "go-coding", Agent: "*"},
	}

	m, cmd := app.Update(rulesSubmittedMsg{rules: rules})
	app = m.(App)
	if len(app.config.Tools) != 3 {
		t.Errorf("expected 3 rules, got %d", len(app.config.Tools))
	}
	// rulesSubmittedMsg saves and returns to dashboard immediately (no confirm screen)
	if app.view != viewDashboard {
		t.Errorf("view = %d, want %d (should return to dashboard)", app.view, viewDashboard)
	}
	if cmd == nil {
		t.Error("expected save command")
	}
}
