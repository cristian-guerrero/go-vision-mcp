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

type GPUInfo struct {
	Present     bool
	Vendor      string
	DriverVer   string
	VRAM        uint64
	BackendType string
}

type HardwareProfile struct {
	TotalRAM     uint64
	AvailableRAM uint64
	GPU          GPUInfo
	FreeDisk     uint64
}

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

func isAppleSilicon() bool {
	cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Apple")
}

func VulkanAvailable() bool {
	_, err := exec.LookPath("vulkaninfo")
	return err == nil
}
