// settings_test.go tests the settings view: buildConfig, visibleItems,
// cycleSetting, navigation, editing, and the done message.
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Blue-Bear-Security/care-bear/internal/engine"
)

// --- buildConfig tests ---

func TestBuildConfig_AllFields(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 120,
		StateTTLHours:   48,
		DefaultAgent:    "cursor",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp/project", nil)
	result := s.buildConfig()

	if result.SkillTTLMinutes != 120 {
		t.Errorf("SkillTTLMinutes = %d, want 120", result.SkillTTLMinutes)
	}
	if result.StateTTLHours != 48 {
		t.Errorf("StateTTLHours = %d, want 48", result.StateTTLHours)
	}
	if result.DefaultAgent != "cursor" {
		t.Errorf("DefaultAgent = %q, want %q", result.DefaultAgent, "cursor")
	}
}

func TestBuildConfig_ZeroValues(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 0,
		StateTTLHours:   0,
		DefaultAgent:    "",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	result := s.buildConfig()

	if result.SkillTTLMinutes != 0 {
		t.Errorf("SkillTTLMinutes = %d, want 0", result.SkillTTLMinutes)
	}
	if result.StateTTLHours != 0 {
		t.Errorf("StateTTLHours = %d, want 0", result.StateTTLHours)
	}
}

// --- visibleItems tests ---

func TestVisibleItems_ProjectLevel(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp/project", nil)
	s.configLevel = "project"

	visible := s.visibleItems()
	// Should include project_root (level="project") + TTL fields (level="both") + default_agent (level="both")
	if len(visible) < 3 {
		t.Errorf("got %d visible items, want at least 3 (project root + 2 TTLs + agent)", len(visible))
	}
}

func TestVisibleItems_GlobalLevel(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp/project", nil)
	s.configLevel = "global"

	visible := s.visibleItems()
	// Global should include "both" items but NOT "project"-only items
	for _, item := range visible {
		if item.level == "project" {
			t.Errorf("global level should not include project-only item: %s", item.key)
		}
	}
}

// --- cycleSetting tests ---

func TestCycleSetting_Forward(t *testing.T) {
	s := &Settings{
		items: []settingItem{
			{
				key:       "test",
				kind:      settingCycle,
				options:   []string{"/path/a", "/path/b", "/path/c"},
				optionIdx: 0,
				value:     "/path/a",
			},
		},
	}

	s.cycleSetting(0, 1)
	if s.items[0].optionIdx != 1 {
		t.Errorf("optionIdx = %d, want 1", s.items[0].optionIdx)
	}
	if s.items[0].value != "/path/b" {
		t.Errorf("value = %q, want %q", s.items[0].value, "/path/b")
	}
}

func TestCycleSetting_Backward(t *testing.T) {
	s := &Settings{
		items: []settingItem{
			{
				key:       "test",
				kind:      settingCycle,
				options:   []string{"/path/a", "/path/b", "/path/c"},
				optionIdx: 0,
				value:     "/path/a",
			},
		},
	}

	s.cycleSetting(0, -1)
	// Should wrap to last
	if s.items[0].optionIdx != 2 {
		t.Errorf("optionIdx = %d, want 2 (wrapped)", s.items[0].optionIdx)
	}
	if s.items[0].value != "/path/c" {
		t.Errorf("value = %q, want %q", s.items[0].value, "/path/c")
	}
}

func TestCycleSetting_WrapForward(t *testing.T) {
	s := &Settings{
		items: []settingItem{
			{
				key:       "test",
				kind:      settingCycle,
				options:   []string{"/path/a", "/path/b"},
				optionIdx: 1,
				value:     "/path/b",
			},
		},
	}

	s.cycleSetting(0, 1)
	if s.items[0].optionIdx != 0 {
		t.Errorf("optionIdx = %d, want 0 (wrapped forward)", s.items[0].optionIdx)
	}
}

func TestCycleSetting_NonCycleItemIgnored(t *testing.T) {
	s := &Settings{
		items: []settingItem{
			{key: "int_val", kind: settingInt, value: "42"},
		},
	}
	s.cycleSetting(0, 1)
	if s.items[0].value != "42" {
		t.Error("non-cycle item should not change")
	}
}

func TestCycleSetting_EmptyOptions(t *testing.T) {
	s := &Settings{
		items: []settingItem{
			{key: "empty", kind: settingCycle, options: nil},
		},
	}
	// Should not panic
	s.cycleSetting(0, 1)
}

// --- Settings navigation tests ---

func TestSettings_UpDownNavigation(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)

	// Start at cursor 0
	if s.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", s.cursor)
	}

	// Move down
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s = m.(Settings)
	if s.cursor != 1 {
		t.Errorf("cursor = %d, want 1", s.cursor)
	}

	// Move up
	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp})
	s = m.(Settings)
	if s.cursor != 0 {
		t.Errorf("cursor = %d, want 0", s.cursor)
	}

	// Up at 0 stays at 0
	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp})
	s = m.(Settings)
	if s.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (should not go negative)", s.cursor)
	}
}

// --- Settings editing tests ---

func TestSettings_EnterStartsEditing(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)

	// Move to skill_ttl_minutes (skip project_root which is readonly)
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s = m.(Settings)

	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(Settings)
	if !s.editing {
		t.Error("expected editing mode after enter on editable item")
	}
}

func TestSettings_EnterOnReadonlyDoesNothing(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	// cursor 0 is project_root which is readonly with single path

	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(Settings)
	if s.editing {
		t.Error("expected no editing on readonly item")
	}
}

func TestSettings_EditingEscCancels(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	s.cursor = 1 // Move past readonly project_root
	s.editing = true
	s.editBuffer = "999"

	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	s = m.(Settings)
	if s.editing {
		t.Error("editing should be false after esc")
	}
}

func TestSettings_EditingEnterCommits(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	s.cursor = 1 // skill_ttl_minutes
	s.editing = true
	s.editBuffer = "90"
	s.editCurPos = 2

	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(Settings)
	if s.editing {
		t.Error("editing should be false after enter")
	}
	// Verify the value was committed
	result := s.buildConfig()
	if result.SkillTTLMinutes != 90 {
		t.Errorf("SkillTTLMinutes = %d, want 90", result.SkillTTLMinutes)
	}
}

func TestSettings_EditingInvalidIntRejects(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	s.cursor = 1 // skill_ttl_minutes (settingInt)
	s.editing = true
	s.editBuffer = "not-a-number"
	s.editCurPos = 12

	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(Settings)
	if s.editing {
		t.Error("editing should exit even on invalid int")
	}
	// Value should NOT have been committed (original value preserved)
	result := s.buildConfig()
	if result.SkillTTLMinutes != 60 {
		t.Errorf("SkillTTLMinutes = %d, want 60 (invalid should not commit)", result.SkillTTLMinutes)
	}
}

// --- Settings level switching ---

func TestSettings_SwitchToGlobal(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	s.cursor = 2 // Non-zero cursor

	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	s = m.(Settings)
	if s.configLevel != "global" {
		t.Errorf("configLevel = %q, want %q", s.configLevel, "global")
	}
	if s.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (should reset on level switch)", s.cursor)
	}
}

func TestSettings_SwitchToProject(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	s.configLevel = "global"

	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	s = m.(Settings)
	if s.configLevel != "project" {
		t.Errorf("configLevel = %q, want %q", s.configLevel, "project")
	}
}

// --- buildDoneMsg tests ---

func TestBuildDoneMsg_NoPathChange(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp/project", nil)

	msg := s.buildDoneMsg().(settingsDoneMsg)
	if msg.config == nil {
		t.Error("config should not be nil")
	}
	if msg.preferredPath != "" {
		t.Errorf("preferredPath = %q, want empty (no path change)", msg.preferredPath)
	}
}

func TestBuildDoneMsg_WithPathChange(t *testing.T) {
	paths := []string{"/path/a", "/path/b", "/path/c"}
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/path/a", paths)

	// Cycle to a different path
	for i, item := range s.items {
		if item.key == "project_root" {
			s.items[i].optionIdx = 2
			s.items[i].value = "/path/c"
			break
		}
	}

	msg := s.buildDoneMsg().(settingsDoneMsg)
	if msg.preferredPath != "/path/c" {
		t.Errorf("preferredPath = %q, want %q", msg.preferredPath, "/path/c")
	}
}

// --- NewSettings with multiple paths ---

func TestNewSettings_MultiPathCreatesDropdown(t *testing.T) {
	paths := []string{"/path/a", "/path/b"}
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/path/a", paths)

	// First item should be a cycle item for project root
	if s.items[0].key != "project_root" {
		t.Errorf("first item key = %q, want %q", s.items[0].key, "project_root")
	}
	if s.items[0].kind != settingCycle {
		t.Error("project_root with multiple paths should be settingCycle")
	}
	if len(s.items[0].options) != 2 {
		t.Errorf("project_root has %d options, want 2", len(s.items[0].options))
	}
}

func TestNewSettings_SinglePathIsReadonly(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/single/path", nil)

	if !s.items[0].readonly {
		t.Error("project_root with single path should be readonly")
	}
}

// --- Editing: backspace, cursor movement ---

func TestSettings_EditingBackspace(t *testing.T) {
	s := Settings{
		editing:    true,
		editBuffer: "hello",
		editCurPos: 5,
	}

	m, _ := s.updateEditing(tea.KeyMsg{Type: tea.KeyBackspace})
	s = m.(Settings)
	if s.editBuffer != "hell" {
		t.Errorf("editBuffer = %q, want %q", s.editBuffer, "hell")
	}
	if s.editCurPos != 4 {
		t.Errorf("editCurPos = %d, want 4", s.editCurPos)
	}
}

func TestSettings_EditingLeftRight(t *testing.T) {
	s := Settings{
		editing:    true,
		editBuffer: "hello",
		editCurPos: 3,
	}

	m, _ := s.updateEditing(tea.KeyMsg{Type: tea.KeyLeft})
	s = m.(Settings)
	if s.editCurPos != 2 {
		t.Errorf("editCurPos = %d, want 2 after left", s.editCurPos)
	}

	m, _ = s.updateEditing(tea.KeyMsg{Type: tea.KeyRight})
	s = m.(Settings)
	if s.editCurPos != 3 {
		t.Errorf("editCurPos = %d, want 3 after right", s.editCurPos)
	}
}

func TestSettings_EditingCtrlA(t *testing.T) {
	s := Settings{
		editing:    true,
		editBuffer: "hello",
		editCurPos: 3,
	}

	m, _ := s.updateEditing(tea.KeyMsg{Type: tea.KeyCtrlA})
	s = m.(Settings)
	if s.editCurPos != 0 {
		t.Errorf("editCurPos = %d, want 0 after ctrl+a", s.editCurPos)
	}
}

func TestSettings_EditingCtrlE(t *testing.T) {
	s := Settings{
		editing:    true,
		editBuffer: "hello",
		editCurPos: 0,
	}

	m, _ := s.updateEditing(tea.KeyMsg{Type: tea.KeyCtrlE})
	s = m.(Settings)
	if s.editCurPos != 5 {
		t.Errorf("editCurPos = %d, want 5 after ctrl+e", s.editCurPos)
	}
}

func TestSettings_EditingCharInsertion(t *testing.T) {
	s := Settings{
		editing:    true,
		editBuffer: "hllo",
		editCurPos: 1,
	}

	m, _ := s.updateEditing(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	s = m.(Settings)
	if s.editBuffer != "hello" {
		t.Errorf("editBuffer = %q, want %q", s.editBuffer, "hello")
	}
	if s.editCurPos != 2 {
		t.Errorf("editCurPos = %d, want 2", s.editCurPos)
	}
}

// --- visibleToRealIndex tests ---

func TestVisibleToRealIndex_ProjectLevel(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp/project", nil)
	s.configLevel = "project"

	visible := s.visibleItems()
	for i := range visible {
		realIdx := s.visibleToRealIndex(i)
		if realIdx < 0 || realIdx >= len(s.items) {
			t.Errorf("visibleToRealIndex(%d) = %d, out of range [0, %d)", i, realIdx, len(s.items))
		}
		// Verify the mapped item matches
		if s.items[realIdx].key != visible[i].key {
			t.Errorf("visibleToRealIndex(%d): item key %q != visible key %q",
				i, s.items[realIdx].key, visible[i].key)
		}
	}
}

func TestVisibleToRealIndex_GlobalLevelSkipsProjectOnly(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp/project", nil)
	s.configLevel = "global"

	visible := s.visibleItems()
	// Global mode should not include project_root
	for _, item := range visible {
		if item.level == "project" {
			t.Errorf("global level should not include project-only item %q", item.key)
		}
	}

	// Index 0 in visible should map to the first "both" item in real items
	realIdx := s.visibleToRealIndex(0)
	if s.items[realIdx].level != "both" {
		t.Errorf("first visible global item maps to level=%q, want %q", s.items[realIdx].level, "both")
	}
}

// --- Settings navigation: down at max stays at max ---

func TestSettings_DownBeyondMaxStays(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)

	// Navigate to last item
	visible := s.visibleItems()
	for i := 0; i < len(visible)+2; i++ {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyDown})
		s = m.(Settings)
	}
	if s.cursor != len(visible)-1 {
		t.Errorf("cursor = %d, want %d (should not exceed last visible)", s.cursor, len(visible)-1)
	}
}

// --- Settings: enter on cycle item cycles ---

func TestSettings_EnterOnCycleItemCycles(t *testing.T) {
	paths := []string{"/path/a", "/path/b", "/path/c"}
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/path/a", paths)
	// cursor 0 is project_root which is a cycle item with multiple paths

	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(Settings)
	// Enter on cycle should cycle forward, not start editing
	if s.editing {
		t.Error("enter on cycle item should not start editing mode")
	}
	if s.items[0].value == "/path/a" {
		// Value should have changed after cycling
		t.Error("expected value to change after enter on cycle item")
	}
}

// --- Settings: left/right on cycle item ---

func TestSettings_LeftRightOnCycleItem(t *testing.T) {
	paths := []string{"/path/a", "/path/b", "/path/c"}
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/path/a", paths)
	// cursor 0 is the cycle item

	// Right should cycle forward
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRight})
	s = m.(Settings)
	if s.items[0].value != "/path/b" {
		t.Errorf("value = %q, want %q after right", s.items[0].value, "/path/b")
	}

	// Left should cycle backward
	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyLeft})
	s = m.(Settings)
	if s.items[0].value != "/path/a" {
		t.Errorf("value = %q, want %q after left", s.items[0].value, "/path/a")
	}
}

// --- Settings View: editing mode shows cursor ---

func TestSettings_ViewEditingMode(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	s.cursor = 1 // skill_ttl_minutes
	s.editing = true
	s.editBuffer = "90"
	s.editCurPos = 2

	output := s.View()
	if output == "" {
		t.Error("View should produce non-empty output in editing mode")
	}
	if !strings.Contains(output, "Editing") {
		t.Error("expected editing indicator in view")
	}
}

// --- Settings View: cycle item shows index ---

func TestSettings_ViewCycleItemShowsIndex(t *testing.T) {
	paths := []string{"/path/a", "/path/b"}
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/path/a", paths)
	// Focus on the project_root (cycle item)
	s.cursor = 0

	output := s.View()
	// When focused on a cycle item, should show index like [1/2]
	if !strings.Contains(output, "[1/2]") {
		t.Error("expected cycle index [1/2] in view for focused cycle item")
	}
}

// --- Settings: WindowSizeMsg ---

func TestSettings_WindowSizeMsg(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)

	m, _ := s.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	s = m.(Settings)
	if s.width != 120 || s.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", s.width, s.height)
	}
}

// --- Settings View tests ---

func TestSettings_ViewShowsLevel(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)
	output := s.View()

	if output == "" {
		t.Error("View should produce non-empty output")
	}
}

// --- visibleToRealIndex: out-of-range returns 0 ---

func TestVisibleToRealIndex_OutOfRange(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp/project", nil)
	s.configLevel = "project"

	// Request an index far beyond the visible items.
	realIdx := s.visibleToRealIndex(999)
	// Should return 0 as fallback when visible index is out of range.
	if realIdx != 0 {
		t.Errorf("visibleToRealIndex(999) = %d, want 0 (fallback for out of range)", realIdx)
	}
}

// --- Settings: editing backspace at position 0 ---

func TestSettings_EditingBackspaceAtStart(t *testing.T) {
	s := Settings{
		editing:    true,
		editBuffer: "hello",
		editCurPos: 0,
	}

	m, _ := s.updateEditing(tea.KeyMsg{Type: tea.KeyBackspace})
	s = m.(Settings)
	// Backspace at position 0 should not change anything.
	if s.editBuffer != "hello" {
		t.Errorf("editBuffer = %q, want %q (backspace at 0 should not change)", s.editBuffer, "hello")
	}
	if s.editCurPos != 0 {
		t.Errorf("editCurPos = %d, want 0", s.editCurPos)
	}
}

// --- Settings: left at 0, right at end ---

func TestSettings_EditingLeftAtZero(t *testing.T) {
	s := Settings{
		editing:    true,
		editBuffer: "abc",
		editCurPos: 0,
	}

	m, _ := s.updateEditing(tea.KeyMsg{Type: tea.KeyLeft})
	s = m.(Settings)
	if s.editCurPos != 0 {
		t.Errorf("editCurPos = %d, want 0 (already at start)", s.editCurPos)
	}
}

func TestSettings_EditingRightAtEnd(t *testing.T) {
	s := Settings{
		editing:    true,
		editBuffer: "abc",
		editCurPos: 3,
	}

	m, _ := s.updateEditing(tea.KeyMsg{Type: tea.KeyRight})
	s = m.(Settings)
	if s.editCurPos != 3 {
		t.Errorf("editCurPos = %d, want 3 (already at end)", s.editCurPos)
	}
}

// --- Settings: view in global mode shows only global items ---

func TestSettings_ViewGlobalMode(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "*",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp/project", nil)
	s.configLevel = "global"
	s.width = 80
	s.height = 30

	output := s.View()
	if output == "" {
		t.Error("View should produce non-empty output in global mode")
	}
	// Should show GLOBAL indicator
	if !strings.Contains(output, "GLOBAL") {
		t.Error("expected GLOBAL indicator in global mode view")
	}
}

// --- Settings: string value editing ---

func TestSettings_EditStringValue(t *testing.T) {
	cfg := &engine.GlobalConfig{
		SkillTTLMinutes: 60,
		StateTTLHours:   24,
		DefaultAgent:    "claude",
	}
	s := NewSettings(cfg, DefaultStyles(), "/tmp", nil)

	// Find the default_agent item.
	agentIdx := -1
	for i, item := range s.items {
		if item.key == "default_agent" {
			agentIdx = i
			break
		}
	}
	if agentIdx == -1 {
		t.Fatal("could not find default_agent item")
	}

	s.cursor = agentIdx
	// Enter editing mode.
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(Settings)
	if !s.editing {
		t.Fatal("expected editing mode after enter on string item")
	}

	// Clear and type new value.
	s.editBuffer = "cursor"
	s.editCurPos = 6

	// Commit.
	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(Settings)
	result := s.buildConfig()
	if result.DefaultAgent != "cursor" {
		t.Errorf("DefaultAgent = %q, want %q", result.DefaultAgent, "cursor")
	}
}
