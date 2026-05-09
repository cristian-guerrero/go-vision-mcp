package setup

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

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
			Foreground(lipgloss.Color("42")).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	InfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	CheckMark = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			SetString("✓")

	CrossMark = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
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
			Foreground(lipgloss.Color("42")).
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
				Foreground(lipgloss.Color("42")).
				Bold(true).
				SetString("RECOMMENDED")

	BadgeWarning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			SetString("WARNING")
)

func Header(step, total int, title string) string {
	s := TitleStyle.Render("Vision MCP - Setup Wizard")
	s += "\n"
	s += StepStyle.Render(fmt.Sprintf("Step %d of %d: %s", step, total, title))
	return s
}

func Box(title string, content string) string {
	return BoxStyle.Copy().SetString(title + "\n" + content).String()
}

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
