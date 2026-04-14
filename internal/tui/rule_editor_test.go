// rule_editor_test.go tests the rule editor: buildSections, selectedFrom,
// autoExpandSelectedPaths, navigation between sections, and the confirm phase.
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Blue-Bear-Security/care-bear/internal/engine"
)

// --- selectedFrom tests ---

func TestSelectedFrom_NoSelections(t *testing.T) {
	re := &RuleEditor{}
	items := []listItem{
		{typ: itemCheckbox, value: "Edit", selected: false},
		{typ: itemCheckbox, value: "Write", selected: false},
	}
	result := re.selectedFrom(items)
	if len(result) != 0 {
		t.Errorf("got %d selections, want 0", len(result))
	}
}

func TestSelectedFrom_SomeSelected(t *testing.T) {
	re := &RuleEditor{}
	items := []listItem{
		{typ: itemCheckbox, value: "Edit", selected: true},
		{typ: itemCheckbox, value: "Write", selected: false},
		{typ: itemCheckbox, value: "Bash", selected: true},
	}
	result := re.selectedFrom(items)
	if len(result) != 2 {
		t.Fatalf("got %d selections, want 2", len(result))
	}
	if result[0] != "Edit" || result[1] != "Bash" {
		t.Errorf("got %v, want [Edit, Bash]", result)
	}
}

func TestSelectedFrom_AllSelected(t *testing.T) {
	re := &RuleEditor{}
	items := []listItem{
		{typ: itemCheckbox, value: "Edit", selected: true},
		{typ: itemCheckbox, value: "Write", selected: true},
	}
	result := re.selectedFrom(items)
	if len(result) != 2 {
		t.Errorf("got %d selections, want 2", len(result))
	}
}

// --- buildSections tests ---

func TestBuildSections_CreatesToolItems(t *testing.T) {
	re := &RuleEditor{
		skillName: "test-skill",
	}
	re.buildSections()

	if len(re.toolItems) != len(ToolOptions) {
		t.Errorf("toolItems has %d items, want %d (one per ToolOption)", len(re.toolItems), len(ToolOptions))
	}

	// Verify wildcard has friendly label
	found := false
	for _, item := range re.toolItems {
		if item.value == "*" && item.label == "* (all tools)" {
			found = true
		}
	}
	if !found {
		t.Error("expected wildcard tool item with label '* (all tools)'")
	}
}

func TestBuildSections_CreatesAgentItems(t *testing.T) {
	re := &RuleEditor{
		skillName: "test-skill",
	}
	re.buildSections()

	if len(re.agentItems) != len(AgentOptions) {
		t.Errorf("agentItems has %d items, want %d", len(re.agentItems), len(AgentOptions))
	}

	found := false
	for _, item := range re.agentItems {
		if item.value == "*" && item.label == "* (all agents)" {
			found = true
		}
	}
	if !found {
		t.Error("expected wildcard agent item with label '* (all agents)'")
	}
}

func TestBuildSections_PathsHaveWildcardItem(t *testing.T) {
	re := &RuleEditor{
		skillName: "test-skill",
	}
	re.buildSections()

	if len(re.pathItems) == 0 {
		t.Fatal("expected at least the ** path item")
	}
	if re.pathItems[0].value != "**" {
		t.Errorf("first path item value = %q, want %q", re.pathItems[0].value, "**")
	}
	if re.pathItems[0].label != "** (all files)" {
		t.Errorf("first path item label = %q, want %q", re.pathItems[0].label, "** (all files)")
	}
}

func TestBuildSections_PreselectsExistingRules(t *testing.T) {
	AgentOptions = []string{"claude", "cursor", "*"}
	re := &RuleEditor{
		skillName: "my-skill",
		existingRules: []engine.Rule{
			{Tool: "Edit", Path: "**/src/**", Skill: "my-skill", Agent: "claude"},
		},
	}
	re.buildSections()

	// Check that Edit is selected in tools
	editSelected := false
	for _, item := range re.toolItems {
		if item.value == "Edit" && item.selected {
			editSelected = true
		}
	}
	if !editSelected {
		t.Error("expected Edit tool to be pre-selected from existing rules")
	}

	// Check that claude is selected in agents
	claudeSelected := false
	for _, item := range re.agentItems {
		if item.value == "claude" && item.selected {
			claudeSelected = true
		}
	}
	if !claudeSelected {
		t.Error("expected claude agent to be pre-selected from existing rules")
	}
}

func TestBuildSections_DoesNotPreselectOtherSkills(t *testing.T) {
	re := &RuleEditor{
		skillName: "my-skill",
		existingRules: []engine.Rule{
			{Tool: "Write", Path: "**", Skill: "other-skill", Agent: "*"},
		},
	}
	re.buildSections()

	for _, item := range re.toolItems {
		if item.value == "Write" && item.selected {
			t.Error("Write should not be selected -- it belongs to other-skill, not my-skill")
		}
	}
}

// --- buildSections with project root for path tree ---

func TestBuildSections_PathTreeFromProjectRoot(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some directories and files
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, "tests"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0o644)

	re := &RuleEditor{
		skillName:   "test-skill",
		projectRoot: tmpDir,
	}
	re.buildSections()

	// Should have ** plus entries from the directory
	if len(re.pathItems) < 2 {
		t.Errorf("expected more than 1 path item (** + dir entries), got %d", len(re.pathItems))
	}

	// Verify directories and files appear
	var values []string
	for _, item := range re.pathItems {
		values = append(values, item.value)
	}
	hasSrc := false
	hasMain := false
	for _, v := range values {
		if v == "src/**" {
			hasSrc = true
		}
		if v == "main.go" {
			hasMain = true
		}
	}
	if !hasSrc {
		t.Errorf("expected src/** in path items, got %v", values)
	}
	if !hasMain {
		t.Errorf("expected main.go in path items, got %v", values)
	}
}

// --- autoExpandSelectedPaths tests ---

func TestAutoExpandSelectedPaths_ExpandsParentDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure: src/handlers/main.go
	os.MkdirAll(filepath.Join(tmpDir, "src", "handlers"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "handlers", "main.go"), []byte("pkg"), 0o644)

	re := &RuleEditor{
		skillName:   "test-skill",
		projectRoot: tmpDir,
		existingRules: []engine.Rule{
			{Tool: "Edit", Path: "**/src/handlers/**", Skill: "test-skill", Agent: "*"},
		},
	}
	re.buildSections()
	re.autoExpandSelectedPaths()

	// The src directory should have been expanded to reveal src/handlers
	foundHandlers := false
	for _, item := range re.pathItems {
		if item.value == "src/handlers/**" {
			foundHandlers = true
			if !item.selected {
				t.Error("src/handlers/** should be marked as selected")
			}
		}
	}
	if !foundHandlers {
		t.Error("expected src/handlers/** to appear after auto-expansion")
	}
}

// --- Navigation between sections ---

func TestRuleEditor_TabCyclesSections(t *testing.T) {
	re := RuleEditor{
		focus:      sectionTools,
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	// Tab: tools -> paths
	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyTab})
	re = m.(RuleEditor)
	if re.focus != sectionPaths {
		t.Errorf("focus = %d, want %d (paths)", re.focus, sectionPaths)
	}

	// Tab: paths -> agents
	m, _ = re.Update(tea.KeyMsg{Type: tea.KeyTab})
	re = m.(RuleEditor)
	if re.focus != sectionAgents {
		t.Errorf("focus = %d, want %d (agents)", re.focus, sectionAgents)
	}

	// Tab: agents -> tools (wrap)
	m, _ = re.Update(tea.KeyMsg{Type: tea.KeyTab})
	re = m.(RuleEditor)
	if re.focus != sectionTools {
		t.Errorf("focus = %d, want %d (tools, wrapped)", re.focus, sectionTools)
	}
}

func TestRuleEditor_ShiftTabReverses(t *testing.T) {
	re := RuleEditor{
		focus:      sectionTools,
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	// Shift+Tab from tools -> agents (wrap backward)
	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	re = m.(RuleEditor)
	if re.focus != sectionAgents {
		t.Errorf("focus = %d, want %d (agents)", re.focus, sectionAgents)
	}
}

func TestRuleEditor_UpAtTopMovesToPreviousSection(t *testing.T) {
	re := RuleEditor{
		focus: sectionPaths,
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit"},
			{typ: itemCheckbox, value: "Write"},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**"},
		},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}
	re.pathScroll.Cursor = 0

	// Up at top of paths -> should move to tools section, cursor at last item
	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyUp})
	re = m.(RuleEditor)
	if re.focus != sectionTools {
		t.Errorf("focus = %d, want %d (tools)", re.focus, sectionTools)
	}
	if re.toolScroll.Cursor != len(re.toolItems)-1 {
		t.Errorf("toolScroll.Cursor = %d, want %d (last item)", re.toolScroll.Cursor, len(re.toolItems)-1)
	}
}

func TestRuleEditor_DownAtBottomMovesToNextSection(t *testing.T) {
	re := RuleEditor{
		focus: sectionTools,
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit"},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**"},
		},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}
	re.toolScroll.Cursor = 0 // At the last (and only) item

	// Down from bottom of tools -> should move to paths
	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyDown})
	re = m.(RuleEditor)
	if re.focus != sectionPaths {
		t.Errorf("focus = %d, want %d (paths)", re.focus, sectionPaths)
	}
	if re.pathScroll.Cursor != 0 {
		t.Errorf("pathScroll.Cursor = %d, want 0", re.pathScroll.Cursor)
	}
}

// --- Space toggle ---

func TestRuleEditor_SpaceTogglesSelection(t *testing.T) {
	re := RuleEditor{
		focus: sectionTools,
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", selected: false},
			{typ: itemCheckbox, value: "Write", selected: false},
		},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	// Space should toggle first tool
	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	re = m.(RuleEditor)
	if !re.toolItems[0].selected {
		t.Error("expected first tool to be selected after space")
	}

	// Space again should deselect
	m, _ = re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	re = m.(RuleEditor)
	if re.toolItems[0].selected {
		t.Error("expected first tool to be deselected after second space")
	}
}

// --- Submit (ctrl+s) ---

func TestRuleEditor_SubmitEmptySelectionsDoesNothing(t *testing.T) {
	re := RuleEditor{
		focus:     sectionTools,
		skillName: "test-skill",
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", selected: false},
		},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**", selected: false}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude", selected: false}},
	}

	_, cmd := re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd != nil {
		t.Error("expected no command when selections are empty")
	}
}

func TestRuleEditor_SubmitWithSelectionsReturnsRules(t *testing.T) {
	re := RuleEditor{
		focus:     sectionTools,
		skillName: "test-skill",
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", selected: true},
		},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**", selected: true}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude", selected: true}},
	}

	_, cmd := re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("expected command when all sections have selections")
	}
	msg := cmd()
	submitted, ok := msg.(rulesSubmittedMsg)
	if !ok {
		t.Fatalf("expected rulesSubmittedMsg, got %T", msg)
	}
	if len(submitted.rules) != 1 {
		t.Fatalf("got %d rules, want 1 (1 tool x 1 path x 1 agent)", len(submitted.rules))
	}
	r := submitted.rules[0]
	if r.Tool != "Edit" || r.Path != "**" || r.Agent != "claude" || r.Skill != "test-skill" {
		t.Errorf("rule = %+v, want Edit/**/claude/test-skill", r)
	}
}

func TestRuleEditor_SubmitCartesianProduct(t *testing.T) {
	re := RuleEditor{
		focus:     sectionTools,
		skillName: "test-skill",
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", selected: true},
			{typ: itemCheckbox, value: "Write", selected: true},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**", selected: true},
			{typ: itemCheckbox, value: "src/**", selected: true},
		},
		agentItems: []listItem{
			{typ: itemCheckbox, value: "claude", selected: true},
		},
	}

	_, cmd := re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("expected command")
	}
	msg := cmd()
	submitted := msg.(rulesSubmittedMsg)
	// 2 tools x 2 paths x 1 agent = 4 rules
	if len(submitted.rules) != 4 {
		t.Errorf("got %d rules, want 4 (2 tools x 2 paths x 1 agent)", len(submitted.rules))
	}
}

// --- Esc cancels ---

func TestRuleEditor_EscCancels(t *testing.T) {
	re := RuleEditor{
		focus:      sectionTools,
		ruleIndex:  -1,
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	_, cmd := re.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command on esc")
	}
	msg := cmd()
	done, ok := msg.(ruleEditorDoneMsg)
	if !ok {
		t.Fatalf("expected ruleEditorDoneMsg, got %T", msg)
	}
	if done.rule != nil {
		t.Error("expected nil rule on cancel")
	}
}

// --- Confirm phase ---

func TestRuleEditor_ConfirmPhaseYes(t *testing.T) {
	re := RuleEditor{
		phase:         phaseAnother,
		confirmCursor: 0, // Yes
		toolItems:     []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:     []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems:    []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyEnter})
	re = m.(RuleEditor)
	if re.phase != phaseEdit {
		t.Errorf("phase = %d, want %d (edit) after selecting Yes", re.phase, phaseEdit)
	}
}

func TestRuleEditor_ConfirmPhaseNo(t *testing.T) {
	re := RuleEditor{
		phase:         phaseAnother,
		confirmCursor: 1, // No
		toolItems:     []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:     []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems:    []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	_, cmd := re.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command on No in confirm")
	}
	msg := cmd()
	if _, ok := msg.(ruleEditorDoneMsg); !ok {
		t.Errorf("expected ruleEditorDoneMsg, got %T", msg)
	}
}

func TestRuleEditor_ConfirmPhaseYKey(t *testing.T) {
	re := RuleEditor{
		phase:      phaseAnother,
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	re = m.(RuleEditor)
	if re.phase != phaseEdit {
		t.Error("pressing 'y' in confirm phase should return to edit")
	}
}

func TestRuleEditor_ConfirmPhaseNKey(t *testing.T) {
	re := RuleEditor{
		phase:      phaseAnother,
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	_, cmd := re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Fatal("expected command on 'n' in confirm phase")
	}
}

// --- Result ---

func TestRuleEditor_Result(t *testing.T) {
	re := RuleEditor{
		skillName: "my-skill",
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", selected: true},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**", selected: true},
		},
		agentItems: []listItem{
			{typ: itemCheckbox, value: "*", selected: true},
		},
	}

	rules := re.Result()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Skill != "my-skill" {
		t.Errorf("Skill = %q, want %q", rules[0].Skill, "my-skill")
	}
}

func TestRuleEditor_Result_EmptyWhenNothingSelected(t *testing.T) {
	re := RuleEditor{
		skillName:  "my-skill",
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit", selected: false}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**", selected: false}},
		agentItems: []listItem{{typ: itemCheckbox, value: "*", selected: false}},
	}
	rules := re.Result()
	if len(rules) != 0 {
		t.Errorf("got %d rules, want 0 when nothing selected", len(rules))
	}
}

// --- deselectAll ---

func TestRuleEditor_DeselectAll(t *testing.T) {
	re := &RuleEditor{
		toolItems:  []listItem{{selected: true}, {selected: true}},
		pathItems:  []listItem{{selected: true}},
		agentItems: []listItem{{selected: true}},
	}
	re.deselectAll()

	for _, item := range re.toolItems {
		if item.selected {
			t.Error("expected all tools deselected")
		}
	}
	for _, item := range re.pathItems {
		if item.selected {
			t.Error("expected all paths deselected")
		}
	}
	for _, item := range re.agentItems {
		if item.selected {
			t.Error("expected all agents deselected")
		}
	}
}

// --- View tests ---

func TestRuleEditor_ViewEdit(t *testing.T) {
	re := RuleEditor{
		skillName: "my-skill",
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

	output := re.View()
	if !strings.Contains(output, "my-skill") {
		t.Error("expected skill name in view output")
	}
	if !strings.Contains(output, "TOOLS") {
		t.Error("expected TOOLS section header")
	}
	if !strings.Contains(output, "PATHS") {
		t.Error("expected PATHS section header")
	}
	if !strings.Contains(output, "AGENTS") {
		t.Error("expected AGENTS section header")
	}
}

func TestRuleEditor_ViewConfirm(t *testing.T) {
	re := RuleEditor{
		skillName: "my-skill",
		phase:     phaseAnother,
		styles:    DefaultStyles(),
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", label: "Edit", selected: true},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**", label: "** (all files)", selected: true},
		},
		agentItems: []listItem{
			{typ: itemCheckbox, value: "claude", label: "claude", selected: true},
		},
	}

	output := re.viewConfirm()
	if !strings.Contains(output, "rule(s) added") {
		t.Error("expected confirmation message in view")
	}
	if !strings.Contains(output, "Yes") {
		t.Error("expected Yes option in confirm view")
	}
	if !strings.Contains(output, "No") {
		t.Error("expected No option in confirm view")
	}
}

// --- activeItems / activeScroll ---

func TestRuleEditor_ActiveItems(t *testing.T) {
	re := &RuleEditor{
		toolItems:  []listItem{{value: "tool1"}},
		pathItems:  []listItem{{value: "path1"}},
		agentItems: []listItem{{value: "agent1"}},
	}

	re.focus = sectionTools
	items := re.activeItems()
	if len(*items) != 1 || (*items)[0].value != "tool1" {
		t.Error("expected toolItems for sectionTools")
	}

	re.focus = sectionPaths
	items = re.activeItems()
	if len(*items) != 1 || (*items)[0].value != "path1" {
		t.Error("expected pathItems for sectionPaths")
	}

	re.focus = sectionAgents
	items = re.activeItems()
	if len(*items) != 1 || (*items)[0].value != "agent1" {
		t.Error("expected agentItems for sectionAgents")
	}
}

func TestRuleEditor_ActiveScroll(t *testing.T) {
	re := &RuleEditor{}
	re.toolScroll.Cursor = 1
	re.pathScroll.Cursor = 2
	re.agentScroll.Cursor = 3

	re.focus = sectionTools
	if re.activeScroll().Cursor != 1 {
		t.Error("expected toolScroll for sectionTools")
	}

	re.focus = sectionPaths
	if re.activeScroll().Cursor != 2 {
		t.Error("expected pathScroll for sectionPaths")
	}

	re.focus = sectionAgents
	if re.activeScroll().Cursor != 3 {
		t.Error("expected agentScroll for sectionAgents")
	}
}

// --- collapsePathDir tests ---

func TestRuleEditor_CollapsePathDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create structure: src/ with a nested child dir
	os.MkdirAll(filepath.Join(tmpDir, "src", "handlers"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "main.go"), []byte("pkg"), 0o644)

	re := &RuleEditor{
		skillName:   "test-skill",
		projectRoot: tmpDir,
	}
	re.buildSections()

	// Find "src" directory in pathItems and expand it
	srcIdx := -1
	for i, item := range re.pathItems {
		if item.value == "src/**" && item.typ == itemTreeDir {
			srcIdx = i
			break
		}
	}
	if srcIdx == -1 {
		t.Fatal("could not find src/** in pathItems")
	}

	countBefore := len(re.pathItems)
	re.expandPathDir(srcIdx)
	countAfterExpand := len(re.pathItems)

	if countAfterExpand <= countBefore {
		t.Fatalf("expand should add items: before=%d, after=%d", countBefore, countAfterExpand)
	}

	// Now collapse
	re.collapsePathDir(srcIdx)
	countAfterCollapse := len(re.pathItems)

	if countAfterCollapse != countBefore {
		t.Errorf("after collapse: got %d items, want %d (same as before expand)", countAfterCollapse, countBefore)
	}

	// The src item should no longer be expanded
	if re.pathItems[srcIdx].expanded {
		t.Error("src should not be expanded after collapse")
	}
}

func TestRuleEditor_CollapseAlreadyCollapsed(t *testing.T) {
	re := &RuleEditor{
		pathItems: []listItem{
			{typ: itemTreeDir, value: "src/**", expanded: false, indent: 0},
		},
	}

	// Should not panic or modify anything
	re.collapsePathDir(0)
	if len(re.pathItems) != 1 {
		t.Errorf("pathItems changed unexpectedly: got %d, want 1", len(re.pathItems))
	}
}

// --- expand on already expanded directory ---

func TestRuleEditor_ExpandAlreadyExpanded(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "file.go"), []byte(""), 0o644)

	re := &RuleEditor{
		skillName:   "test-skill",
		projectRoot: tmpDir,
	}
	re.buildSections()

	srcIdx := -1
	for i, item := range re.pathItems {
		if item.value == "src/**" {
			srcIdx = i
			break
		}
	}
	if srcIdx == -1 {
		t.Fatal("could not find src/**")
	}

	re.expandPathDir(srcIdx)
	countAfterFirst := len(re.pathItems)

	// Expanding again should be a no-op
	re.expandPathDir(srcIdx)
	if len(re.pathItems) != countAfterFirst {
		t.Errorf("second expand changed item count: got %d, want %d", len(re.pathItems), countAfterFirst)
	}
}

// --- SwitchToConfirm ---

func TestRuleEditor_SwitchToConfirm(t *testing.T) {
	re := &RuleEditor{
		phase: phaseEdit,
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", selected: true},
		},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**", selected: true}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude", selected: true}},
	}

	cmd := re.SwitchToConfirm()
	if cmd != nil {
		t.Error("SwitchToConfirm should return nil cmd")
	}
	if re.phase != phaseAnother {
		t.Errorf("phase = %d, want %d (phaseAnother)", re.phase, phaseAnother)
	}
	if re.confirmCursor != 0 {
		t.Errorf("confirmCursor = %d, want 0", re.confirmCursor)
	}
}

// --- Confirm phase navigation ---

func TestRuleEditor_ConfirmUpDown(t *testing.T) {
	re := RuleEditor{
		phase:         phaseAnother,
		confirmCursor: 0,
		toolItems:     []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:     []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems:    []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	// Down moves to No
	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyDown})
	re = m.(RuleEditor)
	if re.confirmCursor != 1 {
		t.Errorf("confirmCursor = %d, want 1 after down", re.confirmCursor)
	}

	// Down at 1 stays at 1
	m, _ = re.Update(tea.KeyMsg{Type: tea.KeyDown})
	re = m.(RuleEditor)
	if re.confirmCursor != 1 {
		t.Errorf("confirmCursor = %d, want 1 (at max)", re.confirmCursor)
	}

	// Up goes back to Yes
	m, _ = re.Update(tea.KeyMsg{Type: tea.KeyUp})
	re = m.(RuleEditor)
	if re.confirmCursor != 0 {
		t.Errorf("confirmCursor = %d, want 0 after up", re.confirmCursor)
	}

	// Up at 0 stays at 0
	m, _ = re.Update(tea.KeyMsg{Type: tea.KeyUp})
	re = m.(RuleEditor)
	if re.confirmCursor != 0 {
		t.Errorf("confirmCursor = %d, want 0 (at min)", re.confirmCursor)
	}
}

// --- Confirm phase: Esc exits ---

func TestRuleEditor_ConfirmEscExits(t *testing.T) {
	re := RuleEditor{
		phase:      phaseAnother,
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	_, cmd := re.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command on esc in confirm phase")
	}
	msg := cmd()
	if _, ok := msg.(ruleEditorDoneMsg); !ok {
		t.Errorf("expected ruleEditorDoneMsg, got %T", msg)
	}
}

// --- Confirm phase: 'Y' uppercase ---

func TestRuleEditor_ConfirmYUppercase(t *testing.T) {
	re := RuleEditor{
		phase:      phaseAnother,
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	re = m.(RuleEditor)
	if re.phase != phaseEdit {
		t.Error("pressing 'Y' in confirm phase should return to edit")
	}
}

// --- Confirm phase: 'N' uppercase ---

func TestRuleEditor_ConfirmNUppercase(t *testing.T) {
	re := RuleEditor{
		phase:      phaseAnother,
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	_, cmd := re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if cmd == nil {
		t.Fatal("expected command on 'N' in confirm phase")
	}
}

// --- 'x' toggle (alias for space) ---

func TestRuleEditor_XTogglesSelection(t *testing.T) {
	re := RuleEditor{
		focus: sectionTools,
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", selected: false},
		},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	re = m.(RuleEditor)
	if !re.toolItems[0].selected {
		t.Error("expected 'x' to toggle selection on")
	}
}

// --- ctrl+c quits ---

func TestRuleEditor_CtrlCQuits(t *testing.T) {
	re := RuleEditor{
		focus:      sectionTools,
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	_, cmd := re.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit command from ctrl+c")
	}
}

// --- WindowSizeMsg ---

func TestRuleEditor_WindowSizeMsg(t *testing.T) {
	re := RuleEditor{
		toolItems:  []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems:  []listItem{{typ: itemCheckbox, value: "**"}},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}

	m, _ := re.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	re = m.(RuleEditor)
	if re.height != 50 {
		t.Errorf("height = %d, want 50", re.height)
	}
}

// --- Left in path section navigates to parent dir ---

func TestRuleEditor_LeftNavigatesToParentDir(t *testing.T) {
	re := RuleEditor{
		focus:     sectionPaths,
		toolItems: []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems: []listItem{
			{typ: itemTreeDir, value: "src/**", indent: 0, expanded: true},
			{typ: itemCheckbox, value: "src/main.go", indent: 1},
		},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}
	re.pathScroll.Cursor = 1 // On the child item

	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyLeft})
	re = m.(RuleEditor)
	if re.pathScroll.Cursor != 0 {
		t.Errorf("cursor = %d, want 0 (should jump to parent dir)", re.pathScroll.Cursor)
	}
}

// --- View: viewConfirm shows count ---

func TestRuleEditor_ViewConfirmShowsCount(t *testing.T) {
	re := RuleEditor{
		skillName:     "my-skill",
		phase:         phaseAnother,
		confirmCursor: 1, // No selected
		styles:        DefaultStyles(),
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", selected: true},
			{typ: itemCheckbox, value: "Write", selected: true},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**", selected: true},
		},
		agentItems: []listItem{
			{typ: itemCheckbox, value: "claude", selected: true},
		},
	}

	output := re.viewConfirm()
	if !strings.Contains(output, "2 rule(s) added") {
		t.Error("expected '2 rule(s) added' in confirm view")
	}
	// With confirmCursor=1, "No" should be highlighted
	if !strings.Contains(output, "No") {
		t.Error("expected 'No' option in confirm view")
	}
}

// --- View: edit mode shows summary ---

func TestRuleEditor_ViewEditShowsSummary(t *testing.T) {
	re := RuleEditor{
		skillName: "my-skill",
		phase:     phaseEdit,
		styles:    DefaultStyles(),
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", label: "Edit", selected: true},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**", label: "** (all files)", selected: true},
			{typ: itemTreeDir, value: "src/**", label: "src", expanded: false},
		},
		agentItems: []listItem{
			{typ: itemCheckbox, value: "claude", label: "claude", selected: true},
		},
	}

	output := re.View()
	// Should show "1 tool(s) x 1 path(s) x 1 agent(s) = 1 rule(s)"
	if !strings.Contains(output, "1 rule(s)") {
		t.Error("expected rule count summary in view")
	}
	// Should show section headers
	if !strings.Contains(output, "TOOLS") {
		t.Error("expected TOOLS section")
	}
	if !strings.Contains(output, "PATHS") {
		t.Error("expected PATHS section")
	}
	if !strings.Contains(output, "AGENTS") {
		t.Error("expected AGENTS section")
	}
}

// --- pathViewportHeight ---

func TestRuleEditor_PathViewportHeight(t *testing.T) {
	re := &RuleEditor{
		toolItems:  make([]listItem, 5),
		agentItems: make([]listItem, 3),
		height:     40,
	}

	vp := re.pathViewportHeight()
	if vp < 3 {
		t.Errorf("pathViewportHeight = %d, want >= 3", vp)
	}

	// Very small height should still return at least 3
	re.height = 5
	vp = re.pathViewportHeight()
	if vp < 3 {
		t.Errorf("pathViewportHeight with small height = %d, want >= 3", vp)
	}
}

// --- NewRuleEditor tests ---

func TestNewRuleEditor_InitializesFields(t *testing.T) {
	re := NewRuleEditor("my-skill", nil, -1, DefaultStyles())

	if re.skillName != "my-skill" {
		t.Errorf("skillName = %q, want %q", re.skillName, "my-skill")
	}
	if re.ruleIndex != -1 {
		t.Errorf("ruleIndex = %d, want -1 for new rule", re.ruleIndex)
	}
	if re.phase != phaseEdit {
		t.Errorf("phase = %d, want %d (phaseEdit)", re.phase, phaseEdit)
	}
}

func TestNewRuleEditor_WithExistingRule(t *testing.T) {
	existing := &engine.Rule{
		Tool:  "Edit",
		Path:  "**/*.go",
		Skill: "my-skill",
		Agent: "claude",
	}
	re := NewRuleEditor("my-skill", existing, 0, DefaultStyles())

	if re.ruleIndex != 0 {
		t.Errorf("ruleIndex = %d, want 0 for existing rule", re.ruleIndex)
	}
}

// --- SetProjectRoot tests ---

func TestSetProjectRoot_BuildsSectionsAndPopulatesItems(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some structure.
	os.MkdirAll(filepath.Join(tmpDir, "src", "handlers"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "main.go"), []byte("pkg"), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0o644)

	re := NewRuleEditor("test-skill", nil, -1, DefaultStyles())
	re.SetExistingRules([]engine.Rule{
		{Tool: "Edit", Path: "**/src/handlers/**", Skill: "test-skill", Agent: "*"},
	})
	re.SetProjectRoot(tmpDir)

	// Tools should be populated.
	if len(re.toolItems) == 0 {
		t.Error("expected toolItems to be populated after SetProjectRoot")
	}

	// Path items should include ** plus directory entries from tmpDir.
	if len(re.pathItems) < 2 {
		t.Errorf("expected at least 2 path items (** + dir entries), got %d", len(re.pathItems))
	}

	// Agents should be populated.
	if len(re.agentItems) == 0 {
		t.Error("expected agentItems to be populated after SetProjectRoot")
	}

	// Verify that "src/**" is present in path items (from directory listing).
	hasSrc := false
	for _, item := range re.pathItems {
		if item.value == "src/**" {
			hasSrc = true
		}
	}
	if !hasSrc {
		t.Error("expected src/** in path items from project root")
	}

	// After auto-expand, src/handlers/** should appear because the existing rule
	// **/src/handlers/** triggers expansion of the src directory.
	hasHandlers := false
	for _, item := range re.pathItems {
		if item.value == "src/handlers/**" {
			hasHandlers = true
			if !item.selected {
				t.Error("src/handlers/** should be marked as selected from existing rule")
			}
		}
	}
	if !hasHandlers {
		t.Error("expected src/handlers/** to appear after auto-expansion for existing rule")
	}
}

// --- RuleEditor: enter on directory expands, left collapses ---

func TestRuleEditor_EnterOnDirExpandsLeftCollapses(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "file.go"), []byte(""), 0o644)

	re := RuleEditor{
		focus:       sectionPaths,
		skillName:   "test-skill",
		projectRoot: tmpDir,
		styles:      DefaultStyles(),
		toolItems:   []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**", label: "** (all files)"},
			{typ: itemTreeDir, value: "src/**", label: "src/", indent: 0, expanded: false},
		},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}
	re.pathScroll.Cursor = 1 // On the directory

	// Enter on collapsed directory should expand it.
	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyEnter})
	re = m.(RuleEditor)
	if !re.pathItems[1].expanded {
		t.Error("expected directory to be expanded after enter")
	}
	countExpanded := len(re.pathItems)

	// Left on expanded directory should collapse it.
	m, _ = re.Update(tea.KeyMsg{Type: tea.KeyLeft})
	re = m.(RuleEditor)
	if re.pathItems[1].expanded {
		t.Error("expected directory to be collapsed after left")
	}
	if len(re.pathItems) >= countExpanded {
		t.Errorf("expected fewer items after collapse: got %d, had %d when expanded", len(re.pathItems), countExpanded)
	}
}

// --- RuleEditor: right on collapsed dir expands ---

func TestRuleEditor_RightExpandsDir(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "file.go"), []byte(""), 0o644)

	re := RuleEditor{
		focus:       sectionPaths,
		skillName:   "test-skill",
		projectRoot: tmpDir,
		styles:      DefaultStyles(),
		toolItems:   []listItem{{typ: itemCheckbox, value: "Edit"}},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**", label: "** (all files)"},
			{typ: itemTreeDir, value: "src/**", label: "src/", indent: 0, expanded: false},
		},
		agentItems: []listItem{{typ: itemCheckbox, value: "claude"}},
	}
	re.pathScroll.Cursor = 1 // On the directory

	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyRight})
	re = m.(RuleEditor)
	if !re.pathItems[1].expanded {
		t.Error("expected right arrow to expand directory")
	}
}

// --- RuleEditor: view with tree items renders correctly ---

func TestRuleEditor_ViewWithTreeDir(t *testing.T) {
	re := RuleEditor{
		skillName: "my-skill",
		phase:     phaseEdit,
		styles:    DefaultStyles(),
		focus:     sectionPaths,
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit", label: "Edit"},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**", label: "** (all files)"},
			{typ: itemTreeDir, value: "src/**", label: "src/", expanded: true},
			{typ: itemCheckbox, value: "src/main.go", label: "main.go", indent: 1},
		},
		agentItems: []listItem{
			{typ: itemCheckbox, value: "claude", label: "claude"},
		},
	}

	output := re.View()
	// Should show expanded directory arrow and child items
	if !strings.Contains(output, "src") {
		t.Error("expected src directory in view")
	}
	if !strings.Contains(output, "main.go") {
		t.Error("expected main.go child item in view")
	}
}

// --- RuleEditor: down at bottom of agents stays put ---

func TestRuleEditor_DownAtBottomOfAgentsStays(t *testing.T) {
	re := RuleEditor{
		focus: sectionAgents,
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit"},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**"},
		},
		agentItems: []listItem{
			{typ: itemCheckbox, value: "claude"},
		},
	}
	re.agentScroll.Cursor = 0 // At the last (and only) item

	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyDown})
	re = m.(RuleEditor)
	// At bottom of the last section, down does nothing (no wrap)
	if re.focus != sectionAgents {
		t.Errorf("focus = %d, want %d (agents, should stay at bottom)", re.focus, sectionAgents)
	}
	if re.agentScroll.Cursor != 0 {
		t.Errorf("agentScroll.Cursor = %d, want 0", re.agentScroll.Cursor)
	}
}

// --- RuleEditor: up at top of tools stays put ---

func TestRuleEditor_UpAtTopOfToolsStays(t *testing.T) {
	re := RuleEditor{
		focus: sectionTools,
		toolItems: []listItem{
			{typ: itemCheckbox, value: "Edit"},
			{typ: itemCheckbox, value: "Write"},
		},
		pathItems: []listItem{
			{typ: itemCheckbox, value: "**"},
		},
		agentItems: []listItem{
			{typ: itemCheckbox, value: "claude"},
			{typ: itemCheckbox, value: "cursor"},
		},
	}
	re.toolScroll.Cursor = 0 // At the first item

	m, _ := re.Update(tea.KeyMsg{Type: tea.KeyUp})
	re = m.(RuleEditor)
	// At top of the first section, up does nothing (no wrap)
	if re.focus != sectionTools {
		t.Errorf("focus = %d, want %d (tools, should stay at top)", re.focus, sectionTools)
	}
	if re.toolScroll.Cursor != 0 {
		t.Errorf("toolScroll.Cursor = %d, want 0 (should not change)", re.toolScroll.Cursor)
	}
}

// TestBuildPathTree_DirWithOnlyFilesIsExpandable verifies that directories
// containing only files (no subdirectories) still show as expandable tree dirs.
func TestBuildPathTree_DirWithOnlyFilesIsExpandable(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory with only files, no subdirs
	scriptsDir := filepath.Join(tmpDir, "scripts")
	_ = os.MkdirAll(scriptsDir, 0o755)
	_ = os.WriteFile(filepath.Join(scriptsDir, "deploy.sh"), []byte("#!/bin/bash"), 0o644)
	_ = os.WriteFile(filepath.Join(scriptsDir, "migrate.py"), []byte("# migration"), 0o644)

	// Create a directory with subdirs (for comparison)
	srcDir := filepath.Join(tmpDir, "src")
	_ = os.MkdirAll(filepath.Join(srcDir, "handlers"), 0o755)

	re := NewRuleEditor("test-skill", nil, -1, DefaultStyles())
	re.SetExistingRules(nil)
	re.SetProjectRoot(tmpDir)

	// Find scripts in pathItems — should be itemTreeDir (expandable), not itemCheckbox
	foundScripts := false
	for _, item := range re.pathItems {
		if item.label == "scripts" && item.section == "paths" {
			foundScripts = true
			if item.typ != itemTreeDir {
				t.Errorf("scripts/ has files but rendered as checkbox, want expandable tree dir")
			}
			break
		}
	}
	if !foundScripts {
		t.Error("scripts/ directory not found in path items")
	}

	// Expand scripts — should show the files
	for i, item := range re.pathItems {
		if item.label == "scripts" && item.typ == itemTreeDir {
			re.expandPathDir(i)
			break
		}
	}

	// Check files are visible after expand
	foundDeploy := false
	foundMigrate := false
	for _, item := range re.pathItems {
		if item.label == "deploy.sh" {
			foundDeploy = true
		}
		if item.label == "migrate.py" {
			foundMigrate = true
		}
	}
	if !foundDeploy {
		t.Error("deploy.sh not visible after expanding scripts/")
	}
	if !foundMigrate {
		t.Error("migrate.py not visible after expanding scripts/")
	}
}

// TestBuildPathTree_EmptyDirIsCheckbox verifies that truly empty directories
// render as plain checkboxes (not expandable).
func TestBuildPathTree_EmptyDirIsCheckbox(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "empty-dir"), 0o755)

	re := NewRuleEditor("test-skill", nil, -1, DefaultStyles())
	re.SetExistingRules(nil)
	re.SetProjectRoot(tmpDir)

	for _, item := range re.pathItems {
		if strings.Contains(item.label, "empty-dir") {
			if item.typ == itemTreeDir {
				t.Error("empty directory should be checkbox, not expandable tree dir")
			}
			return
		}
	}
	t.Error("empty-dir not found in path items")
}
