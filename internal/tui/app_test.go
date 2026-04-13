package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/scanner"
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
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
	if len(app.config.Tools) != 2 {
		t.Errorf("expected 2 rules, got %d", len(app.config.Tools))
	}
	if app.view != viewDashboard {
		t.Errorf("expected dashboard view")
	}
}

func TestDashboardRendersSkillNames(t *testing.T) {
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
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
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = m.(App)
	if app.dashboard.skillCursor != 1 {
		t.Errorf("expected cursor=1, got %d", app.dashboard.skillCursor)
	}
}

func TestDashboardPanelSwitch(t *testing.T) {
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
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
	app := NewApp(engine.Config{Version: 1}, "/tmp/test.json", testSkills(), nil)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestDashboardSave(t *testing.T) {
	tmpFile := t.TempDir() + "/config.json"
	app := NewApp(testConfig(), tmpFile, testSkills(), nil)
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
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
	app.dashboard.focusPanel = 1
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = m.(App)
	if len(app.config.Tools) != 1 {
		t.Errorf("expected 1 rule after delete, got %d", len(app.config.Tools))
	}
}

func TestRuleEditorCancel(t *testing.T) {
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
	app.view = viewRuleEditor
	m, _ := app.Update(ruleEditorDoneMsg{rule: nil, ruleIndex: -1})
	app = m.(App)
	if app.view != viewDashboard {
		t.Errorf("expected dashboard after cancel")
	}
}

func TestRuleEditorSubmit(t *testing.T) {
	app := NewApp(engine.Config{Version: 1}, "/tmp/test.json", testSkills(), nil)
	app.view = viewRuleEditor
	rule := engine.Rule{Tool: "Edit", Path: "**/*.go", Skill: "linear", Agent: "*"}
	m, _ := app.Update(rulesSubmittedMsg{rules: []engine.Rule{rule}})
	app = m.(App)
	if len(app.config.Tools) != 1 {
		t.Errorf("expected 1 rule, got %d", len(app.config.Tools))
	}
}

func TestLoadedSkillsShown(t *testing.T) {
	loaded := map[string]*SkillStatus{"linear": {Agents: []string{"claude"}}}
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), loaded)
	app.width, app.height = 120, 40
	app.dashboard.width, app.dashboard.height = 120, 40
	output := app.View()
	// Loaded skills should have the green dot indicator
	if !strings.Contains(output, "claude") {
		t.Errorf("expected loaded indicator in output:\n%s", output)
	}
}

func TestWindowSizeMsg(t *testing.T) {
	app := NewApp(engine.Config{Version: 1}, "/tmp/test.json", nil, nil)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app = m.(App)
	if app.width != 120 || app.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", app.width, app.height)
	}
}

func TestHelpBarContent(t *testing.T) {
	app := NewApp(engine.Config{Version: 1}, "/tmp/test.json", testSkills(), nil)
	app.width, app.height = 120, 40
	app.dashboard.width, app.dashboard.height = 120, 40
	output := app.View()
	if !strings.Contains(output, "navigate") || !strings.Contains(output, "save") || !strings.Contains(output, "quit") {
		t.Errorf("help bar missing text:\n%s", output)
	}
}

// --- Three-panel navigation tests ---

func TestThreePanelTabCycle(t *testing.T) {
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
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
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
	// Shift+Tab from panel 0 → panel 2
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	app = m.(App)
	if app.dashboard.focusPanel != 2 {
		t.Errorf("expected panel 2, got %d", app.dashboard.focusPanel)
	}
}

func TestLogPanelNavigation(t *testing.T) {
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
	app.dashboard.eventLines = []string{
		"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | linear",
		"2026-04-13T00:00:01Z | proj | claude | abc12 | SKILL-LOAD | | LOAD | linear",
		"2026-04-13T00:00:02Z | proj | claude | abc12 | Edit | test.go | ALLOW | linear",
	}
	app.dashboard.focusPanel = 2

	// Down
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = m.(App)
	if app.dashboard.logCursor != 1 {
		t.Errorf("expected logCursor=1, got %d", app.dashboard.logCursor)
	}

	// Down again
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = m.(App)
	if app.dashboard.logCursor != 2 {
		t.Errorf("expected logCursor=2, got %d", app.dashboard.logCursor)
	}

	// Up
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyUp})
	app = m.(App)
	if app.dashboard.logCursor != 1 {
		t.Errorf("expected logCursor=1, got %d", app.dashboard.logCursor)
	}
}

func TestProjectFilterCycle(t *testing.T) {
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)
	app.dashboard.eventLines = []string{
		"2026-04-13T00:00:00Z | blueden | claude | abc12 | Edit | test.go | BLOCK | git",
		"2026-04-13T00:00:01Z | baloo   | cursor | def45 | Edit | main.go | BLOCK | review",
	}
	app.dashboard.focusPanel = 2

	// Press f — should set filter to first project (blueden)
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app = m.(App)
	if app.dashboard.logProjectFilter != "blueden" {
		t.Errorf("expected filter 'blueden', got %q", app.dashboard.logProjectFilter)
	}

	// Press f again — should cycle to next project (baloo)
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app = m.(App)
	if app.dashboard.logProjectFilter != "baloo" {
		t.Errorf("expected filter 'baloo', got %q", app.dashboard.logProjectFilter)
	}

	// Press f again — should clear filter
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app = m.(App)
	if app.dashboard.logProjectFilter != "" {
		t.Errorf("expected empty filter, got %q", app.dashboard.logProjectFilter)
	}
}

func TestJumpToLogEntry(t *testing.T) {
	skills := testSkills() // go-coding, linear, testing
	app := NewApp(testConfig(), "/tmp/test.json", skills, nil)
	app.dashboard.eventLines = []string{
		"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | linear",
	}
	app.dashboard.focusPanel = 2
	app.dashboard.logCursor = 0

	// Press enter — should jump to "linear" skill
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = m.(App)

	if app.dashboard.focusPanel != 1 {
		t.Errorf("expected focusPanel=1 (rules), got %d", app.dashboard.focusPanel)
	}
	if app.dashboard.skillCursor != 1 { // linear is second skill (go-coding, linear, testing)
		t.Errorf("expected skillCursor=1 (linear), got %d", app.dashboard.skillCursor)
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
	app := NewApp(testConfig(), "/tmp/test.json", testSkills(), nil)

	// Press P — should trigger quit with SwitchRequested
	SwitchRequested = false
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	if cmd == nil {
		t.Fatal("expected command from P")
	}
}
