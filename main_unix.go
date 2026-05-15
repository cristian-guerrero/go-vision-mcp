//go:build !windows

package main

import (
	"fmt"
	"os"
)

// showMessageBox is a stub for Unix systems. It prints the title and
// message to stderr (no GUI dialog available).
func showMessageBox(title, msg string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, msg)
}
