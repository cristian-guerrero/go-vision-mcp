// Package image resolves image references (URLs, file paths, data URIs)
// into data:image/...;base64,... URIs suitable for llama-server's
// vision API. WebP images are automatically converted to PNG.
package image

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveToDataURI converts an image reference to a data URI.
// Accepted formats:
//   - data: URIs (passed through)
//   - http:// or https:// URLs (downloaded)
//   - file:/// URIs (decoded and read)
//   - Local file paths (read from disk)
func ResolveToDataURI(ref string) (string, error) {
	if strings.HasPrefix(ref, "data:") {
		return ref, nil
	}

	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return downloadToDataURI(ref)
	}

	if strings.HasPrefix(ref, "file:///") {
		path := strings.TrimPrefix(ref, "file:///")
		path, _ = url.PathUnescape(path)
		if runtime.GOOS == "windows" {
			path = strings.ReplaceAll(path, "/", "\\")
		}
		return fileToDataURI(path)
	}

	return fileToDataURI(ref)
}

// fileToDataURI reads a local image file and returns its data URI.
// WebP files are transparently converted to PNG.
func fileToDataURI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read image file: %w", err)
	}

	mime := mimeType(path)
	if mime == "image/webp" {
		pngData, err := DecodeWebPToPNG(data)
		if err != nil {
			return "", fmt.Errorf("convert webp to png: %w", err)
		}
		return encodeDataURI("image/png", pngData), nil
	}

	return encodeDataURI(mime, data), nil
}

// downloadToDataURI fetches an image via HTTP and returns its data URI.
func downloadToDataURI(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image download HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read image body: %w", err)
	}

	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "image/jpeg"
	}

	return encodeDataURI(mime, data), nil
}

// encodeDataURI builds a data: URI from MIME type and raw bytes.
func encodeDataURI(mime string, data []byte) string {
	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mime, b64)
}

// mimeType returns the MIME type for common image extensions
// (png → image/png, jpg/jpeg → image/jpeg, webp → image/webp, etc.).
func mimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"
	default:
		return "image/jpeg"
	}
}
