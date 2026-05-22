//go:build !windows

package mcp

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
)

// CheckClipboardDeps returns a warning message if the system is missing
// the required clipboard tool (xclip for X11, wl-paste for Wayland),
// or an empty string if all dependencies are met.
func CheckClipboardDeps() string {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-paste"); err != nil {
			return "Clipboard requires 'wl-clipboard'. Install: sudo apt install wl-clipboard"
		}
		return ""
	}
	if os.Getenv("DISPLAY") != "" {
		if _, err := exec.LookPath("xclip"); err != nil {
			return "Clipboard requires 'xclip'. Install: sudo apt install xclip"
		}
		return ""
	}
	return "Clipboard requires X11 (xclip) or Wayland (wl-paste) — neither display server detected"
}

// clipboardImageDataURIImpl detects the display server and delegates
// to xclip (X11) or wl-paste (Wayland) to read the clipboard image.
func clipboardImageDataURIImpl() (string, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return waylandClipboard()
	}
	if os.Getenv("DISPLAY") != "" {
		return x11Clipboard()
	}
	return "", fmt.Errorf("clipboard reading requires X11 (xclip) or Wayland (wl-paste) — neither display server detected")
}

// waylandClipboard calls "wl-paste --type image/png" to retrieve the
// clipboard image under Wayland.
func waylandClipboard() (string, error) {
	cmd := exec.Command("wl-paste", "--type", "image/png")
	data, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("wl-paste failed (stderr: %s): install wl-clipboard: sudo apt install wl-clipboard", string(ee.Stderr))
		}
		return "", fmt.Errorf("wl-paste not found: install wl-clipboard: sudo apt install wl-clipboard")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no image found in clipboard")
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:image/png;base64,%s", b64), nil
}

// x11Clipboard calls "xclip -selection clipboard -t image/png -o"
// to retrieve the clipboard image under X11.
func x11Clipboard() (string, error) {
	cmd := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-o")
	data, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("xclip failed (stderr: %s): install xclip: sudo apt install xclip", string(ee.Stderr))
		}
		return "", fmt.Errorf("xclip not found: install xclip: sudo apt install xclip")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no image found in clipboard")
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:image/png;base64,%s", b64), nil
}
