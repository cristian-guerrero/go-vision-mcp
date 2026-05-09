package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	RepoID         string `json:"repo_id"`
	Quantization   string `json:"quantization"`
	MMProj         string `json:"mmproj"`
	LlamaBackend   string `json:"llama_backend"`
	LlamaBin       string `json:"llama_bin"`
	ModelsDir      string `json:"models_dir"`
	Port           int    `json:"port"`
	NCtx           int    `json:"n_ctx"`
	NGL            int    `json:"ngl"`
	FlashAttn      bool   `json:"flash_attn"`
	AutoDownload   bool   `json:"auto_download"`
	DownloadMirror string `json:"download_mirror"`
	CustomPrompt   string `json:"custom_prompt"`
}

func DefaultConfig() Config {
	return Config{
		RepoID:         "unsloth/Qwen3.5-4B-GGUF",
		Quantization:   "Q4_K_M",
		MMProj:         "mmproj-F16.gguf",
		LlamaBackend:   "cuda",
		LlamaBin:       llamaBinDefault(),
		ModelsDir:      DefaultModelsDir(),
		Port:           8001,
		NCtx:           8192,
		NGL:            99,
		FlashAttn:      true,
		AutoDownload:   true,
		DownloadMirror: "https://github.com/ggml-org/llama.cpp/releases",
		CustomPrompt:   "Analyze this image and respond to: %s",
	}
}

func InstallDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".go-vision-mcp")
}

func ConfigPath() string {
	return filepath.Join(InstallDir(), "config.json")
}

func portableConfigPath() string {
	return "vision-mcp.json"
}

func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	path := ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = portableConfigPath()
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return &cfg, nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Save() error {
	path := ConfigPath()
	os.MkdirAll(InstallDir(), 0755)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func llamaBinDefault() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}

func DefaultModelsDir() string {
	return filepath.Join(InstallDir(), "models")
}

func (c *Config) ModelPath() string {
	return filepath.Join(c.ModelsDir, fmt.Sprintf("Qwen3.5-4B-%s.gguf", c.Quantization))
}

func (c *Config) MMProjPath() string {
	return filepath.Join(c.ModelsDir, c.MMProj)
}
