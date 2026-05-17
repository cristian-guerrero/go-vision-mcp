// Package mcp — CLI helper functions for the --analyze-clipboard flag.
package mcp

import (
	"context"
	"fmt"
	"net/http"
)

// ClipboardImageDataURI is an exported wrapper for clipboardImageDataURIImpl,
// used by the CLI --analyze-clipboard flow in main.go.
func ClipboardImageDataURI() (string, error) {
	return clipboardImageDataURIImpl()
}

// CLIAnalyzeClipboard performs a one-shot clipboard analysis from the
// command line (--analyze-clipboard). It reads the clipboard and
// delegates the HTTP request to LlamaClient.
func CLIAnalyzeClipboard(ctx context.Context, prompt, llamaURL string, httpClient *http.Client) (string, error) {
	dataURI, err := ClipboardImageDataURI()
	if err != nil {
		return "", fmt.Errorf("clipboard: %w", err)
	}
	client := NewLlamaClient(llamaURL, httpClient)
	return client.ChatCompletion(ctx, prompt, dataURI)
}
