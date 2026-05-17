package llamaserver

import (
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

// isProcessAlive checks whether a Windows process with the given PID exists.
// Uses OpenProcess with PROCESS_QUERY_LIMITED_INFORMATION (Vista+).
func isProcessAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	windows.CloseHandle(handle)
	return true
}

// FindProcessOnPort returns the PID of the process listening on the given
// TCP port. Uses netstat on Windows and lsof on Unix.
func FindProcessOnPort(port int) int {
	pid := 0
	portStr := fmt.Sprintf(":%d ", port)

	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "LISTENING") && strings.Contains(line, portStr) {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				fmt.Sscanf(fields[len(fields)-1], "%d", &pid)
			}
		}
	}
	return pid
}

// KillAnyLlamaServer forcefully terminates all llama-server processes
// using taskkill on Windows or pkill on Unix.
func KillAnyLlamaServer() {
	cmd := exec.Command("taskkill", "/F", "/IM", "llama-server.exe")
	if err := cmd.Run(); err != nil {
		fmt.Println("No llama-server processes found.")
	} else {
		fmt.Println("Killed remaining llama-server processes.")
	}
}
