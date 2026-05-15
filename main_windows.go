//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// showMessageBox displays a Win32 MessageBoxW with MB_OK and
// MB_ICONINFORMATION. Used when vision-mcp is double-clicked
// (non-interactive) on Windows.
func showMessageBox(title, msg string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	msgPtr, _ := syscall.UTF16PtrFromString(msg)

	const (
		MB_OK              = 0x00000000
		MB_ICONINFORMATION = 0x00000040
		MB_SETFOREGROUND   = 0x00010000
	)

	messageBox.Call(
		0,
		uintptr(unsafe.Pointer(msgPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(MB_OK|MB_ICONINFORMATION|MB_SETFOREGROUND),
	)
}
