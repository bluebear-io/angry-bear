// settings.go implements the settings view for editing config.json values.
// It renders a navigable list of settings with inline editing support.
// Supports two config levels (global and project) and checkout switching
// for repos with multiple local paths.
package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Blue-Bear-Security/angry-bear/internal/engine"
)

// settingItem represents a single configurable setting.
type settingItem struct {
	key         string
	label       string
	description string
	value       string
	kind        settingKind
	level       string   // "project", "global", or "both" — controls visibility per config level
	readonly    bool     // true for display-only items like project root
	options     []string // for cycle-able items (e.g., checkout paths)
	optionIdx   int      // current index in options
}

// settingKind identifies the type of a setting for validation and display.
type settingKind int

const (
	settingInt    settingKind = iota // Integer value (e.g., TTL)
	settingString                    // Free-text string
	settingCycle                     // Cycle through options with up/down
)

// Settings is the Bubble Tea model for the settings view.
type Settings struct {
	items           []settingItem
	cursor          int
	editing         bool
	editBuffer      string
	editCurPos      int
	configLevel     string // "global" or "project"
	projectRoot     string // current project root
	availablePaths  []string
	originalPathIdx int // original index in availablePaths for detecting changes
	styles          Styles
	width           int
	height          int
}

// settingsDoneMsg is sent when the user exits the settings view.
type settingsDoneMsg struct {
	config        *engine.GlobalConfig // nil if cancelled without changes
	configLevel   string               // "global" or "project" -- where to save
	preferredPath string               // non-empty when the user changed the checkout path
}

// NewSettings creates a settings view from the current global config.
// projectRoot is the current project root. availablePaths lists all local
// checkout paths for this repo (may be nil for single-checkout repos).
func NewSettings(
	cfg *engine.GlobalConfig,
	styles Styles,
	projectRoot string,
	availablePaths []string,
) Settings {
	var items []settingItem

	// Project-only: Project Root item.
	if len(availablePaths) > 1 {
		currentIdx := 0
		for i, p := range availablePaths {
			if p == projectRoot {
				currentIdx = i
				break
			}
		}
		items = append(items, settingItem{
			key:         "project_root",
			label:       "Project Root",
			description: "Current checkout path. Use left/right to switch between local copies.",
			value:       projectRoot,
			kind:        settingCycle,
			options:     availablePaths,
			optionIdx:   currentIdx,
			level:       "project",
		})
	} else {
		items = append(items, settingItem{
			key:         "project_root",
			label:       "Project Root",
			description: "Current project root directory.",
			value:       projectRoot,
			kind:        settingString,
			readonly:    true,
			level:       "project",
		})
	}

	// Settings available at both levels.
	items = append(items,
		settingItem{
			key:         "skill_ttl_minutes",
			label:       "Skill TTL (minutes)",
			description: "How long a loaded skill stays valid. 0 = no expiry.",
			value:       strconv.Itoa(cfg.SkillTTLMinutes),
			kind:        settingInt,
			level:       "both",
		},
		settingItem{
			key:         "state_ttl_hours",
			label:       "Session TTL (hours)",
			description: "How long session state files are kept before pruning.",
			value:       strconv.Itoa(cfg.StateTTLHours),
			kind:        settingInt,
			level:       "both",
		},
		settingItem{
			key:         "default_agent",
			label:       "Default Agent",
			description: "Default agent scope for new rules: claude, cursor, or * (all).",
			value:       cfg.DefaultAgent,
			kind:        settingString,
			level:       "both",
		},
	)

	// Find original path index for detecting changes on exit.
	originalIdx := 0
	if len(availablePaths) > 1 {
		for i, p := range availablePaths {
			if p == projectRoot {
				originalIdx = i
				break
			}
		}
	}

	return Settings{
		items:           items,
		configLevel:     "project",
		projectRoot:     projectRoot,
		availablePaths:  availablePaths,
		originalPathIdx: originalIdx,
		styles:          styles,
	}
}

// visibleItems returns items that should be shown for the current config level.
func (s *Settings) visibleItems() []settingItem {
	var visible []settingItem
	for _, item := range s.items {
		if item.level == "both" || item.level == s.configLevel {
			visible = append(visible, item)
		}
	}
	return visible
}

// visibleToRealIndex maps a visible index to the real index in s.items.
func (s *Settings) visibleToRealIndex(visIdx int) int {
	count := 0
	for i, item := range s.items {
		if item.level == "both" || item.level == s.configLevel {
			if count == visIdx {
				return i
			}
			count++
		}
	}
	return 0
}

// Init returns nil -- no initial command needed.
func (s Settings) Init() tea.Cmd { return nil }

// Update handles key input for the settings view.
func (s Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		return s, nil

	case tea.KeyMsg:
		if s.editing {
			return s.updateEditing(msg)
		}

		visible := s.visibleItems()
		realIdx := s.visibleToRealIndex(s.cursor)

		switch msg.String() {
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
			return s, nil

		case "down", "j":
			if s.cursor < len(visible)-1 {
				s.cursor++
			}
			return s, nil

		case "left":
			// Cycle backward on cycle items
			if s.items[realIdx].kind == settingCycle {
				s.cycleSetting(realIdx, -1)
			}
			return s, nil

		case "right":
			// Cycle forward on cycle items
			if s.items[realIdx].kind == settingCycle {
				s.cycleSetting(realIdx, 1)
			}
			return s, nil

		case "enter":
			item := s.items[realIdx]
			if item.readonly {
				return s, nil
			}
			if item.kind == settingCycle {
				s.cycleSetting(realIdx, 1)
				return s, nil
			}
			s.editing = true
			s.editBuffer = item.value
			s.editCurPos = len(s.editBuffer)
			return s, nil

		case "g":
			s.configLevel = "global"
			s.cursor = 0 // Reset cursor when switching levels
			return s, nil

		case "p":
			s.configLevel = "project"
			s.cursor = 0 // Reset cursor when switching levels
			return s, nil

		case "esc", "q":
			return s, s.buildDoneMsg
		}
	}
	return s, nil
}

// cycleSetting moves through the options of a settingCycle item.
func (s *Settings) cycleSetting(realIdx, delta int) {
	item := &s.items[realIdx]
	if item.kind != settingCycle || len(item.options) == 0 {
		return
	}
	item.optionIdx = (item.optionIdx + delta + len(item.options)) % len(item.options)
	item.value = item.options[item.optionIdx]
}

// updateEditing handles key input while editing a setting value.
func (s Settings) updateEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Validate and commit
		realIdx := s.visibleToRealIndex(s.cursor)
		item := s.items[realIdx]
		if item.kind == settingInt {
			_, err := strconv.Atoi(s.editBuffer)
			if err != nil {
				// Invalid integer -- don't commit
				s.editing = false
				return s, nil
			}
		}
		s.items[realIdx].value = s.editBuffer
		s.editing = false
		return s, nil

	case "esc":
		s.editing = false
		return s, nil

	case "backspace":
		if s.editCurPos > 0 {
			s.editBuffer = s.editBuffer[:s.editCurPos-1] + s.editBuffer[s.editCurPos:]
			s.editCurPos--
		}
		return s, nil

	case "left":
		if s.editCurPos > 0 {
			s.editCurPos--
		}
		return s, nil

	case "right":
		if s.editCurPos < len(s.editBuffer) {
			s.editCurPos++
		}
		return s, nil

	case "ctrl+a":
		s.editCurPos = 0
		return s, nil

	case "ctrl+e":
		s.editCurPos = len(s.editBuffer)
		return s, nil

	default:
		key := msg.String()
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.editBuffer = s.editBuffer[:s.editCurPos] + key + s.editBuffer[s.editCurPos:]
			s.editCurPos++
		}
		return s, nil
	}
}

// buildDoneMsg constructs a settingsDoneMsg from the current state.
func (s Settings) buildDoneMsg() tea.Msg {
	cfg := s.buildConfig()

	// Detect checkout path change.
	var preferredPath string
	for _, item := range s.items {
		if item.key == "project_root" && item.kind == settingCycle {
			if item.optionIdx != s.originalPathIdx {
				preferredPath = item.value
			}
			break
		}
	}

	return settingsDoneMsg{
		config:        cfg,
		configLevel:   s.configLevel,
		preferredPath: preferredPath,
	}
}

// buildConfig constructs a GlobalConfig from the current setting values.
func (s Settings) buildConfig() *engine.GlobalConfig {
	cfg := &engine.GlobalConfig{}
	for _, item := range s.items {
		switch item.key {
		case "skill_ttl_minutes":
			cfg.SkillTTLMinutes, _ = strconv.Atoi(item.value)
		case "state_ttl_hours":
			cfg.StateTTLHours, _ = strconv.Atoi(item.value)
		case "default_agent":
			cfg.DefaultAgent = item.value
		}
	}
	return cfg
}

// View renders the settings view.
func (s Settings) View() string {
	var b strings.Builder

	title := s.styles.RuleHeader.Render("  SETTINGS")
	b.WriteString(title + "\n")

	levelTag := s.styles.Success.Render(strings.ToUpper(s.configLevel))
	b.WriteString(s.styles.Description.Render("  Editing: ") + levelTag +
		s.styles.Description.Render("  (g=global, p=project)  Press enter to edit, esc to save & exit.") + "\n\n")

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))
	descStyle := s.styles.Description
	readonlyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

	visible := s.visibleItems()
	for i, item := range visible {
		focused := i == s.cursor

		// Value display
		displayVal := item.value
		if s.editing && focused {
			before := s.editBuffer[:s.editCurPos]
			cursor := "\u2588"
			after := ""
			if s.editCurPos < len(s.editBuffer) {
				cursor = string(s.editBuffer[s.editCurPos])
				after = s.editBuffer[s.editCurPos+1:]
			}
			displayVal = before + s.styles.Selected.Render(cursor) + after
		}

		label := fmt.Sprintf("  %-25s", item.label)
		if focused && !s.editing {
			line := fmt.Sprintf("  %-25s  %s", item.label, item.value)
			b.WriteString(s.styles.Selected.Render("\u25b8"+line[1:]) + "\n")
		} else if item.readonly {
			b.WriteString(keyStyle.Render(label) + "  " + readonlyStyle.Render(displayVal) + "\n")
		} else {
			b.WriteString(keyStyle.Render(label) + "  " + valStyle.Render(displayVal) + "\n")
		}

		// Show cycle hint for checkout paths.
		desc := item.description
		if item.kind == settingCycle && focused {
			desc = fmt.Sprintf("%s  [%d/%d]", desc, item.optionIdx+1, len(item.options))
		}
		b.WriteString("  " + descStyle.Render("  "+desc) + "\n\n")
	}

	if s.editing {
		b.WriteString("\n")
		b.WriteString(s.styles.Success.Render("  Editing -- enter to save, esc to cancel") + "\n")
	}

	return b.String()
}
