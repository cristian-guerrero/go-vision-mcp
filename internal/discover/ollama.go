// Package discover — Ollama model discovery.
package discover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ollamaManifest represents an Ollama model manifest JSON file.
type ollamaManifest struct {
	Layers []ollamaLayer `json:"layers"`
}

type ollamaLayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

func ollamaModelsDir() string {
	home, _ := os.UserHomeDir()
	if env := os.Getenv("OLLAMA_MODELS"); env != "" {
		return env
	}
	return filepath.Join(home, ".ollama", "models")
}

// FindOllamaModels walks the Ollama manifests directory and returns
// vision models (those with a projector layer), with their blob paths.
func FindOllamaModels() ([]ModelInfo, error) {
	base := ollamaModelsDir()
	manifestsDir := filepath.Join(base, "manifests")

	if _, err := os.Stat(manifestsDir); os.IsNotExist(err) {
		return nil, nil
	}

	var models []ModelInfo

	filepath.Walk(manifestsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(manifestsDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) != 4 {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var mf ollamaManifest
		if err := json.Unmarshal(data, &mf); err != nil {
			return nil
		}

		var modelBlob, projectorBlob string
		var modelSize int64
		hasProjector := false

		for _, layer := range mf.Layers {
			digestFile := strings.ReplaceAll(layer.Digest, ":", "-")
			blobPath := filepath.Join(base, "blobs", digestFile)

			switch layer.MediaType {
			case "application/vnd.ollama.image.model":
				modelBlob = blobPath
				modelSize = layer.Size
			case "application/vnd.ollama.image.projector":
				projectorBlob = blobPath
				hasProjector = true
			}
		}

		if modelBlob == "" {
			return nil
		}
		if !hasProjector {
			return nil
		}

		host := parts[0]
		namespace := parts[1]
		modelName := parts[2]
		tag := parts[3]

		fullName := modelName
		if tag != "latest" {
			fullName = modelName + ":" + tag
		}
		if namespace != "library" {
			fullName = namespace + "/" + fullName
		}
		if host != "registry.ollama.ai" {
			fullName = host + "/" + fullName
		}

		models = append(models, ModelInfo{
			Name:       fullName,
			Path:       modelBlob,
			HasMMProj:  true,
			MMProjPath: projectorBlob,
			Size:       modelSize,
		})

		return nil
	})

	return models, nil
}
