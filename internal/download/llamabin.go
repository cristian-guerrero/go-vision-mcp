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

type matchResult struct {
	primary   *ReleaseAsset
	secondary *ReleaseAsset // extra runtime DLLs (cudart)
}

func findAssets(release *Release, backend string) *matchResult {
	osKey := osKey()
	var standard, extra []*ReleaseAsset

	for i := range release.Assets {
		asset := &release.Assets[i]
		name := strings.ToLower(asset.Name)

		if !strings.Contains(name, osKey) {
			continue
		}

		switch backend {
		case "cuda":
			if strings.Contains(name, "cuda") {
				if strings.HasPrefix(name, "llama") {
					standard = append(standard, asset)
				} else if strings.HasPrefix(name, "cudart") {
					extra = append(extra, asset)
				}
			}
		case "vulkan":
			if strings.Contains(name, "vulkan") {
				standard = append(standard, asset)
			}
		case "metal":
			if strings.Contains(name, "macos") || strings.Contains(name, "metal") {
				standard = append(standard, asset)
			}
		case "cpu":
			if !strings.Contains(name, "cuda") && !strings.Contains(name, "vulkan") {
				standard = append(standard, asset)
			}
		}
	}

	if len(standard) == 0 {
		return nil
	}

	result := &matchResult{primary: standard[0]}
	if len(extra) > 0 {
		result.secondary = extra[0]
	}
	return result
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

func llamaServerDir(destDir string) string {
	return filepath.Join(destDir, "llama-server")
}

func LlamaServerDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".go-mcp", "vision", "llama-server")
}

func EnsureLlamaBinary(backend, destDir string, progress ProgressFunc) (string, error) {
	binDir := llamaServerDir(destDir)
	binName := executableName("llama-server")
	binaryPath := filepath.Join(binDir, binName)

	if _, err := os.Stat(binaryPath); err == nil {
		hasDLLs := false
		entries, _ := os.ReadDir(binDir)
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if strings.HasSuffix(name, ".dll") || strings.HasSuffix(name, ".so") || strings.HasSuffix(name, ".dylib") {
				hasDLLs = true
				break
			}
		}
		if hasDLLs {
			return binaryPath, nil
		}
		os.Remove(binaryPath)
	}

	if _, err := os.Stat(filepath.Join(binDir, "llama-server")); err == nil && runtime.GOOS != "windows" {
		return filepath.Join(binDir, "llama-server"), nil
	}

	release, err := FetchLatestRelease()
	if err != nil {
		fmt.Printf("Warning: Failed to fetch releases: %v\n", err)
		return "", fmt.Errorf("could not fetch releases and binary not found: %w", err)
	}

	matches := findAssets(release, backend)
	if matches == nil {
		return "", fmt.Errorf("no matching llama-server release found for backend %q on %s", backend, runtime.GOOS)
	}

	fmt.Printf("Found release: %s\n", release.TagName)

	os.MkdirAll(binDir, 0755)

	binPath, err := downloadAndExtract(matches.primary, binDir, progress)
	if err != nil {
		return "", err
	}

	if matches.secondary != nil {
		if err := downloadAndExtractExtra(matches.secondary, binDir, progress); err != nil {
			fmt.Printf("Warning: could not download extra runtime DLLs: %v\n", err)
		}
	}

	return binPath, nil
}

func downloadAndExtract(asset *ReleaseAsset, binDir string, progress ProgressFunc) (string, error) {
	fmt.Printf("Downloading %s...\n", asset.Name)

	tmpPath := filepath.Join(binDir, asset.Name)
	if err := DownloadFile(asset.URL, tmpPath, progress); err != nil {
		return "", fmt.Errorf("download binary archive: %w", err)
	}
	defer os.Remove(tmpPath)

	binaryPath, err := extractLlamaBinary(tmpPath, binDir)
	if err != nil {
		return "", fmt.Errorf("extract binary: %w", err)
	}

	return binaryPath, nil
}

func downloadAndExtractExtra(asset *ReleaseAsset, binDir string, progress ProgressFunc) error {
	fmt.Printf("Downloading %s (CUDA runtime)...\n", asset.Name)

	tmpPath := filepath.Join(binDir, asset.Name)
	if err := DownloadFile(asset.URL, tmpPath, progress); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer os.Remove(tmpPath)

	return extractArchive(tmpPath, binDir)
}

func extractArchive(archivePath, destDir string) error {
	ext := strings.ToLower(filepath.Ext(archivePath))
	switch ext {
	case ".zip":
		_, err := extractFromZip(archivePath, destDir, false)
		return err
	case ".gz":
		_, err := extractFromTarGz(archivePath, destDir, false)
		return err
	case ".tar":
		_, err := extractFromTar(archivePath, destDir, false)
		return err
	default:
		return fmt.Errorf("unsupported archive format: %s", ext)
	}
}

func extractLlamaBinary(archivePath, destDir string) (string, error) {
	ext := strings.ToLower(filepath.Ext(archivePath))

	switch ext {
	case ".zip":
		return extractFromZip(archivePath, destDir, true)
	case ".gz":
		return extractFromTarGz(archivePath, destDir, true)
	case ".tar":
		return extractFromTar(archivePath, destDir, true)
	default:
		return "", fmt.Errorf("unsupported archive format: %s", ext)
	}
}

func extractFromZip(zipPath, destDir string, findServer bool) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	var binaryPath string

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		base := strings.ToLower(filepath.Base(f.Name))
		isServer := findServer && strings.Contains(base, "llama-server") && strings.HasSuffix(base, ".exe")

		destPath := filepath.Join(destDir, filepath.Base(f.Name))

		src, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("open zip entry %s: %w", f.Name, err)
		}

		dst, err := os.Create(destPath)
		if err != nil {
			src.Close()
			return "", fmt.Errorf("create %s: %w", f.Name, err)
		}

		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			return "", fmt.Errorf("extract %s: %w", f.Name, err)
		}

		if isServer {
			binaryPath = destPath
			if runtime.GOOS != "windows" {
				os.Chmod(destPath, 0755)
			}
		}
	}

	if findServer && binaryPath == "" {
		var names []string
		for _, f := range r.File {
			names = append(names, f.Name)
		}
		return "", fmt.Errorf("llama-server not found in archive. Contents: %s", strings.Join(names, ", "))
	}

	return binaryPath, nil
}

func extractFromTarGz(tgzPath, destDir string, findServer bool) (string, error) {
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

	return extractTar(gzr, destDir, findServer)
}

func extractFromTar(tarPath, destDir string, findServer bool) (string, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return "", fmt.Errorf("open tar: %w", err)
	}
	defer f.Close()

	return extractTar(f, destDir, findServer)
}

func extractTar(r io.Reader, destDir string, findServer bool) (string, error) {
	tr := tar.NewReader(r)
	var binaryPath string

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar entry: %w", err)
		}

		if hdr.FileInfo().IsDir() {
			continue
		}

		base := strings.ToLower(filepath.Base(hdr.Name))
		isServer := findServer && strings.Contains(base, "llama-server") && strings.HasSuffix(base, ".exe")

		destPath := filepath.Join(destDir, filepath.Base(hdr.Name))

		dst, err := os.Create(destPath)
		if err != nil {
			return "", fmt.Errorf("create %s: %w", hdr.Name, err)
		}

		_, err = io.Copy(dst, tr)
		dst.Close()
		if err != nil {
			return "", fmt.Errorf("extract %s: %w", hdr.Name, err)
		}

		if isServer {
			binaryPath = destPath
			if runtime.GOOS != "windows" {
				os.Chmod(destPath, 0755)
			}
		}
	}

	if findServer && binaryPath == "" {
		return "", fmt.Errorf("llama-server not found in archive")
	}
	return binaryPath, nil
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
