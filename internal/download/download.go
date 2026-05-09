package download

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vision-mcp/internal/config"
)

type ProgressFunc func(downloaded, total int64)

func DownloadFile(url, destPath string, progress ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	total := resp.ContentLength
	pw := &progressWriter{
		total:    total,
		progress: progress,
	}

	_, err = io.Copy(f, io.TeeReader(resp.Body, pw))
	f.Close()

	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

type progressWriter struct {
	total    int64
	written  int64
	progress ProgressFunc
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	if pw.progress != nil {
		pw.progress(pw.written, pw.total)
	}
	return n, nil
}

func HFDownloadURL(repoID, filename string) string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repoID, filename)
}

func EnsureModels(cfg *config.Config, onProgress ProgressFunc) error {
	modelPath := cfg.ModelPath()
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		filename := fmt.Sprintf("Qwen3.5-4B-%s.gguf", cfg.Quantization)
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
