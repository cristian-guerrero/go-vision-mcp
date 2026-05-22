// Package download — llama.cpp release asset management.
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

	"github.com/cristian-guerrero/go-vision-mcp/internal/config"
)

// ReleaseAsset represents a downloadable file in a GitHub release.
type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Release represents a GitHub release with tag name and assets.
type Release struct {
	TagName string         `json:"tag_name"`
	Assets  []ReleaseAsset `json:"assets"`
}

const llamaReleasesURL = "https://api.github.com/repos/ggml-org/llama.cpp/releases/latest"

// FetchLatestRelease calls the GitHub API to get the latest
// llama.cpp release tag and its assets.
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

// matchResult holds the primary archive and optional secondary
// (CUDA runtime DLLs) asset for a backend-specific download.
type matchResult struct {
	primary   *ReleaseAsset
	secondary *ReleaseAsset // extra runtime DLLs (cudart)
}

// findAssets filters the release assets by OS key and backend
// (cuda, vulkan, metal, cpu). Returns the first matching primary
// asset and optionally a secondary CUDA runtime archive.
func findAssets(release *Release, backend string) *matchResult {
	osKey := osKey()
	archExclude := archExcludeKeys()
	var standard, extra []*ReleaseAsset

	for i := range release.Assets {
		asset := &release.Assets[i]
		name := strings.ToLower(asset.Name)

		if !strings.Contains(name, osKey) && !strings.Contains(name, "linux") {
			continue
		}

		if containsAny(name, archExclude...) {
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
			if !containsAny(name, "cuda", "vulkan", "rocm", "sycl", "openvino", "hip") {
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

// osKey returns the platform substring used in release asset names
// ("win" for Windows, "ubuntu" for Linux, "macos" for Darwin).
// llama.cpp CI publishes Ubuntu-based builds for Linux.
func osKey() string {
	switch runtime.GOOS {
	case "windows":
		return "win"
	case "darwin":
		return "macos"
	default:
		return "ubuntu"
	}
}

// archExcludeKeys returns substrings to exclude based on the current
// architecture: on amd64, exclude ARM64/aarch64 assets; on arm64,
// exclude x86_64 assets. Returns nil when no exclusion is needed.
func archExcludeKeys() []string {
	switch runtime.GOARCH {
	case "amd64":
		return []string{"arm64", "aarch64", "s390x"}
	case "arm64":
		return []string{"x64", "s390x"}
	default:
		return nil
	}
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// llamaServerDir returns the destination directory for llama-server
// binaries (currently the same as destDir).
func llamaServerDir(destDir string) string {
	return destDir
}

// LlamaServerDir returns the default directory for llama-server.
func LlamaServerDir() string {
	return config.DefaultLlamaServerDir()
}

// EnsureLlamaBinary downloads llama-server for the given backend
// (cuda/vulkan/metal/cpu) if not already present at destDir.
// It checks for existing binaries and required DLLs before downloading.
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

// downloadAndExtract downloads a release archive, cleans the target
// directory, and extracts the llama-server binary from it.
func downloadAndExtract(asset *ReleaseAsset, binDir string, progress ProgressFunc) (string, error) {
	fmt.Printf("Downloading %s...\n", asset.Name)

	tmpPath := filepath.Join(binDir, asset.Name)
	if err := DownloadFile(asset.URL, tmpPath, progress); err != nil {
		return "", fmt.Errorf("download binary archive: %w", err)
	}
	defer os.Remove(tmpPath)

	// Remove stale files from previous failed extractions
	cleanDir(binDir)

	binaryPath, err := extractLlamaBinary(tmpPath, binDir)
	if err != nil {
		return "", fmt.Errorf("extract binary: %w", err)
	}

	return binaryPath, nil
}

// cleanDir removes all non-archive files from a directory.
func cleanDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if strings.HasSuffix(e.Name(), ".tar.gz") || strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		os.Remove(p)
	}
}

// downloadAndExtractExtra downloads a secondary archive (e.g. CUDA
// runtime DLLs) and extracts all files to the bin directory.
func downloadAndExtractExtra(asset *ReleaseAsset, binDir string, progress ProgressFunc) error {
	fmt.Printf("Downloading %s (CUDA runtime)...\n", asset.Name)

	tmpPath := filepath.Join(binDir, asset.Name)
	if err := DownloadFile(asset.URL, tmpPath, progress); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer os.Remove(tmpPath)

	return extractArchive(tmpPath, binDir)
}

// extractArchive dispatches to zip/tar.gz/tar extraction based on
// the file extension.
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

// extractLlamaBinary extracts an archive and returns the path to
// the llama-server executable found inside.
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

// extractFromZip extracts all files from a ZIP archive. When
// findServer is true, it returns the path to llama-server.exe.
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
		isServer := findServer && (base == "llama-server" || base == "llama-server.exe")

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

// extractFromTarGz opens a .tar.gz archive and extracts the llama-server.
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

// extractFromTar opens a .tar archive and extracts the llama-server.
func extractFromTar(tarPath, destDir string, findServer bool) (string, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return "", fmt.Errorf("open tar: %w", err)
	}
	defer f.Close()

	return extractTar(f, destDir, findServer)
}

// extractTar reads a tar stream and extracts files to destDir.
// If findServer is true, it tracks and returns llama-server's path.
// Symlinks are recreated; .so/.dylib files are NOT matched as server.
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

		destPath := filepath.Join(destDir, filepath.Base(hdr.Name))
		_ = os.Remove(destPath)

		switch hdr.Typeflag {
		case tar.TypeSymlink:
			os.Symlink(hdr.Linkname, destPath)
			continue
		case tar.TypeLink:
			os.Link(filepath.Join(destDir, hdr.Linkname), destPath)
			continue
		}

		dst, err := os.Create(destPath)
		if err != nil {
			return "", fmt.Errorf("create %s: %w", hdr.Name, err)
		}

		_, err = io.Copy(dst, tr)
		dst.Close()
		if err != nil {
			return "", fmt.Errorf("extract %s: %w", hdr.Name, err)
		}

		if findServer {
			base := strings.ToLower(filepath.Base(hdr.Name))
			if base == "llama-server" || base == "llama-server.exe" {
				binaryPath = destPath
				if runtime.GOOS != "windows" {
					os.Chmod(destPath, 0755)
				}
			}
		}
	}

	if findServer && binaryPath == "" {
		return "", fmt.Errorf("llama-server not found in archive")
	}
	return binaryPath, nil
}

// executableName appends ".exe" on Windows for the given base name.
func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
