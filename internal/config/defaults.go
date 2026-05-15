// Package config — hardware defaults detection.
package config

import (
	"fmt"

	"github.com/cristian-guerrero/go-vision-mcp/internal/hardware"
)

// ApplyHardwareDefaults detects the system hardware (RAM, GPU/VRAM) and
// updates the Config's LlamaBackend and Quantization with recommended values.
func ApplyHardwareDefaults(cfg *Config) error {
	hw, err := hardware.DetectHardware()
	if err != nil {
		return fmt.Errorf("detect hardware: %w", err)
	}

	backend := hardware.RecommendBackend(hw)
	cfg.LlamaBackend = backend

	quant := hardware.RecommendQuantization(hw)
	cfg.Quantization = quant

	return nil
}
