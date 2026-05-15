package clipboard

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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

type PollResult struct {
	Data         []byte
	OriginalPath string
}

type Entry struct {
	Index        int       `json:"index"`
	Timestamp    time.Time `json:"timestamp"`
	OriginalPath string    `json:"original_path,omitempty"`
	CachedPath   string    `json:"cached_path,omitempty"`
}

type Monitor struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	cfg    *config.Config

	entries     []Entry
	lastHash    string
	cacheDir    string
	historyPath string
	limit       int
	intervalMs  int
}

func NewMonitor(cfg *config.Config) *Monitor {
	cacheDir := cfg.ClipboardCacheDirPath()
	return &Monitor{
		cfg:         cfg,
		cacheDir:    cacheDir,
		historyPath: filepath.Join(cacheDir, "history.json"),
		limit:       cfg.ClipboardHistoryLimit,
		intervalMs:  500,
	}
}

func (m *Monitor) Start() {
	m.ctx, m.cancel = context.WithCancel(context.Background())
	os.MkdirAll(m.cacheDir, 0755)
	m.loadHistory()
	log.Printf("Clipboard monitor started (cache: %s, limit: %d)", m.cacheDir, m.limit)
	go m.pollLoop()
}

func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeCache()
	log.Printf("Clipboard monitor stopped, cache cleared")
}

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

func (m *Monitor) ClearHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeCache()
}

func (m *Monitor) pollLoop() {
	ticker := time.NewTicker(time.Duration(m.intervalMs) * time.Millisecond)
	defer ticker.Stop()

	lastSeenHash := ""

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}

		result, err := clipboardPollImage()
		if err != nil || result == nil {
			continue
		}

		h := m.hashResult(result)
		if h == lastSeenHash {
			continue
		}
		lastSeenHash = h

		m.mu.Lock()
		m.saveEntry(result, h)
		m.mu.Unlock()
	}
}

func (m *Monitor) hashResult(r *PollResult) string {
	if r.OriginalPath != "" {
		hash := sha256.Sum256([]byte(r.OriginalPath))
		return "file:" + base64.RawURLEncoding.EncodeToString(hash[:16])
	}
	if len(r.Data) > 0 {
		hash := sha256.Sum256(r.Data)
		return "raw:" + base64.RawURLEncoding.EncodeToString(hash[:16])
	}
	return ""
}

func (m *Monitor) saveEntry(result *PollResult, hash string) {
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

	if result.OriginalPath != "" {
		e.OriginalPath = result.OriginalPath
		log.Printf("Clipboard monitor: image #%d -> %s (original file)", index, result.OriginalPath)
	} else if len(result.Data) > 0 {
		shortHash := hash
		if idx := strings.IndexByte(hash, ':'); idx >= 0 {
			shortHash = hash[idx+1:]
		}
		if len(shortHash) > 8 {
			shortHash = shortHash[:8]
		}
		filename := fmt.Sprintf("clip-%s.png", shortHash)
		filePath := filepath.Join(m.cacheDir, filename)
		if err := os.WriteFile(filePath, result.Data, 0644); err != nil {
			log.Printf("Clipboard monitor: failed to save image #%d: %v", index, err)
			return
		}
		e.CachedPath = filePath
		log.Printf("Clipboard monitor: saved image #%d (%d bytes) -> %s", index, len(result.Data), filePath)
	}

	m.entries = append(m.entries, e)
	m.saveHistory()
}

func (m *Monitor) purgeCache() {
	for _, e := range m.entries {
		if e.CachedPath != "" {
			os.Remove(e.CachedPath)
		}
	}
	m.entries = nil
	os.Remove(m.historyPath)
}

func (m *Monitor) historyFilePath() string {
	return m.historyPath
}

func (m *Monitor) loadHistory() {
	data, err := os.ReadFile(m.historyPath)
	if err != nil {
		return
	}
	var entries []Entry
	if json.Unmarshal(data, &entries) != nil {
		return
	}
	valid := entries[:0]
	for _, e := range entries {
		if e.OriginalPath != "" {
			if _, err := os.Stat(e.OriginalPath); err == nil {
				valid = append(valid, e)
			}
		} else if e.CachedPath != "" {
			if _, err := os.Stat(e.CachedPath); err == nil {
				valid = append(valid, e)
			}
		}
	}
	m.entries = valid
	if len(entries) != len(valid) {
		m.saveHistory()
	}
}

func (m *Monitor) saveHistory() {
	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(m.historyPath, data, 0644)
}
