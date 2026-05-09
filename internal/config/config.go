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

type Config struct {
	RepoID             string `json:"repo_id"`
	Quantization       string `json:"quantization"`
	MMProj             string `json:"mmproj"`
	LlamaBackend       string `json:"llama_backend"`
	LlamaBin           string `json:"llama_bin"`
	ModelsDir          string `json:"models_dir"`
	Port               int    `json:"port"`
	NCtx               int    `json:"n_ctx"`
	NGL                int    `json:"ngl"`
	FlashAttn          bool   `json:"flash_attn"`
	AutoDownload       bool   `json:"auto_download"`
	DownloadMirror     string `json:"download_mirror"`
	CustomPrompt       string `json:"custom_prompt"`
	ModelPathOverride  string `json:"model_path"`
	MMProjPathOverride string `json:"mmproj_path"`
	LlamaServerPath    string `json:"llama_server_path"`
	LlamaServerMode    string `json:"llama_server_mode"`
	KvCacheTypeK       string `json:"kv_cache_type_k"`
	KvCacheTypeV       string `json:"kv_cache_type_v"`
	IdleTimeout        int    `json:"idle_timeout"`
}

func DefaultConfig() Config {
	return Config{
		RepoID:         "unsloth/Qwen3.5-4B-GGUF",
		Quantization:   "UD-IQ3_XXS",
		MMProj:         "mmproj-F16.gguf",
		LlamaBackend:   "cuda",
		LlamaBin:       llamaBinDefault(),
		ModelsDir:      DefaultModelsDir(),
		Port:           8001,
		NCtx:           8192,
		NGL:            999,
		FlashAttn:      true,
		AutoDownload:   true,
		DownloadMirror: "https://github.com/ggml-org/llama.cpp/releases",
		CustomPrompt:   "Analyze this image and respond to: %s",
		KvCacheTypeK:   "q4_0",
		KvCacheTypeV:   "q4_0",
		IdleTimeout:    5,
	}
}

func InstallDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".go-vision-mcp")
}

func ConfigPath() string {
	return filepath.Join(InstallDir(), "config.json")
}

func PortableConfigPath() string {
	return "vision-mcp.json"
}

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

func (c *Config) ModelDir() string {
	return filepath.Join(c.ModelsDir, c.RepoID)
}

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

func modelNameFromRepo(repoID string) string {
	parts := strings.Split(repoID, "/")
	return strings.TrimSuffix(parts[len(parts)-1], "-GGUF")
}

type DetectedModels struct {
	ModelPath  string
	MMProjPath string
}

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
