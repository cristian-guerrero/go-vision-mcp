package discover

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/vision-mcp/internal/config"
)

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

	exe, err := os.Executable()
	if err == nil {
		candidate = filepath.Join(filepath.Dir(exe), binName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", exec.ErrNotFound
}
