package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

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
	} else {
		if err := createLauncherSH(installDir); err != nil {
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

func createLauncherSH(installDir string) error {
	content := fmt.Sprintf("#!/bin/sh\nexec \"%s/%s\" \"$@\"\n", installDir, executableName())
	path := filepath.Join(installDir, "vision-mcp")
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return err
	}
	return nil
}

func InstallDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".go-vision-mcp")
}

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
