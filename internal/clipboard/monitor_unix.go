//go:build !windows

package clipboard

import (
	"os"
	"os/exec"
	"strings"
)

func clipboardPollImage() (*PollResult, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return waylandPoll()
	}
	if os.Getenv("DISPLAY") != "" {
		return x11Poll()
	}
	return nil, nil
}

func waylandPoll() (*PollResult, error) {
	uriData, err := exec.Command("wl-paste", "--type", "text/uri-list").Output()
	if err == nil && len(uriData) > 0 {
		path := strings.TrimSpace(string(uriData))
		path = strings.TrimPrefix(path, "file://")
		if _, err := os.Stat(path); err == nil {
			return &PollResult{OriginalPath: path}, nil
		}
	}

	data, err := exec.Command("wl-paste", "--type", "image/png").Output()
	if err != nil || len(data) == 0 {
		return nil, nil
	}
	return &PollResult{Data: data}, nil
}

func x11Poll() (*PollResult, error) {
	uriData, err := exec.Command("xclip", "-selection", "clipboard", "-t", "text/uri-list", "-o").Output()
	if err == nil && len(uriData) > 0 {
		path := strings.TrimSpace(string(uriData))
		path = strings.TrimPrefix(path, "file://")
		if _, err := os.Stat(path); err == nil {
			return &PollResult{OriginalPath: path}, nil
		}
	}

	data, err := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-o").Output()
	if err != nil || len(data) == 0 {
		return nil, nil
	}
	return &PollResult{Data: data}, nil
}
