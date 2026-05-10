package setup

import (
	"fmt"
	"os"
	"path/filepath"
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
	clipMonOn       bool

	lmModels []discover.ModelInfo
	input    string
	inputErr string

	done      bool
	cancelled bool
	err       error
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
	if result.cancelled {
		return nil, nil
	}
	return result.cfg, nil
}

func NewManualWizard() *ManualWizard {
	return &ManualWizard{
		cfg:         loadOrDefaultConfig(),
		step:        0,
		totalSteps:  6,
		stepCount:   1,
		modelSource: "download",
		llamaSource: "system",
		clipMonOn:   false,
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
		if w.step == 3 && w.modelSource == "custom" {
			return w.handleTextInput(msg)
		}
		if w.step == 4 && w.llamaSource == "custom" {
			return w.handleTextInput(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			w.cancelled = true
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
				// Skip back over custom-only steps that don't apply
				for w.step > 0 {
					skipped := false
					if w.step == 3 && w.modelSource != "custom" {
						w.step--
						skipped = true
					}
					if w.step == 4 && w.llamaSource != "custom" {
						w.step--
						skipped = true
					}
					if !skipped {
						break
					}
				}
			}

		case "enter":
			if w.step >= w.totalSteps-1 {
				if w.hasOptions() {
					w.handleSelection(w.cursorIdx)
				}
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
		w.cancelled = true
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
		w.nextStep()

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
		w.nextStep()
	}
}

func (w *ManualWizard) hasOptions() bool {
	return w.step == 0 || w.step == 2 || w.step == 5
}

func (w *ManualWizard) handleSelection(idx int) {
	switch w.step {
	case 5:
		w.clipMonOn = idx == 0
	}
}

func (w *ManualWizard) advanceStep() {
	switch w.step {
	case 0:
		switch w.cursorIdx {
		case 0:
			w.modelSource = "download"
			w.modelPath = ""
			w.mmprojPath = ""
		case 1:
			w.modelSource = "lmstudio"
			models, _ := discover.FindLMModels()
			var visionModels []discover.ModelInfo
			for _, m := range models {
				if m.HasMMProj {
					visionModels = append(visionModels, m)
				}
			}
			w.lmModels = visionModels
		case 2:
			w.modelSource = "ollama"
			models, _ := discover.FindOllamaModels()
			var visionModels []discover.ModelInfo
			for _, m := range models {
				if m.HasMMProj {
					visionModels = append(visionModels, m)
				}
			}
			w.lmModels = visionModels
		case 3:
			w.modelSource = "custom"
		}
		if w.modelSource == "custom" {
			w.step = 3
			w.cursorIdx = 0
			return
		}
	case 1:
		if len(w.lmModels) > 0 && w.cursorIdx >= 0 && w.cursorIdx < len(w.lmModels) {
			m := w.lmModels[w.cursorIdx]
			w.modelPath = m.Path
			if m.HasMMProj {
				if m.MMProjPath != "" {
					w.mmprojPath = m.MMProjPath
				} else {
					entries, _ := os.ReadDir(m.MMDir)
					for _, e := range entries {
						name := strings.ToLower(e.Name())
						if strings.Contains(name, "mmproj") && strings.HasSuffix(name, ".gguf") {
							w.mmprojPath = filepath.Join(m.MMDir, e.Name())
							break
						}
					}
				}
			}
		}
	case 2:
		switch w.cursorIdx {
		case 0:
			w.llamaSource = "system"
			w.llamaServerPath = ""
			if path, err := discover.FindSystemLlamaServer(); err == nil {
				w.llamaServerPath = path
			}
		case 1:
			w.llamaSource = "download"
			w.llamaServerPath = ""
		case 2:
			w.llamaSource = "custom"
		}
		if w.llamaSource == "custom" {
			w.step = 4
			w.cursorIdx = 0
			return
		}
	}

	w.nextStep()
}

func (w *ManualWizard) nextStep() {
	w.step++
	w.cursorIdx = 0

	for w.step < w.totalSteps-1 {
		skipped := false
		if w.step == 3 && w.modelSource != "custom" {
			w.step++
			skipped = true
		}
		if w.step == 4 && w.llamaSource != "custom" {
			w.step++
			skipped = true
		}
		if !skipped {
			break
		}
	}
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
	switch w.llamaSource {
	case "download":
		w.cfg.LlamaServerMode = "auto"
		w.cfg.LlamaServerPath = ""
	case "system":
		w.cfg.LlamaServerMode = ""
		w.cfg.LlamaServerPath = w.llamaServerPath
	case "custom":
		w.cfg.LlamaServerMode = "custom"
		w.cfg.LlamaServerPath = w.llamaServerPath
	}
	w.cfg.ClipboardMonitorEnabled = w.clipMonOn
}

func (w *ManualWizard) viewSaveAndClipboard() string {
	var s strings.Builder
	s.WriteString("Configuration Summary\n\n")

	sourceLabel := w.modelSource
	if sourceLabel == "lmstudio" {
		sourceLabel = "LM Studio"
	}
	content := fmt.Sprintf("Model source:  %s\nModel:         %s\nMMProj:        %s\nllama-server:  %s",
		HighlightStyle.Render(sourceLabel),
		HighlightStyle.Render(shortPath(w.modelPath, 50)),
		HighlightStyle.Render(shortPath(w.mmprojPath, 50)),
		HighlightStyle.Render(w.llamaSource),
	)
	s.WriteString(Box("Settings", content))
	s.WriteString("\n\n")

	s.WriteString("Clipboard Monitoring\n\n")
	s.WriteString("Keep a history of copied images for multi-image analysis.\n")
	s.WriteString("History is cleared when the server stops.\n\n")

	options := []struct {
		label string
		desc  string
	}{
		{"Yes", "Enable clipboard history (limit: 5 images)"},
		{"No", "Keep disabled — only analyze current clipboard image"},
	}

	w.stepCount = len(options)

	for i, opt := range options {
		bullet := "  ○"
		label := opt.label

		if i == w.cursorIdx {
			bullet = CursorStyle.String()
			label = SelectedStyle.Render(label)
		}

		s.WriteString(fmt.Sprintf("%s %s  %s\n", bullet, label, DimStyle.Render(opt.desc)))
	}

	s.WriteString(fmt.Sprintf("\n  %s Press Enter to save and exit.\n", ArrowStyle))

	return s.String()
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
		if w.modelSource == "custom" {
			s.WriteString(w.viewCustomPathInput())
		} else {
			s.WriteString(w.viewSaveAndClipboard())
		}
	case 4:
		if w.llamaSource == "custom" {
			s.WriteString(w.viewLlamaPathInput())
		} else {
			s.WriteString(w.viewSaveAndClipboard())
		}
	case 5:
		s.WriteString(w.viewSaveAndClipboard())
	default:
		if w.step >= w.totalSteps-1 {
			s.WriteString(w.viewSaveAndClipboard())
		}
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
	case 5:
		return "[↑/↓] select  [Enter] save and exit  [←/esc] back  [q] quit"
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
	case 5:
		return "Clipboard Monitoring"
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
		{"Ollama", "Use models from Ollama"},
		{"Custom path", "Specify path to an existing .gguf file"},
	}

	w.stepCount = len(options)

	for i, opt := range options {
		bullet := "  ○"
		label := opt.label

		if i == w.cursorIdx {
			bullet = CursorStyle.String()
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

	if w.modelSource == "ollama" {
		s.WriteString("Select a vision model from Ollama:\n\n")
	} else {
		s.WriteString("Select a vision model from LM Studio:\n\n")
	}

	w.stepCount = len(w.lmModels)
	if w.stepCount == 0 {
		if w.modelSource == "ollama" {
			s.WriteString(ErrorStyle.Render("No vision models found in Ollama"))
		} else {
			s.WriteString(ErrorStyle.Render("No vision GGUF models found in ~/.lmstudio/models/"))
		}
		s.WriteString(fmt.Sprintf("\n\n  %s Press Enter to go back.\n", ArrowStyle))
		w.stepCount = 1
		return s.String()
	}

	for i, m := range w.lmModels {
		bullet := "  ○"
		name := m.Name
		size := download.FormatBytes(m.Size)

		if i == w.cursorIdx {
			bullet = CursorStyle.String()
			name = SelectedStyle.Render(name)
		}

		s.WriteString(fmt.Sprintf("%s %s  %s  %s vision\n", bullet, name, DimStyle.Render(size), CheckMark))
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
			bullet = CursorStyle.String()
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

	prompt := "  "
	prompt += TitleStyle.Render(">")
	prompt += " "
	if w.input != "" {
		prompt += InfoStyle.Render(w.input)
	} else {
		prompt += DimStyle.Render("type path...")
	}

	s.WriteString(prompt)
	s.WriteString("\n")
	if w.inputErr != "" {
		s.WriteString("\n" + CrossMark.String() + " " + ErrorStyle.Render(w.inputErr))
	}

	return s.String()
}

func (w *ManualWizard) viewLlamaPathInput() string {
	var s strings.Builder
	s.WriteString("Enter path to llama-server binary:\n")
	s.WriteString("(press Enter with empty input to use system PATH)\n\n")

	prompt := "  "
	prompt += TitleStyle.Render(">")
	prompt += " "
	if w.input != "" {
		prompt += InfoStyle.Render(w.input)
	} else {
		prompt += DimStyle.Render("type path or leave empty...")
	}

	s.WriteString(prompt)
	s.WriteString("\n")
	if w.inputErr != "" {
		s.WriteString("\n" + CrossMark.String() + " " + ErrorStyle.Render(w.inputErr))
	}

	return s.String()
}

func (w *ManualWizard) viewSaveConfirm() string {
	var s strings.Builder
	s.WriteString("Configuration Summary\n\n")

	sourceLabel := w.modelSource
	if sourceLabel == "lmstudio" {
		sourceLabel = "LM Studio"
	}
	content := fmt.Sprintf("Model source:  %s\nModel:         %s\nMMProj:        %s\nllama-server:  %s",
		HighlightStyle.Render(sourceLabel),
		HighlightStyle.Render(shortPath(w.modelPath, 50)),
		HighlightStyle.Render(shortPath(w.mmprojPath, 50)),
		HighlightStyle.Render(w.llamaSource),
	)
	s.WriteString(Box("Settings", content))
	s.WriteString("\n")
	s.WriteString(fmt.Sprintf("  %s Press Enter to save and exit.\n", ArrowStyle))

	w.stepCount = 1
	return s.String()
}

func shortPath(path string, max int) string {
	if path == "" {
		return "auto-download"
	}
	if len(path) <= max {
		return path
	}
	return "..." + path[len(path)-max:]
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
	if w.clipMonOn {
		s.WriteString(fmt.Sprintf("  %s Clipboard monitoring: Enabled\n", CheckMark))
	}

	s.WriteString(fmt.Sprintf("\n  Config: %s\n", config.ConfigPath()))
	s.WriteString("\n")
	s.WriteString(InfoStyle.Render("Run: vision-mcp"))

	return BorderStyle.Render(s.String())
}
