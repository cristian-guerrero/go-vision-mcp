package download

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type Release struct {
	TagName string         `json:"tag_name"`
	Assets  []ReleaseAsset `json:"assets"`
}

const llamaReleasesURL = "https://api.github.com/repos/ggml-org/llama.cpp/releases/latest"

func FetchLatestRelease() (*Release, error) {
	req, err := http.NewRequest("GET", llamaReleasesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "vision-mcp")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	return &release, nil
}

func findAsset(release *Release, backend string) *ReleaseAsset {
	osKey := osKey()
	for i := range release.Assets {
		asset := &release.Assets[i]
		name := strings.ToLower(asset.Name)

		if !strings.Contains(name, osKey) {
			continue
		}

		switch backend {
		case "cuda":
			if strings.Contains(name, "cuda") {
				return asset
			}
		case "vulkan":
			if strings.Contains(name, "vulkan") {
				return asset
			}
		case "metal":
			if strings.Contains(name, "macos") || strings.Contains(name, "metal") {
				return asset
			}
		case "cpu":
			if !strings.Contains(name, "cuda") && !strings.Contains(name, "vulkan") {
				return asset
			}
		}
	}

	for i := range release.Assets {
		asset := &release.Assets[i]
		name := strings.ToLower(asset.Name)

		if !strings.Contains(name, osKey) {
			continue
		}
		if backend == "cpu" {
			return asset
		}
		return asset
	}

	return nil
}

func osKey() string {
	switch runtime.GOOS {
	case "windows":
		return "win"
	case "linux":
		return "linux"
	case "darwin":
		return "macos"
	default:
		return "linux"
	}
}

func DownloadLlamaBinary(asset *ReleaseAsset, destDir string, progress ProgressFunc) (string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	fmt.Printf("Downloading %s...\n", asset.Name)

	tmpPath := filepath.Join(destDir, asset.Name)
	if err := DownloadFile(asset.URL, tmpPath, progress); err != nil {
		return "", fmt.Errorf("download binary archive: %w", err)
	}
	defer os.Remove(tmpPath)

	binaryPath, err := extractLlamaBinary(tmpPath, destDir)
	if err != nil {
		return "", fmt.Errorf("extract binary: %w", err)
	}

	return binaryPath, nil
}

func extractLlamaBinary(archivePath, destDir string) (string, error) {
	ext := strings.ToLower(filepath.Ext(archivePath))

	switch ext {
	case ".zip":
		return extractFromZip(archivePath, destDir)
	case ".gz":
		return extractFromTarGz(archivePath, destDir)
	case ".tar":
		return extractFromTar(archivePath, destDir)
	default:
		return "", fmt.Errorf("unsupported archive format: %s", ext)
	}
}

func extractFromZip(zipPath, destDir string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	var binaryPath string
	binName := executableName("llama-server")

	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), binName) || strings.EqualFold(filepath.Base(f.Name), binName) {
			src, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("open zip entry: %w", err)
			}
			defer src.Close()

			binaryPath = filepath.Join(destDir, binName)
			dst, err := os.Create(binaryPath)
			if err != nil {
				return "", fmt.Errorf("create binary: %w", err)
			}
			defer dst.Close()

			if _, err := io.Copy(dst, src); err != nil {
				return "", fmt.Errorf("extract binary: %w", err)
			}

			if runtime.GOOS != "windows" {
				os.Chmod(binaryPath, 0755)
			}
			return binaryPath, nil
		}
	}

	return "", fmt.Errorf("llama-server not found in archive")
}

func extractFromTarGz(tgzPath, destDir string) (string, error) {
	f, err := os.Open(tgzPath)
	if err != nil {
		return "", fmt.Errorf("open tgz: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()

	return extractTar(gzr, destDir)
}

func extractFromTar(tarPath, destDir string) (string, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return "", fmt.Errorf("open tar: %w", err)
	}
	defer f.Close()

	return extractTar(f, destDir)
}

func extractTar(r io.Reader, destDir string) (string, error) {
	tr := tar.NewReader(r)
	binName := executableName("llama-server")

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar entry: %w", err)
		}

		if strings.HasSuffix(strings.ToLower(hdr.Name), binName) {
			binaryPath := filepath.Join(destDir, binName)
			dst, err := os.Create(binaryPath)
			if err != nil {
				return "", fmt.Errorf("create binary: %w", err)
			}
			defer dst.Close()

			if _, err := io.Copy(dst, tr); err != nil {
				return "", fmt.Errorf("extract binary: %w", err)
			}

			if runtime.GOOS != "windows" {
				os.Chmod(binaryPath, 0755)
			}
			return binaryPath, nil
		}
	}

	return "", fmt.Errorf("llama-server not found in archive")
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func EnsureLlamaBinary(backend, destDir string, progress ProgressFunc) (string, error) {
	binName := executableName("llama-server")
	binaryPath := filepath.Join(destDir, binName)

	if _, err := os.Stat(binaryPath); err == nil {
		return binaryPath, nil
	}

	if _, err := os.Stat(filepath.Join(destDir, "llama-server")); err == nil && runtime.GOOS != "windows" {
		return filepath.Join(destDir, "llama-server"), nil
	}

	release, err := FetchLatestRelease()
	if err != nil {
		fmt.Printf("Warning: Failed to fetch releases: %v\n", err)
		return "", fmt.Errorf("could not fetch releases and binary not found: %w", err)
	}

	asset := findAsset(release, backend)
	if asset == nil {
		return "", fmt.Errorf("no matching llama-server release found for backend %q on %s", backend, runtime.GOOS)
	}

	fmt.Printf("Found release: %s\n", release.TagName)
	return DownloadLlamaBinary(asset, destDir, progress)
}
