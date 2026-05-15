// Package mcp — CLI helper functions for the --analyze-clipboard flag.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ClipboardImageDataURI is an exported wrapper for clipboardImageDataURIImpl,
// used by the CLI --analyze-clipboard flow in main.go.
func ClipboardImageDataURI() (string, error) {
	return clipboardImageDataURIImpl()
}

// CLIAnalyzeClipboard performs a one-shot clipboard analysis from the
// command line (--analyze-clipboard). It reads the clipboard, builds
// the chat request, and returns the model's text response.
func CLIAnalyzeClipboard(ctx context.Context, prompt, llamaURL string, httpClient *http.Client) (string, error) {
	dataURI, err := ClipboardImageDataURI()
	if err != nil {
		return "", fmt.Errorf("clipboard: %w", err)
	}

	req := chatRequest{
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
		Messages: []chatMessage{
			{
				Role: "user",
				Content: []chatContent{
					{Type: "image_url", ImageURL: &imageURL{URL: dataURI}},
					{Type: "text", Text: prompt},
				},
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", llamaURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("llama-server request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama-server returned HTTP %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from model")
	}

	return chatResp.Choices[0].Message.Content, nil
}
