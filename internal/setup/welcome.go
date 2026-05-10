package setup

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type WelcomeModel struct {
	cursor int
	choice int
	done   bool
	quit   bool
}

var welcomeOptions = []struct {
	number string
	label  string
}{
	{"1", "Quick setup (auto-detect + download)"},
	{"2", "Guided wizard (TUI step by step)"},
	{"3", "Manual config (use existing models)"},
	{"4", "MCP setup (configure agents)"},
	{"5", "Show status and exit"},
	{"6", "Exit"},
}

func RunWelcome() int {
	m, err := tea.NewProgram(WelcomeModel{cursor: 0}).Run()
	if err != nil {
		return 0
	}
	result := m.(WelcomeModel)
	if result.quit {
		return 0
	}
	return result.choice
}

func (m WelcomeModel) Init() tea.Cmd { return nil }

func (m WelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quit = true
			return m, tea.Quit

		case "up", "k":
			m.cursor = (m.cursor - 1 + len(welcomeOptions)) % len(welcomeOptions)

		case "down", "j":
			m.cursor = (m.cursor + 1) % len(welcomeOptions)

		case "enter":
			m.choice = m.cursor + 1
			m.done = true
			return m, tea.Quit

		case "1", "2", "3", "4", "5", "6":
			m.choice = int(msg.String()[0] - '0')
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m WelcomeModel) View() string {
	if m.done || m.quit {
		return ""
	}

	var s strings.Builder

	s.WriteString(TitleStyle.Render("Vision MCP"))
	s.WriteString("\n\n")
	s.WriteString("No configuration found.")
	s.WriteString("\n\n")
	s.WriteString("What would you like to do?")
	s.WriteString("\n\n")

	for i, opt := range welcomeOptions {
		bullet := "  ○"
		label := fmt.Sprintf("[%s] %s", opt.number, opt.label)

		if i == m.cursor {
			bullet = CursorStyle.String()
			label = SelectedStyle.Render(label)
		}

		s.WriteString(fmt.Sprintf("%s %s\n", bullet, label))
	}

	s.WriteString("\n\n")
	s.WriteString(FooterStyle.Render("[↑/↓] navigate  [1-6] shortcut  [Enter] select  [q] quit"))

	return BorderStyle.Render(s.String())
}

var _ = lipgloss.NewStyle()
