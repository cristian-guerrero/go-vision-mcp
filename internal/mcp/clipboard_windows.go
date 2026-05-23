//go:build windows

package mcp

import (
	"encoding/base64"
	"fmt"

	"github.com/cristian-guerrero/go-vision-mcp/internal/clipboard"
	"github.com/cristian-guerrero/go-vision-mcp/internal/image"
)

// CheckClipboardDeps returns an empty string on Windows — clipboard
// access uses the built-in Win32 API and requires no external tools.
func CheckClipboardDeps() string {
	return ""
}

// clipboardImageDataURIImpl reads the clipboard image using Win32 API
// and returns a data:image/...;base64,... URI.
func clipboardImageDataURIImpl() (string, error) {
	pngData, origPath, _, err := clipboard.ReadClipboardImage()
	if err != nil {
		return "", fmt.Errorf("clipboard: %w", err)
	}

	// File drops (including WebP/AVIF) are resolved by
	// image.ResolveToDataURI which handles conversion.
	if origPath != "" {
		return image.ResolveToDataURI(origPath)
	}

	b64 := base64.StdEncoding.EncodeToString(pngData)
	return fmt.Sprintf("data:image/png;base64,%s", b64), nil
}
