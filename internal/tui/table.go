// table.go provides a reusable table component with auto-sizing columns,
// header rendering, scroll support, and per-cell styling. Used by the
// rules panel and event log in the dashboard.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TableCell is a single cell with text and an optional style override.
type TableCell struct {
	Text  string
	Style lipgloss.Style
}

// TableRow is a row of cells.
type TableRow struct {
	Cells []TableCell
}

// TableColumn defines a column's header name and optional width constraints.
type TableColumn struct {
	Name     string
	MinWidth int // 0 = use header length
	MaxWidth int // 0 = unlimited
}

// Table is a reusable scrollable table with auto-sized columns.
type Table struct {
	Columns      []TableColumn
	Rows         []TableRow
	HeaderStyle  lipgloss.Style
	SelectedStyle lipgloss.Style
	Scroll       *ScrollView
}

// computeWidths calculates the width for each column based on data.
func (t *Table) computeWidths() []int {
	widths := make([]int, len(t.Columns))

	// Start with header widths.
	for i, col := range t.Columns {
		widths[i] = len(col.Name)
		if col.MinWidth > widths[i] {
			widths[i] = col.MinWidth
		}
	}

	// Expand to fit data.
	for _, row := range t.Rows {
		for i, cell := range row.Cells {
			if i >= len(widths) {
				break
			}
			if len(cell.Text) > widths[i] {
				widths[i] = len(cell.Text)
			}
		}
	}

	// Apply max constraints.
	for i, col := range t.Columns {
		if col.MaxWidth > 0 && widths[i] > col.MaxWidth {
			widths[i] = col.MaxWidth
		}
	}

	return widths
}

// Render renders the table as a string with header, visible rows, and scroll.
// focusRow is the index of the focused row (-1 for none).
// focused indicates whether this table's panel has focus.
func (t *Table) Render(height, focusRow int, focused bool) string {
	widths := t.computeWidths()

	var b strings.Builder

	// Header
	b.WriteString("  ")
	for i, col := range t.Columns {
		w := widths[i]
		b.WriteString(t.HeaderStyle.Render(fmt.Sprintf("%-*s", w, col.Name)))
		if i < len(t.Columns)-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("\n")

	// Visible rows
	visible := height
	if visible < 1 {
		visible = len(t.Rows)
	}

	start := 0
	end := len(t.Rows)
	if t.Scroll != nil {
		start, end = t.Scroll.VisibleRange(len(t.Rows), visible)
	}

	for ri := start; ri < end; ri++ {
		row := t.Rows[ri]
		isFocused := ri == focusRow && focused

		if isFocused {
			// Render entire row with selected style.
			var line strings.Builder
			line.WriteString("  ")
			for i, cell := range row.Cells {
				if i >= len(widths) {
					break
				}
				text := cell.Text
				if len(text) > widths[i] {
					text = text[:widths[i]-3] + "..."
				}
				line.WriteString(fmt.Sprintf("%-*s", widths[i], text))
				if i < len(row.Cells)-1 {
					line.WriteString(" ")
				}
			}
			b.WriteString(t.SelectedStyle.Render(line.String()))
			// Append last column styled (e.g., SOURCE) outside selection
			// if the last cell has a custom style.
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
			for i, cell := range row.Cells {
				if i >= len(widths) {
					break
				}
				text := cell.Text
				if len(text) > widths[i] {
					text = text[:widths[i]-3] + "..."
				}
				styled := fmt.Sprintf("%-*s", widths[i], text)
				b.WriteString(cell.Style.Render(styled))
				if i < len(row.Cells)-1 {
					b.WriteString(" ")
				}
			}
			b.WriteString("\n")
		}
	}

	// Scroll indicator
	if t.Scroll != nil && len(t.Rows) > visible {
		indicator := fmt.Sprintf("  [%d/%d]", t.Scroll.Cursor+1, len(t.Rows))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(indicator) + "\n")
	}

	return b.String()
}
