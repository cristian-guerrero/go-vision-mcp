package setup

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vision-mcp/internal/config"
	"github.com/vision-mcp/internal/hardware"
)

type Wizard struct {
	hw          *hardware.HardwareProfile
	cfg         *config.Config
	step        int
	totalSteps  int
	backend     string
	quant       string
	installDir  string
	downloadDir string

	done  bool
	err   error
	width int
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
		totalSteps:  5,
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
	return result.cfg, nil
}

func (w *Wizard) Init() tea.Cmd {
	return nil
}

func (w *Wizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return w, tea.Quit

		case "enter":
			w.nextStep()
			if w.step >= w.totalSteps {
				w.done = true
				return w, tea.Quit
			}

		case "1", "2", "3", "4", "5", "6", "7", "8":
			idx := int(msg.String()[0] - '1')
			w.handleSelection(idx)

		case "y", "Y":
			w.handleYesNo(true)

		case "n", "N":
			w.handleYesNo(false)
		}
	}

	return w, nil
}

func (w *Wizard) handleSelection(idx int) {
	switch w.step {
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
	}
}

func (w *Wizard) handleYesNo(yes bool) {
	switch w.step {
	case 3:
		if !yes {
			w.installDir = "."
		}
	}
}

func (w *Wizard) nextStep() {
	w.step++
	if w.step == 5 {
		w.cfg.Quantization = w.quant
		w.cfg.LlamaBackend = w.backend
		w.cfg.ModelsDir = w.downloadDir + "/models"
	}
}

func (w *Wizard) View() string {
	if w.done {
		return w.viewSummary()
	}

	var s strings.Builder

	s.WriteString(Header(w.step+1, w.totalSteps, w.stepTitle()))
	s.WriteString("\n\n")

	switch w.step {
	case 0:
		s.WriteString(w.viewHardware())
	case 1:
		s.WriteString(w.viewBackend())
	case 2:
		s.WriteString(w.viewQuantization())
	case 3:
		s.WriteString(w.viewInstallPath())
	case 4:
		s.WriteString(w.viewDownload())
	}

	s.WriteString("\n\n")
	s.WriteString(FooterStyle.Render("[Enter] continue  [q] quit"))

	if w.err != nil {
		s.WriteString("\n" + ErrorStyle.Render(fmt.Sprintf("Error: %v", w.err)))
	}

	return s.String()
}

func (w *Wizard) stepTitle() string {
	switch w.step {
	case 0:
		return "Hardware Detection"
	case 1:
		return "Select Backend"
	case 2:
		return "Select Quantization"
	case 3:
		return "Installation Path"
	case 4:
		return "Download & Install"
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

	recommended := w.cfg.LlamaBackend

	for i, b := range backends {
		num := fmt.Sprintf("[%d]", i+1)
		name := b.label
		desc := b.desc

		if b.key == recommended {
			name = HighlightStyle.Render(name + " " + BadgeRecommended.String())
		}

		s.WriteString(fmt.Sprintf("  %s %-30s %s %s\n", num, name, DimStyle.Render(desc), ""))
	}

	w.backend = recommended
	s.WriteString(fmt.Sprintf("\n  Press 1-%d to select, or Enter for recommended", len(backends)))
	return s.String()
}

func (w *Wizard) viewQuantization() string {
	var s strings.Builder
	s.WriteString("Select model quantization:\n\n")

	quants := hardware.AvailableQuantizations()
	rec := w.cfg.Quantization

	for i, q := range quants {
		num := fmt.Sprintf("[%d]", i+1)
		name := q.Name
		size := q.Size
		label := q.Label

		if name == rec {
			name = HighlightStyle.Render(name + " " + BadgeRecommended.String())
		}

		s.WriteString(fmt.Sprintf("  %s %-20s %-10s %s\n", num, name, DimStyle.Render(size), DimStyle.Render(label)))
	}

	w.quant = rec
	s.WriteString(fmt.Sprintf("\n  Press 1-%d to select, or Enter for recommended", len(quants)))
	return s.String()
}

func (w *Wizard) viewInstallPath() string {
	var s strings.Builder
	s.WriteString("Installation directory:\n\n")
	s.WriteString(fmt.Sprintf("  %s\n\n", InfoStyle.Render(w.installDir)))

	s.WriteString("The binary, models, and config will be stored here.\n")
	s.WriteString(fmt.Sprintf("Estimated space needed with %s: ~3-4 GB\n\n", HighlightStyle.Render(w.quant)))

	s.WriteString("Add to PATH? [y/n]\n")
	s.WriteString(fmt.Sprintf("  %s Press Y to install to %s\n", ArrowStyle, w.installDir))
	s.WriteString(fmt.Sprintf("  %s Press N for portable mode (config in current dir)\n", ArrowStyle))

	return s.String()
}

func (w *Wizard) viewDownload() string {
	var s strings.Builder
	s.WriteString("Ready to configure Vision MCP\n\n")

	s.WriteString(BoxStyle.Render("Summary",
		fmt.Sprintf("Backend:      %s\nQuantization: %s\nInstall dir:  %s\n",
			HighlightStyle.Render(w.backend),
			HighlightStyle.Render(w.quant),
			InfoStyle.Render(w.installDir),
		),
	))

	s.WriteString(fmt.Sprintf("\n  Press Enter to save configuration and exit.\n"))
	s.WriteString(fmt.Sprintf("  Models will download on first server start.\n"))

	return s.String()
}

func (w *Wizard) viewSummary() string {
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
