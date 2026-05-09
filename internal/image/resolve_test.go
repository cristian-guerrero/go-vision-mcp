package image

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDataURIPassThrough(t *testing.T) {
	dataURI := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="
	result, err := ResolveToDataURI(dataURI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != dataURI {
		t.Errorf("expected passthrough of data URI, got: %s", result)
	}
}

func TestResolveHTTPURL(t *testing.T) {
	testImage := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(testImage)
	}))
	defer ts.Close()

	result, err := ResolveToDataURI(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(result, "data:image/png;base64,") {
		t.Errorf("expected data URI prefix, got: %s", result)
	}
}

func TestResolveLocalFile(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "test.png")
	testImage := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	if err := os.WriteFile(imgPath, testImage, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	result, err := ResolveToDataURI(imgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(result, "data:image/png;base64,") {
		t.Errorf("expected data URI prefix, got: %s", result)
	}
}

func TestResolveInvalidPath(t *testing.T) {
	_, err := ResolveToDataURI("/nonexistent/path/image.png")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestMimeTypeDetection(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"image.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"graphic.webp", "image/webp"},
		{"anim.gif", "image/gif"},
		{"screen.bmp", "image/bmp"},
		{"unknown.xyz", "image/jpeg"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := mimeType(tt.path)
			if result != tt.expected {
				t.Errorf("mimeType(%s) = %s, want %s", tt.path, result, tt.expected)
			}
		})
	}
}

func TestEncodeDataURI(t *testing.T) {
	data := []byte("test")
	result := encodeDataURI("image/png", data)
	expected := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	if result != expected {
		t.Errorf("encodeDataURI() = %s, want %s", result, expected)
	}
}
