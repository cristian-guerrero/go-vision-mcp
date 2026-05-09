package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.RepoID != "unsloth/Qwen3.5-4B-GGUF" {
		t.Errorf("expected unsloth/Qwen3.5-4B-GGUF, got %s", cfg.RepoID)
	}
	if cfg.Quantization != "Q4_K_M" {
		t.Errorf("expected Q4_K_M, got %s", cfg.Quantization)
	}
	if cfg.Port != 8001 {
		t.Errorf("expected 8001, got %d", cfg.Port)
	}
	if cfg.NCtx != 8192 {
		t.Errorf("expected 8192, got %d", cfg.NCtx)
	}
}

func TestConfigSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	cfg.Quantization = "Q5_K_M"
	cfg.LlamaBackend = "cuda"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	read, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(read, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if loaded.Quantization != "Q5_K_M" {
		t.Errorf("expected Q5_K_M, got %s", loaded.Quantization)
	}
	if loaded.LlamaBackend != "cuda" {
		t.Errorf("expected cuda, got %s", loaded.LlamaBackend)
	}
}

func TestModelPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelsDir = "/test/models"
	cfg.Quantization = "Q5_K_M"

	path := cfg.ModelPath()
	if !strings.HasSuffix(path, "Qwen3.5-4B-Q5_K_M.gguf") {
		t.Errorf("expected path ending with Qwen3.5-4B-Q5_K_M.gguf, got %s", path)
	}
}

func TestModelPathWithOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelPathOverride = "/custom/path/model.gguf"

	path := cfg.ModelPath()
	if path != "/custom/path/model.gguf" {
		t.Errorf("expected override path, got %s", path)
	}
}

func TestMMProjPathWithOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MMProjPathOverride = "/custom/path/mmproj.gguf"

	path := cfg.MMProjPath()
	if path != "/custom/path/mmproj.gguf" {
		t.Errorf("expected override path, got %s", path)
	}
}

func TestModelNameFromRepo(t *testing.T) {
	tests := []struct {
		repoID   string
		expected string
	}{
		{"unsloth/Qwen3.5-4B-GGUF", "Qwen3.5-4B"},
		{"unsloth/Llama-3.2-11B-Vision-GGUF", "Llama-3.2-11B-Vision"},
		{"bartowski/Qwen2.5-VL-7B-Instruct-GGUF", "Qwen2.5-VL-7B-Instruct"},
	}

	for _, tt := range tests {
		result := modelNameFromRepo(tt.repoID)
		if result != tt.expected {
			t.Errorf("modelNameFromRepo(%q) = %q, want %q", tt.repoID, result, tt.expected)
		}
	}
}

func TestMMProjPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelsDir = "/test/models"
	cfg.MMProj = "mmproj-F16.gguf"

	path := cfg.MMProjPath()
	if !strings.HasSuffix(path, "mmproj-F16.gguf") {
		t.Errorf("expected path ending with mmproj-F16.gguf, got %s", path)
	}
}
