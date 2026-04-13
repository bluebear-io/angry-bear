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

// RuleEditor is a single continuous scrollable list.
type RuleEditor struct {
	skillName     string
	ruleIndex     int
	phase         editorPhase
	styles        Styles
	existingRules []engine.Rule
	projectRoot   string

	items     []listItem
	cursor    int
	scrollTop int
	height    int

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

// SetProjectRoot sets the project root and builds the item list.
func (re *RuleEditor) SetProjectRoot(root string) {
	re.projectRoot = root
	re.items = re.buildItems()
}

// SetExistingRules provides the current ruleset.
func (re *RuleEditor) SetExistingRules(rules []engine.Rule) {
	re.existingRules = rules
}

// buildItems creates the unified list with tools, paths, and agents.
// Pre-selects items that already have rules for this skill.
func (re *RuleEditor) buildItems() []listItem {
	// Compute what's already selected for this skill
	existingTools := make(map[string]bool)
	existingPaths := make(map[string]bool)
	existingAgents := make(map[string]bool)
	for _, r := range re.existingRules {
		if r.Skill == re.skillName {
			existingTools[r.Tool] = true
			existingPaths[r.Path] = true
			existingAgents[r.Agent] = true
		}
	}

	var items []listItem

	// ── TOOLS section ──
	items = append(items, listItem{typ: itemSectionHeader, label: "TOOLS", section: "tools"})
	for _, t := range ToolOptions {
		label := t
		if t == "*" {
			label = "* (all tools)"
		}
		items = append(items, listItem{
			typ: itemCheckbox, label: label, value: t, section: "tools",
			selected: existingTools[t],
		})
	}

	// ── PATHS section ──
	items = append(items, listItem{typ: itemSectionHeader, label: "PATHS", section: "paths"})
	items = append(items, listItem{
		typ: itemCheckbox, label: "** (all files)", value: "**", section: "paths",
		selected: existingPaths["**"],
	})
	// Add top-level directories from project as expandable tree nodes
	pathTreeItems := re.buildPathTree()
	// Pre-select any paths that match existing rules
	for i := range pathTreeItems {
		if existingPaths[pathTreeItems[i].value] {
			pathTreeItems[i].selected = true
		}
	}
	items = append(items, pathTreeItems...)

	// ── AGENTS section ──
	items = append(items, listItem{typ: itemSectionHeader, label: "AGENTS", section: "agents"})
	for _, a := range AgentOptions {
		label := a
		if a == "*" {
			label = "* (all agents)"
		}
		items = append(items, listItem{
			typ: itemCheckbox, label: label, value: a, section: "agents",
			selected: existingAgents[a],
		})
	}

	return items
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

// expandDir inserts child directory items after the given index.
// Children are discovered lazily from the filesystem and are themselves
// expandable tree dirs if they contain subdirectories.
func (re *RuleEditor) expandDir(idx int) {
	item := &re.items[idx]
	if item.expanded {
		return
	}
	item.expanded = true

	parentPath := strings.TrimSuffix(item.value, "/**")

	root := re.projectRoot
	if root == "" {
		root, _ = os.Getwd()
	}

	// Read children from filesystem
	absDir := filepath.Join(root, parentPath)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return
	}

	// Separate dirs and files
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

	// Directories first
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
				typ:      itemTreeDir,
				label:    e.Name(),
				value:    childPath + "/**",
				section:  "paths",
				indent:   item.indent + 1,
				children: grandchildren,
			})
		} else {
			// Leaf directory
			newItems = append(newItems, listItem{
				typ:     itemCheckbox,
				label:   e.Name() + "/",
				value:   childPath + "/**",
				section: "paths",
				indent:  item.indent + 1,
			})
		}
	}

	// Files after directories
	for _, e := range fileEntries {
		childPath := parentPath + "/" + e.Name()
		newItems = append(newItems, listItem{
			typ:     itemCheckbox,
			label:   e.Name(),
			value:   childPath,
			section: "paths",
			indent:  item.indent + 1,
		})
	}

	if len(newItems) == 0 {
		return
	}

	// Insert after idx
	after := make([]listItem, len(re.items[idx+1:]))
	copy(after, re.items[idx+1:])
	re.items = append(re.items[:idx+1], newItems...)
	re.items = append(re.items, after...)
}

// collapseDir removes child items after the given index.
func (re *RuleEditor) collapseDir(idx int) {
	item := &re.items[idx]
	if !item.expanded {
		return
	}
	item.expanded = false

	// Remove items after idx that have deeper indent
	removeStart := idx + 1
	removeEnd := removeStart
	for removeEnd < len(re.items) && re.items[removeEnd].indent > item.indent {
		removeEnd++
	}
	re.items = append(re.items[:removeStart], re.items[removeEnd:]...)
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
	if len(re.items) == 0 {
		re.items = re.buildItems()
	}
	// Start cursor on first selectable item (skip section header)
	re.cursor = 1
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

func (re RuleEditor) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return re, func() tea.Msg {
			return ruleEditorDoneMsg{rule: nil, ruleIndex: re.ruleIndex}
		}

	case "up", "k":
		re.moveCursor(-1)
		return re, nil

	case "down", "j":
		re.moveCursor(1)
		return re, nil

	case " ", "x":
		// Toggle selection
		if re.cursor >= 0 && re.cursor < len(re.items) {
			item := &re.items[re.cursor]
			if item.typ == itemCheckbox {
				item.selected = !item.selected
			} else if item.typ == itemTreeDir {
				// Space on a dir toggles it as a path selection
				item.selected = !item.selected
			}
		}
		return re, nil

	case "enter", "right", "l":
		// Expand directory (or no-op on non-dirs)
		if re.cursor >= 0 && re.cursor < len(re.items) {
			item := &re.items[re.cursor]
			if item.typ == itemTreeDir && !item.expanded {
				re.expandDir(re.cursor)
			}
		}
		return re, nil

	case "left", "h":
		// Collapse directory, or jump to parent
		if re.cursor >= 0 && re.cursor < len(re.items) {
			item := &re.items[re.cursor]
			if item.typ == itemTreeDir && item.expanded {
				// Collapse this dir
				re.collapseDir(re.cursor)
			} else if item.indent > 0 {
				// Jump up to parent dir
				for i := re.cursor - 1; i >= 0; i-- {
					if re.items[i].typ == itemTreeDir && re.items[i].indent < item.indent {
						re.cursor = i
						re.ensureVisible()
						break
					}
				}
			}
		}
		return re, nil

	case "ctrl+s", "s":
		// Save rules (s is an alias since many terminals intercept ctrl+s)
		tools := re.selectedValues("tools")
		paths := re.selectedValues("paths")
		agents := re.selectedValues("agents")

		if len(tools) == 0 || len(paths) == 0 || len(agents) == 0 {
			// Don't save if nothing selected
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
			// Yes — reset and add more
			re.phase = phaseEdit
			re.deselectAll()
			re.cursor = 1
			return re, nil
		}
		// No — back to dashboard
		return re, func() tea.Msg {
			return ruleEditorDoneMsg{rule: nil, ruleIndex: -1}
		}
	case "y", "Y":
		re.phase = phaseEdit
		re.deselectAll()
		re.cursor = 1
		return re, nil
	case "n", "N", "esc":
		return re, func() tea.Msg {
			return ruleEditorDoneMsg{rule: nil, ruleIndex: -1}
		}
	}
	return re, nil
}

// moveCursor moves up or down, skipping section headers.
func (re *RuleEditor) moveCursor(delta int) {
	next := re.cursor + delta
	// Skip section headers
	for next >= 0 && next < len(re.items) && re.items[next].typ == itemSectionHeader {
		next += delta
	}
	if next >= 0 && next < len(re.items) {
		re.cursor = next
	}
	re.ensureVisible()
}

// ensureVisible adjusts scrollTop so cursor is visible.
// If all items fit on screen, scrollTop stays at 0 (no scrolling).
func (re *RuleEditor) ensureVisible() {
	visible := re.height - 8
	if visible < 5 {
		visible = 20
	}
	// If all items fit on screen, never scroll
	if len(re.items) <= visible {
		re.scrollTop = 0
		return
	}
	if re.cursor < re.scrollTop {
		re.scrollTop = re.cursor
	}
	if re.cursor >= re.scrollTop+visible {
		re.scrollTop = re.cursor - visible + 1
	}
}

// selectedValues returns all selected values for a given section.
func (re *RuleEditor) selectedValues(section string) []string {
	var vals []string
	for _, item := range re.items {
		if item.section == section && item.selected {
			vals = append(vals, item.value)
		}
	}
	return vals
}

// deselectAll clears all selections.
func (re *RuleEditor) deselectAll() {
	for i := range re.items {
		re.items[i].selected = false
	}
}

// View renders the editor.
func (re RuleEditor) View() string {
	if re.phase == phaseAnother {
		return re.viewConfirm()
	}

	header := re.styles.Header.Render(" Rules for: " + re.skillName + " ")

	// Summary line
	tools := re.selectedValues("tools")
	paths := re.selectedValues("paths")
	agents := re.selectedValues("agents")
	count := len(tools) * len(paths) * len(agents)

	summaryStyle := re.styles.Description
	if count > 0 {
		summaryStyle = re.styles.Success
	}
	summary := summaryStyle.Render(fmt.Sprintf(
		"  %d tool(s) × %d path(s) × %d agent(s) = %d rule(s)",
		len(tools), len(paths), len(agents), count,
	))

	// Render list — if all items fit, show everything (no scrolling)
	var b strings.Builder
	visible := re.height - 8
	if visible < 5 || len(re.items) <= visible {
		visible = len(re.items)
		re.scrollTop = 0
	}
	end := re.scrollTop + visible
	if end > len(re.items) {
		end = len(re.items)
	}

	check := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))                   // green
	uncheck := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))                 // gray
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA")) // violet
	treeArrow := lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))               // yellow

	for i := re.scrollTop; i < end; i++ {
		item := re.items[i]
		focused := i == re.cursor
		indent := strings.Repeat("  ", item.indent)

		var line string
		switch item.typ {
		case itemSectionHeader:
			line = "  " + sectionStyle.Render("── "+item.label+" ──")

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

		b.WriteString(line + "\n")
	}

	// Help bar
	helpStyle := re.styles.Help
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	help := keyStyle.Render("↑↓") + helpStyle.Render(" navigate") + "  " +
		keyStyle.Render("space") + helpStyle.Render(" toggle") + "  " +
		keyStyle.Render("←→") + helpStyle.Render(" expand/collapse") + "  " +
		keyStyle.Render("s") + helpStyle.Render(" save") + "  " +
		keyStyle.Render("esc") + helpStyle.Render(" cancel")

	return header + "\n" + summary + "\n\n" + b.String() + "\n" + help
}

// viewConfirm renders the "add another?" screen.
func (re RuleEditor) viewConfirm() string {
	header := re.styles.Header.Render(" Rules for: " + re.skillName + " ")

	tools := re.selectedValues("tools")
	paths := re.selectedValues("paths")
	agents := re.selectedValues("agents")
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
	tools := re.selectedValues("tools")
	paths := re.selectedValues("paths")
	agents := re.selectedValues("agents")
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
