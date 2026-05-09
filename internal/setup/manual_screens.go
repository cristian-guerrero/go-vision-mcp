package setup

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vision-mcp/internal/config"
	"github.com/vision-mcp/internal/discover"
	"github.com/vision-mcp/internal/download"
	"github.com/vision-mcp/internal/hardware"
)

type ManualWizard struct {
	cfg *config.Config

	step       int
	totalSteps int
	cursorIdx  int
	stepCount  int

	modelSource     string
	modelPath       string
	mmprojPath      string
	llamaSource     string
	llamaServerPath string

	lmModels []discover.ModelInfo
	input    string
	inputErr string

	done bool
	err  error
}

func RunManualWizard() (*config.Config, error) {
	w := NewManualWizard()
	p := tea.NewProgram(w, tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := m.(*ManualWizard)
	if result.err != nil {
		return nil, result.err
	}
	return result.cfg, nil
}

func NewManualWizard() *ManualWizard {
	return &ManualWizard{
		cfg:         loadOrDefaultConfig(),
		step:        0,
		totalSteps:  5,
		stepCount:   1,
		modelSource: "download",
		llamaSource: "system",
	}
}

func loadOrDefaultConfig() *config.Config {
	cfg, err := config.LoadConfig()
	if err != nil {
		d := config.DefaultConfig()
		return &d
	}
	return cfg
}

func (w *ManualWizard) Init() tea.Cmd {
	return nil
}

func (w *ManualWizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if w.step == 3 {
			return w.handleTextInput(msg)
		}
		if w.step == 4 {
			return w.handleTextInput(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return w, tea.Quit

		case "up", "k":
			if w.stepCount > 0 {
				w.cursorIdx = (w.cursorIdx - 1 + w.stepCount) % w.stepCount
			}

		case "down", "j":
			if w.stepCount > 0 {
				w.cursorIdx = (w.cursorIdx + 1) % w.stepCount
			}

		case "left", "esc", "escape", "backspace":
			if w.step > 0 {
				w.step--
				w.cursorIdx = 0
			}

		case "enter":
			if w.step == w.totalSteps-1 {
				w.saveConfig()
				w.done = true
				return w, tea.Quit
			}
			w.advanceStep()
		}
	}

	return w, nil
}

func (w *ManualWizard) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return w, tea.Quit

	case "enter":
		w.textSubmit()

	case "backspace":
		if len(w.input) > 0 {
			w.input = w.input[:len(w.input)-1]
		}

	default:
		if len(msg.String()) == 1 {
			w.input += msg.String()
		}
	}

	return w, nil
}

func (w *ManualWizard) textSubmit() {
	w.input = strings.TrimSpace(w.input)

	switch w.step {
	case 3:
		if w.input == "" {
			w.inputErr = "Path cannot be empty"
			return
		}
		if _, err := os.Stat(w.input); os.IsNotExist(err) {
			w.inputErr = fmt.Sprintf("File not found: %s", w.input)
			return
		}
		w.modelPath = w.input
		w.input = ""
		w.inputErr = ""
		w.step++
		w.cursorIdx = 0

	case 4:
		if w.input != "" {
			if w.input != "." {
				if _, err := os.Stat(w.input); os.IsNotExist(err) {
					w.inputErr = fmt.Sprintf("File not found: %s", w.input)
					return
				}
				w.llamaServerPath = w.input
			}
		}
		w.input = ""
		w.inputErr = ""
		w.step++
		w.cursorIdx = 0
	}
}

func (w *ManualWizard) advanceStep() {
	switch w.step {
	case 0:
		switch w.cursorIdx {
		case 0:
			w.modelSource = "download"
		case 1:
			w.modelSource = "lmstudio"
			models, _ := discover.FindLMModels()
			w.lmModels = models
		case 2:
			w.modelSource = "custom"
		}
		if w.modelSource == "custom" {
			w.step = 3
			w.cursorIdx = 0
			return
		}
	case 1:
		if w.modelSource == "lmstudio" && len(w.lmModels) > 0 && w.cursorIdx >= 0 && w.cursorIdx < len(w.lmModels) {
			m := w.lmModels[w.cursorIdx]
			w.modelPath = m.Path
			if m.HasMMProj {
				entries, _ := os.ReadDir(m.MMDir)
				for _, e := range entries {
					name := strings.ToLower(e.Name())
					if strings.Contains(name, "mmproj") && strings.HasSuffix(name, ".gguf") {
						w.mmprojPath = m.MMDir + "/" + e.Name()
						break
					}
				}
			}
		}
	case 2:
		switch w.cursorIdx {
		case 0:
			w.llamaSource = "system"
			if path, err := discover.FindSystemLlamaServer(); err == nil {
				w.llamaServerPath = path
			}
		case 1:
			w.llamaSource = "download"
		case 2:
			w.llamaSource = "custom"
		}
		if w.llamaSource == "custom" {
			w.step = 4
			w.cursorIdx = 0
			return
		}
	}

	w.step++
	w.cursorIdx = 0
}

func (w *ManualWizard) saveConfig() {
	if w.modelSource == "download" && w.modelPath == "" {
		return
	}

	if w.modelPath != "" {
		w.cfg.ModelPathOverride = w.modelPath
	}
	if w.mmprojPath != "" {
		w.cfg.MMProjPathOverride = w.mmprojPath
	}
	if w.llamaServerPath != "" {
		w.cfg.LlamaServerPath = w.llamaServerPath
	}
}

func (w *ManualWizard) View() string {
	if w.done {
		return w.viewComplete()
	}

	var s strings.Builder
	s.WriteString(Header(w.step+1, w.totalSteps, w.stepTitle()))
	s.WriteString("\n\n")

	switch w.step {
	case 0:
		s.WriteString(w.viewModelSource())
	case 1:
		s.WriteString(w.viewModelSelection())
	case 2:
		s.WriteString(w.viewLlamaSource())
	case 3:
		s.WriteString(w.viewCustomPathInput())
	case 4:
		s.WriteString(w.viewLlamaPathInput())
	}

	s.WriteString("\n\n")
	s.WriteString(FooterStyle.Render(w.footer()))

	if w.err != nil {
		s.WriteString("\n" + ErrorStyle.Render(fmt.Sprintf("Error: %v", w.err)))
	}

	return s.String()
}

func (w *ManualWizard) footer() string {
	switch w.step {
	case 3, 4:
		return "[Enter] submit  [esc] back  [q] quit"
	default:
		return "[↑/↓] navigate  [Enter] confirm  [←/esc] back  [q] quit"
	}
}

func (w *ManualWizard) stepTitle() string {
	switch w.step {
	case 0:
		return "Model Source"
	case 1:
		return "Select Model"
	case 2:
		return "llama-server Source"
	case 3:
		return "Custom Model Path"
	case 4:
		return "llama-server Path"
	}
	return ""
}

func (w *ManualWizard) viewModelSource() string {
	var s strings.Builder
	s.WriteString("Where should the model come from?\n\n")

	options := []struct {
		label string
		desc  string
	}{
		{"Download new", "Download a model from Hugging Face"},
		{"LM Studio", "Use models already downloaded by LM Studio"},
		{"Custom path", "Specify path to an existing .gguf file"},
	}

	w.stepCount = len(options)

	for i, opt := range options {
		bullet := "  ○"
		label := opt.label

		if i == w.cursorIdx {
			bullet = DimStyle.Render(" ●")
			label = SelectedStyle.Render(label)
		}

		s.WriteString(fmt.Sprintf("%s %s  %s\n", bullet, label, DimStyle.Render(opt.desc)))
	}

	return s.String()
}

func (w *ManualWizard) viewModelSelection() string {
	var s strings.Builder

	if w.modelSource == "download" {
		s.WriteString("Selected: Download from Hugging Face\n\n")

		hw, _ := hardware.DetectHardware()
		recommended := config.DefaultConfig()
		if hw != nil {
			recommended.Quantization = hardware.RecommendQuantization(hw)
		}

		w.stepCount = 1

		s.WriteString(fmt.Sprintf("Repository: %s\n", w.cfg.RepoID))
		s.WriteString(fmt.Sprintf("Quantization: %s\n", recommended.Quantization))
		s.WriteString(fmt.Sprintf("\n  %s Press Enter to continue.\n", ArrowStyle))
		return s.String()
	}

	s.WriteString("Select a model from LM Studio:\n\n")

	w.stepCount = len(w.lmModels)
	if w.stepCount == 0 {
		s.WriteString(ErrorStyle.Render("No GGUF models found in ~/.lmstudio/models/"))
		s.WriteString(fmt.Sprintf("\n\n  %s Press Enter to go back.\n", ArrowStyle))
		w.stepCount = 1
		return s.String()
	}

	for i, m := range w.lmModels {
		bullet := "  ○"
		name := m.Name
		vision := ""

		if i == w.cursorIdx {
			bullet = DimStyle.Render(" ●")
			name = SelectedStyle.Render(name)
		}

		if m.HasMMProj {
			vision = " " + CheckMark.String() + " vision"
		} else {
			vision = " " + CrossMark.String() + " no vision"
		}

		size := download.FormatBytes(m.Size)

		s.WriteString(fmt.Sprintf("%s %s  %s%s\n", bullet, name, DimStyle.Render(size), vision))
	}

	return s.String()
}

func (w *ManualWizard) viewLlamaSource() string {
	var s strings.Builder
	s.WriteString("Where should llama-server come from?\n\n")

	options := []struct {
		label string
		desc  string
	}{
		{"System PATH", "Use llama-server already installed on your system"},
		{"Download", "Download llama-server from llama.cpp releases"},
		{"Custom path", "Specify path to an existing llama-server binary"},
	}

	w.stepCount = len(options)

	for i, opt := range options {
		bullet := "  ○"
		label := opt.label

		if i == w.cursorIdx {
			bullet = DimStyle.Render(" ●")
			label = SelectedStyle.Render(label)
		}

		s.WriteString(fmt.Sprintf("%s %s  %s\n", bullet, label, DimStyle.Render(opt.desc)))
	}

	if w.llamaSource == "system" {
		if path, err := discover.FindSystemLlamaServer(); err == nil {
			s.WriteString(fmt.Sprintf("\n  %s Found: %s\n", CheckMark, path))
		} else {
			s.WriteString(fmt.Sprintf("\n  %s Not found in PATH\n", CrossMark))
		}
	}

	return s.String()
}

func (w *ManualWizard) viewCustomPathInput() string {
	var s strings.Builder
	s.WriteString("Enter path to the model .gguf file:\n\n")

	prompt := "  > "
	if w.input != "" {
		prompt += InfoStyle.Render(w.input)
	} else {
		prompt += DimStyle.Render("type path...")
	}

	s.WriteString(prompt)
	s.WriteString("\n")
	if w.inputErr != "" {
		s.WriteString("\n" + ErrorStyle.Render(w.inputErr))
	}

	return s.String()
}

func (w *ManualWizard) viewLlamaPathInput() string {
	var s strings.Builder
	s.WriteString("Enter path to llama-server binary:\n")
	s.WriteString("(press Enter with empty input to use system PATH)\n\n")

	prompt := "  > "
	if w.input != "" {
		prompt += InfoStyle.Render(w.input)
	} else {
		prompt += DimStyle.Render("type path or leave empty...")
	}

	s.WriteString(prompt)
	s.WriteString("\n")
	if w.inputErr != "" {
		s.WriteString("\n" + ErrorStyle.Render(w.inputErr))
	}

	return s.String()
}

func (w *ManualWizard) viewComplete() string {
	var s strings.Builder
	s.WriteString(TitleStyle.Render("Configuration Saved!"))
	s.WriteString("\n\n")

	if w.modelPath != "" {
		s.WriteString(fmt.Sprintf("  %s Model: %s\n", CheckMark, w.modelPath))
	}
	if w.mmprojPath != "" {
		s.WriteString(fmt.Sprintf("  %s MMProj: %s\n", CheckMark, w.mmprojPath))
	}
	if w.llamaServerPath != "" {
		s.WriteString(fmt.Sprintf("  %s Server: %s\n", CheckMark, w.llamaServerPath))
	}

	s.WriteString(fmt.Sprintf("\n  Config: %s\n", config.ConfigPath()))
	s.WriteString("\n")
	s.WriteString(InfoStyle.Render("Run: vision-mcp"))

	return BorderStyle.Render(s.String())
}
