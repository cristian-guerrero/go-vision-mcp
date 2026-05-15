// Package discover — LM Studio model discovery.
package discover

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ModelInfo describes a discovered GGUF vision model on the local
// filesystem (from LM Studio or Ollama).
type ModelInfo struct {
	Name       string
	Path       string
	MMDir      string
	HasMMProj  bool
	MMProjPath string // direct path to mmproj (for Ollama), takes precedence over MMDir
	Size       int64
}

// FindLMModels walks the LM Studio models directory (~/.lmstudio/models/
// and ~/Library/Application Support/LM Studio/models/ on macOS) and
// returns all GGUF model files with their associated mmproj files.
func FindLMModels() ([]ModelInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	var searchDirs []string
	switch runtime.GOOS {
	case "windows":
		searchDirs = []string{filepath.Join(home, ".lmstudio", "models")}
	case "darwin":
		searchDirs = []string{
			filepath.Join(home, ".lmstudio", "models"),
			filepath.Join(home, "Library", "Application Support", "LM Studio", "models"),
		}
	default:
		searchDirs = []string{filepath.Join(home, ".lmstudio", "models")}
	}

	var models []ModelInfo
	for _, searchDir := range searchDirs {
		found, _ := scanLMDir(searchDir)
		models = append(models, found...)
	}
	return models, nil
}

func scanLMDir(dir string) ([]ModelInfo, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var models []ModelInfo
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".gguf") {
			name := strings.ToLower(info.Name())
			if strings.Contains(name, "mmproj") || strings.HasPrefix(name, "mm0") {
				return nil
			}

			dir := filepath.Dir(path)
			mi := ModelInfo{
				Name:  strings.TrimSuffix(info.Name(), filepath.Ext(info.Name())),
				Path:  path,
				MMDir: dir,
				Size:  info.Size(),
			}

			entries, err := os.ReadDir(dir)
			if err == nil {
				for _, e := range entries {
					name := strings.ToLower(e.Name())
					if strings.Contains(name, "mmproj") && strings.HasSuffix(name, ".gguf") {
						mi.HasMMProj = true
						break
					}
				}
				if !mi.HasMMProj {
					for _, e := range entries {
						if e.IsDir() && strings.Contains(strings.ToLower(e.Name()), "mmproj") {
							mi.HasMMProj = true
							mi.MMDir = filepath.Join(dir, e.Name())
							break
						}
					}
				}
			}

			models = append(models, mi)
		}

		return nil
	})

	return models, nil
}
