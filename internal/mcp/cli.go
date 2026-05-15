package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func ClipboardImageDataURI() (string, error) {
	return clipboardImageDataURIImpl()
}

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
