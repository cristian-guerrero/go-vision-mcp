//go:build !windows

package mcp

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
)

func clipboardImageDataURIImpl() (string, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return waylandClipboard()
	}
	if os.Getenv("DISPLAY") != "" {
		return x11Clipboard()
	}
	return "", fmt.Errorf("clipboard reading requires X11 (xclip) or Wayland (wl-paste) — neither display server detected")
}

func waylandClipboard() (string, error) {
	cmd := exec.Command("wl-paste", "--type", "image/png")
	data, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("wl-paste failed (stderr: %s): is wl-clipboard installed?", string(ee.Stderr))
		}
		return "", fmt.Errorf("wl-paste not found: install wl-clipboard package")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no image found in clipboard")
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:image/png;base64,%s", b64), nil
}

func x11Clipboard() (string, error) {
	cmd := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-o")
	data, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("xclip failed (stderr: %s): is xclip installed?", string(ee.Stderr))
		}
		return "", fmt.Errorf("xclip not found: install xclip package")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no image found in clipboard")
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:image/png;base64,%s", b64), nil
}
