// Package hardware detects system resources (RAM, VRAM, GPU vendor,
// free disk) and recommends optimal llama.cpp backends and
// quantization levels.
package hardware

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// GPUInfo describes the detected GPU (if any).
type GPUInfo struct {
	Present     bool
	Vendor      string
	DriverVer   string
	VRAM        uint64
	BackendType string
}

// HardwareProfile holds the detected system resources.
type HardwareProfile struct {
	TotalRAM     uint64
	AvailableRAM uint64
	GPU          GPUInfo
	FreeDisk     uint64
}

// DetectHardware probes RAM (via gopsutil), free disk, and GPU
// (via nvidia-smi on Windows/Linux, sysctl on Apple Silicon).
func DetectHardware() (*HardwareProfile, error) {
	hw := &HardwareProfile{}

	vmem, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("detect RAM: %w", err)
	}
	hw.TotalRAM = vmem.Total
	hw.AvailableRAM = vmem.Available

	usage, err := disk.Usage(".")
	if err == nil {
		hw.FreeDisk = usage.Free
	}

	hw.GPU = detectGPU()

	return hw, nil
}

// detectGPU returns GPU info by running nvidia-smi (NVIDIA CUDA)
// or checking sysctl for Apple Silicon (Metal).
func detectGPU() GPUInfo {
	gpu := GPUInfo{}

	if runtime.GOOS == "darwin" {
		if isAppleSilicon() {
			gpu.Present = true
			gpu.Vendor = "apple"
			gpu.BackendType = "metal"
		} else {
			gpu.BackendType = "cpu"
		}
		return gpu
	}

	cmd := exec.Command("nvidia-smi", "--query-gpu=memory.total,driver_version,name", "--format=csv,noheader")
	out, err := cmd.Output()
	if err != nil {
		gpu.BackendType = "cpu"
		return gpu
	}

	gpu.Present = true
	gpu.Vendor = "nvidia"
	gpu.BackendType = "cuda"

	line := strings.TrimSpace(string(out))
	parts := strings.SplitN(line, ",", 3)
	if len(parts) >= 2 {
		vramStr := strings.TrimSpace(parts[0])
		vramStr = strings.TrimSuffix(vramStr, " MiB")
		vramStr = strings.TrimSpace(vramStr)
		vramMiB, err := strconv.ParseUint(vramStr, 10, 64)
		if err == nil {
			gpu.VRAM = vramMiB * 1024 * 1024
		}
		gpu.DriverVer = strings.TrimSpace(parts[1])
	}

	return gpu
}

// isAppleSilicon returns true when running on Apple Silicon (M1+).
func isAppleSilicon() bool {
	cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Apple")
}

// VulkanAvailable checks whether vulkaninfo is on PATH.
func VulkanAvailable() bool {
	_, err := exec.LookPath("vulkaninfo")
	return err == nil
}
