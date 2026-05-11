package tui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// NewTable creates a borderless table with bold headers and consistent padding for CLI output.
func NewTable() *table.Table {
	return table.New().
		Border(lipgloss.Border{}).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return Bold.PaddingRight(3)
			}
			return NoStyle.PaddingRight(3)
		})
}
