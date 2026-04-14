// Package tui implements the Charmbracelet-based Terminal User Interface for angry-bear.
package tui

import "github.com/charmbracelet/lipgloss"

// Styles holds all Lip Gloss style definitions used across the TUI.
type Styles struct {
	Header      lipgloss.Style
	SkillName   lipgloss.Style
	Description lipgloss.Style
	RuleRow     lipgloss.Style
	RuleRowAlt  lipgloss.Style
	Selected    lipgloss.Style
	Action      lipgloss.Style
	Error       lipgloss.Style
	Success     lipgloss.Style
	Border      lipgloss.Style
	Help        lipgloss.Style
	Tool        lipgloss.Style
	Path        lipgloss.Style
	Agent       lipgloss.Style
	StatusBar   lipgloss.Style
	SkillBlock  lipgloss.Style
	RuleHeader  lipgloss.Style
	Divider     lipgloss.Style
}

// DefaultStyles returns a polished set of styles with a cohesive colour palette.
func DefaultStyles() Styles {
	accent := lipgloss.Color("#7C3AED")      // Violet
	accentLight := lipgloss.Color("#A78BFA") // Light violet
	cyan := lipgloss.Color("#22D3EE")
	green := lipgloss.Color("#34D399")
	yellow := lipgloss.Color("#FBBF24")
	pink := lipgloss.Color("#F472B6")
	red := lipgloss.Color("#EF4444")
	dim := lipgloss.Color("#6B7280")
	dimLight := lipgloss.Color("#9CA3AF")
	fg := lipgloss.Color("#F9FAFB")
	return Styles{
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(fg).
			Background(accent).
			Padding(0, 2).
			MarginBottom(1),

		SkillName: lipgloss.NewStyle().
			Bold(true).
			Foreground(accentLight),

		Description: lipgloss.NewStyle().
			Foreground(dim),

		RuleRow: lipgloss.NewStyle().
			PaddingLeft(2),

		RuleRowAlt: lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(dimLight),

		Selected: lipgloss.NewStyle().
			Bold(true).
			Foreground(fg).
			Background(accent).
			Padding(0, 1),

		Action: lipgloss.NewStyle().
			Foreground(green).
			PaddingLeft(2),

		Error: lipgloss.NewStyle().
			Foreground(red).
			Bold(true),

		Success: lipgloss.NewStyle().
			Foreground(green).
			Bold(true),

		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2),

		Help: lipgloss.NewStyle().
			Foreground(dim),

		Tool: lipgloss.NewStyle().
			Foreground(pink).
			Bold(true),

		Path: lipgloss.NewStyle().
			Foreground(yellow),

		Agent: lipgloss.NewStyle().
			Foreground(cyan),

		StatusBar: lipgloss.NewStyle().
			Foreground(dim),

		SkillBlock: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#374151")).
			Padding(0, 1).
			MarginBottom(1),

		RuleHeader: lipgloss.NewStyle().
			Foreground(dim).
			Bold(true).
			PaddingLeft(2),

		Divider: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#374151")),
	}
}
