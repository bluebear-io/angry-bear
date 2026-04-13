// dashboard_test.go tests the dashboard functions: filters, word wrap, column
// cycling, event log rendering, and helper utilities.
package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Blue-Bear-Security/care-bear/internal/engine"
	"github.com/Blue-Bear-Security/care-bear/internal/scanner"
	"github.com/Blue-Bear-Security/care-bear/internal/state"
)

// --- matchesFilters tests ---

func TestMatchesFilters_NoFilters(t *testing.T) {
	d := Dashboard{logFilters: make(map[filterCol]string)}
	if !d.matchesFilters("BLOCK", "proj", "sess1", "claude", "Edit", "git") {
		t.Error("expected match when no filters are set")
	}
}

func TestMatchesFilters_SingleColumnMatch(t *testing.T) {
	d := Dashboard{logFilters: map[filterCol]string{
		filterAction: "BLOCK",
	}}
	if !d.matchesFilters("BLOCK", "proj", "sess1", "claude", "Edit", "git") {
		t.Error("expected match when action matches")
	}
	if d.matchesFilters("ALLOW", "proj", "sess1", "claude", "Edit", "git") {
		t.Error("expected no match when action does not match")
	}
}

func TestMatchesFilters_MultiColumnAnd(t *testing.T) {
	d := Dashboard{logFilters: map[filterCol]string{
		filterAction:  "BLOCK",
		filterProject: "blueden",
		filterAgent:   "claude",
	}}

	// All match
	if !d.matchesFilters("BLOCK", "blueden", "s1", "claude", "Edit", "git") {
		t.Error("expected match when all filters match")
	}

	// Action matches but project doesn't
	if d.matchesFilters("BLOCK", "other-proj", "s1", "claude", "Edit", "git") {
		t.Error("expected no match when project doesn't match")
	}

	// Action and project match but agent doesn't
	if d.matchesFilters("BLOCK", "blueden", "s1", "cursor", "Edit", "git") {
		t.Error("expected no match when agent doesn't match")
	}
}

func TestMatchesFilters_EmptyFilterValueIgnored(t *testing.T) {
	d := Dashboard{logFilters: map[filterCol]string{
		filterAction: "", // empty = all
	}}
	if !d.matchesFilters("ALLOW", "proj", "s1", "claude", "Edit", "git") {
		t.Error("expected match when filter value is empty (means all)")
	}
}

func TestMatchesFilters_AllColumns(t *testing.T) {
	d := Dashboard{logFilters: map[filterCol]string{
		filterAction:  "BLOCK",
		filterProject: "proj",
		filterSess:    "s1",
		filterAgent:   "claude",
		filterTool:    "Edit",
		filterSkill:   "git",
	}}
	if !d.matchesFilters("BLOCK", "proj", "s1", "claude", "Edit", "git") {
		t.Error("expected match when all column filters match")
	}
	if d.matchesFilters("BLOCK", "proj", "s1", "claude", "Edit", "WRONG") {
		t.Error("expected no match when skill column doesn't match")
	}
}

// --- filterColName tests ---

func TestFilterColName(t *testing.T) {
	tests := []struct {
		col  filterCol
		want string
	}{
		{filterAction, "ACTION"},
		{filterProject, "PROJECT"},
		{filterSess, "SESS"},
		{filterAgent, "AGENT"},
		{filterTool, "TOOL"},
		{filterSkill, "SKILL"},
		{filterColCount, ""}, // sentinel value returns empty
	}
	for _, tt := range tests {
		got := filterColName(tt.col)
		if got != tt.want {
			t.Errorf("filterColName(%d) = %q, want %q", tt.col, got, tt.want)
		}
	}
}

// --- cycleFilterValue tests ---

func TestCycleFilterValue_Forward(t *testing.T) {
	d := &Dashboard{
		logFilters: make(map[filterCol]string),
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | f.go | BLOCK | git",
			"2026-04-13T00:00:01Z | proj | claude | abc12 | Edit | f.go | ALLOW | linear",
		},
		filterCursor: filterAction,
	}

	// First cycle: from "" (all) to first value
	d.cycleFilterValue(1)
	v := d.logFilters[filterAction]
	if v == "" {
		t.Error("expected filter to be set after first forward cycle")
	}

	// Keep cycling until we wrap back to "" (all)
	for i := 0; i < 10; i++ {
		d.cycleFilterValue(1)
		if _, ok := d.logFilters[filterAction]; !ok {
			break // Wrapped back to "all"
		}
	}
}

func TestCycleFilterValue_Backward(t *testing.T) {
	d := &Dashboard{
		logFilters: make(map[filterCol]string),
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | f.go | BLOCK | git",
		},
		filterCursor: filterAction,
	}

	// Cycle backward from "" (all) should wrap to last value
	d.cycleFilterValue(-1)
	v := d.logFilters[filterAction]
	if v == "" {
		t.Error("expected filter to be set after backward cycle from start")
	}
}

func TestCycleFilterValue_EmptyEvents(t *testing.T) {
	d := &Dashboard{
		logFilters:   make(map[filterCol]string),
		eventLines:   nil,
		filterCursor: filterAction,
	}
	// Should not panic with no events
	d.cycleFilterValue(1)
	if len(d.logFilters) != 0 {
		t.Error("expected no filters set when events are empty")
	}
}

func TestCycleFilterValue_ResetsLogCursor(t *testing.T) {
	d := &Dashboard{
		logFilters: make(map[filterCol]string),
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | f.go | BLOCK | git",
		},
		filterCursor: filterAction,
	}
	d.logScroll.Cursor = 5

	d.cycleFilterValue(1)

	if d.logScroll.Cursor != 0 {
		t.Errorf("logScroll.Cursor = %d, want 0 (should reset after filter change)", d.logScroll.Cursor)
	}
}

// --- uniqueColumnValues tests ---

func TestUniqueColumnValues(t *testing.T) {
	d := &Dashboard{
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj1 | claude | abc12 | Edit | f.go | BLOCK | git",
			"2026-04-13T00:00:01Z | proj2 | cursor | def34 | Write | m.go | ALLOW | linear",
			"2026-04-13T00:00:02Z | proj1 | claude | abc12 | Bash | t.sh | BLOCK | git",
		},
	}

	tests := []struct {
		col      filterCol
		wantVals []string // Values we expect to see (order-independent)
	}{
		{filterAction, []string{"BLOCK", "ALLOW"}},
		{filterProject, []string{"proj1", "proj2"}},
		{filterAgent, []string{"claude", "cursor"}},
	}

	for _, tt := range tests {
		vals := d.uniqueColumnValues(tt.col)
		if len(vals) != len(tt.wantVals) {
			t.Errorf("uniqueColumnValues(%d): got %d values %v, want %d values %v",
				tt.col, len(vals), vals, len(tt.wantVals), tt.wantVals)
			continue
		}
		// Check all expected values are present
		valSet := make(map[string]bool)
		for _, v := range vals {
			valSet[v] = true
		}
		for _, want := range tt.wantVals {
			if !valSet[want] {
				t.Errorf("uniqueColumnValues(%d): missing expected value %q in %v", tt.col, want, vals)
			}
		}
	}
}

func TestUniqueColumnValues_NoDuplicates(t *testing.T) {
	d := &Dashboard{
		eventLines: []string{
			"ts | proj | claude | s1 | Edit | f.go | BLOCK | git",
			"ts | proj | claude | s2 | Edit | g.go | BLOCK | git",
			"ts | proj | claude | s3 | Edit | h.go | BLOCK | git",
		},
	}

	vals := d.uniqueColumnValues(filterAgent)
	if len(vals) != 1 {
		t.Errorf("got %d unique agents %v, want 1 (all same agent)", len(vals), vals)
	}
}

// --- wordWrap tests ---

func TestWordWrap(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{
			name:  "empty input",
			text:  "",
			width: 40,
			want:  "",
		},
		{
			name:  "single word fits",
			text:  "hello",
			width: 10,
			want:  "hello",
		},
		{
			name:  "wraps at width",
			text:  "hello world foo bar",
			width: 11,
			want:  "hello world\nfoo bar",
		},
		{
			name:  "word longer than width stays on its own line",
			text:  "a verylongwordthatdoesnotfit b",
			width: 10,
			want:  "a\nverylongwordthatdoesnotfit\nb",
		},
		{
			name:  "zero width returns original",
			text:  "hello world",
			width: 0,
			want:  "hello world",
		},
		{
			name:  "negative width returns original",
			text:  "hello world",
			width: -5,
			want:  "hello world",
		},
		{
			name:  "exact fit no wrap",
			text:  "ab cd",
			width: 5,
			want:  "ab cd",
		},
		{
			name:  "multiple spaces collapsed by Fields",
			text:  "  hello   world  ",
			width: 40,
			want:  "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wordWrap(tt.text, tt.width)
			if got != tt.want {
				t.Errorf("wordWrap(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
			}
		})
	}
}

// --- nextTool / nextAgent tests ---

func TestNextTool(t *testing.T) {
	tests := []struct {
		current string
		want    string
	}{
		{"Edit", "Write"},
		{"*", "Edit"},       // Wrap around: last -> first
		{"unknown", "Edit"}, // Unknown defaults to first
	}
	for _, tt := range tests {
		got := nextTool(tt.current)
		if got != tt.want {
			t.Errorf("nextTool(%q) = %q, want %q", tt.current, got, tt.want)
		}
	}
}

func TestNextAgent(t *testing.T) {
	tests := []struct {
		current string
		want    string
	}{
		{"claude", "cursor"},
		{"*", "claude"},       // Wrap around
		{"unknown", "claude"}, // Unknown defaults to first
	}
	for _, tt := range tests {
		got := nextAgent(tt.current)
		if got != tt.want {
			t.Errorf("nextAgent(%q) = %q, want %q", tt.current, got, tt.want)
		}
	}
}

// --- NewDashboard tests ---

func TestNewDashboard_NilLoadedSkills(t *testing.T) {
	d := NewDashboard(nil, engine.Config{}, DefaultStyles(), nil)
	if d.loadedSkills == nil {
		t.Error("loadedSkills should be initialized even when nil is passed")
	}
}

func TestNewDashboard_WithLoadedSkills(t *testing.T) {
	loaded := map[string]*state.SkillStatus{
		"git": {Agents: []string{"claude", "cursor"}},
	}
	d := NewDashboard(testSkills(), testConfig(), DefaultStyles(), loaded)

	if len(d.loadedSkills) != 1 {
		t.Errorf("loadedSkills has %d entries, want 1", len(d.loadedSkills))
	}
	if d.loadedSkills["git"] == nil {
		t.Error("expected git skill in loadedSkills")
	}
}

func TestNewDashboard_InitializesFilters(t *testing.T) {
	d := NewDashboard(nil, engine.Config{}, DefaultStyles(), nil)
	if d.logFilters == nil {
		t.Error("logFilters should be initialized")
	}
	if len(d.logFilters) != 0 {
		t.Error("logFilters should be empty on creation")
	}
}

// --- renderEventLog tests ---

func TestRenderEventLog_EmptyEvents(t *testing.T) {
	d := Dashboard{
		eventLines: nil,
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
	}
	output := d.renderEventLog(80, 20)
	if !strings.Contains(output, "No events yet") {
		t.Errorf("expected 'No events yet' message, got:\n%s", output)
	}
}

func TestRenderEventLog_FilteredToEmpty(t *testing.T) {
	d := Dashboard{
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | git",
		},
		logFilters: map[filterCol]string{
			filterAction: "ALLOW", // No ALLOW events exist
		},
		styles: DefaultStyles(),
	}
	output := d.renderEventLog(80, 20)
	if !strings.Contains(output, "No matching events") {
		t.Errorf("expected 'No matching events' message, got:\n%s", output)
	}
}

func TestRenderEventLog_ShowsEvents(t *testing.T) {
	d := Dashboard{
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | git",
			"2026-04-13T00:00:01Z | proj | claude | abc12 | Write | main.go | ALLOW | linear",
		},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		width:      120,
		height:     40,
	}
	output := d.renderEventLog(100, 20)

	// Should contain EVENT LOG header
	if !strings.Contains(output, "EVENT LOG") {
		t.Error("expected EVENT LOG header in output")
	}

	// Should contain event data
	if !strings.Contains(output, "BLOCK") {
		t.Error("expected BLOCK action in output")
	}
	if !strings.Contains(output, "ALLOW") {
		t.Error("expected ALLOW action in output")
	}
}

func TestRenderEventLog_FilterLabelShown(t *testing.T) {
	d := Dashboard{
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | git",
		},
		logFilters: map[filterCol]string{
			filterAction: "BLOCK",
		},
		styles: DefaultStyles(),
	}
	output := d.renderEventLog(100, 20)
	if !strings.Contains(output, "ACTION=BLOCK") {
		t.Error("expected filter label 'ACTION=BLOCK' in output")
	}
}

// --- stripAnsi tests ---

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no ansi", "hello world", "hello world"},
		{"bold", "\033[1mhello\033[0m", "hello"},
		{"color", "\033[31mred\033[0m text", "red text"},
		{"empty", "", ""},
		{"multiple sequences", "\033[1m\033[31mhello\033[0m", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAnsi(tt.input)
			if got != tt.want {
				t.Errorf("stripAnsi(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- logPageSize tests ---

func TestLogPageSize(t *testing.T) {
	tests := []struct {
		name   string
		height int
		minVal int
	}{
		{"small terminal", 10, 5},
		{"medium terminal", 30, 5},
		{"very small", 0, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Dashboard{height: tt.height}
			got := d.logPageSize()
			if got < tt.minVal {
				t.Errorf("logPageSize() = %d, want >= %d (height=%d)", got, tt.minVal, tt.height)
			}
		})
	}
}

// --- rulesForSkill tests ---

func TestRulesForSkill_CursorBeyondSkills(t *testing.T) {
	d := &Dashboard{
		skills:      []scanner.Skill{{Name: "git"}},
		config:      testConfig(),
		skillScroll: ScrollView{Cursor: 5},
	}
	rules := d.rulesForSkill()
	if rules != nil {
		t.Error("expected nil when cursor is beyond skills list")
	}
}

func TestRulesForSkill_MatchesCorrectSkill(t *testing.T) {
	d := &Dashboard{
		skills:      testSkills(),
		config:      testConfig(),
		skillScroll: ScrollView{Cursor: 0}, // go-coding
	}
	rules := d.rulesForSkill()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1 for go-coding", len(rules))
	}
	if rules[0].rule.Skill != "go-coding" {
		t.Errorf("rule.Skill = %q, want %q", rules[0].rule.Skill, "go-coding")
	}
}

// --- Dashboard Update tests ---

func TestDashboard_UpdateUpDownSkillPanel(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 0,
	}

	// Move down in skills panel
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyDown})
	d = m.(Dashboard)
	if d.skillScroll.Cursor != 1 {
		t.Errorf("cursor = %d, want 1 after down", d.skillScroll.Cursor)
	}

	// Moving to a new skill should reset the rule scroll cursor
	if d.ruleScroll.Cursor != 0 {
		t.Errorf("ruleScroll.Cursor = %d, want 0 after skill change", d.ruleScroll.Cursor)
	}

	// Move up
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	d = m.(Dashboard)
	if d.skillScroll.Cursor != 0 {
		t.Errorf("cursor = %d, want 0 after up", d.skillScroll.Cursor)
	}
}

func TestDashboard_UpdateRightLeftPanelSwitch(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 0,
	}

	// Right from skills panel goes to rules panel
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRight})
	d = m.(Dashboard)
	if d.focusPanel != 1 {
		t.Errorf("focusPanel = %d, want 1 after right", d.focusPanel)
	}

	// Left from rules panel goes back to skills
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyLeft})
	d = m.(Dashboard)
	if d.focusPanel != 0 {
		t.Errorf("focusPanel = %d, want 0 after left", d.focusPanel)
	}
}

func TestDashboard_UpdateLeftFromLogPanel(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 2,
	}

	// Left from log panel goes to skills
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyLeft})
	d = m.(Dashboard)
	if d.focusPanel != 0 {
		t.Errorf("focusPanel = %d, want 0 after left from logs", d.focusPanel)
	}
}

func TestDashboard_FilterModeLeftRightCyclesColumns(t *testing.T) {
	d := Dashboard{
		skills:       testSkills(),
		config:       testConfig(),
		logFilters:   make(map[filterCol]string),
		styles:       DefaultStyles(),
		focusPanel:   2,
		filterMode:   true,
		filterCursor: filterAction,
	}

	// Right cycles to next filter column
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRight})
	d = m.(Dashboard)
	if d.filterCursor != filterProject {
		t.Errorf("filterCursor = %d, want %d (PROJECT)", d.filterCursor, filterProject)
	}

	// Left cycles back
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyLeft})
	d = m.(Dashboard)
	if d.filterCursor != filterAction {
		t.Errorf("filterCursor = %d, want %d (ACTION)", d.filterCursor, filterAction)
	}

	// Left wraps around
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyLeft})
	d = m.(Dashboard)
	if d.filterCursor != filterSkill {
		t.Errorf("filterCursor = %d, want %d (SKILL, wrapped)", d.filterCursor, filterSkill)
	}
}

func TestDashboard_PageUpPageDownInLogPanel(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 2,
		height:     40,
		eventLines: make([]string, 50),
	}
	d.logScroll.Cursor = 30

	// PageUp should decrease cursor
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	d = m.(Dashboard)
	if d.logScroll.Cursor >= 30 {
		t.Errorf("cursor = %d, want less than 30 after pgup", d.logScroll.Cursor)
	}

	// PageDown should increase cursor
	prev := d.logScroll.Cursor
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	d = m.(Dashboard)
	if d.logScroll.Cursor <= prev {
		t.Errorf("cursor = %d, want greater than %d after pgdown", d.logScroll.Cursor, prev)
	}
}

func TestDashboard_HomeEndInLogPanel(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 2,
		eventLines: make([]string, 20),
	}
	d.logScroll.Cursor = 10

	// Home jumps to top
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyHome})
	d = m.(Dashboard)
	if d.logScroll.Cursor != 0 {
		t.Errorf("cursor = %d, want 0 after home", d.logScroll.Cursor)
	}

	// End jumps to bottom
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnd})
	d = m.(Dashboard)
	if d.logScroll.Cursor != 19 {
		t.Errorf("cursor = %d, want 19 after end", d.logScroll.Cursor)
	}
}

func TestDashboard_EscClearsFilters(t *testing.T) {
	d := Dashboard{
		skills:       testSkills(),
		config:       testConfig(),
		logFilters:   map[filterCol]string{filterAction: "BLOCK"},
		styles:       DefaultStyles(),
		focusPanel:   2,
		filterMode:   true,
		filterCursor: filterAction,
	}

	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	d = m.(Dashboard)
	if d.filterMode {
		t.Error("filterMode should be false after esc")
	}
	if len(d.logFilters) != 0 {
		t.Errorf("logFilters should be empty after esc, got %v", d.logFilters)
	}
	if d.logScroll.Cursor != 0 {
		t.Errorf("logScroll.Cursor = %d, want 0 after esc", d.logScroll.Cursor)
	}
}

func TestDashboard_CycleTool(t *testing.T) {
	cfg := engine.Config{
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "*"},
		},
	}
	d := Dashboard{
		skills:     []scanner.Skill{{Name: "go-coding"}},
		config:     cfg,
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1,
	}

	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	d = m.(Dashboard)
	if d.config.Tools[0].Tool == "Edit" {
		t.Error("expected tool to change after pressing 't'")
	}
}

func TestDashboard_CycleAgent(t *testing.T) {
	cfg := engine.Config{
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**", Skill: "go-coding", Agent: "claude"},
		},
	}
	d := Dashboard{
		skills:     []scanner.Skill{{Name: "go-coding"}},
		config:     cfg,
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1,
	}

	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	d = m.(Dashboard)
	if d.config.Tools[0].Agent == "claude" {
		t.Error("expected agent to change after pressing 'g'")
	}
}

func TestDashboard_DuplicateRule(t *testing.T) {
	cfg := engine.Config{
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**", Skill: "go-coding", Agent: "*"},
		},
	}
	d := Dashboard{
		skills:     []scanner.Skill{{Name: "go-coding"}},
		config:     cfg,
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1,
	}

	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	d = m.(Dashboard)
	if len(d.config.Tools) != 2 {
		t.Fatalf("expected 2 rules after duplicate, got %d", len(d.config.Tools))
	}
	if d.config.Tools[1].Tool != "Edit" || d.config.Tools[1].Skill != "go-coding" {
		t.Error("duplicated rule should match the original")
	}
}

func TestDashboard_DeleteRule(t *testing.T) {
	cfg := engine.Config{
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**", Skill: "go-coding", Agent: "*"},
			{Tool: "Write", Path: "**", Skill: "go-coding", Agent: "*"},
		},
	}
	d := Dashboard{
		skills:     []scanner.Skill{{Name: "go-coding"}},
		config:     cfg,
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1,
	}

	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	d = m.(Dashboard)
	if len(d.config.Tools) != 1 {
		t.Fatalf("expected 1 rule after delete, got %d", len(d.config.Tools))
	}
}

func TestDashboard_EnterPathEditMode(t *testing.T) {
	cfg := engine.Config{
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "*"},
		},
	}
	d := Dashboard{
		skills:     []scanner.Skill{{Name: "go-coding"}},
		config:     cfg,
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1,
	}

	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	d = m.(Dashboard)
	if !d.editingPath {
		t.Error("expected editingPath to be true after pressing 'p'")
	}
	if d.pathBuffer != "**/*.go" {
		t.Errorf("pathBuffer = %q, want %q", d.pathBuffer, "**/*.go")
	}
}

func TestDashboard_SettingsKeyOpensSettings(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
	}

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("expected command from 'c'")
	}
	msg := cmd()
	if _, ok := msg.(openSettingsMsg); !ok {
		t.Errorf("expected openSettingsMsg, got %T", msg)
	}
}

func TestDashboard_SwitchProjectKey(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
	}

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	if cmd == nil {
		t.Fatal("expected command from 'P'")
	}
	msg := cmd()
	if _, ok := msg.(switchProjectMsg); !ok {
		t.Errorf("expected switchProjectMsg, got %T", msg)
	}
}

func TestDashboard_EnterOnSkillOpensRuleEditor(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 0,
	}

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from enter on skill")
	}
	msg := cmd()
	oreMsg, ok := msg.(openRuleEditorMsg)
	if !ok {
		t.Fatalf("expected openRuleEditorMsg, got %T", msg)
	}
	if oreMsg.skillName != "go-coding" {
		t.Errorf("skillName = %q, want %q", oreMsg.skillName, "go-coding")
	}
}

func TestDashboard_AddRuleKey(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 0,
	}

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd == nil {
		t.Fatal("expected command from 'a'")
	}
	msg := cmd()
	if _, ok := msg.(openRuleEditorMsg); !ok {
		t.Errorf("expected openRuleEditorMsg, got %T", msg)
	}
}

func TestDashboard_SaveKey(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
	}

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("expected command from 's'")
	}
	msg := cmd()
	if _, ok := msg.(saveRequestMsg); !ok {
		t.Errorf("expected saveRequestMsg, got %T", msg)
	}
}

// --- updatePathEdit tests ---

func TestDashboard_PathEditCommit(t *testing.T) {
	d := Dashboard{
		skills: []scanner.Skill{{Name: "go-coding"}},
		config: engine.Config{
			Tools: []engine.Rule{
				{Tool: "Edit", Path: "old/path", Skill: "go-coding", Agent: "*"},
			},
		},
		logFilters:  make(map[filterCol]string),
		styles:      DefaultStyles(),
		focusPanel:  1,
		editingPath: true,
		pathBuffer:  "new/path/**",
		pathCurPos:  11,
	}

	m, _ := d.updatePathEdit(tea.KeyMsg{Type: tea.KeyEnter})
	d = m.(Dashboard)
	if d.editingPath {
		t.Error("editingPath should be false after enter")
	}
	if d.config.Tools[0].Path != "new/path/**" {
		t.Errorf("path = %q, want %q", d.config.Tools[0].Path, "new/path/**")
	}
}

func TestDashboard_PathEditCancel(t *testing.T) {
	d := Dashboard{
		skills: []scanner.Skill{{Name: "go-coding"}},
		config: engine.Config{
			Tools: []engine.Rule{
				{Tool: "Edit", Path: "original", Skill: "go-coding", Agent: "*"},
			},
		},
		logFilters:  make(map[filterCol]string),
		styles:      DefaultStyles(),
		focusPanel:  1,
		editingPath: true,
		pathBuffer:  "changed",
		pathCurPos:  7,
	}

	m, _ := d.updatePathEdit(tea.KeyMsg{Type: tea.KeyEsc})
	d = m.(Dashboard)
	if d.editingPath {
		t.Error("editingPath should be false after esc")
	}
	// Path should NOT have been committed
	if d.config.Tools[0].Path != "original" {
		t.Errorf("path = %q, want %q (should not commit on cancel)", d.config.Tools[0].Path, "original")
	}
}

func TestDashboard_PathEditBackspace(t *testing.T) {
	d := Dashboard{
		editingPath: true,
		pathBuffer:  "hello",
		pathCurPos:  5,
		logFilters:  make(map[filterCol]string),
		styles:      DefaultStyles(),
	}

	m, _ := d.updatePathEdit(tea.KeyMsg{Type: tea.KeyBackspace})
	d = m.(Dashboard)
	if d.pathBuffer != "hell" {
		t.Errorf("pathBuffer = %q, want %q", d.pathBuffer, "hell")
	}
	if d.pathCurPos != 4 {
		t.Errorf("pathCurPos = %d, want 4", d.pathCurPos)
	}
}

func TestDashboard_PathEditLeftRight(t *testing.T) {
	d := Dashboard{
		editingPath: true,
		pathBuffer:  "hello",
		pathCurPos:  3,
		logFilters:  make(map[filterCol]string),
		styles:      DefaultStyles(),
	}

	m, _ := d.updatePathEdit(tea.KeyMsg{Type: tea.KeyLeft})
	d = m.(Dashboard)
	if d.pathCurPos != 2 {
		t.Errorf("pathCurPos = %d, want 2 after left", d.pathCurPos)
	}

	m, _ = d.updatePathEdit(tea.KeyMsg{Type: tea.KeyRight})
	d = m.(Dashboard)
	if d.pathCurPos != 3 {
		t.Errorf("pathCurPos = %d, want 3 after right", d.pathCurPos)
	}
}

func TestDashboard_PathEditCtrlACtrlE(t *testing.T) {
	d := Dashboard{
		editingPath: true,
		pathBuffer:  "hello",
		pathCurPos:  3,
		logFilters:  make(map[filterCol]string),
		styles:      DefaultStyles(),
	}

	m, _ := d.updatePathEdit(tea.KeyMsg{Type: tea.KeyCtrlA})
	d = m.(Dashboard)
	if d.pathCurPos != 0 {
		t.Errorf("pathCurPos = %d, want 0 after ctrl+a", d.pathCurPos)
	}

	m, _ = d.updatePathEdit(tea.KeyMsg{Type: tea.KeyCtrlE})
	d = m.(Dashboard)
	if d.pathCurPos != 5 {
		t.Errorf("pathCurPos = %d, want 5 after ctrl+e", d.pathCurPos)
	}
}

func TestDashboard_PathEditCharInsert(t *testing.T) {
	d := Dashboard{
		editingPath: true,
		pathBuffer:  "hllo",
		pathCurPos:  1,
		logFilters:  make(map[filterCol]string),
		styles:      DefaultStyles(),
	}

	m, _ := d.updatePathEdit(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	d = m.(Dashboard)
	if d.pathBuffer != "hello" {
		t.Errorf("pathBuffer = %q, want %q", d.pathBuffer, "hello")
	}
	if d.pathCurPos != 2 {
		t.Errorf("pathCurPos = %d, want 2", d.pathCurPos)
	}
}

// --- renderSkillList tests ---

func TestRenderSkillList_WithLoadedSkills(t *testing.T) {
	d := Dashboard{
		skills: testSkills(),
		config: testConfig(),
		loadedSkills: map[string]*state.SkillStatus{
			"linear": {Agents: []string{"claude", "cursor"}},
		},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		width:      100,
		height:     40,
	}

	output := d.renderSkillList(40, 20)
	if !strings.Contains(output, "SKILLS") {
		t.Error("expected SKILLS header")
	}
	// Loaded skill should have agent tag or loaded indicator
	if !strings.Contains(output, "claude") {
		t.Error("expected 'claude' tag for loaded skill")
	}
}

func TestRenderSkillList_NoSkills(t *testing.T) {
	d := Dashboard{
		skills:     nil,
		config:     engine.Config{},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		width:      100,
		height:     40,
	}

	output := d.View()
	if !strings.Contains(output, "No skills discovered") {
		t.Error("expected 'No skills discovered' message for empty skills")
	}
}

// --- renderRulePanel tests ---

func TestRenderRulePanel_NoRules(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     engine.Config{},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
	}

	output := d.renderRulePanel(60, 20)
	if !strings.Contains(output, "No rules configured") {
		t.Error("expected 'No rules configured' message")
	}
	if !strings.Contains(output, "add rules") {
		t.Error("expected help text about adding rules")
	}
}

func TestRenderRulePanel_CursorBeyondSkills(t *testing.T) {
	d := Dashboard{
		skills:      testSkills(),
		config:      testConfig(),
		logFilters:  make(map[filterCol]string),
		styles:      DefaultStyles(),
		skillScroll: ScrollView{Cursor: 99},
	}

	output := d.renderRulePanel(60, 20)
	if output != "" {
		t.Errorf("expected empty output when cursor beyond skills, got %q", output)
	}
}

// --- renderEventLog with filter mode header ---

func TestRenderEventLog_FilterModeHeader(t *testing.T) {
	d := Dashboard{
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | git",
		},
		logFilters:   make(map[filterCol]string),
		styles:       DefaultStyles(),
		filterMode:   true,
		filterCursor: filterAction,
	}

	output := d.renderEventLog(100, 20)
	// In filter mode, the active column header should be highlighted
	// We check that the output contains the column headers
	if !strings.Contains(output, "EVENT LOG") {
		t.Error("expected EVENT LOG header in filter mode output")
	}
}

// --- jumpToLogEntry tests ---

func TestJumpToLogEntry_CommaSkill(t *testing.T) {
	d := &Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 2,
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | linear,testing",
		},
	}
	d.logScroll.Cursor = 0

	d.jumpToLogEntry()

	// Should jump to "linear" (first skill in comma list), which is index 1
	if d.skillScroll.Cursor != 1 {
		t.Errorf("skillScroll.Cursor = %d, want 1 (linear)", d.skillScroll.Cursor)
	}
	if d.focusPanel != 1 {
		t.Errorf("focusPanel = %d, want 1 after jump", d.focusPanel)
	}
}

func TestJumpToLogEntry_SkillNotFound(t *testing.T) {
	d := &Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 2,
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | nonexistent-skill",
		},
	}
	d.logScroll.Cursor = 0
	d.skillScroll.Cursor = 0

	d.jumpToLogEntry()

	// Skill not found — should not change panel or cursor
	if d.focusPanel != 2 {
		t.Errorf("focusPanel = %d, want 2 (should not change)", d.focusPanel)
	}
	if d.skillScroll.Cursor != 0 {
		t.Errorf("skillScroll.Cursor = %d, want 0 (should not change)", d.skillScroll.Cursor)
	}
}

func TestJumpToLogEntry_CursorOutOfRange(t *testing.T) {
	d := &Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 2,
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | linear",
		},
	}
	d.logScroll.Cursor = 99 // Out of range

	d.jumpToLogEntry()

	// Should not panic and should not change anything
	if d.focusPanel != 2 {
		t.Errorf("focusPanel = %d, want 2 (should not change)", d.focusPanel)
	}
}

func TestJumpToLogEntry_UnparseableLine(t *testing.T) {
	d := &Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 2,
		eventLines: []string{"not a valid line"},
	}
	d.logScroll.Cursor = 0

	d.jumpToLogEntry()

	// Should not panic and should not change panel
	if d.focusPanel != 2 {
		t.Errorf("focusPanel = %d, want 2 after unparseable line", d.focusPanel)
	}
}

// --- uniqueColumnValues with all filterable columns ---

func TestUniqueColumnValues_SessionAndToolAndSkill(t *testing.T) {
	d := &Dashboard{
		eventLines: []string{
			"ts | proj | claude | sess1 | Edit | f.go | BLOCK | git",
			"ts | proj | cursor | sess2 | Write | m.go | ALLOW | linear",
		},
	}

	sessions := d.uniqueColumnValues(filterSess)
	if len(sessions) != 2 {
		t.Errorf("sessions: got %d values, want 2", len(sessions))
	}

	tools := d.uniqueColumnValues(filterTool)
	if len(tools) != 2 {
		t.Errorf("tools: got %d values, want 2", len(tools))
	}

	skills := d.uniqueColumnValues(filterSkill)
	if len(skills) != 2 {
		t.Errorf("skills: got %d values, want 2", len(skills))
	}
}

// --- renderRulePanel with inline path editing ---

func TestRenderRulePanel_EditingPathShowsCursor(t *testing.T) {
	d := Dashboard{
		skills: []scanner.Skill{{Name: "go-coding"}},
		config: engine.Config{
			Tools: []engine.Rule{
				{Tool: "Edit", Path: "src/**", Skill: "go-coding", Agent: "claude"},
			},
		},
		logFilters:  make(map[filterCol]string),
		styles:      DefaultStyles(),
		focusPanel:  1,
		editingPath: true,
		pathBuffer:  "new/path",
		pathCurPos:  3,
	}

	output := d.renderRulePanel(80, 20)
	// When editing path, the panel should show the editing hint
	if !strings.Contains(output, "Editing path") {
		t.Error("expected 'Editing path' hint when editingPath is true")
	}
}

func TestRenderRulePanel_WithRulesShowsColumnHeader(t *testing.T) {
	d := Dashboard{
		skills: []scanner.Skill{{Name: "go-coding"}},
		config: engine.Config{
			Tools: []engine.Rule{
				{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "claude"},
			},
		},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 0,
	}

	output := d.renderRulePanel(80, 20)
	if !strings.Contains(output, "TOOL") {
		t.Error("expected TOOL column header")
	}
	if !strings.Contains(output, "PATH") {
		t.Error("expected PATH column header")
	}
	if !strings.Contains(output, "AGENT") {
		t.Error("expected AGENT column header")
	}
}

func TestRenderRulePanel_FocusedRuleHighlighted(t *testing.T) {
	d := Dashboard{
		skills: []scanner.Skill{{Name: "go-coding"}},
		config: engine.Config{
			Tools: []engine.Rule{
				{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "claude"},
				{Tool: "Write", Path: "**/*.ts", Skill: "go-coding", Agent: "*"},
			},
		},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1, // Rules panel focused
	}

	output := d.renderRulePanel(80, 20)
	// Should contain the help bar with tool/path/agent/dup/del keys
	if !strings.Contains(output, "tool") || !strings.Contains(output, "path") {
		t.Error("expected help bar with editing keys when rules panel focused")
	}
}

func TestRenderRulePanel_LongPathTruncated(t *testing.T) {
	longPath := "very/long/deeply/nested/path/to/some/file/that/exceeds/width/limit.go"
	d := Dashboard{
		skills: []scanner.Skill{{Name: "go-coding"}},
		config: engine.Config{
			Tools: []engine.Rule{
				{Tool: "Edit", Path: longPath, Skill: "go-coding", Agent: "*"},
			},
		},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 0, // Not focused on rules, so path gets truncated normally
	}

	output := d.renderRulePanel(60, 20)
	// Long paths should be truncated with "..."
	if !strings.Contains(output, "...") {
		t.Error("expected '...' truncation for long path")
	}
}

func TestRenderRulePanel_ShowsSkillDescription(t *testing.T) {
	d := Dashboard{
		skills: []scanner.Skill{{Name: "go-coding", Description: "Go coding standards and best practices"}},
		config: engine.Config{
			Tools: []engine.Rule{
				{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "*"},
			},
		},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
	}

	output := d.renderRulePanel(80, 20)
	if !strings.Contains(output, "Go coding standards") {
		t.Error("expected skill description in rule panel")
	}
}

// --- renderRulePanel with many rules to test scroll indicators ---

func TestRenderRulePanel_ScrollIndicator(t *testing.T) {
	var rules []engine.Rule
	for i := 0; i < 30; i++ {
		rules = append(rules, engine.Rule{
			Tool: "Edit", Path: fmt.Sprintf("path_%d/**", i), Skill: "go-coding", Agent: "*",
		})
	}
	d := Dashboard{
		skills:     []scanner.Skill{{Name: "go-coding"}},
		config:     engine.Config{Tools: rules},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1,
	}

	output := d.renderRulePanel(80, 15) // Small height forces scrolling
	// With 30 rules and only 15 height, there should be a scroll indicator
	if !strings.Contains(output, "[1/30]") {
		t.Error("expected scroll indicator [1/30] when rules overflow viewport")
	}
}

// --- renderSkillList with many skills to test scroll indicators ---

func TestRenderSkillList_ScrollIndicators(t *testing.T) {
	var skills []scanner.Skill
	for i := 0; i < 20; i++ {
		skills = append(skills, scanner.Skill{Name: fmt.Sprintf("skill-%02d", i)})
	}
	d := Dashboard{
		skills:       skills,
		config:       engine.Config{},
		loadedSkills: make(map[string]*state.SkillStatus),
		logFilters:   make(map[filterCol]string),
		styles:       DefaultStyles(),
	}

	output := d.renderSkillList(40, 10) // Only 10 rows, 20 skills
	if !strings.Contains(output, "[1/20]") {
		t.Error("expected scroll indicator [1/20] when skills overflow viewport")
	}
}

func TestRenderSkillList_LoadedWithUnknownAgentSkipped(t *testing.T) {
	d := Dashboard{
		skills: []scanner.Skill{{Name: "git"}, {Name: "linear"}},
		config: engine.Config{},
		loadedSkills: map[string]*state.SkillStatus{
			// git has unknown+claude, linear has no loaded status
			"git": {Agents: []string{"unknown", "claude"}},
		},
		logFilters:  make(map[filterCol]string),
		styles:      DefaultStyles(),
		focusPanel:  1,                     // Focus on rules panel so skill list items are NOT focused
		skillScroll: ScrollView{Cursor: 1}, // Cursor on linear, not git
		width:       100,
		height:      40,
	}

	output := d.renderSkillList(40, 20)
	// The "git" skill is not focused, so it renders agent tags (not " loaded" text).
	// "unknown" agents should be skipped in the rendered output.
	if !strings.Contains(output, "claude") {
		t.Error("expected 'claude' agent tag in loaded skill")
	}
}

func TestRenderSkillList_CursorOnNonFocusedPanel(t *testing.T) {
	d := Dashboard{
		skills: testSkills(),
		config: testConfig(),
		loadedSkills: map[string]*state.SkillStatus{
			"linear": {Agents: []string{"cursor"}},
		},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1, // Rules panel focused, not skills
		width:      100,
		height:     40,
	}

	output := d.renderSkillList(40, 20)
	// Should still render skill names
	if !strings.Contains(output, "go-coding") {
		t.Error("expected skill names rendered even when panel not focused")
	}
	// Loaded skill "cursor" tag should appear
	if !strings.Contains(output, "cursor") {
		t.Error("expected cursor agent tag on loaded skill")
	}
}

// --- renderEventLog with LOAD events ---

func TestRenderEventLog_LoadEventsRenderCyan(t *testing.T) {
	d := Dashboard{
		eventLines: []string{
			"2026-04-13T00:00:00Z | proj | claude | abc12 | SKILL-LOAD | | LOAD | linear",
		},
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		width:      120,
		height:     40,
	}

	output := d.renderEventLog(100, 20)
	if !strings.Contains(output, "LOAD") {
		t.Error("expected LOAD action in event log output")
	}
}

// --- Dashboard View with various panel sizes ---

func TestDashboard_ViewSmallTerminal(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		width:      40,
		height:     15,
	}

	output := d.View()
	// Should not panic with small terminal
	if output == "" {
		t.Error("expected non-empty output even with small terminal")
	}
}

// --- Dashboard: up/down in rules panel ---

func TestDashboard_UpDownInRulesPanel(t *testing.T) {
	cfg := engine.Config{
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "*"},
			{Tool: "Write", Path: "**/*.ts", Skill: "go-coding", Agent: "*"},
		},
	}
	d := Dashboard{
		skills:     []scanner.Skill{{Name: "go-coding"}},
		config:     cfg,
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1,
	}

	// Move down in rules
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyDown})
	d = m.(Dashboard)
	if d.ruleScroll.Cursor != 1 {
		t.Errorf("ruleScroll.Cursor = %d, want 1 after down in rules panel", d.ruleScroll.Cursor)
	}

	// Move up
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	d = m.(Dashboard)
	if d.ruleScroll.Cursor != 0 {
		t.Errorf("ruleScroll.Cursor = %d, want 0 after up in rules panel", d.ruleScroll.Cursor)
	}
}

// --- Dashboard: filter mode up/down cycles values ---

func TestDashboard_FilterModeUpDownCyclesValues(t *testing.T) {
	d := Dashboard{
		skills: testSkills(),
		config: testConfig(),
		logFilters: map[filterCol]string{
			filterAgent: "claude",
		},
		eventLines: []string{
			"ts | proj | claude | s1 | Edit | f.go | BLOCK | git",
			"ts | proj | cursor | s2 | Write | g.go | ALLOW | linear",
		},
		styles:       DefaultStyles(),
		focusPanel:   2,
		filterMode:   true,
		filterCursor: filterAgent,
	}

	// Up should cycle the agent filter backward
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyUp})
	d = m.(Dashboard)
	currentFilter := d.logFilters[filterAgent]
	// The value should have changed from "claude" to either "" (all) or "cursor"
	if currentFilter == "claude" {
		t.Error("expected agent filter to change after up cycle")
	}
}

// --- Dashboard: delete last rule adjusts cursor ---

func TestDashboard_DeleteLastRuleAdjustsCursor(t *testing.T) {
	cfg := engine.Config{
		Tools: []engine.Rule{
			{Tool: "Edit", Path: "**/*.go", Skill: "go-coding", Agent: "*"},
			{Tool: "Write", Path: "**/*.ts", Skill: "go-coding", Agent: "*"},
		},
	}
	d := Dashboard{
		skills:     []scanner.Skill{{Name: "go-coding"}},
		config:     cfg,
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 1,
	}
	d.ruleScroll.Cursor = 1 // Select second (last) rule

	// Delete the last rule
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	d = m.(Dashboard)
	if len(d.config.Tools) != 1 {
		t.Fatalf("expected 1 rule after delete, got %d", len(d.config.Tools))
	}
	// Cursor should adjust down since we deleted the last item
	if d.ruleScroll.Cursor != 0 {
		t.Errorf("ruleScroll.Cursor = %d, want 0 after deleting last rule", d.ruleScroll.Cursor)
	}
}

// --- Dashboard: right from skills resets rule cursor ---

func TestDashboard_RightFromSkillsResetsRuleCursor(t *testing.T) {
	d := Dashboard{
		skills:     testSkills(),
		config:     testConfig(),
		logFilters: make(map[filterCol]string),
		styles:     DefaultStyles(),
		focusPanel: 0,
	}
	d.ruleScroll.Cursor = 5 // Set to non-zero

	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRight})
	d = m.(Dashboard)
	if d.focusPanel != 1 {
		t.Errorf("focusPanel = %d, want 1", d.focusPanel)
	}
	if d.ruleScroll.Cursor != 0 {
		t.Errorf("ruleScroll.Cursor = %d, want 0 (should reset on panel switch)", d.ruleScroll.Cursor)
	}
}
