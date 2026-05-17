package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// LlamaClient communicates with llama-server via the OpenAI-compatible
// /v1/chat/completions endpoint. It manages the loaded state (whether
// llama-server is running) and provides the lazy restart lifecycle
// used by ToolHandler.
type LlamaClient struct {
	llamaURL   string
	httpClient HTTPClient

	mu          sync.Mutex
	restartMu   sync.Mutex
	loaded      bool
	restartFunc func(context.Context) error
}

// NewLlamaClient creates a client for the given llama-server base URL.
// When httpClient is nil, http.DefaultClient is used.
func NewLlamaClient(llamaURL string, httpClient HTTPClient) *LlamaClient {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &LlamaClient{
		llamaURL:   llamaURL,
		httpClient: httpClient,
	}
}

// SetLoaded marks whether llama-server is currently running and ready.
func (c *LlamaClient) SetLoaded(v bool) {
	c.mu.Lock()
	c.loaded = v
	c.mu.Unlock()
}

// IsLoaded returns true if llama-server is currently running.
func (c *LlamaClient) IsLoaded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loaded
}

// SetRestartFunc registers a function that downloads models (if needed)
// and starts/restarts llama-server. Called on-demand from ChatCompletion.
func (c *LlamaClient) SetRestartFunc(f func(context.Context) error) {
	c.mu.Lock()
	c.restartFunc = f
	c.mu.Unlock()
}

// SetLlamaURL sets the base URL for the llama-server API endpoint.
func (c *LlamaClient) SetLlamaURL(url string) {
	c.llamaURL = url
}

// ensureLoaded starts/restarts llama-server if not running.
// Uses restartMu to serialize concurrent restart attempts: only one
// goroutine starts the server; others wait and re-check loaded.
func (c *LlamaClient) ensureLoaded(ctx context.Context) error {
	c.restartMu.Lock()
	defer c.restartMu.Unlock()

	if c.loaded || c.restartFunc == nil {
		return nil
	}

	log.Printf("llama-server not loaded, starting...")
	return c.restartFunc(context.Background())
}

// ChatCompletion sends a single-image vision request to llama-server.
// Automatically starts/restarts llama-server if not loaded.
func (c *LlamaClient) ChatCompletion(ctx context.Context, prompt, dataURI string) (string, error) {
	if err := c.ensureLoaded(ctx); err != nil {
		return "", fmt.Errorf("start llama-server: %w", err)
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

	return c.sendChatRequest(ctx, body)
}

// ChatCompletionMulti sends a vision request with multiple images
// to llama-server. Automatically starts/restarts llama-server if not
// loaded.
func (c *LlamaClient) ChatCompletionMulti(ctx context.Context, prompt string, dataURIs []string) (string, error) {
	if err := c.ensureLoaded(ctx); err != nil {
		return "", fmt.Errorf("start llama-server: %w", err)
	}

	var contents []chatContent
	for _, uri := range dataURIs {
		contents = append(contents, chatContent{Type: "image_url", ImageURL: &imageURL{URL: uri}})
	}
	contents = append(contents, chatContent{Type: "text", Text: prompt})

	req := chatRequest{
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
		Messages: []chatMessage{
			{
				Role:    "user",
				Content: contents,
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	return c.sendChatRequest(ctx, body)
}

// sendChatRequest POSTs a JSON body to /v1/chat/completions with an
// automatic retry on connection failure (attempts to restart
// llama-server before the retry).
func (c *LlamaClient) sendChatRequest(ctx context.Context, body []byte) (string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.llamaURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			if attempt == 0 {
				log.Printf("llama-server unreachable, restarting...")
				if restartErr := c.ensureLoaded(context.Background()); restartErr != nil {
					return "", fmt.Errorf("llama-server request: %w (restart failed: %v)", err, restartErr)
				}
				continue
			}
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

	return "", fmt.Errorf("llama-server request failed after restart")
}

// chatRequest is the JSON body sent to llama-server's
// /v1/chat/completions endpoint.
type chatRequest struct {
	Messages           []chatMessage  `json:"messages"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

// chatMessage represents a single message in the chat request
// (role + content array).
type chatMessage struct {
	Role    string        `json:"role"`
	Content []chatContent `json:"content"`
}

// chatContent is a single item within a message: either "text"
// or "image_url".
type chatContent struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

// imageURL wraps the data URI or URL of an image for the vision model.
type imageURL struct {
	URL string `json:"url"`
}

// chatResponse maps the OpenAI-compatible response from llama-server.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
