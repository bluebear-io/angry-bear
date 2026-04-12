// dashboard.go implements a split-pane dashboard: left panel shows skills,
// right panel shows full skill description + rules with inline editing.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/scanner"
)

// Dashboard is a split-pane view: skills on the left, rules on the right.
type Dashboard struct {
	skills       []scanner.Skill
	config       engine.Config
	loadedSkills map[string]*SkillStatus // Skills loaded with agent info
	skillCursor  int
	ruleCursor   int
	focusPanel   int // 0=left (skills), 1=right (rules)
	editingPath  bool   // true when inline-editing a path
	pathBuffer   string // text being edited
	pathCurPos   int    // cursor position in path edit
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
		// Navigation
		case "up", "k":
			if d.focusPanel == 0 {
				if d.skillCursor > 0 {
					d.skillCursor--
					d.ruleCursor = 0
				}
			} else {
				if d.ruleCursor > 0 {
					d.ruleCursor--
				}
			}
			return d, nil

		case "down", "j":
			if d.focusPanel == 0 {
				if d.skillCursor < len(d.skills)-1 {
					d.skillCursor++
					d.ruleCursor = 0
				}
			} else {
				rules := d.rulesForSkill()
				if d.ruleCursor < len(rules)-1 {
					d.ruleCursor++
				}
			}
			return d, nil

		case "tab", "right", "l":
			if d.focusPanel == 0 {
				rules := d.rulesForSkill()
				if len(rules) > 0 {
					d.focusPanel = 1
					d.ruleCursor = 0
				}
			}
			return d, nil

		case "shift+tab", "left", "h":
			if d.focusPanel == 1 {
				d.focusPanel = 0
			}
			return d, nil

		// Actions on left panel (skills)
		case "enter":
			if d.focusPanel == 0 {
				// Open rule editor for this skill
				if d.skillCursor < len(d.skills) {
					return d, func() tea.Msg {
						return openRuleEditorMsg{
							skillName: d.skills[d.skillCursor].Name,
							ruleIndex: -1,
							existing:  nil,
						}
					}
				}
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

	left := d.renderSkillList(leftWidth, panelHeight)
	right := d.renderRulePanel(rightWidth, panelHeight)

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
		Render(right)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)
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
