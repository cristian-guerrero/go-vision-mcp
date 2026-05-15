// Package hardware — recommendation algorithms.
package hardware

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
func RecommendBackend(hw *HardwareProfile) string {
	if hw.GPU.Present {
		switch hw.GPU.Vendor {
		case "nvidia":
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
// VRAM and RAM: Q4_K_M for ≥4 GB VRAM or ≥8 GB RAM, otherwise
// UD-IQ3_XXS for ultra-low-memory systems.
func RecommendQuantization(hw *HardwareProfile) string {
	vramGB := float64(hw.GPU.VRAM) / (1024 * 1024 * 1024)
	ramGB := float64(hw.TotalRAM) / (1024 * 1024 * 1024)

	if vramGB >= 4 || ramGB >= 8 {
		return "Q4_K_M"
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
