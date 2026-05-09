package setup

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vision-mcp/internal/discover"
)

type AgentSetupModel struct {
	agents  []discover.AgentInfo
	checked map[int]bool
	cursor  int
	done    bool
	quit    bool
	confirm bool
	result  []discover.AgentInfo // agents to configure
	err     string
}

func RunAgentSetup(agents []discover.AgentInfo) ([]discover.AgentInfo, error) {
	if len(agents) == 0 {
		return nil, nil
	}

	checked := make(map[int]bool)
	for i, a := range agents {
		if !a.Configured {
			checked[i] = true
		}
	}

	m := AgentSetupModel{
		agents:  agents,
		checked: checked,
		cursor:  0,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, err
	}

	final := result.(AgentSetupModel)
	if final.quit || !final.confirm {
		return nil, nil
	}

	var selected []discover.AgentInfo
	for i, checked := range final.checked {
		if checked {
			selected = append(selected, final.agents[i])
		}
	}
	return selected, nil
}

func (m AgentSetupModel) Init() tea.Cmd { return nil }

func (m AgentSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quit = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.agents) {
				m.cursor++
			}

		case " ", "enter":
			if m.cursor == len(m.agents) {
				m.confirm = true
				m.done = true
				return m, tea.Quit
			}
			m.checked[m.cursor] = !m.checked[m.cursor]

		case "esc", "escape", "backspace":
			m.quit = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m AgentSetupModel) View() string {
	if m.done || m.quit {
		return ""
	}

	var s strings.Builder

	s.WriteString(TitleStyle.Render("MCP Agent Setup"))
	s.WriteString("\n\n")
	s.WriteString(InfoStyle.Render("Select agents to configure with vision-mcp:"))
	s.WriteString("\n\n")

	maxNameLen := 0
	for _, a := range m.agents {
		if len(a.Name) > maxNameLen {
			maxNameLen = len(a.Name)
		}
	}

	for i, a := range m.agents {
		checkbox := "[ ]"
		if m.checked[i] {
			checkbox = "[x]"
		}

		name := a.Name
		status := ""
		if a.Configured {
			status = DimStyle.Render("(already configured)")
		} else {
			status = DimStyle.Render("(not configured)")
		}

		padding := strings.Repeat(" ", maxNameLen-len(name))

		if m.cursor == i {
			checkbox = SelectedStyle.Render(checkbox)
			name = SelectedStyle.Render(name)
			s.WriteString(fmt.Sprintf("  %s %s %s  %s\n", "●", checkbox, name, status))
		} else {
			s.WriteString(fmt.Sprintf("  ○ %s %s  %s\n", checkbox, name+padding, status))
		}
	}

	separator := strings.Repeat("─", 50)
	s.WriteString(fmt.Sprintf("\n  %s\n\n", DimStyle.Render(separator)))

	confirmBtn := "[  Confirm Selection  ]"
	if m.cursor == len(m.agents) {
		confirmBtn = SelectedStyle.Render(confirmBtn)
	}

	s.WriteString(fmt.Sprintf("  %s\n", confirmBtn))

	s.WriteString("\n\n")
	s.WriteString(FooterStyle.Render("[↑/↓] navigate  [Space] toggle  [Enter] confirm  [q] quit"))

	return s.String()
}
