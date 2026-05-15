// Package setup — download progress TUI screen.
package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cristian-guerrero/go-vision-mcp/internal/config"
	"github.com/cristian-guerrero/go-vision-mcp/internal/download"
	"github.com/cristian-guerrero/go-vision-mcp/internal/hardware"
)

// downloadStage tracks the status of a single download stage (model or
// llama-server).
type downloadStage struct {
	label  string
	size   string
	pct    float64
	status string // "waiting", "downloading", "done", "skipped", "error"
	err    error
}

// downloadScreenModel is the Bubble Tea model that shows real-time
// download progress bars for model files and llama-server.
type downloadScreenModel struct {
	cfg    *config.Config
	stages []downloadStage
	done   bool
	err    error
	ch     chan progressUpdate
}

func modelSize(cfg *config.Config) string {
	for _, q := range hardware.AvailableQuantizations() {
		if q.Name == cfg.Quantization {
			return q.Size
		}
	}
	return ""
}

type progressUpdate struct {
	stageIdx int
	pct      float64
	done     bool
	skipped  bool
	err      error
}

// NewDownloadScreen creates a TUI download screen. It dynamically
// adds stages: one for model files, and optionally one for llama-server
// if it is not already present.
func NewDownloadScreen(cfg *config.Config) *downloadScreenModel {
	modelSz := modelSize(cfg)
	modelLabel := "Model files (GGUF + mmproj)"
	if modelSz != "" {
		modelLabel = fmt.Sprintf("Model files (GGUF + mmproj)  ~%s", modelSz)
	}

	stages := []downloadStage{
		{label: modelLabel, pct: 0, status: "waiting"},
	}
	if needsLlamaDownload(cfg) {
		stages = append(stages, downloadStage{label: "llama-server binary  ~300 MB", pct: 0, status: "waiting"})
	}

	return &downloadScreenModel{
		cfg:    cfg,
		stages: stages,
		ch:     make(chan progressUpdate, 10),
	}
}

// needsLlamaDownload checks whether llama-server already exists in the
// configured directory, skipping the download stage if found.
func needsLlamaDownload(cfg *config.Config) bool {
	if cfg.LlamaServerMode == "custom" {
		return false
	}
	binDir := cfg.LlamaServerDir
	binName := "llama-server"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	_, err := os.Stat(filepath.Join(binDir, binName))
	return os.IsNotExist(err)
}

func (m *downloadScreenModel) Init() tea.Cmd {
	go m.runDownloads()
	return m.waitMsg()
}

func (m *downloadScreenModel) waitMsg() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-m.ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *downloadScreenModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressUpdate:
		if msg.err != nil {
			if msg.stageIdx >= 0 && msg.stageIdx < len(m.stages) {
				m.stages[msg.stageIdx].status = "error"
				m.stages[msg.stageIdx].err = msg.err
			}
			m.err = msg.err
			m.done = true
			return m, tea.Quit
		}

		if msg.stageIdx >= 0 && msg.stageIdx < len(m.stages) {
			if msg.skipped {
				m.stages[msg.stageIdx].status = "skipped"
				m.stages[msg.stageIdx].pct = 100
			} else if msg.done {
				m.stages[msg.stageIdx].status = "done"
				m.stages[msg.stageIdx].pct = 100
			} else {
				m.stages[msg.stageIdx].status = "downloading"
				m.stages[msg.stageIdx].pct = msg.pct
			}
		}

		allDone := true
		for _, s := range m.stages {
			if s.status == "waiting" || s.status == "downloading" {
				allDone = false
				break
			}
		}
		if allDone {
			m.done = true
			return m, tea.Quit
		}

		return m, m.waitMsg()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m *downloadScreenModel) View() string {
	var s strings.Builder

	s.WriteString(TitleStyle.Render("Downloading required assets"))
	s.WriteString("\n\n")
	s.WriteString(InfoStyle.Render("We need to download the model and llama-server (if not already present)."))
	s.WriteString("\n")
	s.WriteString(InfoStyle.Render("This may take a few minutes depending on your connection speed and model size."))
	s.WriteString("\n")
	s.WriteString(Divider())
	s.WriteString("\n\n")

	for i, stage := range m.stages {
		statusIcon := DimStyle.Render("○")
		statusText := DimStyle.Render("waiting...")
		bar := ""

		switch stage.status {
		case "downloading":
			statusIcon = InfoStyle.Render("↓")
			statusText = InfoStyle.Render(fmt.Sprintf("%.0f%%", stage.pct))
			bar = "\n  " + ProgressBar(stage.pct, 40) + "\n"
		case "done":
			statusIcon = CheckMark.String()
			statusText = HighlightStyle.Render("done")
		case "skipped":
			statusIcon = CheckMark.String()
			statusText = DimStyle.Render("already exists")
		case "error":
			statusIcon = CrossMark.String()
			statusText = ErrorStyle.Render(fmt.Sprintf("error: %v", stage.err))
		}

		s.WriteString(fmt.Sprintf("  %s %s  %s\n", statusIcon, stage.label, statusText))
		if bar != "" {
			s.WriteString(bar)
		}

		if i < len(m.stages)-1 {
			s.WriteString("\n")
		}
	}

	s.WriteString("\n")
	s.WriteString(FooterStyle.Render("[q] cancel"))

	return s.String()
}

func (m *downloadScreenModel) runDownloads() {
	defer close(m.ch)

	// Stage 0: Model + mmproj via EnsureModels
	modelsExist := true
	if _, err := os.Stat(m.cfg.ModelPath()); os.IsNotExist(err) {
		modelsExist = false
	}
	if m.cfg.MMProjPathOverride == "" {
		if _, err := os.Stat(m.cfg.MMProjPath()); os.IsNotExist(err) {
			modelsExist = false
		}
	}

	if modelsExist {
		os.Remove(m.cfg.ModelPath() + ".tmp")
		os.Remove(m.cfg.MMProjPath() + ".tmp")
		m.ch <- progressUpdate{stageIdx: 0, pct: 100, done: true, skipped: true}
	} else {
		err := download.EnsureModels(m.cfg, func(downloaded, total int64) {
			pct := float64(0)
			if total > 0 {
				pct = float64(downloaded) / float64(total) * 100
			}
			m.ch <- progressUpdate{stageIdx: 0, pct: pct, done: downloaded == total && total > 0}
		})
		if err != nil {
			m.ch <- progressUpdate{stageIdx: 0, err: err}
			return
		}
		m.ch <- progressUpdate{stageIdx: 0, pct: 100, done: true}
	}

	// Stage 1: llama-server (if needed)
	if len(m.stages) > 1 {
		_, err := download.EnsureLlamaBinary(m.cfg.LlamaBackend, m.cfg.LlamaServerDir, func(downloaded, total int64) {
			pct := float64(0)
			if total > 0 {
				pct = float64(downloaded) / float64(total) * 100
			}
			m.ch <- progressUpdate{stageIdx: 1, pct: pct, done: downloaded == total && total > 0}
		})
		if err != nil {
			m.ch <- progressUpdate{stageIdx: 1, err: err}
			return
		}
		m.ch <- progressUpdate{stageIdx: 1, pct: 100, done: true}
	}
}
