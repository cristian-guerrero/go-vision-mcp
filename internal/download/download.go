package download

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vision-mcp/internal/config"
)

type ProgressFunc func(downloaded, total int64)

func DownloadFile(url, destPath string, progress ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	tmpPath := destPath + ".tmp"
	existing := int64(0)

	if fi, err := os.Stat(tmpPath); err == nil {
		existing = fi.Size()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if existing > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existing))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		resp.Body.Close()
		return os.Rename(tmpPath, destPath)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	var total int64
	if resp.StatusCode == http.StatusPartialContent {
		total = parseContentRange(resp.Header.Get("Content-Range"))
	} else {
		total = resp.ContentLength
		existing = 0
		os.Remove(tmpPath)
	}

	if total == 0 {
		total = resp.ContentLength + existing
	}

	flag := os.O_CREATE | os.O_WRONLY
	if existing > 0 && resp.StatusCode == http.StatusPartialContent {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(tmpPath, flag, 0644)
	if err != nil {
		return fmt.Errorf("open temp file: %w", err)
	}

	pw := &progressWriter{
		offset:   existing,
		total:    total,
		progress: progress,
	}

	if existing > 0 {
		pw.written = existing
	}

	_, err = io.Copy(f, io.TeeReader(resp.Body, pw))
	f.Close()

	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	if fi, _ := os.Stat(tmpPath); fi != nil && fi.Size() == 0 {
		os.Remove(tmpPath)
		return fmt.Errorf("downloaded file is empty")
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

func parseContentRange(header string) int64 {
	if header == "" {
		return 0
	}
	parts := strings.Split(header, "/")
	if len(parts) < 2 {
		return 0
	}
	total, _ := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	return total
}

type progressWriter struct {
	offset   int64
	total    int64
	written  int64
	progress ProgressFunc
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	if pw.progress != nil {
		pw.progress(pw.written-pw.offset, pw.total-pw.offset)
	}
	return n, nil
}

func HFDownloadURL(repoID, filename string) string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repoID, filename)
}

func EnsureModels(cfg *config.Config, onProgress ProgressFunc) error {
	if cfg.ModelPathOverride != "" {
		if _, err := os.Stat(cfg.ModelPathOverride); err != nil {
			return fmt.Errorf("model file not found: %s", cfg.ModelPathOverride)
		}
	}

	modelPath := cfg.ModelPath()
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		filename := filepath.Base(modelPath)
		url := HFDownloadURL(cfg.RepoID, filename)
		if err := DownloadFile(url, modelPath, onProgress); err != nil {
			return fmt.Errorf("download model: %w", err)
		}
		if onProgress != nil {
			onProgress(0, 0)
		}
		fmt.Println()
	}

	mmprojPath := cfg.MMProjPath()
	if cfg.MMProjPathOverride != "" {
		if _, err := os.Stat(cfg.MMProjPathOverride); err != nil {
			return fmt.Errorf("mmproj file not found: %s", cfg.MMProjPathOverride)
		}
		return nil
	}

	if _, err := os.Stat(mmprojPath); os.IsNotExist(err) {
		url := HFDownloadURL(cfg.RepoID, cfg.MMProj)
		if err := DownloadFile(url, mmprojPath, onProgress); err != nil {
			return fmt.Errorf("download mmproj: %w", err)
		}
		if onProgress != nil {
			onProgress(0, 0)
		}
		fmt.Println()
	}

	return nil
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
