// Package setup provides Bubble Tea TUI models for configuration
// wizards, manual setup, model selection, download progress, and
// agent configuration screens.
package setup

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cristian-guerrero/go-vision-mcp/internal/config"
	"github.com/cristian-guerrero/go-vision-mcp/internal/discover"
	"github.com/cristian-guerrero/go-vision-mcp/internal/hardware"
)

// Wizard implements a 5-step Bubble Tea model: Model → Backend →
// Quantization → Clipboard → Summary. It walks the user through the
// entire initial configuration interactively.
type Wizard struct {
	hw            *hardware.HardwareProfile
	cfg           *config.Config
	step          int
	totalSteps    int
	stepCount     int
	cursorIdx     int
	selectedModel int
	backend       string
	quant         string
	installDir    string
	downloadDir   string
	clipMonOn     bool

	done      bool
	cancelled bool
	err       error
}

// NewWizard creates a Wizard pre-populated with detected hardware and
// default config values.
func NewWizard() *Wizard {
	hw, _ := hardware.DetectHardware()
	cfg := config.DefaultConfig()

	if hw != nil {
		cfg.Quantization = hardware.RecommendQuantization(hw)
		cfg.LlamaBackend = hardware.RecommendBackend(hw)
	}

	return &Wizard{
		hw:            hw,
		cfg:           &cfg,
		step:          0,
		totalSteps:    5,
		stepCount:     1,
		selectedModel: DefaultModelIndex(),
		backend:       cfg.LlamaBackend,
		quant:         cfg.Quantization,
		installDir:    config.InstallDir(),
		downloadDir:   config.InstallDir(),
		clipMonOn:     false,
	}
}

// RunWizard creates and runs the interactive TUI wizard, returning the
// final config (or nil if cancelled) and any error.
func RunWizard() (*config.Config, error) {
	w := NewWizard()
	p := tea.NewProgram(w, tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := m.(*Wizard)
	if result.err != nil {
		return nil, result.err
	}
	if result.cancelled {
		return nil, nil
	}
	return result.cfg, nil
}

func (w *Wizard) Init() tea.Cmd {
	return nil
}

func (w *Wizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			w.cancelled = true
			return w, tea.Quit

		case "up", "k":
			if w.hasOptions() && w.stepCount > 0 {
				w.cursorIdx = (w.cursorIdx - 1 + w.stepCount) % w.stepCount
			}

		case "down", "j":
			if w.hasOptions() && w.stepCount > 0 {
				w.cursorIdx = (w.cursorIdx + 1) % w.stepCount
			}

		case "left", "esc", "escape", "backspace":
			if w.step > 0 {
				w.step--
				w.cursorIdx = 0
			}

		case "enter":
			if w.step == w.totalSteps-1 {
				w.cfg.RepoID = AvailableModels[w.selectedModel].RepoID
				w.cfg.Quantization = w.quant
				w.cfg.LlamaBackend = w.backend
				w.cfg.ClipboardMonitorEnabled = w.clipMonOn
				w.done = true
				return w, tea.Quit
			}
			if w.hasOptions() {
				w.handleSelection(w.cursorIdx)
			}
			w.step++
			w.cursorIdx = 0
		}
	}

	return w, nil
}

func (w *Wizard) hasOptions() bool {
	return w.step >= 0 && w.step <= 3
}

func (w *Wizard) handleSelection(idx int) {
	switch w.step {
	case 0:
		if idx >= 0 && idx < len(AvailableModels) {
			w.selectedModel = idx
		}
	case 1:
		backends := []string{"cuda", "cpu", "vulkan", "metal"}
		if idx >= 0 && idx < len(backends) {
			w.backend = backends[idx]
		}
	case 2:
		quants := hardware.AvailableQuantizations()
		if idx >= 0 && idx < len(quants) {
			w.quant = quants[idx].Name
		}
	case 3:
		w.clipMonOn = idx == 0
	}
}

func (w *Wizard) View() string {
	if w.done {
		return w.viewComplete()
	}

	var s strings.Builder

	s.WriteString(Header(w.step+1, w.totalSteps, w.stepTitle()))
	s.WriteString("\n\n")

	switch w.step {
	case 0:
		s.WriteString(w.viewModelSelect())
	case 1:
		s.WriteString(w.viewBackend())
	case 2:
		s.WriteString(w.viewQuantization())
	case 3:
		s.WriteString(w.viewClipboardMonitor())
	case 4:
		s.WriteString(w.viewSummary())
	}

	s.WriteString("\n\n")
	s.WriteString(FooterStyle.Render(w.footer()))

	if w.err != nil {
		s.WriteString("\n" + ErrorStyle.Render(fmt.Sprintf("Error: %v", w.err)))
	}

	return s.String()
}

func (w *Wizard) footer() string {
	switch w.step {
	case 4:
		return "[Enter] save and exit  [q] quit"
	default:
		return "[↑/↓] navigate  [Enter] confirm  [←/esc] back  [q] quit"
	}
}

func (w *Wizard) stepTitle() string {
	switch w.step {
	case 0:
		return "Select Model"
	case 1:
		return "Select Backend"
	case 2:
		return "Select Quantization"
	case 3:
		return "Clipboard Monitoring"
	case 4:
		return "Summary"
	}
	return ""
}

func (w *Wizard) viewHardware() string {
	var s strings.Builder

	if w.hw == nil {
		s.WriteString(ErrorStyle.Render("Could not detect hardware"))
		return s.String()
	}

	s.WriteString(InfoStyle.Render("Detected Hardware") + "\n\n")

	ramGB := float64(w.hw.TotalRAM) / (1024 * 1024 * 1024)
	availGB := float64(w.hw.AvailableRAM) / (1024 * 1024 * 1024)
	diskGB := float64(w.hw.FreeDisk) / (1024 * 1024 * 1024)

	s.WriteString(fmt.Sprintf("  RAM:    %.1f GB (%.1f GB available)\n", ramGB, availGB))
	s.WriteString(fmt.Sprintf("  Disk:   %.1f GB free\n", diskGB))

	if w.hw.GPU.Present {
		vramGB := float64(w.hw.GPU.VRAM) / (1024 * 1024 * 1024)
		s.WriteString(fmt.Sprintf("  GPU:    %s (%.1f GB VRAM, driver %s)\n", w.hw.GPU.Vendor, vramGB, w.hw.GPU.DriverVer))
		s.WriteString(fmt.Sprintf("  Backend: %s (auto-detected)\n", w.hw.GPU.BackendType))
	} else {
		s.WriteString("  GPU:    none (CPU only)\n")
	}

	s.WriteString(fmt.Sprintf("\n  %s Recommended backend: %s\n", ArrowStyle, HighlightStyle.Render(w.cfg.LlamaBackend)))
	s.WriteString(fmt.Sprintf("  %s Recommended quantization: %s\n", ArrowStyle, HighlightStyle.Render(w.cfg.Quantization)))

	return BorderStyle.Render(s.String())
}

func (w *Wizard) viewModelSelect() string {
	var s strings.Builder
	s.WriteString("Select vision model:\n\n")

	w.stepCount = len(AvailableModels)

	for i, m := range AvailableModels {
		bullet := "  ○"
		name := fmt.Sprintf("%s (%s)", m.RepoID, m.Params)
		desc := m.Desc
		extra := ""

		if i == w.cursorIdx {
			bullet = CursorStyle.String()
			name = SelectedStyle.Render(name)
			if i == DefaultModelIndex() {
				extra = " " + BadgeRecommended.String()
			}
		} else if i == DefaultModelIndex() {
			extra = " " + BadgeRecommended.String()
		}

		s.WriteString(fmt.Sprintf("%s %s  %s%s\n", bullet, name, DimStyle.Render(desc), extra))
	}

	return s.String()
}

func (w *Wizard) viewBackend() string {
	var s strings.Builder
	s.WriteString("Select llama-server backend:\n\n")

	backends := []struct {
		key   string
		label string
		desc  string
	}{
		{"cuda", "CUDA", "NVIDIA GPU acceleration (fastest)"},
		{"cpu", "CPU", "Works on any system (slower)"},
		{"vulkan", "Vulkan", "AMD/Intel GPU via Vulkan"},
		{"metal", "Metal", "Apple Silicon / Metal"},
	}

	w.stepCount = len(backends)
	recommended := w.cfg.LlamaBackend

	for i, b := range backends {
		bullet := "  ○"
		name := b.label
		desc := b.desc
		extra := ""

		if i == w.cursorIdx {
			bullet = CursorStyle.String()
			name = SelectedStyle.Render(name)
			if b.key == recommended {
				extra = " " + BadgeRecommended.String()
			}
		} else if b.key == recommended {
			extra = " " + BadgeRecommended.String()
		}

		s.WriteString(fmt.Sprintf("%s %s  %s%s\n", bullet, name, DimStyle.Render(desc), extra))
	}

	return s.String()
}

func (w *Wizard) viewQuantization() string {
	var s strings.Builder
	s.WriteString("Select model quantization:\n\n")

	quants := hardware.AvailableQuantizations()
	rec := w.cfg.Quantization
	w.stepCount = len(quants)

	for i, q := range quants {
		bullet := "  ○"
		name := q.Name
		size := q.Size
		label := q.Label
		extra := ""

		if i == w.cursorIdx {
			bullet = CursorStyle.String()
			name = SelectedStyle.Render(name)
			if q.Name == rec {
				extra = " " + BadgeRecommended.String()
			}
		} else if q.Name == rec {
			extra = " " + BadgeRecommended.String()
		}

		s.WriteString(fmt.Sprintf("%s %s  %s  %s%s\n", bullet, name, DimStyle.Render(size), DimStyle.Render(label), extra))
	}

	return s.String()
}

func (w *Wizard) viewClipboardMonitor() string {
	var s strings.Builder
	s.WriteString("Enable Clipboard Monitoring?\n\n")
	s.WriteString("When enabled, Vision MCP monitors your clipboard for copied images\n")
	s.WriteString("and keeps a history (up to the configured limit). This allows you to\n")
	s.WriteString("analyze multiple images copied in sequence, e.g.:\n\n")
	s.WriteString(DimStyle.Render("  \"la primera imagen copiada es el antes y la segunda el después\"") + "\n\n")
	s.WriteString("The history is stored only in memory and disk cache — it is\n")
	s.WriteString("purged when you close the MCP server. This does NOT send data\n")
	s.WriteString("anywhere; images stay on your machine.\n\n")

	options := []struct {
		label string
		desc  string
	}{
		{"Yes", "Monitor clipboard for image copies (history limit: 5)"},
		{"No", "Only analyze the current clipboard image"},
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

func (w *Wizard) viewSummary() string {
	w.cfg.Quantization = w.quant
	w.cfg.LlamaBackend = w.backend

	var s strings.Builder
	s.WriteString("Configuration Summary\n\n")

	clipMonStatus := "Disabled"
	if w.clipMonOn {
		clipMonStatus = "Enabled"
	}

	modelLabel := AvailableModels[w.selectedModel].RepoID

	s.WriteString(Box("Summary",
		fmt.Sprintf("Model:        %s\nBackend:      %s\nQuantization: %s\nInstall dir:  %s\nClipboard:    %s\n",
			HighlightStyle.Render(modelLabel),
			HighlightStyle.Render(w.backend),
			HighlightStyle.Render(w.quant),
			InfoStyle.Render(w.installDir),
			HighlightStyle.Render(clipMonStatus),
		),
	))

	modelSize := ""
	for _, q := range hardware.AvailableQuantizations() {
		if q.Name == w.quant {
			modelSize = q.Size
			break
		}
	}

	s.WriteString(fmt.Sprintf("\n  %s First run will download:\n", ArrowStyle))
	if modelSize != "" {
		s.WriteString(fmt.Sprintf("    - Model (~%s): %s\n", modelSize, HighlightStyle.Render(modelLabel)))
	} else {
		s.WriteString(fmt.Sprintf("    - Model: %s\n", HighlightStyle.Render(modelLabel)))
	}

	if _, err := discover.FindSystemLlamaServer(); err != nil {
		s.WriteString(fmt.Sprintf("    - llama-server (~300 MB)\n"))
	} else {
		s.WriteString(fmt.Sprintf("    - llama-server: found in system\n"))
	}

	s.WriteString(fmt.Sprintf("\n  %s Press Enter to save config and exit.\n", ArrowStyle))

	return s.String()
}

func (w *Wizard) viewComplete() string {
	var s strings.Builder
	s.WriteString(TitleStyle.Render("Configuration Complete!"))
	s.WriteString("\n\n")

	s.WriteString(fmt.Sprintf("  %s Backend: %s\n", CheckMark, w.backend))
	s.WriteString(fmt.Sprintf("  %s Quantization: %s\n", CheckMark, w.quant))
	if w.clipMonOn {
		s.WriteString(fmt.Sprintf("  %s Clipboard monitoring: Enabled\n", CheckMark))
	}
	s.WriteString(fmt.Sprintf("  %s Config saved to: %s\n", CheckMark, config.ConfigPath()))

	s.WriteString("\n")
	s.WriteString(InfoStyle.Render("Models will be downloaded when you start the server."))
	s.WriteString("\n")
	s.WriteString(InfoStyle.Render("Run: vision-mcp"))

	return BorderStyle.Render(s.String())
}
