// table.go provides a reusable table component wrapping lipgloss/table
// for proper ANSI-aware column alignment. Used by the rules panel.
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

	t := table.New().
		Headers(headers...).
		Rows(strRows...).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		BorderHeader(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			// Header row.
			if row == table.HeaderRow {
				return headerStyle
			}

			// Map visible row index back to original data index.
			dataIdx := start + row

			// Focused row.
			if dataIdx == focusRow {
				return selectedStyle
			}

			// Per-cell style from the row data.
			if dataIdx < len(rows) && col < len(rows[dataIdx].Cells) {
				return rows[dataIdx].Cells[col].Style
			}

			return lipgloss.NewStyle()
		})

	return t.Render()
}
