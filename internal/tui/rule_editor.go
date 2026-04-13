// rule_editor.go implements the rule editor as ONE continuous scrollable list.
// Tools, paths (tree), and agents are all items in a single list.
// Up/down navigates everything. Space toggles. Enter expands/collapses dirs.
// ctrl+s saves. esc cancels.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
)

// Message types for communication with the App.
type ruleSubmittedMsg struct {
	rule      *engine.Rule
	ruleIndex int
}

type rulesSubmittedMsg struct {
	rules []engine.Rule
}

// itemType identifies what kind of row this is in the unified list.
type itemType int

const (
	itemSectionHeader itemType = iota // "── TOOLS ──" etc (not selectable)
	itemCheckbox                      // toggleable item (tool, path, agent)
	itemTreeDir                       // expandable directory in path tree
)

// listItem is a single row in the unified list.
type listItem struct {
	typ      itemType
	label    string // Display text
	value    string // Underlying value (tool name, glob pattern, agent name)
	section  string // "tools", "paths", "agents"
	selected bool
	indent   int      // Tree depth (0 for top-level)
	expanded bool     // For directories
	children []string // Child dir names (for lazy expansion)
}

// editorPhase tracks main vs confirm.
type editorPhase int

const (
	phaseEdit editorPhase = iota
	phaseAnother
)

// section identifies which section has keyboard focus.
type section int

const (
	sectionTools section = iota
	sectionPaths
	sectionAgents
)

// RuleEditor has three pinned sections: TOOLS, PATHS, AGENTS.
// TOOLS and AGENTS always show all items. PATHS scrolls independently.
type RuleEditor struct {
	skillName     string
	ruleIndex     int
	phase         editorPhase
	styles        Styles
	existingRules []engine.Rule
	projectRoot   string

	toolItems   []listItem
	pathItems   []listItem
	agentItems  []listItem
	focus       section    // which section has keyboard focus
	toolScroll  ScrollView // cursor within tools
	pathScroll  ScrollView // cursor + scroll within paths
	agentScroll ScrollView // cursor within agents
	height      int

	addAnother    bool
	confirmCursor int // 0=Yes, 1=No
}

// NewRuleEditor creates a new RuleEditor.
func NewRuleEditor(skillName string, existing *engine.Rule, ruleIndex int, styles Styles) RuleEditor {
	re := RuleEditor{
		skillName: skillName,
		ruleIndex: ruleIndex,
		phase:     phaseEdit,
		styles:    styles,
	}
	return re
}

// SetProjectRoot sets the project root and builds all section item lists.
// Auto-expands directories containing existing rules so they're visible.
func (re *RuleEditor) SetProjectRoot(root string) {
	re.projectRoot = root
	re.buildSections()
	re.autoExpandSelectedPaths()
}

// SetExistingRules provides the current ruleset.
func (re *RuleEditor) SetExistingRules(rules []engine.Rule) {
	re.existingRules = rules
}

// buildSections populates the three section item lists.
// Pre-selects items that already have rules for this skill.
func (re *RuleEditor) buildSections() {
	existingTools := make(map[string]bool)
	existingPaths := make(map[string]bool)
	existingAgents := make(map[string]bool)
	for _, r := range re.existingRules {
		if r.Skill == re.skillName {
			existingTools[r.Tool] = true
			// Strip **/ prefix from NormalizeGlob — tree items use relative paths
			existingPaths[strings.TrimPrefix(r.Path, "**/")] = true
			existingAgents[r.Agent] = true
		}
	}

	// Tools
	re.toolItems = nil
	for _, t := range ToolOptions {
		label := t
		if t == "*" {
			label = "* (all tools)"
		}
		re.toolItems = append(re.toolItems, listItem{
			typ: itemCheckbox, label: label, value: t, section: "tools",
			selected: existingTools[t],
		})
	}

	// Paths
	re.pathItems = nil
	re.pathItems = append(re.pathItems, listItem{
		typ: itemCheckbox, label: "** (all files)", value: "**", section: "paths",
		selected: existingPaths["**"],
	})
	pathTreeItems := re.buildPathTree()
	for i := range pathTreeItems {
		if existingPaths[pathTreeItems[i].value] {
			pathTreeItems[i].selected = true
		}
	}
	re.pathItems = append(re.pathItems, pathTreeItems...)

	// Agents
	re.agentItems = nil
	for _, a := range AgentOptions {
		label := a
		if a == "*" {
			label = "* (all agents)"
		}
		re.agentItems = append(re.agentItems, listItem{
			typ: itemCheckbox, label: label, value: a, section: "agents",
			selected: existingAgents[a],
		})
	}
}

// buildPathTree creates expandable directory entries from the project root.
func (re *RuleEditor) buildPathTree() []listItem {
	root := re.projectRoot
	if root == "" {
		root, _ = os.Getwd()
	}

	var items []listItem
	entries, err := os.ReadDir(root)
	if err != nil {
		return items
	}

	var dirNames []string
	var fileNames []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || DefaultIgnoreSet[e.Name()] {
			continue
		}
		if e.IsDir() {
			dirNames = append(dirNames, e.Name())
		} else {
			fileNames = append(fileNames, e.Name())
		}
	}
	sort.Strings(dirNames)
	sort.Strings(fileNames)

	// Directories first (expandable)
	for _, name := range dirNames {
		var children []string
		subEntries, err := os.ReadDir(filepath.Join(root, name))
		if err == nil {
			for _, se := range subEntries {
				if se.IsDir() && !DefaultIgnoreSet[se.Name()] && !strings.HasPrefix(se.Name(), ".") {
					children = append(children, se.Name())
				}
			}
		}
		sort.Strings(children)

		items = append(items, listItem{
			typ:      itemTreeDir,
			label:    name,
			value:    name + "/**",
			section:  "paths",
			indent:   0,
			children: children,
		})
	}

	// Files after directories
	for _, name := range fileNames {
		items = append(items, listItem{
			typ:     itemCheckbox,
			label:   name,
			value:   name,
			section: "paths",
			indent:  0,
		})
	}

	return items
}

// SwitchToConfirm transitions to the "add another?" phase.
func (re *RuleEditor) SwitchToConfirm() tea.Cmd {
	re.phase = phaseAnother
	re.addAnother = true
	re.confirmCursor = 0
	return nil
}

// Init returns the initial command.
func (re RuleEditor) Init() tea.Cmd {
	if len(re.toolItems) == 0 {
		re.buildSections()
	}
	re.focus = sectionTools
	re.toolScroll.Cursor = 0
	return nil
}

// Update handles messages.
func (re RuleEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		re.height = msg.Height
		return re, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return re, tea.Quit
		}

		if re.phase == phaseAnother {
			return re.updateConfirm(msg)
		}
		return re.updateEdit(msg)
	}
	return re, nil
}

// activeItems returns the item list for the focused section.
func (re *RuleEditor) activeItems() *[]listItem {
	switch re.focus {
	case sectionTools:
		return &re.toolItems
	case sectionPaths:
		return &re.pathItems
	case sectionAgents:
		return &re.agentItems
	}
	return &re.toolItems
}

// activeScroll returns the ScrollView for the focused section.
func (re *RuleEditor) activeScroll() *ScrollView {
	switch re.focus {
	case sectionTools:
		return &re.toolScroll
	case sectionPaths:
		return &re.pathScroll
	case sectionAgents:
		return &re.agentScroll
	}
	return &re.toolScroll
}

func (re RuleEditor) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := re.activeItems()
	sv := re.activeScroll()
	noSkip := func(int) bool { return false }

	switch msg.String() {
	case "esc":
		return re, func() tea.Msg {
			return ruleEditorDoneMsg{rule: nil, ruleIndex: re.ruleIndex}
		}

	case "up", "k":
		if sv.Cursor > 0 {
			sv.MoveUp(len(*items), noSkip)
		} else {
			// At top of section — move to previous section
			if re.focus > sectionTools {
				re.focus--
				newItems := re.activeItems()
				newSv := re.activeScroll()
				newSv.Cursor = len(*newItems) - 1
			}
		}
		return re, nil

	case "down", "j":
		if sv.Cursor < len(*items)-1 {
			sv.MoveDown(len(*items), noSkip)
		} else {
			// At bottom of section — move to next section
			if re.focus < sectionAgents {
				re.focus++
				re.activeScroll().Cursor = 0
			}
		}
		return re, nil

	case "tab":
		re.focus = (re.focus + 1) % 3
		return re, nil

	case "shift+tab":
		re.focus = (re.focus + 2) % 3
		return re, nil

	case " ", "x":
		if sv.Cursor >= 0 && sv.Cursor < len(*items) {
			item := &(*items)[sv.Cursor]
			item.selected = !item.selected
		}
		return re, nil

	case "enter", "right", "l":
		if re.focus == sectionPaths && sv.Cursor >= 0 && sv.Cursor < len(*items) {
			item := &(*items)[sv.Cursor]
			if item.typ == itemTreeDir && !item.expanded {
				re.expandPathDir(sv.Cursor)
			}
		}
		return re, nil

	case "left", "h":
		if re.focus == sectionPaths && sv.Cursor >= 0 && sv.Cursor < len(*items) {
			item := &(*items)[sv.Cursor]
			if item.typ == itemTreeDir && item.expanded {
				re.collapsePathDir(sv.Cursor)
			} else if item.indent > 0 {
				for i := sv.Cursor - 1; i >= 0; i-- {
					if re.pathItems[i].typ == itemTreeDir && re.pathItems[i].indent < item.indent {
						sv.Cursor = i
						break
					}
				}
			}
		}
		return re, nil

	case "ctrl+s", "s":
		tools := re.selectedFrom(re.toolItems)
		paths := re.selectedFrom(re.pathItems)
		agents := re.selectedFrom(re.agentItems)

		if len(tools) == 0 || len(paths) == 0 || len(agents) == 0 {
			return re, nil
		}

		var rules []engine.Rule
		for _, tool := range tools {
			for _, path := range paths {
				for _, agent := range agents {
					rules = append(rules, engine.Rule{
						Tool:  tool,
						Path:  path,
						Skill: re.skillName,
						Agent: agent,
					})
				}
			}
		}
		return re, func() tea.Msg {
			return rulesSubmittedMsg{rules: rules}
		}
	}
	return re, nil
}

func (re RuleEditor) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if re.confirmCursor > 0 {
			re.confirmCursor--
		}
		return re, nil
	case "down", "j":
		if re.confirmCursor < 1 {
			re.confirmCursor++
		}
		return re, nil
	case "enter", " ":
		if re.confirmCursor == 0 {
			re.phase = phaseEdit
			re.deselectAll()
			re.focus = sectionTools
			re.toolScroll.Cursor = 0
			return re, nil
		}
		return re, func() tea.Msg {
			return ruleEditorDoneMsg{rule: nil, ruleIndex: -1}
		}
	case "y", "Y":
		re.phase = phaseEdit
		re.deselectAll()
		re.focus = sectionTools
		re.toolScroll.Cursor = 0
		return re, nil
	case "n", "N", "esc":
		return re, func() tea.Msg {
			return ruleEditorDoneMsg{rule: nil, ruleIndex: -1}
		}
	}
	return re, nil
}

// selectedFrom returns selected values from an item list.
func (re *RuleEditor) selectedFrom(items []listItem) []string {
	var vals []string
	for _, item := range items {
		if item.selected {
			vals = append(vals, item.value)
		}
	}
	return vals
}

// deselectAll clears all selections in all sections.
func (re *RuleEditor) deselectAll() {
	for i := range re.toolItems {
		re.toolItems[i].selected = false
	}
	for i := range re.pathItems {
		re.pathItems[i].selected = false
	}
	for i := range re.agentItems {
		re.agentItems[i].selected = false
	}
}

// expandPathDir expands a directory in pathItems at the given index.
func (re *RuleEditor) expandPathDir(idx int) {
	item := &re.pathItems[idx]
	if item.expanded {
		return
	}
	item.expanded = true

	parentPath := strings.TrimSuffix(item.value, "/**")
	root := re.projectRoot
	if root == "" {
		root, _ = os.Getwd()
	}

	absDir := filepath.Join(root, parentPath)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return
	}

	var dirEntries []os.DirEntry
	var fileEntries []os.DirEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || DefaultIgnoreSet[e.Name()] {
			continue
		}
		if e.IsDir() {
			dirEntries = append(dirEntries, e)
		} else {
			fileEntries = append(fileEntries, e)
		}
	}

	var newItems []listItem
	for _, e := range dirEntries {
		childPath := parentPath + "/" + e.Name()
		var grandchildren []string
		subEntries, err := os.ReadDir(filepath.Join(root, childPath))
		if err == nil {
			for _, se := range subEntries {
				if se.IsDir() && !DefaultIgnoreSet[se.Name()] && !strings.HasPrefix(se.Name(), ".") {
					grandchildren = append(grandchildren, se.Name())
				}
			}
		}
		if len(grandchildren) > 0 {
			newItems = append(newItems, listItem{
				typ: itemTreeDir, label: e.Name(), value: childPath + "/**",
				section: "paths", indent: item.indent + 1, children: grandchildren,
			})
		} else {
			newItems = append(newItems, listItem{
				typ: itemCheckbox, label: e.Name() + "/", value: childPath + "/**",
				section: "paths", indent: item.indent + 1,
			})
		}
	}
	for _, e := range fileEntries {
		childPath := parentPath + "/" + e.Name()
		newItems = append(newItems, listItem{
			typ: itemCheckbox, label: e.Name(), value: childPath,
			section: "paths", indent: item.indent + 1,
		})
	}

	if len(newItems) == 0 {
		return
	}
	after := make([]listItem, len(re.pathItems[idx+1:]))
	copy(after, re.pathItems[idx+1:])
	re.pathItems = append(re.pathItems[:idx+1], newItems...)
	re.pathItems = append(re.pathItems, after...)
}

// collapsePathDir removes child items from pathItems after the given index.
func (re *RuleEditor) collapsePathDir(idx int) {
	item := &re.pathItems[idx]
	if !item.expanded {
		return
	}
	item.expanded = false
	removeStart := idx + 1
	removeEnd := removeStart
	for removeEnd < len(re.pathItems) && re.pathItems[removeEnd].indent > item.indent {
		removeEnd++
	}
	re.pathItems = append(re.pathItems[:removeStart], re.pathItems[removeEnd:]...)
}

// autoExpandSelectedPaths expands directories to reveal pre-selected paths.
// Called after buildSections to ensure existing rules are visible.
func (re *RuleEditor) autoExpandSelectedPaths() {
	// Collect existing paths, stripping the **/ prefix that NormalizeGlob adds.
	// Rules store "**/tests/**" but tree items use "tests/**".
	existingPaths := make(map[string]bool)
	for _, r := range re.existingRules {
		if r.Skill == re.skillName {
			path := r.Path
			path = strings.TrimPrefix(path, "**/")
			existingPaths[path] = true
		}
	}
	if len(existingPaths) == 0 {
		return
	}

	// Iterate and expand any directory whose children include selected paths.
	// Repeat until no more expansions needed (handles nested paths).
	for round := 0; round < 10; round++ {
		expanded := false
		for i := 0; i < len(re.pathItems); i++ {
			item := re.pathItems[i]
			if item.typ != itemTreeDir || item.expanded {
				continue
			}
			prefix := strings.TrimSuffix(item.value, "/**")
			for path := range existingPaths {
				if strings.HasPrefix(path, prefix+"/") && path != item.value {
					re.expandPathDir(i)
					expanded = true
					break
				}
			}
		}
		if !expanded {
			break
		}
	}

	// Mark matching items as selected
	for i := range re.pathItems {
		if existingPaths[re.pathItems[i].value] {
			re.pathItems[i].selected = true
		}
	}
}

// pathViewportHeight returns how many path rows fit on screen.
// TOOLS and AGENTS are always fully visible; PATHS gets the rest.
func (re *RuleEditor) pathViewportHeight() int {
	// header(1) + summary(1) + blank(1) = 3
	// tools header(1) + tools items + blank
	// paths header(1)
	// agents header(1) + agents items
	// help(1) + blank(1) = 2
	fixed := 3 + 1 + len(re.toolItems) + 1 + 1 + 1 + len(re.agentItems) + 2
	available := re.height - fixed
	if available < 3 {
		available = 3
	}
	return available
}

// View renders three pinned sections: TOOLS (full), PATHS (scrollable), AGENTS (full).
func (re RuleEditor) View() string {
	if re.phase == phaseAnother {
		return re.viewConfirm()
	}

	header := re.styles.Header.Render(" Rules for: " + re.skillName + " ")

	tools := re.selectedFrom(re.toolItems)
	paths := re.selectedFrom(re.pathItems)
	agents := re.selectedFrom(re.agentItems)
	count := len(tools) * len(paths) * len(agents)

	summaryStyle := re.styles.Description
	if count > 0 {
		summaryStyle = re.styles.Success
	}
	summary := summaryStyle.Render(fmt.Sprintf(
		"  %d tool(s) × %d path(s) × %d agent(s) = %d rule(s)",
		len(tools), len(paths), len(agents), count,
	))

	check := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))
	uncheck := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	treeArrow := lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))
	activeSec := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1F2937")).Background(lipgloss.Color("#A78BFA"))

	renderItem := func(item listItem, focused bool) string {
		indent := strings.Repeat("  ", item.indent)
		var line string
		switch item.typ {
		case itemCheckbox:
			box := uncheck.Render("[ ]")
			if item.selected {
				box = check.Render("[x]")
			}
			line = "  " + indent + box + " " + item.label
		case itemTreeDir:
			box := uncheck.Render("[ ]")
			if item.selected {
				box = check.Render("[x]")
			}
			arrow := treeArrow.Render("▸")
			if item.expanded {
				arrow = treeArrow.Render("▾")
			}
			line = "  " + indent + box + " " + arrow + " " + item.label + "/"
		}
		if focused {
			line = re.styles.Selected.Render(stripAnsi(line))
		}
		return line
	}

	var b strings.Builder

	// ── TOOLS (always fully visible) ──
	if re.focus == sectionTools {
		b.WriteString("  " + activeSec.Render(" TOOLS ") + "\n")
	} else {
		b.WriteString("  " + sectionStyle.Render("── TOOLS ──") + "\n")
	}
	for i, item := range re.toolItems {
		focused := re.focus == sectionTools && i == re.toolScroll.Cursor
		b.WriteString(renderItem(item, focused) + "\n")
	}

	// ── PATHS (scrollable) ──
	if re.focus == sectionPaths {
		b.WriteString("  " + activeSec.Render(" PATHS ") + "\n")
	} else {
		b.WriteString("  " + sectionStyle.Render("── PATHS ──") + "\n")
	}
	pathVP := re.pathViewportHeight()
	pathStart, pathEnd := re.pathScroll.VisibleRange(len(re.pathItems), pathVP)
	for i := pathStart; i < pathEnd; i++ {
		focused := re.focus == sectionPaths && i == re.pathScroll.Cursor
		b.WriteString(renderItem(re.pathItems[i], focused) + "\n")
	}
	if len(re.pathItems) > pathVP {
		indicator := re.styles.Description.Render(
			fmt.Sprintf("  [%d/%d]", re.pathScroll.Cursor+1, len(re.pathItems)))
		if pathStart > 0 {
			indicator += re.styles.Description.Render(" ↑")
		}
		if pathEnd < len(re.pathItems) {
			indicator += re.styles.Description.Render(" ↓")
		}
		b.WriteString(indicator + "\n")
	}

	// ── AGENTS (always fully visible) ──
	if re.focus == sectionAgents {
		b.WriteString("  " + activeSec.Render(" AGENTS ") + "\n")
	} else {
		b.WriteString("  " + sectionStyle.Render("── AGENTS ──") + "\n")
	}
	for i, item := range re.agentItems {
		focused := re.focus == sectionAgents && i == re.agentScroll.Cursor
		b.WriteString(renderItem(item, focused) + "\n")
	}

	helpStyle := re.styles.Help
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	help := keyStyle.Render("↑↓") + helpStyle.Render(" navigate") + "  " +
		keyStyle.Render("tab") + helpStyle.Render(" section") + "  " +
		keyStyle.Render("space") + helpStyle.Render(" toggle") + "  " +
		keyStyle.Render("←→") + helpStyle.Render(" expand/collapse") + "  " +
		keyStyle.Render("s") + helpStyle.Render(" save") + "  " +
		keyStyle.Render("esc") + helpStyle.Render(" cancel")

	return header + "\n" + summary + "\n\n" + b.String() + "\n" + help
}

// viewConfirm renders the "add another?" screen.
func (re RuleEditor) viewConfirm() string {
	header := re.styles.Header.Render(" Rules for: " + re.skillName + " ")

	tools := re.selectedFrom(re.toolItems)
	paths := re.selectedFrom(re.pathItems)
	agents := re.selectedFrom(re.agentItems)
	count := len(tools) * len(paths) * len(agents)

	done := re.styles.Success.Render(fmt.Sprintf("  ✓ %d rule(s) added!", count))

	yes := "    Yes, add more"
	no := "    No, back to dashboard"
	if re.confirmCursor == 0 {
		yes = re.styles.Selected.Render("  > Yes, add more")
	} else {
		no = re.styles.Selected.Render("  > No, back to dashboard")
	}

	return header + "\n\n" + done + "\n\n  Add more rules for " +
		re.styles.SkillName.Render(re.skillName) + "?\n\n" + yes + "\n" + no + "\n"
}

// Result returns the currently configured rules (for testing).
func (re RuleEditor) Result() []engine.Rule {
	tools := re.selectedFrom(re.toolItems)
	paths := re.selectedFrom(re.pathItems)
	agents := re.selectedFrom(re.agentItems)
	var rules []engine.Rule
	for _, tool := range tools {
		for _, path := range paths {
			for _, agent := range agents {
				rules = append(rules, engine.Rule{
					Tool:  tool,
					Path:  path,
					Skill: re.skillName,
					Agent: agent,
				})
			}
		}
	}
	return rules
}
