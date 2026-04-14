// table.go provides a reusable table component wrapping lipgloss/table
// for proper ANSI-aware column alignment.
package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// TableCell is a single cell with text and a foreground color.
type TableCell struct {
	Text  string
	Style lipgloss.Style
}

// TableRow is a row of cells.
type TableRow struct {
	Cells []TableCell
}

// baseStyle is the consistent cell style used for all cells.
// Only foreground color varies per cell.
var baseStyle = lipgloss.NewStyle().PaddingRight(2)

// RenderTable renders rows with headers using lipgloss/table for proper alignment.
// focusRow highlights a specific row (-1 for none).
func RenderTable(
	headers []string,
	rows []TableRow,
	headerStyle lipgloss.Style,
	selectedStyle lipgloss.Style,
	focusRow int,
	scroll *ScrollView,
	visibleRows int,
	width int,
) string {
	if len(rows) == 0 {
		return ""
	}

	// Apply scroll range.
	start := 0
	end := len(rows)
	if scroll != nil && visibleRows > 0 {
		start, end = scroll.VisibleRange(len(rows), visibleRows)
	}

	// Build string data for lipgloss/table.
	var strRows [][]string
	for i := start; i < end; i++ {
		var cells []string
		for _, c := range rows[i].Cells {
			cells = append(cells, c.Text)
		}
		strRows = append(strRows, cells)
	}

	tb := table.New().
		Headers(headers...).
		Rows(strRows...).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		BorderHeader(false)

	if width > 0 {
		tb = tb.Width(width)
	}

	tb = tb.
		StyleFunc(func(row, col int) lipgloss.Style {
			// Header row — use base style with header foreground.
			if row == table.HeaderRow {
				return baseStyle.Inherit(headerStyle)
			}

			// Map visible row index back to original data index.
			dataIdx := start + row

			// Focused row.
			if dataIdx == focusRow {
				return baseStyle.Inherit(selectedStyle)
			}

			// Per-cell color: use base style + only the foreground from the cell style.
			if dataIdx < len(rows) && col < len(rows[dataIdx].Cells) {
				cellStyle := rows[dataIdx].Cells[col].Style
				fg := cellStyle.GetForeground()
				s := baseStyle.Foreground(fg)
				if cellStyle.GetBold() {
					s = s.Bold(true)
				}
				return s
			}

			return baseStyle
		})

	return tb.Render()
}
