package hardware

type QuantOption struct {
	Name        string
	Size        string
	Label       string
	Recommended bool
}

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

func RecommendQuantization(hw *HardwareProfile) string {
	vramGB := float64(hw.GPU.VRAM) / (1024 * 1024 * 1024)
	ramGB := float64(hw.TotalRAM) / (1024 * 1024 * 1024)

	if vramGB >= 6 || ramGB >= 16 {
		return "Q5_K_M"
	}
	if vramGB >= 4 || ramGB >= 8 {
		return "Q4_K_M"
	}
	if vramGB >= 2 || ramGB >= 6 {
		return "Q3_K_M"
	}
	return "IQ4_XS"
}

func AvailableQuantizations() []QuantOption {
	return []QuantOption{
		{Name: "Q5_K_M", Size: "3.14 GB", Label: "High quality"},
		{Name: "Q4_K_M", Size: "2.74 GB", Label: "Balanced"},
		{Name: "Q3_K_M", Size: "2.29 GB", Label: "Economy"},
		{Name: "Q8_0", Size: "4.48 GB", Label: "Maximum quality"},
		{Name: "Q6_K", Size: "3.53 GB", Label: "High quality+"},
		{Name: "Q4_K_S", Size: "2.59 GB", Label: "Medium-low"},
		{Name: "Q3_K_S", Size: "2.11 GB", Label: "Minimum"},
		{Name: "IQ4_XS", Size: "2.48 GB", Label: "Low RAM"},
		{Name: "UD-IQ3_XXS", Size: "1.82 GB", Label: "Ultra low RAM"},
	}
}
