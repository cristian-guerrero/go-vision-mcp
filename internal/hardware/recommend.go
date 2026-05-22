// Package hardware — recommendation algorithms.
package hardware

import "runtime"

// QuantOption describes a single quantization option with human-readable
// size and quality label.
type QuantOption struct {
	Name        string
	Size        string
	Label       string
	Recommended bool
}

// RecommendBackend selects the best llama.cpp backend based on
// detected GPU vendor: CUDA (NVIDIA), Metal (Apple), Vulkan (AMD/Intel),
// or CPU fallback.
//
// On Linux, CUDA pre-built binaries are not published by llama.cpp, so
// NVIDIA GPUs fall back to Vulkan (if available) or CPU.
func RecommendBackend(hw *HardwareProfile) string {
	if hw.GPU.Present {
		switch hw.GPU.Vendor {
		case "nvidia":
			if runtime.GOOS == "linux" {
				if VulkanAvailable() {
					return "vulkan"
				}
				return "cpu"
			}
			return "cuda"
		case "apple":
			return "metal"
		case "amd", "intel":
			if VulkanAvailable() {
				return "vulkan"
			}
			return "cpu"
		}
	}
	if VulkanAvailable() {
		return "vulkan"
	}
	return "cpu"
}

// RecommendQuantization picks a quantization level based on available
// VRAM (when a GPU is present) or RAM (CPU-only). On mid-range GPUs
// (4–8 GB VRAM) the smaller IQ4_XS is preferred to leave more VRAM
// headroom for KV cache / context. High-end GPUs (≥8 GB) use Q4_K_M.
// Systems without a GPU fall back to system-RAM-based sizing.
func RecommendQuantization(hw *HardwareProfile) string {
	if hw.GPU.Present && hw.GPU.VRAM > 0 {
		vramGB := float64(hw.GPU.VRAM) / (1024 * 1024 * 1024)
		if vramGB >= 8 {
			return "Q4_K_M"
		}
		if vramGB >= 4 {
			return "IQ4_XS"
		}
		return "UD-IQ3_XXS"
	}
	ramGB := float64(hw.TotalRAM) / (1024 * 1024 * 1024)
	if ramGB >= 16 {
		return "Q4_K_M"
	}
	if ramGB >= 8 {
		return "IQ4_XS"
	}
	return "UD-IQ3_XXS"
}

// AvailableQuantizations returns the list of quantization options
// sorted by size (largest first).
func AvailableQuantizations() []QuantOption {
	return []QuantOption{
		{Name: "Q4_K_M", Size: "2.74 GB", Label: "Balanced"},
		{Name: "Q8_0", Size: "4.48 GB", Label: "Maximum quality"},
		{Name: "Q4_K_S", Size: "2.59 GB", Label: "Medium-low"},
		{Name: "IQ4_XS", Size: "2.48 GB", Label: "Low RAM"},
		{Name: "UD-IQ3_XXS", Size: "1.82 GB", Label: "Ultra low RAM"},
	}
}
