// settings.go implements the settings view for editing config.json values.
// It renders a navigable list of settings with inline editing support.
package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
)

// settingItem represents a single configurable setting.
type settingItem struct {
	key         string
	label       string
	description string
	value       string
	kind        settingKind
}

// settingKind identifies the type of a setting for validation and display.
type settingKind int

const (
	settingInt    settingKind = iota // Integer value (e.g., TTL)
	settingString                    // Free-text string
)

// Settings is the Bubble Tea model for the settings view.
type Settings struct {
	items      []settingItem
	cursor     int
	editing    bool
	editBuffer string
	editCurPos int
	styles     Styles
	width      int
	height     int
}

// settingsDoneMsg is sent when the user exits the settings view.
type settingsDoneMsg struct {
	config *engine.GlobalConfig // nil if cancelled without changes
}

// NewSettings creates a settings view from the current global config.
func NewSettings(cfg *engine.GlobalConfig, styles Styles) Settings {
	items := []settingItem{
		{
			key:         "skill_ttl_minutes",
			label:       "Skill TTL (minutes)",
			description: "How long a loaded skill stays valid. 0 = no expiry.",
			value:       strconv.Itoa(cfg.SkillTTLMinutes),
			kind:        settingInt,
		},
		{
			key:         "state_ttl_hours",
			label:       "Session TTL (hours)",
			description: "How long session state files are kept before pruning.",
			value:       strconv.Itoa(cfg.StateTTLHours),
			kind:        settingInt,
		},
		{
			key:         "default_agent",
			label:       "Default Agent",
			description: "Default agent scope for new rules: claude, cursor, or * (all).",
			value:       cfg.DefaultAgent,
			kind:        settingString,
		},
	}
	return Settings{
		items:  items,
		styles: styles,
	}
}

// Init returns nil — no initial command needed.
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

		switch msg.String() {
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
			return s, nil

		case "down", "j":
			if s.cursor < len(s.items)-1 {
				s.cursor++
			}
			return s, nil

		case "enter":
			s.editing = true
			s.editBuffer = s.items[s.cursor].value
			s.editCurPos = len(s.editBuffer)
			return s, nil

		case "esc", "q":
			return s, func() tea.Msg {
				return settingsDoneMsg{config: s.buildConfig()}
			}
		}
	}
	return s, nil
}

// updateEditing handles key input while editing a setting value.
func (s Settings) updateEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Validate and commit
		item := s.items[s.cursor]
		if item.kind == settingInt {
			_, err := strconv.Atoi(s.editBuffer)
			if err != nil {
				// Invalid integer — don't commit
				s.editing = false
				return s, nil
			}
		}
		s.items[s.cursor].value = s.editBuffer
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
	b.WriteString(s.styles.Description.Render("  Configure care-bare behavior. Press enter to edit, esc to save & exit.") + "\n\n")

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))
	descStyle := s.styles.Description

	for i, item := range s.items {
		focused := i == s.cursor

		// Value display
		displayVal := item.value
		if s.editing && focused {
			before := s.editBuffer[:s.editCurPos]
			cursor := "█"
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
			b.WriteString(s.styles.Selected.Render("▸"+line[1:]) + "\n")
		} else {
			b.WriteString(keyStyle.Render(label) + "  " + valStyle.Render(displayVal) + "\n")
		}
		b.WriteString("  " + descStyle.Render("  "+item.description) + "\n\n")
	}

	if s.editing {
		b.WriteString("\n")
		b.WriteString(s.styles.Success.Render("  Editing — enter to save, esc to cancel") + "\n")
	}

	return b.String()
}
