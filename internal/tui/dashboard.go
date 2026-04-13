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

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/scanner"
)

// Dashboard is a split-pane view: skills (left), rules+logs (right).
type Dashboard struct {
	skills       []scanner.Skill
	config       engine.Config
	loadedSkills map[string]*SkillStatus
	eventLines   []string // Recent event log lines
	projectRoot  string   // For reading events.log
	skillCursor  int
	ruleCursor   int
	logCursor    int
	focusPanel   int // 0=skills, 1=rules, 2=event log
	editingPath  bool
	pathBuffer   string
	pathCurPos   int
	width        int
	height       int
	styles       Styles
}

// NewDashboard creates a new split-pane Dashboard.
func NewDashboard(skills []scanner.Skill, cfg engine.Config, styles Styles, loadedSkills map[string]*SkillStatus) Dashboard {
	if loadedSkills == nil {
		loadedSkills = make(map[string]*SkillStatus)
	}
	return Dashboard{
		skills:       skills,
		config:       cfg,
		loadedSkills: loadedSkills,
		styles:       styles,
	}
}

type indexedRule struct {
	rule        engine.Rule
	configIndex int
}

// rulesForSkill returns rules matching the currently selected skill.
func (d *Dashboard) rulesForSkill() []indexedRule {
	if d.skillCursor >= len(d.skills) {
		return nil
	}
	name := d.skills[d.skillCursor].Name
	var rules []indexedRule
	for i, r := range d.config.Tools {
		if r.Skill == name {
			rules = append(rules, indexedRule{rule: r, configIndex: i})
		}
	}
	return rules
}

// Init returns the initial command.
func (d Dashboard) Init() tea.Cmd { return nil }

// saveRequestMsg is sent when the user presses 's'.
type saveRequestMsg struct{}

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
			switch d.focusPanel {
			case 0:
				if d.skillCursor > 0 {
					d.skillCursor--
					d.ruleCursor = 0
				}
			case 1:
				if d.ruleCursor > 0 {
					d.ruleCursor--
				}
			case 2:
				if d.logCursor > 0 {
					d.logCursor--
				}
			}
			return d, nil

		case "down", "j":
			switch d.focusPanel {
			case 0:
				if d.skillCursor < len(d.skills)-1 {
					d.skillCursor++
					d.ruleCursor = 0
				}
			case 1:
				rules := d.rulesForSkill()
				if d.ruleCursor < len(rules)-1 {
					d.ruleCursor++
				}
			case 2:
				if d.logCursor < len(d.eventLines)-1 {
					d.logCursor++
				}
			}
			return d, nil

		// Tab cycles: skills(0) → rules(1) → logs(2) → skills(0)
		case "tab":
			d.focusPanel = (d.focusPanel + 1) % 3
			return d, nil

		case "shift+tab":
			d.focusPanel = (d.focusPanel + 2) % 3
			return d, nil

		// Right arrow: skills → rules or logs
		case "right", "l":
			if d.focusPanel == 0 {
				d.focusPanel = 1
				d.ruleCursor = 0
			}
			return d, nil

		// Left arrow: rules/logs → skills
		case "left", "h":
			if d.focusPanel == 1 || d.focusPanel == 2 {
				d.focusPanel = 0
			}
			return d, nil

		// Enter — context-dependent
		case "enter":
			switch d.focusPanel {
			case 0:
				// Skills panel: open rule editor
				if d.skillCursor < len(d.skills) {
					return d, func() tea.Msg {
						return openRuleEditorMsg{
							skillName: d.skills[d.skillCursor].Name,
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
			if d.skillCursor < len(d.skills) {
				return d, func() tea.Msg {
					return openRuleEditorMsg{
						skillName: d.skills[d.skillCursor].Name,
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
				if d.ruleCursor >= 0 && d.ruleCursor < len(rules) {
					idx := rules[d.ruleCursor].configIndex
					d.config.Tools = append(d.config.Tools[:idx], d.config.Tools[idx+1:]...)
					rules = d.rulesForSkill()
					if d.ruleCursor >= len(rules) && d.ruleCursor > 0 {
						d.ruleCursor--
					}
				}
			}
			return d, nil

		case "t":
			// Cycle tool on selected rule
			if d.focusPanel == 1 {
				rules := d.rulesForSkill()
				if d.ruleCursor >= 0 && d.ruleCursor < len(rules) {
					idx := rules[d.ruleCursor].configIndex
					d.config.Tools[idx].Tool = nextTool(d.config.Tools[idx].Tool)
				}
			}
			return d, nil

		case "g":
			// Cycle agent on selected rule
			if d.focusPanel == 1 {
				rules := d.rulesForSkill()
				if d.ruleCursor >= 0 && d.ruleCursor < len(rules) {
					idx := rules[d.ruleCursor].configIndex
					d.config.Tools[idx].Agent = nextAgent(d.config.Tools[idx].Agent)
				}
			}
			return d, nil

		case "p":
			// Edit path inline
			if d.focusPanel == 1 {
				rules := d.rulesForSkill()
				if d.ruleCursor >= 0 && d.ruleCursor < len(rules) {
					d.editingPath = true
					d.pathBuffer = rules[d.ruleCursor].rule.Path
					d.pathCurPos = len(d.pathBuffer)
				}
			}
			return d, nil

		case "y":
			// Duplicate selected rule
			if d.focusPanel == 1 {
				rules := d.rulesForSkill()
				if d.ruleCursor >= 0 && d.ruleCursor < len(rules) {
					dup := rules[d.ruleCursor].rule
					d.config.Tools = append(d.config.Tools, dup)
					// Move cursor to the new duplicate
					newRules := d.rulesForSkill()
					d.ruleCursor = len(newRules) - 1
				}
			}
			return d, nil

		case "s":
			return d, func() tea.Msg { return saveRequestMsg{} }

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
		if d.ruleCursor >= 0 && d.ruleCursor < len(rules) {
			idx := rules[d.ruleCursor].configIndex
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

// nextTool cycles through tool options.
func nextTool(current string) string {
	tools := []string{"Edit", "Write", "Bash", "Read", "Glob", "Grep", "Agent", "*"}
	for i, t := range tools {
		if t == current {
			return tools[(i+1)%len(tools)]
		}
	}
	return tools[0]
}

// nextAgent cycles through agent options.
func nextAgent(current string) string {
	agents := []string{"claude", "cursor", "*"}
	for i, a := range agents {
		if a == current {
			return agents[(i+1)%len(agents)]
		}
	}
	return agents[0]
}

// View renders the split-pane layout.
func (d Dashboard) View() string {
	if len(d.skills) == 0 {
		return d.styles.Description.Render("  No skills discovered. Add skill paths to .care-bare/config.json")
	}

	leftWidth := d.width*30/100 - 2
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

	// Split right panel: top = rules (60%), bottom = event log (40%)
	rulesHeight := panelHeight * 55 / 100
	logsHeight := panelHeight - rulesHeight - 3 // account for borders

	left := d.renderSkillList(leftWidth, panelHeight)
	rulesContent := d.renderRulePanel(rightWidth, rulesHeight)
	logsContent := d.renderEventLog(rightWidth, logsHeight)

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

	rulesPanel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rightBorderColor).
		Width(rightWidth).
		Height(rulesHeight).
		Render(rulesContent)

	logsBorderColor := lipgloss.Color("#374151")
	if d.focusPanel == 2 {
		logsBorderColor = activeBorder
	}
	logsPanel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(logsBorderColor).
		Width(rightWidth).
		Height(logsHeight).
		Render(logsContent)

	rightCombined := lipgloss.JoinVertical(lipgloss.Left, rulesPanel, logsPanel)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightCombined)
}

// renderSkillList renders the left panel.
func (d Dashboard) renderSkillList(width, height int) string {
	title := d.styles.RuleHeader.Render("SKILLS") + "\n\n"

	var b strings.Builder
	scrollStart := 0
	visible := height - 3
	if visible < 1 {
		visible = len(d.skills)
	}
	if d.skillCursor >= scrollStart+visible {
		scrollStart = d.skillCursor - visible + 1
	}
	if d.skillCursor < scrollStart {
		scrollStart = d.skillCursor
	}
	end := scrollStart + visible
	if end > len(d.skills) {
		end = len(d.skills)
	}

	for i := scrollStart; i < end; i++ {
		skill := d.skills[i]
		focused := i == d.skillCursor && d.focusPanel == 0

		ruleCount := 0
		for _, r := range d.config.Tools {
			if r.Skill == skill.Name {
				ruleCount++
			}
		}

		name := skill.Name
		if len(name) > width-8 {
			name = name[:width-11] + "..."
		}

		status := d.loadedSkills[skill.Name]
		isLoaded := status != nil && len(status.Agents) > 0

		countStr := d.styles.Description.Render(fmt.Sprintf(" (%d)", ruleCount))
		if ruleCount > 0 {
			countStr = d.styles.Success.Render(fmt.Sprintf(" (%d)", ruleCount))
		}

		// Loaded indicator: show agent names as colored tags
		loadedTag := ""
		if isLoaded {
			var tags []string
			for _, a := range status.Agents {
				if a == "unknown" {
					continue // Skip old sessions without agent info
				}
				switch a {
				case "claude":
					tags = append(tags, lipgloss.NewStyle().
						Foreground(lipgloss.Color("#1F2937")).
						Background(lipgloss.Color("#A78BFA")).
						Padding(0, 1).
						Render("claude"))
				case "cursor":
					tags = append(tags, lipgloss.NewStyle().
						Foreground(lipgloss.Color("#1F2937")).
						Background(lipgloss.Color("#22D3EE")).
						Padding(0, 1).
						Render("cursor"))
				}
			}
			if len(tags) > 0 {
				loadedTag = " " + strings.Join(tags, " ")
			}
		}

		if focused {
			suffix := fmt.Sprintf(" (%d)", ruleCount)
			if loadedTag != "" {
				suffix += " loaded"
			}
			line := d.styles.Selected.Render(" ▸ " + name + suffix)
			b.WriteString(line + "\n")
		} else if i == d.skillCursor {
			nameStyle := d.styles.SkillName
			if loadedTag != "" {
				nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#34D399"))
			}
			b.WriteString(" ▸ " + nameStyle.Render(name) + countStr + loadedTag + "\n")
		} else {
			nameStyle := lipgloss.NewStyle()
			if loadedTag != "" {
				nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))
			}
			b.WriteString("   " + nameStyle.Render(name) + countStr + loadedTag + "\n")
		}
	}

	return title + b.String()
}

// renderRulePanel renders the right panel with full description and rules.
func (d Dashboard) renderRulePanel(width, height int) string {
	if d.skillCursor >= len(d.skills) {
		return ""
	}

	skill := d.skills[d.skillCursor]
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
	header := fmt.Sprintf("  %-10s %-28s %s", "TOOL", "PATH", "AGENT")
	b.WriteString(d.styles.RuleHeader.Render(header) + "\n")

	for i, ir := range rules {
		focused := i == d.ruleCursor && d.focusPanel == 1

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
			if len(pathStr) > 28 {
				pathStr = pathStr[:28]
			}
		} else if len(pathStr) > 28 {
			pathStr = pathStr[:25] + "..."
		}

		if focused && !d.editingPath {
			line := fmt.Sprintf("  %-10s %-28s %s", toolStr, pathStr, agentStr)
			b.WriteString(d.styles.Selected.Render(line) + "\n")
		} else {
			tool := d.styles.Tool.Render(fmt.Sprintf("%-10s", toolStr))
			path := d.styles.Path.Render(fmt.Sprintf("%-28s", pathStr))
			agent := d.styles.Agent.Render(agentStr)
			b.WriteString("  " + tool + " " + path + " " + agent + "\n")
		}
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
func (d Dashboard) renderEventLog(width, height int) string {
	title := d.styles.RuleHeader.Render("  EVENT LOG") + "\n"

	if len(d.eventLines) == 0 {
		return title + "\n" + d.styles.Description.Render("  No events yet. Hook activity will appear here.")
	}

	var b strings.Builder

	// Column widths
	colAct := 7
	colAgent := 8
	colTool := 8
	colSkill := 16
	pathWidth := width - 75
	if pathWidth < 10 {
		pathWidth = 10
	}

	// Header
	title += d.styles.Description.Render(fmt.Sprintf("  %-*s %-13s %-6s %-*s %-*s %-*s %-*s",
		colAct, "ACTION", "PROJECT", "SESS", colAgent, "AGENT", colTool, "TOOL", colSkill, "SKILL", pathWidth, "PATH")) + "\n"
	title += d.styles.Divider.Render(strings.Repeat("─", width-2)) + "\n"

	visible := height - 4
	if visible < 3 {
		visible = 3
	}

	start := len(d.eventLines) - visible
	if start < 0 {
		start = 0
	}
	if d.focusPanel == 2 {
		if d.logCursor < start {
			start = d.logCursor
		}
		if d.logCursor >= start+visible {
			start = d.logCursor - visible + 1
		}
	}
	end := start + visible
	if end > len(d.eventLines) {
		end = len(d.eventLines)
	}

	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE"))

	for idx := start; idx < end; idx++ {
		line := d.eventLines[idx]
		parts := strings.Split(line, " | ")
		if len(parts) < 6 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}

		// 8-column format: timestamp|project|agent|session|tool|path|action|skill
		project := ""
		agent := ""
		sess := ""
		tool := ""
		path := ""
		skill := ""
		if len(parts) >= 8 {
			project = parts[1]
			agent = parts[2]
			sess = parts[3]
			tool = parts[4]
			path = parts[5]
			skill = parts[7]
		} else if len(parts) >= 7 {
			// 7-column (old): timestamp|agent|session|tool|path|action|skill
			agent = parts[1]
			sess = parts[2]
			tool = parts[3]
			path = parts[4]
			skill = parts[6]
		} else if len(parts) >= 6 {
			agent = parts[1]
			tool = parts[2]
			path = parts[3]
			skill = parts[5]
		}


		if len(path) > pathWidth {
			path = "…" + path[len(path)-pathWidth+1:]
		}

		// Build plain text row with consistent widths
		var act string
		var sty lipgloss.Style
		if strings.Contains(line, "| BLOCK") {
			act = "BLOCK"
			sty = red
		} else if strings.Contains(line, "SKILL-LOAD") {
			act = "LOAD"
			tool = "—"
			path = ""
			sty = cyan
		} else {
			act = "ALLOW"
			sty = green
		}

		plainRow := fmt.Sprintf("  %-*s %-13s %-6s %-*s %-*s %-*s %-*s", colAct, act, project, sess, colAgent, agent, colTool, tool, colSkill, skill, pathWidth, path)

		focused := idx == d.logCursor && d.focusPanel == 2
		if focused {
			b.WriteString(d.styles.Selected.Render("▸" + plainRow[1:]) + "\n")
		} else {
			b.WriteString(sty.Render(plainRow) + "\n")
		}
	}

	return title + b.String()
}

// jumpToLogEntry parses the focused log line and navigates to the skill/rule.
func (d *Dashboard) jumpToLogEntry() {
	if d.logCursor < 0 || d.logCursor >= len(d.eventLines) {
		return
	}

	line := d.eventLines[d.logCursor]
	parts := strings.Split(line, " | ")
	if len(parts) < 6 {
		return
	}

	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	skill := ""
	if len(parts) > 5 {
		skill = parts[5]
	}
	// For SKILL-LOAD events, the skill name is in parts[2] (tool column)
	if strings.Contains(line, "SKILL-LOAD") {
		skill = parts[2]
	}

	if skill == "" {
		return
	}

	// Find the skill in the skills list
	for i, s := range d.skills {
		if s.Name == skill {
			d.skillCursor = i
			d.focusPanel = 1
			d.ruleCursor = 0
			return
		}
	}
}

// LoadEventLog reads the event log file and stores lines from the last 7 days.
// Older lines are pruned from the file to keep it manageable.
func (d *Dashboard) LoadEventLog(projectRoot string) {
	d.projectRoot = projectRoot
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, ".care-bare", "events.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		d.eventLines = nil
		return
	}

	allLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	cutoff := time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02")

	// Keep only lines from the last 7 days that have a skill attached
	var recent []string
	for _, line := range allLines {
		if len(line) < 10 || line[:10] < cutoff {
			continue
		}
		// Only keep events with a skill (BLOCK with skill, SKILL-LOAD, or ALLOW with skill)
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

	// Prune the file if we removed old lines
	if len(recent) < len(allLines) && len(recent) > 0 {
		pruned := strings.Join(recent, "\n") + "\n"
		_ = os.WriteFile(logPath, []byte(pruned), 0644)
	}

	// Keep last 200 lines for display
	if len(recent) > 200 {
		recent = recent[len(recent)-200:]
	}
	d.eventLines = recent

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
