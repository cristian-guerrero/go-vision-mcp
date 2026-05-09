package setup

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vision-mcp/internal/config"
	"github.com/vision-mcp/internal/discover"
	"github.com/vision-mcp/internal/hardware"
)

type Wizard struct {
	hw          *hardware.HardwareProfile
	cfg         *config.Config
	step        int
	totalSteps  int
	stepCount   int
	cursorIdx   int
	backend     string
	quant       string
	installDir  string
	downloadDir string

	done      bool
	cancelled bool
	err       error
}

func NewWizard() *Wizard {
	hw, _ := hardware.DetectHardware()
	cfg := config.DefaultConfig()

	if hw != nil {
		cfg.Quantization = hardware.RecommendQuantization(hw)
		cfg.LlamaBackend = hardware.RecommendBackend(hw)
	}

	return &Wizard{
		hw:          hw,
		cfg:         &cfg,
		step:        0,
		totalSteps:  4,
		stepCount:   1,
		backend:     cfg.LlamaBackend,
		quant:       cfg.Quantization,
		installDir:  config.InstallDir(),
		downloadDir: config.InstallDir(),
	}
}

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
				w.cfg.Quantization = w.quant
				w.cfg.LlamaBackend = w.backend
				w.cfg.ModelsDir = w.downloadDir + "/models"
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
	return w.step == 0 || w.step == 1 || w.step == 2
}

func (w *Wizard) handleSelection(idx int) {
	switch w.step {
	case 0:
		backends := []string{"cuda", "cpu", "vulkan", "metal"}
		if idx >= 0 && idx < len(backends) {
			w.backend = backends[idx]
		}
	case 1:
		quants := hardware.AvailableQuantizations()
		if idx >= 0 && idx < len(quants) {
			w.quant = quants[idx].Name
		}
	case 2:
		if idx == 1 {
			w.installDir = "."
		}
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
		s.WriteString(w.viewBackend())
	case 1:
		s.WriteString(w.viewQuantization())
	case 2:
		s.WriteString(w.viewInstallPath())
	case 3:
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
	case 3:
		return "[Enter] save and exit  [q] quit"
	default:
		return "[↑/↓] navigate  [Enter] confirm  [←/esc] back  [q] quit"
	}
}

func (w *Wizard) stepTitle() string {
	switch w.step {
	case 0:
		return "Select Backend"
	case 1:
		return "Select Quantization"
	case 2:
		return "Installation Path"
	case 3:
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
			bullet = DimStyle.Render(" ●")
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
			bullet = DimStyle.Render(" ●")
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

func (w *Wizard) viewInstallPath() string {
	var s strings.Builder
	s.WriteString("Installation directory:\n\n")
	s.WriteString(fmt.Sprintf("  %s\n\n", InfoStyle.Render(w.installDir)))

	s.WriteString("The binary, models, and config will be stored here.\n")
	s.WriteString(fmt.Sprintf("Estimated space needed with %s: ~3-4 GB\n\n", HighlightStyle.Render(w.quant)))

	s.WriteString("Add to PATH?\n\n")

	type pathOption struct {
		label string
		desc  string
	}
	options := []pathOption{
		{"Yes", fmt.Sprintf("Install to %s (add to PATH)", w.installDir)},
		{"No", "Portable mode (config in current directory)"},
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

func (w *Wizard) viewSummary() string {
	w.cfg.Quantization = w.quant
	w.cfg.LlamaBackend = w.backend
	w.cfg.ModelsDir = w.downloadDir + "/models"

	var s strings.Builder
	s.WriteString("Configuration Summary\n\n")

	s.WriteString(BoxStyle.Render("Summary",
		fmt.Sprintf("Backend:      %s\nQuantization: %s\nInstall dir:  %s\n",
			HighlightStyle.Render(w.backend),
			HighlightStyle.Render(w.quant),
			InfoStyle.Render(w.installDir),
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
		s.WriteString(fmt.Sprintf("    - Model (~%s): %s\n", modelSize, HighlightStyle.Render(config.DefaultConfig().RepoID)))
	} else {
		s.WriteString(fmt.Sprintf("    - Model: %s\n", HighlightStyle.Render(config.DefaultConfig().RepoID)))
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
	s.WriteString(fmt.Sprintf("  %s Config saved to: %s\n", CheckMark, config.ConfigPath()))

	s.WriteString("\n")
	s.WriteString(InfoStyle.Render("Models will be downloaded when you start the server."))
	s.WriteString("\n")
	s.WriteString(InfoStyle.Render("Run: vision-mcp"))

	return BorderStyle.Render(s.String())
}
