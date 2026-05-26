// Package config manages the application configuration: loading from
// disk, saving, path resolution for models/mmproj/llama-server, and
// detection of existing GGUF files in the models directory.
package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config holds all user-facing and internal settings for vision-mcp.
// All fields are emitted in config.json; empty strings signal "use default".
type Config struct {
	RepoID                  string `json:"repo_id"`
	Quantization            string `json:"quantization"`
	MMProj                  string `json:"mmproj"`
	LlamaBackend            string `json:"llama_backend"`
	LlamaBin                string `json:"llama_bin"`
	ModelsDir               string `json:"models_dir"`
	LlamaServerDir          string `json:"llama_server_dir"`
	Port                    int    `json:"port"`
	NCtx                    int    `json:"n_ctx"`
	NGL                     int    `json:"ngl"`
	FlashAttn               bool   `json:"flash_attn"`
	AutoDownload            bool   `json:"auto_download"`
	DownloadMirror          string `json:"download_mirror"`
	CustomPrompt            string `json:"custom_prompt"`
	ModelPathOverride       string `json:"model_path"`
	MMProjPathOverride      string `json:"mmproj_path"`
	LlamaServerPath         string `json:"llama_server_path"`
	LlamaServerMode         string `json:"llama_server_mode"`
	KvCacheTypeK            string `json:"kv_cache_type_k"`
	KvCacheTypeV            string `json:"kv_cache_type_v"`
	IdleTimeout             int    `json:"idle_timeout"`
	ClipboardMonitorEnabled bool   `json:"clipboard_monitor_enabled"`
	ClipboardHistoryLimit   int    `json:"clipboard_history_limit"`
	ClipboardCacheDir       string `json:"clipboard_cache_dir"`
	ScreenshotFolder        string `json:"screenshot_folder"`
	AutoUpdate              bool   `json:"auto_update"`

	// Backend selects the inference engine: "local" (llama-server)
	// or "gemini" (Google Gemini API). Default: "local".
	Backend string `json:"backend"`

	// Gemini holds Google Gemini API authentication and model settings.
	Gemini GeminiConfig `json:"gemini"`
}

// GeminiConfig configures the Google Gemini API backend.
type GeminiConfig struct {
	// APIKey is a Gemini API key from aistudio.google.com/app/api-keys.
	APIKey string `json:"api_key,omitempty"`

	// Model is the Gemini model name for vision tasks,
	// e.g. "gemini-3.5-flash".
	Model string `json:"model,omitempty"`
}

// DefaultConfig returns a Config pre-filled with sensible defaults:
//   - Model: unsloth/Qwen3-VL-4B-Instruct-GGUF (IQ4_XS)
//   - Backend: CUDA (fallback: CPU on detect)
//   - Port: 8001, Context: 8192, Flash attention on
//   - KV cache quantization: q4_0
//   - Idle timeout: 5 min, clipboard monitor: enabled (limit: 5)
//   - AutoUpdate: enabled (checks for new builds on startup)
func DefaultConfig() Config {
	return Config{
		RepoID:                  "unsloth/Qwen3-VL-4B-Instruct-GGUF",
		Quantization:            "IQ4_XS",
		MMProj:                  "mmproj-F16.gguf",
		LlamaBackend:            "cuda",
		LlamaBin:                llamaBinDefault(),
		ModelsDir:               DefaultModelsDir(),
		LlamaServerDir:          DefaultLlamaServerDir(),
		Port:                    8001,
		NCtx:                    8192,
		NGL:                     999,
		FlashAttn:               true,
		LlamaServerMode:         "auto",
		AutoDownload:            true,
		DownloadMirror:          "https://github.com/ggml-org/llama.cpp/releases",
		CustomPrompt:            "Analyze this image and respond to: %s",
		KvCacheTypeK:            "q4_0",
		KvCacheTypeV:            "q4_0",
		IdleTimeout:             5,
		ClipboardMonitorEnabled: true,
		ClipboardHistoryLimit:   5,
		ClipboardCacheDir:       "",
		ScreenshotFolder:        "",
		AutoUpdate:              true,
		Backend:                 "local",
		Gemini: GeminiConfig{
			Model: "gemini-3.5-flash",
		},
	}
}

// InstallDir returns ~/.go-mcp/vision/ — the canonical install and
// config directory.
func InstallDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".go-mcp", "vision")
}

// ConfigPath returns the standard config file location:
// ~/.go-mcp/vision/config.json.
func ConfigPath() string {
	return filepath.Join(InstallDir(), "config.json")
}

// PortableConfigPath returns "vision-mcp.json" for a portable config
// that lives beside the executable or in the working directory.
func PortableConfigPath() string {
	return "vision-mcp.json"
}

// LoadConfig reads the JSON config from the standard path, falling
// back to the portable path. If neither exists, returns DefaultConfig.
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	path := ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = PortableConfigPath()
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

	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 5
	}

	return &cfg, nil
}

// Save writes the config as indented JSON to ConfigPath(), creating
// directories as needed.
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

// llamaBinDefault returns the default llama-server binary name
// (with .exe on Windows).
func llamaBinDefault() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}

// DefaultModelsDir returns ~/.go-mcp/models/llm/.
func DefaultModelsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".go-mcp", "models", "llm")
}

// DefaultLlamaServerDir returns ~/.go-mcp/llama-cpp/.
func DefaultLlamaServerDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".go-mcp", "llama-cpp")
}

// ModelDir returns the subdirectory under ModelsDir for the current
// repo, e.g. ~/.go-mcp/models/llm/unsloth/Qwen3-VL-4B-Instruct-GGUF/.
func (c *Config) ModelDir() string {
	return filepath.Join(c.ModelsDir, c.RepoID)
}

// ModelPath returns the absolute path to the model GGUF file.
// Respects ModelPathOverride when set. Checks both the repo
// subdirectory and the old flat location for backwards compatibility.
func (c *Config) ModelPath() string {
	if c.ModelPathOverride != "" {
		return c.ModelPathOverride
	}
	newPath := filepath.Join(c.ModelDir(), fmt.Sprintf("%s-%s.gguf", modelNameFromRepo(c.RepoID), c.Quantization))
	oldPath := filepath.Join(c.ModelsDir, fmt.Sprintf("%s-%s.gguf", modelNameFromRepo(c.RepoID), c.Quantization))
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		if _, err := os.Stat(oldPath); err == nil {
			return oldPath
		}
	}
	return newPath
}

// MMProjPath returns the absolute path to the mmproj GGUF file
// (vision projector). Respects MMProjPathOverride when set.
func (c *Config) MMProjPath() string {
	if c.MMProjPathOverride != "" {
		return c.MMProjPathOverride
	}
	newPath := filepath.Join(c.ModelDir(), c.MMProj)
	oldPath := filepath.Join(c.ModelsDir, c.MMProj)
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		if _, err := os.Stat(oldPath); err == nil {
			return oldPath
		}
	}
	return newPath
}

// ClipboardCacheDirPath returns the directory for clipboard image
// cache files. Uses ClipboardCacheDir override if set, otherwise
// defaults to ~/.go-mcp/vision/clipboard-cache/.
func (c *Config) ClipboardCacheDirPath() string {
	if c.ClipboardCacheDir != "" {
		return c.ClipboardCacheDir
	}
	return filepath.Join(InstallDir(), "clipboard-cache")
}

// modelNameFromRepo extracts the model name from a HuggingFace repo ID
// by taking the last path segment and stripping the "-GGUF" suffix.
// Example: "unsloth/Qwen3-VL-4B-Instruct-GGUF" → "Qwen3-VL-4B-Instruct".
func modelNameFromRepo(repoID string) string {
	parts := strings.Split(repoID, "/")
	return strings.TrimSuffix(parts[len(parts)-1], "-GGUF")
}

// DetectedModels holds paths to existing GGUF files found on disk.
type DetectedModels struct {
	ModelPath  string
	MMProjPath string
}

// DetectExistingModels walks the default models directory and returns
// the largest GGUF model file and the first mmproj file found.
// Returns nil when nothing is found.
func DetectExistingModels() *DetectedModels {
	modelsDir := DefaultModelsDir()
	if _, err := os.Stat(modelsDir); os.IsNotExist(err) {
		return nil
	}

	var modelPath, mmprojPath string

	filepath.WalkDir(modelsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".gguf") {
			return nil
		}
		name := strings.ToLower(d.Name())

		if strings.Contains(name, "mmproj") || strings.Contains(name, "mm0") {
			if mmprojPath == "" {
				mmprojPath = path
			}
		} else {
			if modelPath == "" {
				modelPath = path
			} else {
				fi1, _ := os.Stat(modelPath)
				fi2, _ := os.Stat(path)
				if fi2 != nil && fi1 != nil && fi2.Size() > fi1.Size() {
					modelPath = path
				}
			}
		}
		return nil
	})

	if modelPath == "" && mmprojPath == "" {
		return nil
	}

	return &DetectedModels{
		ModelPath:  modelPath,
		MMProjPath: mmprojPath,
	}
}
