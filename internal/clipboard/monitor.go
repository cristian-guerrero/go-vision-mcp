// Package clipboard monitors the system clipboard for new images and
// maintains a history (capped to a configurable limit). It supports
// Windows (PowerShell), X11 (xclip), and Wayland (wl-paste).
package clipboard

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cristian-guerrero/go-vision-mcp/internal/config"
	"github.com/cristian-guerrero/go-vision-mcp/internal/image"
)

// PollResult is the outcome of a clipboard poll: either raw PNG bytes
// or the path to an image file found in the clipboard (file drop list).
type PollResult struct {
	Data         []byte
	OriginalPath string
}

// Entry represents a single clipboard image in the history.
type Entry struct {
	Index        int       `json:"index"`
	Timestamp    time.Time `json:"timestamp"`
	OriginalPath string    `json:"original_path,omitempty"`
	CachedPath   string    `json:"cached_path,omitempty"`
}

// Monitor periodically polls the system clipboard for new images,
// stores them in a ring buffer (up to limit), and persists history
// to disk for recovery across restarts.
type Monitor struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	cfg    *config.Config

	entries           []Entry
	cacheDir          string
	historyPath       string
	limit             int
	intervalMs        int
	screenshotFolder  string
	screenshotLastMod time.Time
}

// NewMonitor creates a clipboard Monitor. Call Start() to begin polling.
func NewMonitor(cfg *config.Config) *Monitor {
	cacheDir := cfg.ClipboardCacheDirPath()
	screenshotFolder := cfg.ScreenshotFolder
	if screenshotFolder == "" {
		screenshotFolder = defaultScreenshotFolder()
	}
	return &Monitor{
		cfg:              cfg,
		cacheDir:         cacheDir,
		historyPath:      filepath.Join(cacheDir, "history.json"),
		limit:            cfg.ClipboardHistoryLimit,
		intervalMs:       1000,
		screenshotFolder: screenshotFolder,
	}
}

// Start begins the background polling goroutine. History starts empty
// and is persisted for recovery across restarts.
func (m *Monitor) Start() {
	m.ctx, m.cancel = context.WithCancel(context.Background())
	os.MkdirAll(m.cacheDir, 0755)
	if m.screenshotFolder != "" {
		log.Printf("Clipboard monitor started (cache: %s, limit: %d, screenshots: %s)", m.cacheDir, m.limit, m.screenshotFolder)
	} else {
		log.Printf("Clipboard monitor started (cache: %s, limit: %d)", m.cacheDir, m.limit)
	}
	m.screenshotLastMod = time.Now()
	go m.pollLoop()
}

// Stop cancels the polling goroutine and purges all cached images.
func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeCache()
	log.Printf("Clipboard monitor stopped, cache cleared")
}

// ListHistory returns a copy of the clipboard history sorted by index.
func (m *Monitor) ListHistory() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Entry, len(m.entries))
	copy(result, m.entries)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Index < result[j].Index
	})
	return result
}

// GetImage returns a data URI for the clipboard history entry at the
// given index. It resolves the image from the original file path or
// the local cache.
func (m *Monitor) GetImage(index int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.Index == index {
			if e.OriginalPath != "" {
				if _, err := os.Stat(e.OriginalPath); err == nil {
					return image.ResolveToDataURI(e.OriginalPath)
				}
				return "", fmt.Errorf("clipboard image #%d original file not found: %s", index, e.OriginalPath)
			}
			if e.CachedPath != "" {
				if _, err := os.Stat(e.CachedPath); err == nil {
					return image.ResolveToDataURI(e.CachedPath)
				}
				return "", fmt.Errorf("clipboard image #%d cached file not found: %s", index, e.CachedPath)
			}
		}
	}
	return "", fmt.Errorf("clipboard image #%d not found in history (available: 1-%d)", index, len(m.entries))
}

// GetLatestImage returns a data URI for the most recent clipboard
// history entry.
func (m *Monitor) GetLatestImage() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.entries) == 0 {
		return "", fmt.Errorf("clipboard history is empty")
	}
	last := m.entries[len(m.entries)-1]
	if last.OriginalPath != "" {
		if _, err := os.Stat(last.OriginalPath); err == nil {
			return image.ResolveToDataURI(last.OriginalPath)
		}
		return "", fmt.Errorf("clipboard image #%d original file not found: %s", last.Index, last.OriginalPath)
	}
	if last.CachedPath != "" {
		if _, err := os.Stat(last.CachedPath); err == nil {
			return image.ResolveToDataURI(last.CachedPath)
		}
		return "", fmt.Errorf("clipboard image #%d cached file not found: %s", last.Index, last.CachedPath)
	}
	return "", fmt.Errorf("clipboard history entry #%d has no image data", last.Index)
}

// ClearHistory removes all cached images and the history file.
func (m *Monitor) ClearHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeCache()
}

// pollLoop runs every intervalMs and checks the clipboard for new images
// and the screenshots folder for new captures.
func (m *Monitor) pollLoop() {
	ticker := time.NewTicker(time.Duration(m.intervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}

		result, err := clipboardPollImage()
		if err == nil && result != nil {
			m.mu.Lock()
			if !m.isDuplicate(result) {
				m.saveEntry(result)
			}
			m.mu.Unlock()
		}

		if path := m.checkScreenshotFolder(); path != "" {
			m.mu.Lock()
			if !m.isDuplicate(&PollResult{OriginalPath: path}) {
				m.saveEntry(&PollResult{OriginalPath: path})
			}
			m.mu.Unlock()
		}
	}
}

// isDuplicate checks whether a file-based PollResult already exists in
// the history by comparing the original file path.
func (m *Monitor) isDuplicate(result *PollResult) bool {
	for _, e := range m.entries {
		if e.OriginalPath == result.OriginalPath {
			return true
		}
	}
	return false
}

// imageExts are the file extensions treated as images by the screenshot
// folder monitor.
var imageExts = map[string]bool{
	".png": true,
}

// checkScreenshotFolder checks if a new image was added to the
// screenshots folder. Returns the path of the newest image, or empty
// string if nothing changed.
func (m *Monitor) checkScreenshotFolder() string {
	if m.screenshotFolder == "" {
		return ""
	}

	info, err := os.Stat(m.screenshotFolder)
	if err != nil {
		return ""
	}
	folderMod := info.ModTime()
	if !folderMod.After(m.screenshotLastMod) && !m.screenshotLastMod.IsZero() {
		return ""
	}

	entries, err := os.ReadDir(m.screenshotFolder)
	if err != nil {
		return ""
	}

	var newestFile string
	var newestMod time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !imageExts[ext] {
			continue
		}
		fi, err := entry.Info()
		if err != nil {
			continue
		}
		mod := fi.ModTime()
		if mod.After(newestMod) {
			newestMod = mod
			newestFile = filepath.Join(m.screenshotFolder, entry.Name())
		}
	}

	if newestFile != "" && newestMod.After(m.screenshotLastMod) {
		m.screenshotLastMod = newestMod
		log.Printf("Screenshot monitor [%s]: new image -> %s", m.screenshotFolder, newestFile)
		return newestFile
	}
	m.screenshotLastMod = folderMod
	return ""
}

// saveEntry inserts a new clipboard entry into the ring buffer. When
// the buffer exceeds the limit, the oldest entry is evicted and its
// cache file deleted.
func (m *Monitor) saveEntry(result *PollResult) {
	if len(m.entries) >= m.limit {
		oldest := m.entries[0]
		if oldest.CachedPath != "" {
			os.Remove(oldest.CachedPath)
		}
		m.entries = m.entries[1:]
		for i := range m.entries {
			m.entries[i].Index = i + 1
		}
	}

	index := len(m.entries) + 1

	var e Entry
	e.Index = index
	e.Timestamp = time.Now()

	e.OriginalPath = result.OriginalPath

	m.entries = append(m.entries, e)
	log.Printf("Clipboard monitor: image #%d -> %s (original file)", index, result.OriginalPath)
	m.saveHistory()
}

// purgeCache removes all cached image files and resets the entry list.
func (m *Monitor) purgeCache() {
	for _, e := range m.entries {
		if e.CachedPath != "" {
			os.Remove(e.CachedPath)
		}
	}
	m.entries = nil
	os.Remove(m.historyPath)
}

// historyFilePath returns the path to the persisted history JSON file.
func (m *Monitor) historyFilePath() string {
	return m.historyPath
}

// saveHistory writes the current entry list to disk as JSON.
func (m *Monitor) saveHistory() {
	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(m.historyPath, data, 0644)
}
