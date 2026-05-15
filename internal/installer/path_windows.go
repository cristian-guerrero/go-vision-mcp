//go:build windows

package installer

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// ensureInPathWindows adds installDir to the user's PATH via the
// HKCU\Environment registry key and broadcasts WM_SETTINGCHANGE so
// running applications see the update.
func ensureInPathWindows(installDir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()

	existing, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("read PATH: %w", err)
	}

	if strings.Contains(existing, installDir) {
		return nil
	}

	newPath := existing
	if newPath == "" {
		newPath = installDir
	} else {
		newPath = strings.TrimRight(existing, ";") + ";" + installDir
	}

	if err := k.SetStringValue("Path", newPath); err != nil {
		return fmt.Errorf("set PATH: %w", err)
	}

	broadcastEnvChange()
	return nil
}

func ensureInPathUnix(installDir string) error { return nil }

// broadcastEnvChange sends a WM_SETTINGCHANGE message to all top-level
// windows so environment variable changes take effect without a reboot.
func broadcastEnvChange() {
	user32 := syscall.NewLazyDLL("user32.dll")
	sendMessage := user32.NewProc("SendMessageTimeoutW")

	envStr, _ := syscall.UTF16PtrFromString("Environment")
	var result uintptr
	sendMessage.Call(
		0xFFFF,                          // HWND_BROADCAST
		0x001A,                          // WM_SETTINGCHANGE
		0,                               // wParam
		uintptr(unsafe.Pointer(envStr)), // lParam
		0x0002,                          // SMTO_ABORTIFHUNG
		5000,                            // timeout ms
		uintptr(unsafe.Pointer(&result)),
	)
}
