// Package setup — shared TUI styling definitions.
package setup

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shared lipgloss styles and utility functions for all TUI screens.
var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("63")).
			Padding(0, 1)

	StepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(0, 1)

	HighlightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("76")).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("160")).
			Bold(true)

	InfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("45"))

	CheckMark = lipgloss.NewStyle().
			Foreground(lipgloss.Color("76")).
			SetString("✓")

	CrossMark = lipgloss.NewStyle().
			Foreground(lipgloss.Color("160")).
			SetString("✗")

	ArrowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("63")).
			SetString("→")

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2)

	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("63")).
				Padding(0, 1)

	TableRowStyle = lipgloss.NewStyle().
			Padding(0, 1)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("63")).
			Bold(true).
			Padding(0, 1)

	DimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2).
			MarginTop(1)

	FooterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	BadgeRecommended = lipgloss.NewStyle().
				Foreground(lipgloss.Color("76")).
				Bold(true).
				SetString("RECOMMENDED")

	BadgeWarning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			SetString("WARNING")

	CursorStyle = DimStyle.Copy().SetString(" ●")

	BulletStyle = DimStyle.Copy().SetString(" ○")
)

// Header renders the wizard header showing "Vision MCP - Setup Wizard"
// and "Step N of M: Title".
func Header(step, total int, title string) string {
	s := TitleStyle.Render("Vision MCP - Setup Wizard")
	s += "\n"
	s += StepStyle.Render(fmt.Sprintf("Step %d of %d: %s", step, total, title))
	s += "\n"
	s += strings.Repeat("─", 60)
	return s
}

// Box renders a bordered box with a title and content.
func Box(title string, content string) string {
	return BoxStyle.Copy().SetString(title + "\n" + content).String()
}

// Divider renders a dimmed horizontal line of 60 dashes.
func Divider() string {
	return DimStyle.Render(strings.Repeat("─", 60))
}

// ProgressBar renders a filled progress bar (█/░) of the given width.
func ProgressBar(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}
