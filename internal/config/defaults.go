package config

import (
	"fmt"

	"github.com/vision-mcp/internal/hardware"
)

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
