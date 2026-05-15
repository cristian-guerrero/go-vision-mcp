// Package discover — llama-server binary discovery.
package discover

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/cristian-guerrero/go-vision-mcp/internal/config"
)

// FindSystemLlamaServer searches for a llama-server binary in PATH,
// the install directory, the llama-cpp directory, and beside the
// current executable.
func FindSystemLlamaServer() (string, error) {
	binName := "llama-server"
	if runtime.GOOS == "windows" {
		binName = "llama-server.exe"
	}

	if path, err := exec.LookPath(binName); err == nil {
		return path, nil
	}

	installDir := config.InstallDir()
	candidate := filepath.Join(installDir, binName)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	llamaDir := config.DefaultLlamaServerDir()
	candidate = filepath.Join(llamaDir, binName)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	exe, err := os.Executable()
	if err == nil {
		candidate = filepath.Join(filepath.Dir(exe), binName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", exec.ErrNotFound
}
