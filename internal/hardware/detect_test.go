package hardware

import (
	"testing"
)

func TestRecommendQuantization(t *testing.T) {
	tests := []struct {
		name     string
		vramGB   float64
		ramGB    float64
		present  bool
		expected string
	}{
		{"high vram", 8, 16, true, "Q4_K_M"},
		{"mid vram", 6, 16, true, "IQ4_XS"},
		{"low vram", 4, 12, true, "IQ4_XS"},
		{"very low vram", 2, 8, true, "UD-IQ3_XXS"},
		{"no gpu high ram", 0, 16, false, "Q4_K_M"},
		{"no gpu mid ram", 0, 10, false, "IQ4_XS"},
		{"no gpu low ram", 0, 6, false, "UD-IQ3_XXS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hw := &HardwareProfile{
				TotalRAM: uint64(tt.ramGB * 1024 * 1024 * 1024),
				GPU: GPUInfo{
					Present: tt.present,
					VRAM:    uint64(tt.vramGB * 1024 * 1024 * 1024),
				},
			}

			result := RecommendQuantization(hw)
			if result != tt.expected {
				t.Errorf("RecommendQuantization() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestRecommendBackend(t *testing.T) {
	tests := []struct {
		name    string
		vendor  string
		present bool
		in      []string
	}{
		{"nvidia", "nvidia", true, []string{"cuda", "vulkan", "cpu"}},
		{"apple", "apple", true, []string{"metal"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hw := &HardwareProfile{
				GPU: GPUInfo{
					Present: tt.present,
					Vendor:  tt.vendor,
				},
			}

			result := RecommendBackend(hw)
			valid := false
			for _, v := range tt.in {
				if result == v {
					valid = true
					break
				}
			}
			if !valid {
				t.Errorf("RecommendBackend() = %s, not in expected set %v", result, tt.in)
			}
		})
	}

	t.Run("no gpu returns something", func(t *testing.T) {
		hw := &HardwareProfile{
			GPU: GPUInfo{Present: false},
		}
		result := RecommendBackend(hw)
		if result == "" {
			t.Error("RecommendBackend should not return empty string")
		}
	})
}

func TestAvailableQuantizations(t *testing.T) {
	quants := AvailableQuantizations()
	if len(quants) == 0 {
		t.Error("expected non-empty quantization list")
	}

	names := make(map[string]bool)
	for _, q := range quants {
		if q.Name == "" {
			t.Error("quantization name should not be empty")
		}
		if q.Size == "" {
			t.Errorf("quantization %s should have a size", q.Name)
		}
		if q.Label == "" {
			t.Errorf("quantization %s should have a label", q.Name)
		}
		if names[q.Name] {
			t.Errorf("duplicate quantization name: %s", q.Name)
		}
		names[q.Name] = true
	}
}
