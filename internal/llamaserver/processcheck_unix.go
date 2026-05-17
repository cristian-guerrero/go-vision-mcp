//go:build !windows

package llamaserver

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// isProcessAlive checks whether a Unix process with the given PID exists.
// Sends signal 0 which tests whether the process exists without actually signaling it.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// FindProcessOnPort returns the PID of the process listening on the given
// TCP port. Uses lsof on Unix and netstat on Windows.
func FindProcessOnPort(port int) int {
	pid := 0
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port)).Output()
	if err != nil {
		return 0
	}
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &pid)
	return pid
}

// KillAnyLlamaServer forcefully terminates all llama-server processes
// using pkill on Unix or taskkill on Windows.
func KillAnyLlamaServer() {
	cmd := exec.Command("pkill", "-f", "llama-server")
	if err := cmd.Run(); err != nil {
		fmt.Println("No llama-server processes found.")
	} else {
		fmt.Println("Killed remaining llama-server processes.")
	}
}
