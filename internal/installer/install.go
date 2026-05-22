// Package installer copies the vision-mcp binary to ~/.go-mcp/vision/
// and adds the directory to PATH. On Windows a .cmd launcher is also
// created.
package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Install copies the binary to installDir, creates a .cmd launcher on
// Windows, and adds installDir to the user's PATH.
func Install(installDir, binaryPath string) error {
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	exePath := filepath.Join(installDir, executableName())
	src, err := os.ReadFile(binaryPath)
	if err != nil {
		exePath = binaryPath
	} else {
		if err := os.WriteFile(exePath, src, 0755); err != nil {
			return fmt.Errorf("copy binary: %w", err)
		}
	}

	if runtime.GOOS == "windows" {
		if err := createLauncherCMD(installDir); err != nil {
			return fmt.Errorf("create launcher: %w", err)
		}
	}

	if err := ensureInPATH(installDir); err != nil {
		fmt.Printf("Warning: Could not add to PATH: %v\n", err)
		fmt.Printf("Please add %s to your PATH manually.\n", installDir)
	}

	return nil
}

func executableName() string {
	if runtime.GOOS == "windows" {
		return "vision-mcp.exe"
	}
	return "vision-mcp"
}

func createLauncherCMD(installDir string) error {
	content := fmt.Sprintf("@echo off\r\n\"%s\\%s\" %%*\r\n", installDir, executableName())
	return os.WriteFile(filepath.Join(installDir, "vision-mcp.cmd"), []byte(content), 0644)
}

// InstallDir returns the canonical install directory (~/.go-mcp/vision/).
func InstallDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".go-mcp", "vision")
}

// IsInstalled checks whether the install directory exists.
func IsInstalled() bool {
	dir := InstallDir()
	fi, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func ensureInPATH(installDir string) error {
	if runtime.GOOS == "windows" {
		return ensureInPathWindows(installDir)
	}
	return ensureInPathUnix(installDir)
}
