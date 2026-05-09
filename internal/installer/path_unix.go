//go:build !windows

package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ensureInPathWindows(installDir string) error { return nil }

func ensureInPathUnix(installDir string) error {
	home, _ := os.UserHomeDir()
	shellRC := filepath.Join(home, ".bashrc")

	if _, err := os.Stat(filepath.Join(home, ".zshrc")); err == nil {
		shellRC = filepath.Join(home, ".zshrc")
	}

	return appendToFile(shellRC, fmt.Sprintf("\nexport PATH=\"$PATH:%s\"\n", installDir))
}

func appendToFile(path, content string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read file: %w", err)
	}

	if strings.Contains(string(data), content) {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open for append: %w", err)
	}
	defer f.Close()

	_, err = f.Write([]byte(content))
	return err
}
