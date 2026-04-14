// dashboard.go implements a split-pane dashboard: left panel shows skills,
// right panel shows full skill description + rules with inline editing.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Blue-Bear-Security/care-bear/internal/engine"
	"github.com/Blue-Bear-Security/care-bear/internal/scanner"
	"github.com/Blue-Bear-Security/care-bear/internal/state"
)

// filterCol identifies a filterable column in the event log.
type filterCol int

const (
	filterAction   filterCol = iota // ACTION column
	filterProject                   // PROJECT column
	filterSess                      // SESS column
	filterAgent                     // AGENT column
	filterTool                      // TOOL column
	filterSkill                     // SKILL column
	filterColCount                  // sentinel — total number of filterable columns
)

// filterColNames returns display names for filter columns.
func filterColName(c filterCol) string {
	switch c {
	case filterAction:
		return "ACTION"
	case filterProject:
		return "PROJECT"
	case filterSess:
		return "SESS"
	case filterAgent:
		return "AGENT"
	case filterTool:
		return "TOOL"
	case filterSkill:
		return "SKILL"
	}
	return ""
}

// Dashboard is a split-pane view: skills (left), rules+logs (right).
type Dashboard struct {
	skills       []scanner.Skill
	config       engine.Config
	ruleSources  []string // Parallel to config.Tools — source per rule (SourceRepo/SourceMachine)
	loadedSkills map[string]*state.SkillStatus
	eventLines   []string // Recent event log lines
	projectRoot  string   // For reading events.log
	skillScroll  ScrollView
	ruleScroll   ScrollView
	logScroll    ScrollView
	focusPanel   int // 0=skills, 1=rules, 2=event log
	editingPath  bool
	pathBuffer   string
	pathCurPos   int
	width        int
	height       int
	styles       Styles

	// Log filtering — multi-column
	filterMode   bool                 // true when filter bar is active
	filterCursor filterCol            // which column the filter cursor is on
	logFilters   map[filterCol]string // active filters: column → value ("" = all)
}

// NewDashboard creates a new split-pane Dashboard.
func NewDashboard(skills []scanner.Skill, cfg engine.Config, styles Styles, loadedSkills map[string]*state.SkillStatus) Dashboard {
	if loadedSkills == nil {
		loadedSkills = make(map[string]*state.SkillStatus)
	}
	return Dashboard{
		skills:       skills,
		config:       cfg,
		loadedSkills: loadedSkills,
		styles:       styles,
		logFilters:   make(map[filterCol]string),
	}
}

type indexedRule struct {
	rule        engine.Rule
	configIndex int
	source      string // engine.SourceRepo or engine.SourceMachine
}

// rulesForSkill returns rules matching the currently selected skill.
func (d *Dashboard) rulesForSkill() []indexedRule {
	if d.skillScroll.Cursor >= len(d.skills) {
		return nil
	}
	name := d.skills[d.skillScroll.Cursor].Name
	var rules []indexedRule
	for i, r := range d.config.Tools {
		source := engine.SourceMachine
		if i < len(d.ruleSources) {
			source = d.ruleSources[i]
		}
		if r.Skill == name {
			rules = append(rules, indexedRule{rule: r, configIndex: i, source: source})
		}
	}
	return rules
}

// Init returns the initial command.
func (d Dashboard) Init() tea.Cmd { return nil }

// saveRequestMsg is sent when the user presses 's'.
type saveRequestMsg struct{}
type saveToRepoMsg struct{}
type saveToMachineMsg struct{}

// Update handles key input.
func (d Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If editing path inline, route to path editor
		if d.editingPath {
			return d.updatePathEdit(msg)
		}

		switch msg.String() {
		// Navigation — up/down within current panel
		case "up", "k":
			noSkip := func(int) bool { return false }
			switch d.focusPanel {
			case 0:
				prev := d.skillScroll.Cursor
				d.skillScroll.MoveUp(len(d.skills), noSkip)
				if d.skillScroll.Cursor != prev {
					d.ruleScroll.Cursor = 0
				}
			case 1:
				d.ruleScroll.MoveUp(len(d.rulesForSkill()), noSkip)
			case 2:
				if d.filterMode {
					d.cycleFilterValue(-1)
				} else {
					d.logScroll.MoveUp(len(d.eventLines), noSkip)
				}
			}
			return d, nil

		case "down", "j":
			noSkip := func(int) bool { return false }
			switch d.focusPanel {
			case 0:
				prev := d.skillScroll.Cursor
				d.skillScroll.MoveDown(len(d.skills), noSkip)
				if d.skillScroll.Cursor != prev {
					d.ruleScroll.Cursor = 0
				}
			case 1:
				d.ruleScroll.MoveDown(len(d.rulesForSkill()), noSkip)
			case 2:
				if d.filterMode {
					d.cycleFilterValue(1)
				} else {
					d.logScroll.MoveDown(len(d.eventLines), noSkip)
				}
			}
			return d, nil

		// Page up/down — jump by a screenful in the log panel
		case "pgup":
			if d.focusPanel == 2 {
				d.logScroll.PageUp(d.logPageSize())
			}
			return d, nil

		case "pgdown":
			if d.focusPanel == 2 {
				d.logScroll.PageDown(len(d.eventLines), d.logPageSize())
			}
			return d, nil

		// Cmd+Up / Home — jump to top of logs
		case "home", "ctrl+up":
			if d.focusPanel == 2 {
				d.logScroll.JumpTop()
			}
			return d, nil

		// Cmd+Down / End — jump to bottom of logs
		case "end", "ctrl+down":
			if d.focusPanel == 2 {
				d.logScroll.JumpBottom(len(d.eventLines))
			}
			return d, nil

		// Tab cycles: skills(0) → rules(1) → logs(2) → skills(0)
		case "tab":
			d.focusPanel = (d.focusPanel + 1) % 3
			return d, nil

		case "shift+tab":
			d.focusPanel = (d.focusPanel + 2) % 3
			return d, nil

		// Right arrow: move filter column when filtering, else switch panel
		case "right", "l":
			if d.focusPanel == 2 && d.filterMode {
				d.filterCursor = (d.filterCursor + 1) % filterColCount
				return d, nil
			}
			if d.focusPanel == 0 {
				d.focusPanel = 1
				d.ruleScroll.Cursor = 0
			}
			return d, nil

		// Left arrow: move filter column when filtering, else switch panel
		case "left", "h":
			if d.focusPanel == 2 && d.filterMode {
				d.filterCursor = (d.filterCursor + filterColCount - 1) % filterColCount
				return d, nil
			}
			if d.focusPanel == 1 || d.focusPanel == 2 {
				d.focusPanel = 0
			}
			return d, nil

		// Enter — context-dependent
		case "enter":
			switch d.focusPanel {
			case 0:
				// Skills panel: open rule editor
				if d.skillScroll.Cursor < len(d.skills) {
					return d, func() tea.Msg {
						return openRuleEditorMsg{
							skillName: d.skills[d.skillScroll.Cursor].Name,
							ruleIndex: -1,
							existing:  nil,
						}
					}
				}
			case 2:
				// Log panel: jump to the skill/rule that caused this event
				d.jumpToLogEntry()
			}
			return d, nil

		case "a":
			// Add rules for current skill
			if d.skillScroll.Cursor < len(d.skills) {
				return d, func() tea.Msg {
					return openRuleEditorMsg{
						skillName: d.skills[d.skillScroll.Cursor].Name,
						ruleIndex: -1,
						existing:  nil,
					}
				}
			}
			return d, nil

		// Actions on right panel (rules) — only when focused
		case "d":
			if d.focusPanel == 1 {
				rules := d.rulesForSkill()
				if d.ruleScroll.Cursor >= 0 && d.ruleScroll.Cursor < len(rules) {
					idx := rules[d.ruleScroll.Cursor].configIndex
					d.config.Tools = append(d.config.Tools[:idx], d.config.Tools[idx+1:]...)
					rules = d.rulesForSkill()
					if d.ruleScroll.Cursor >= len(rules) && d.ruleScroll.Cursor > 0 {
						d.ruleScroll.Cursor--
					}
				}
			}
			return d, nil

		case "t":
			// Cycle tool on selected rule
			if d.focusPanel == 1 {
				rules := d.rulesForSkill()
				if d.ruleScroll.Cursor >= 0 && d.ruleScroll.Cursor < len(rules) {
					idx := rules[d.ruleScroll.Cursor].configIndex
					d.config.Tools[idx].Tool = nextTool(d.config.Tools[idx].Tool)
				}
			}
			return d, nil

		case "g":
			// Cycle agent on selected rule
			if d.focusPanel == 1 {
				rules := d.rulesForSkill()
				if d.ruleScroll.Cursor >= 0 && d.ruleScroll.Cursor < len(rules) {
					idx := rules[d.ruleScroll.Cursor].configIndex
					d.config.Tools[idx].Agent = nextAgent(d.config.Tools[idx].Agent)
				}
			}
			return d, nil

		case "p":
			// Edit path inline
			if d.focusPanel == 1 {
				rules := d.rulesForSkill()
				if d.ruleScroll.Cursor >= 0 && d.ruleScroll.Cursor < len(rules) {
					d.editingPath = true
					d.pathBuffer = rules[d.ruleScroll.Cursor].rule.Path
					d.pathCurPos = len(d.pathBuffer)
				}
			}
			return d, nil

		case "y":
			// Duplicate selected rule
			if d.focusPanel == 1 {
				rules := d.rulesForSkill()
				if d.ruleScroll.Cursor >= 0 && d.ruleScroll.Cursor < len(rules) {
					dup := rules[d.ruleScroll.Cursor].rule
					d.config.Tools = append(d.config.Tools, dup)
					// Move cursor to the new duplicate
					newRules := d.rulesForSkill()
					d.ruleScroll.Cursor = len(newRules) - 1
				}
			}
			return d, nil

		case "s":
			return d, func() tea.Msg { return saveRequestMsg{} }

		case "f":
			// Toggle filter mode or advance to next column
			if d.focusPanel == 2 {
				if !d.filterMode {
					d.filterMode = true
					d.filterCursor = filterAction
				} else {
					// Move to next column
					d.filterCursor = (d.filterCursor + 1) % filterColCount
				}
			}
			return d, nil

		case "F":
			// Clear all filters
			if d.focusPanel == 2 {
				d.logFilters = make(map[filterCol]string)
				d.filterMode = false
				d.logScroll.Cursor = 0
			}
			return d, nil

		case "K":
			// Clear all logs from view and delete the events.log file
			if d.focusPanel == 2 {
				d.eventLines = nil
				d.logScroll.Cursor = 0
				d.logFilters = make(map[filterCol]string)
				d.filterMode = false
				// Delete the events.log file
				home, _ := os.UserHomeDir()
				if home != "" {
					_ = os.Remove(filepath.Join(home, ".care-bear", "events.log"))
				}
			}
			return d, nil

		case "esc":
			if d.focusPanel == 2 {
				d.filterMode = false
				d.logFilters = make(map[filterCol]string)
				d.logScroll.Cursor = 0
				return d, nil
			}

		case "c":
			// Open settings view
			return d, func() tea.Msg { return openSettingsMsg{} }

		case "P":
			// Switch project — return to project picker
			return d, func() tea.Msg { return switchProjectMsg{} }

		case "q":
			return d, tea.Quit
		}
	}
	return d, nil
}

// updatePathEdit handles inline path editing.
func (d Dashboard) updatePathEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Commit the edit
		rules := d.rulesForSkill()
		if d.ruleScroll.Cursor >= 0 && d.ruleScroll.Cursor < len(rules) {
			idx := rules[d.ruleScroll.Cursor].configIndex
			d.config.Tools[idx].Path = d.pathBuffer
		}
		d.editingPath = false
		return d, nil
	case "esc":
		// Cancel
		d.editingPath = false
		return d, nil
	case "backspace":
		if d.pathCurPos > 0 {
			d.pathBuffer = d.pathBuffer[:d.pathCurPos-1] + d.pathBuffer[d.pathCurPos:]
			d.pathCurPos--
		}
		return d, nil
	case "left":
		if d.pathCurPos > 0 {
			d.pathCurPos--
		}
		return d, nil
	case "right":
		if d.pathCurPos < len(d.pathBuffer) {
			d.pathCurPos++
		}
		return d, nil
	case "ctrl+a":
		d.pathCurPos = 0
		return d, nil
	case "ctrl+e":
		d.pathCurPos = len(d.pathBuffer)
		return d, nil
	default:
		key := msg.String()
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			d.pathBuffer = d.pathBuffer[:d.pathCurPos] + key + d.pathBuffer[d.pathCurPos:]
			d.pathCurPos++
		}
		return d, nil
	}
}

// nextTool cycles through ToolOptions (defined in constants.go).
func nextTool(current string) string {
	for i, t := range ToolOptions {
		if t == current {
			return ToolOptions[(i+1)%len(ToolOptions)]
		}
	}
	return ToolOptions[0]
}

// nextAgent cycles through AgentOptions (defined in constants.go).
func nextAgent(current string) string {
	for i, a := range AgentOptions {
		if a == current {
			return AgentOptions[(i+1)%len(AgentOptions)]
		}
	}
	return AgentOptions[0]
}

// View renders the split-pane layout.
func (d Dashboard) View() string {
	if len(d.skills) == 0 {
		return d.styles.Description.Render("  No skills discovered. Add skill paths to .care-bear/config.json")
	}

	leftWidth := d.width*25/100 - 2
	rightWidth := d.width - leftWidth - 5
	if leftWidth < 20 {
		leftWidth = 25
	}
	if rightWidth < 30 {
		rightWidth = 40
	}
	panelHeight := d.height - 5
	if panelHeight < 5 {
		panelHeight = 20
	}

	// Dynamic split: rules take only what they need, logs get the rest.
	// Render rules first to measure actual height, then allocate remaining to logs.
	maxRulesHeight := panelHeight * 50 / 100 // Cap rules at 50% max
	left := d.renderSkillList(leftWidth, panelHeight)
	rulesContent := d.renderRulePanel(rightWidth, maxRulesHeight)

	// Measure actual rules content height
	rulesLines := strings.Count(rulesContent, "\n") + 1
	rulesHeight := rulesLines
	if rulesHeight > maxRulesHeight {
		rulesHeight = maxRulesHeight
	}
	minRulesHeight := 5 // Always show at least skill name + description
	if rulesHeight < minRulesHeight {
		rulesHeight = minRulesHeight
	}

	logsHeight := panelHeight - rulesHeight - 1 // -1 for divider line
	if logsHeight < 5 {
		logsHeight = 5
	}

	logsContent := d.renderEventLog(rightWidth, logsHeight)

	// Pad each section to its exact allocated height so the divider
	// stays at the correct position and logs fill their box.
	rulesContent = padToHeight(rulesContent, rulesHeight)
	logsContent = padToHeight(logsContent, logsHeight)

	// Combine rules + divider + logs into one right panel
	divider := d.styles.Divider.Render(strings.Repeat("─", rightWidth))
	rightContent := rulesContent + "\n" + divider + "\n" + logsContent

	activeBorder := lipgloss.Color("#7C3AED")
	dimBorder := lipgloss.Color("#374151")

	leftBorderColor := dimBorder
	rightBorderColor := dimBorder
	if d.focusPanel == 0 {
		leftBorderColor = activeBorder
	} else {
		rightBorderColor = activeBorder
	}

	leftPanel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(leftBorderColor).
		Width(leftWidth).
		Height(panelHeight).
		Render(left)

	rightPanel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rightBorderColor).
		Width(rightWidth).
		Height(panelHeight).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)
}

// renderSkillList renders the left panel.
// isProjectSkill returns true if the skill source is under the project root.
func isProjectSkill(source, projectRoot string) bool {
	if projectRoot == "" {
		return false
	}
	return source == projectRoot || strings.HasPrefix(source, projectRoot+"/")
}

func (d Dashboard) renderSkillList(width, height int) string {
	title := d.styles.RuleHeader.Render("SKILLS") + "\n"

	var b strings.Builder
	visible := height - 3
	if visible < 1 {
		visible = len(d.skills)
	}
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))

	// Build ordered index: project skills first, then global
	var ordered []int
	for i, skill := range d.skills {
		if isProjectSkill(skill.Source, d.projectRoot) {
			ordered = append(ordered, i)
		}
	}
	projectCount := len(ordered)
	for i, skill := range d.skills {
		if !isProjectSkill(skill.Source, d.projectRoot) {
			ordered = append(ordered, i)
		}
	}
	_ = projectCount // used below for section headers

	// Scroll over the ordered list
	scrollStart, end := d.skillScroll.VisibleRange(len(ordered), visible)

	renderedHeader := false
	for oi := scrollStart; oi < end; oi++ {
		i := ordered[oi]
		skill := d.skills[i]

		// Section header
		if oi == 0 && projectCount > 0 {
			b.WriteString("  " + sectionStyle.Render("── PROJECT ──") + "\n")
			renderedHeader = true
		}
		if oi == projectCount && !renderedHeader {
			b.WriteString("  " + sectionStyle.Render("── GLOBAL ──") + "\n")
			renderedHeader = true
		} else if oi == projectCount && renderedHeader {
			b.WriteString("\n  " + sectionStyle.Render("── GLOBAL ──") + "\n")
		}
		focused := i == d.skillScroll.Cursor && d.focusPanel == 0

		ruleCount := 0
		hasRepoRule := false
		for idx, r := range d.config.Tools {
			if r.Skill == skill.Name {
				ruleCount++
				if idx < len(d.ruleSources) && d.ruleSources[idx] == engine.SourceRepo {
					hasRepoRule = true
				}
			}
		}

		name := skill.Name
		if len(name) > width-8 {
			name = name[:width-11] + "..."
		}

		repoTag := ""
		if hasRepoRule {
			repoTag = lipgloss.NewStyle().Foreground(lipgloss.Color("#F97316")).Render(" ")
		}

		countStr := d.styles.Description.Render(fmt.Sprintf(" (%d)", ruleCount))
		if ruleCount > 0 {
			countStr = d.styles.Success.Render(fmt.Sprintf(" (%d)", ruleCount))
		}

		if focused {
			suffix := fmt.Sprintf(" (%d)", ruleCount)

			line := d.styles.Selected.Render(" ▸ " + name + suffix) + repoTag
			b.WriteString(line + "\n")
		} else if i == d.skillScroll.Cursor {
			nameStyle := d.styles.SkillName

			b.WriteString(" ▸ " + nameStyle.Render(name) + countStr + repoTag + "\n")
		} else {
			nameStyle := lipgloss.NewStyle()

			b.WriteString("   " + nameStyle.Render(name) + countStr + repoTag + "\n")
		}
	}

	// Show scroll indicators if not all skills are visible
	if len(d.skills) > visible {
		indicator := d.styles.Description.Render(
			fmt.Sprintf("  [%d/%d]", d.skillScroll.Cursor+1, len(d.skills)))
		if scrollStart > 0 {
			indicator += d.styles.Description.Render(" ↑")
		}
		if end < len(d.skills) {
			indicator += d.styles.Description.Render(" ↓")
		}
		b.WriteString(indicator + "\n")
	}

	return title + b.String()
}

// renderRulePanel renders the right panel with full description and rules.
func (d Dashboard) renderRulePanel(width, height int) string {
	if d.skillScroll.Cursor >= len(d.skills) {
		return ""
	}

	skill := d.skills[d.skillScroll.Cursor]
	rules := d.rulesForSkill()

	var b strings.Builder

	// Skill name
	b.WriteString(d.styles.SkillName.Render(skill.Name) + "\n")

	// Full description — word-wrapped
	if skill.Description != "" {
		wrapped := wordWrap(skill.Description, width-2)
		b.WriteString(d.styles.Description.Render(wrapped) + "\n")
	}
	b.WriteString("\n")

	if len(rules) == 0 {
		b.WriteString(d.styles.Description.Render("  No rules configured.") + "\n\n")
		b.WriteString(d.styles.Action.Render("  Press 'a' or 'enter' to add rules.") + "\n")
		return b.String()
	}

	// Column header
	header := fmt.Sprintf("  %-10s %-22s %-8s %s", "TOOL", "PATH", "AGENT", "SOURCE")
	b.WriteString(d.styles.RuleHeader.Render(header) + "\n")
	repoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F97316"))
	machineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

	// Scrolling for rules list
	visibleRules := height - 7
	if visibleRules < 3 {
		visibleRules = 3
	}
	ruleStart, ruleEnd := d.ruleScroll.VisibleRange(len(rules), visibleRules)

	for i := ruleStart; i < ruleEnd; i++ {
		ir := rules[i]
		focused := i == d.ruleScroll.Cursor && d.focusPanel == 1

		toolStr := ir.rule.Tool
		pathStr := ir.rule.Path
		agentStr := ir.rule.Agent

		// If editing this row's path
		if d.editingPath && focused {
			// Show path with cursor
			before := d.pathBuffer[:d.pathCurPos]
			cursor := "█"
			after := ""
			if d.pathCurPos < len(d.pathBuffer) {
				cursor = string(d.pathBuffer[d.pathCurPos])
				after = d.pathBuffer[d.pathCurPos+1:]
			}
			pathStr = before + d.styles.Selected.Render(cursor) + after
			if len(pathStr) > 22 {
				pathStr = pathStr[:22]
			}
		} else if len(pathStr) > 22 {
			pathStr = pathStr[:19] + "..."
		}

		sourceStr := machineStyle.Render("machine")
		if ir.source == engine.SourceRepo {
			sourceStr = repoStyle.Render(" repo")
		}

		if focused && !d.editingPath {
			line := fmt.Sprintf("  %-10s %-22s %-8s", toolStr, pathStr, agentStr)
			b.WriteString(d.styles.Selected.Render(line) + " " + sourceStr + "\n")
		} else {
			tool := d.styles.Tool.Render(fmt.Sprintf("%-10s", toolStr))
			path := d.styles.Path.Render(fmt.Sprintf("%-22s", pathStr))
			agent := d.styles.Agent.Render(fmt.Sprintf("%-8s", agentStr))
			b.WriteString("  " + tool + " " + path + " " + agent + " " + sourceStr + "\n")
		}
	}

	// Scroll indicator for rules
	if len(rules) > visibleRules {
		indicator := d.styles.Description.Render(
			fmt.Sprintf("  [%d/%d]", d.ruleScroll.Cursor+1, len(rules)))
		if ruleStart > 0 {
			indicator += d.styles.Description.Render(" ↑")
		}
		if ruleEnd < len(rules) {
			indicator += d.styles.Description.Render(" ↓")
		}
		b.WriteString(indicator + "\n")
	}

	// Context help for right panel when focused
	if d.focusPanel == 1 && !d.editingPath {
		b.WriteString("\n")
		helpStyle := d.styles.Help
		keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
		b.WriteString(
			keyStyle.Render("t") + helpStyle.Render(" tool") + "  " +
				keyStyle.Render("p") + helpStyle.Render(" path") + "  " +
				keyStyle.Render("g") + helpStyle.Render(" agent") + "  " +
				keyStyle.Render("y") + helpStyle.Render(" dup") + "  " +
				keyStyle.Render("d") + helpStyle.Render(" del") + "\n",
		)
	}

	if d.editingPath {
		b.WriteString("\n")
		b.WriteString(d.styles.Success.Render("  Editing path — enter to save, esc to cancel") + "\n")
	}

	return b.String()
}

// logPageSize returns the number of visible log rows for page up/down jumps.
func (d *Dashboard) logPageSize() int {
	logsHeight := d.height - 5
	if logsHeight < 5 {
		logsHeight = 10
	}
	// Approximate: logs get ~50-100% of panel, minus header lines
	visible := logsHeight/2 - 4
	if visible < 5 {
		visible = 5
	}
	return visible
}

// padToHeight ensures content has exactly `height` lines.
// Shorter content is padded with empty lines; longer content is truncated.
func padToHeight(content string, height int) string {
	lines := strings.Split(content, "\n")
	// Trim trailing empty line from a trailing newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// wordWrap wraps text to the given width.
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = w
		} else {
			line += " " + w
		}
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n")
}

// renderEventLog renders the bottom-right panel showing recent hook events.
// Columns auto-size based on actual data width.
func (d Dashboard) renderEventLog(width, height int) string {
	// Build filter label showing active filters
	filterLabel := ""
	for col := filterAction; col < filterColCount; col++ {
		if v, ok := d.logFilters[col]; ok && v != "" {
			filterLabel += " [" + filterColName(col) + "=" + v + "]"
		}
	}
	title := d.styles.RuleHeader.Render("  EVENT LOG"+filterLabel) + "\n"

	if len(d.eventLines) == 0 {
		return title + "\n" + d.styles.Description.Render("  No events yet. Hook activity will appear here.")
	}

	// Parse all events, then apply filters.
	parsedRows := parseAllEvents(d.eventLines)

	var allRows []ParsedEvent
	for _, r := range parsedRows {
		if !d.matchesFilters(r.Action, r.Project, r.Session, r.Agent, r.Tool, r.Skill) {
			continue
		}
		allRows = append(allRows, r)
	}

	if len(allRows) == 0 {
		return title + "\n" + d.styles.Description.Render("  No matching events.")
	}

	// Calculate max width for each column from actual data
	colW := map[string]int{
		"time": 5, "act": 6, "project": 7, "sess": 4, "agent": 5, "tool": 4, "skill": 5, "path": 4,
	}
	for _, r := range allRows {
		if len(r.Time) > colW["time"] {
			colW["time"] = len(r.Time)
		}
		if len(r.Action) > colW["act"] {
			colW["act"] = len(r.Action)
		}
		if len(r.Project) > colW["project"] {
			colW["project"] = len(r.Project)
		}
		if len(r.Session) > colW["sess"] {
			colW["sess"] = len(r.Session)
		}
		if len(r.Agent) > colW["agent"] {
			colW["agent"] = len(r.Agent)
		}
		if len(r.Tool) > colW["tool"] {
			colW["tool"] = len(r.Tool)
		}
		if len(r.Skill) > colW["skill"] {
			colW["skill"] = len(r.Skill)
		}
		if len(r.Path) > colW["path"] {
			colW["path"] = len(r.Path)
		}
	}

	// Cap path width to fill remaining space
	used := colW["time"] + colW["act"] + colW["project"] + colW["sess"] + colW["agent"] + colW["tool"] + colW["skill"] + 16 // padding
	maxPath := width - used - 4
	if maxPath < 10 {
		maxPath = 10
	}
	if colW["path"] > maxPath {
		colW["path"] = maxPath
	}

	// Build format string
	fmtStr := fmt.Sprintf("  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds",
		colW["time"], colW["act"], colW["project"], colW["sess"], colW["agent"], colW["tool"], colW["skill"], colW["path"])

	// Header — highlight the active filter column when in filter mode
	if d.filterMode {
		activeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1F2937")).Background(lipgloss.Color("#FBBF24"))
		headers := []string{"ACTION", "PROJECT", "SESS", "AGENT", "TOOL", "SKILL", "PATH"}
		colWidths := []int{colW["act"], colW["project"], colW["sess"], colW["agent"], colW["tool"], colW["skill"], colW["path"]}
		var headerParts []string
		for i, h := range headers {
			padded := fmt.Sprintf("%-*s", colWidths[i], h)
			if filterCol(i) == d.filterCursor && i < int(filterColCount) {
				headerParts = append(headerParts, activeStyle.Render(padded))
			} else if v, ok := d.logFilters[filterCol(i)]; ok && v != "" && i < int(filterColCount) {
				filteredStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24"))
				headerParts = append(headerParts, filteredStyle.Render(padded))
			} else {
				headerParts = append(headerParts, d.styles.Description.Render(padded))
			}
		}
		title += "  " + strings.Join(headerParts, "  ")
	} else {
		title += d.styles.Description.Render(fmt.Sprintf(fmtStr, "TIME", "ACTION", "PROJECT", "SESS", "AGENT", "TOOL", "SKILL", "PATH"))
	}
	title += "\n" + d.styles.Divider.Render(strings.Repeat("─", width-2)) + "\n"

	// Scrolling — use shared ScrollView
	var b strings.Builder
	visible := height - 4
	if visible < 3 {
		visible = 3
	}
	start, end := d.logScroll.VisibleRange(len(allRows), visible)

	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE"))

	for fi := start; fi < end; fi++ {
		r := allRows[fi]

		// Truncate path
		path := r.Path
		if len(path) > colW["path"] {
			path = "…" + path[len(path)-colW["path"]+1:]
		}

		var sty lipgloss.Style
		if r.IsBlock {
			sty = red
		} else if r.IsExpire {
			sty = lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Bold(true)
		} else if r.IsLoad {
			sty = cyan
		} else {
			sty = green
		}

		focused := fi == d.logScroll.Cursor && d.focusPanel == 2
		plain := fmt.Sprintf(fmtStr, r.Time, r.Action, r.Project, r.Session, r.Agent, r.Tool, r.Skill, path)

		if focused {
			b.WriteString(d.styles.Selected.Render("▸"+plain[1:]) + "\n")
		} else {
			b.WriteString(sty.Render(plain) + "\n")
		}
	}

	return title + b.String()
}

// uniqueColumnValues returns the unique values for a given filter column
// from all parsed event lines.
func (d *Dashboard) uniqueColumnValues(col filterCol) []string {
	seen := make(map[string]bool)
	var vals []string
	for _, ev := range parseAllEvents(d.eventLines) {
		var val string
		switch col {
		case filterAction:
			val = ev.Action
		case filterProject:
			val = ev.Project
		case filterSess:
			val = ev.Session
		case filterAgent:
			val = ev.Agent
		case filterTool:
			val = ev.Tool
		case filterSkill:
			val = ev.Skill
		}
		if val != "" && !seen[val] {
			seen[val] = true
			vals = append(vals, val)
		}
	}
	return vals
}

// cycleFilterValue cycles the filter value for the current column.
// direction: 1 = next, -1 = previous.
func (d *Dashboard) cycleFilterValue(direction int) {
	vals := d.uniqueColumnValues(d.filterCursor)
	if len(vals) == 0 {
		return
	}
	// Prepend "" (all) as first option
	options := append([]string{""}, vals...)
	current := d.logFilters[d.filterCursor]

	idx := 0
	for i, v := range options {
		if v == current {
			idx = i
			break
		}
	}
	idx += direction
	if idx < 0 {
		idx = len(options) - 1
	}
	if idx >= len(options) {
		idx = 0
	}

	if options[idx] == "" {
		delete(d.logFilters, d.filterCursor)
	} else {
		d.logFilters[d.filterCursor] = options[idx]
	}
	d.logScroll.Cursor = 0
}

// matchesFilters checks if a row passes all active filters.
func (d *Dashboard) matchesFilters(act, project, sess, agent, tool, skill string) bool {
	for col, want := range d.logFilters {
		if want == "" {
			continue
		}
		var got string
		switch col {
		case filterAction:
			got = act
		case filterProject:
			got = project
		case filterSess:
			got = sess
		case filterAgent:
			got = agent
		case filterTool:
			got = tool
		case filterSkill:
			got = skill
		}
		if got != want {
			return false
		}
	}
	return true
}

// jumpToLogEntry parses the focused log line and navigates to the skill/rule.
// Works with the filtered log view used by renderEventLog.
func (d *Dashboard) jumpToLogEntry() {
	if d.logScroll.Cursor < 0 || d.logScroll.Cursor >= len(d.eventLines) {
		return
	}

	ev, ok := parseEventLine(d.eventLines[d.logScroll.Cursor], d.logScroll.Cursor)
	if !ok {
		return
	}

	skill := ev.Skill

	// Skills can be comma-separated (e.g., "linear,sst-architect") -- use first one.
	if strings.Contains(skill, ",") {
		skill = strings.Split(skill, ",")[0]
	}

	if skill == "" {
		return
	}

	// Find the skill in the skills list and jump to it.
	for i, s := range d.skills {
		if s.Name == skill {
			d.skillScroll.Cursor = i
			d.focusPanel = 1
			d.ruleScroll.Cursor = 0
			return
		}
	}
}

// LoadEventLog reads the event log file and stores lines from the last 7 days.
// This is a pure read operation -- no file modifications are performed.
func (d *Dashboard) LoadEventLog(projectRoot string) {
	d.projectRoot = projectRoot
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, ".care-bear", "events.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		d.eventLines = nil
		return
	}

	allLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	cutoff := time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02")

	// Keep only lines from the last 7 days that have a skill attached.
	var recent []string
	for _, line := range allLines {
		if len(line) < 10 || line[:10] < cutoff {
			continue
		}
		// Only keep events with a skill (BLOCK with skill, SKILL-LOAD, or ALLOW with skill).
		parts := strings.Split(line, " | ")
		hasSkill := false
		if strings.Contains(line, "SKILL-LOAD") {
			hasSkill = true
		} else if len(parts) >= 8 && strings.TrimSpace(parts[7]) != "" {
			hasSkill = true
		}
		if hasSkill {
			recent = append(recent, line)
		}
	}

	// Keep last 200 lines for display.
	if len(recent) > 200 {
		recent = recent[len(recent)-200:]
	}
	d.eventLines = recent
	// Auto-scroll to latest event
	if len(d.eventLines) > 0 {
		d.logScroll.Cursor = len(d.eventLines) - 1
	}
}

// stripAnsi removes ANSI escape sequences.
func stripAnsi(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}
