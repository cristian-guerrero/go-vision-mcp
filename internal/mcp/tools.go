package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/vision-mcp/internal/image"
)

type ToolHandler struct {
	llamaURL     string
	customPrompt string
	httpClient   *http.Client
	ready        chan struct{}

	mu           sync.Mutex
	lastActivity time.Time
	loaded       bool
	restartFunc  func(context.Context) error
	stopFunc     func()
}

func NewToolHandler(llamaURL, customPrompt string) *ToolHandler {
	return &ToolHandler{
		llamaURL:     llamaURL,
		customPrompt: customPrompt,
		httpClient:   &http.Client{},
		ready:        make(chan struct{}),
		lastActivity: time.Now(),
	}
}

func (h *ToolHandler) SetLoaded(v bool) {
	h.mu.Lock()
	h.loaded = v
	h.mu.Unlock()
}

func (h *ToolHandler) IsLoaded() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.loaded
}

func (h *ToolHandler) IdleTime() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Since(h.lastActivity)
}

func (h *ToolHandler) SetRestartFunc(f func(context.Context) error) {
	h.mu.Lock()
	h.restartFunc = f
	h.mu.Unlock()
}

func (h *ToolHandler) SetStopFunc(f func()) {
	h.mu.Lock()
	h.stopFunc = f
	h.mu.Unlock()
}

func (h *ToolHandler) Stop() {
	h.mu.Lock()
	f := h.stopFunc
	h.mu.Unlock()
	if f != nil {
		f()
	}
}

func (h *ToolHandler) trackActivity() {
	h.mu.Lock()
	h.lastActivity = time.Now()
	h.mu.Unlock()
}

func (h *ToolHandler) SetLlamaURL(url string) {
	h.llamaURL = url
}

func (h *ToolHandler) SetReady() {
	close(h.ready)
}

func (h *ToolHandler) waitReady(ctx context.Context) error {
	select {
	case <-h.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *ToolHandler) RegisterTools(s *server.MCPServer) {
	s.AddTool(analyzeImageTool(), h.handleAnalyzeImage)
	s.AddTool(describeImageTool(), h.handleDescribeImage)
	s.AddTool(describeClipboardTool(), h.handleDescribeClipboard)
	s.AddTool(analyzeClipboardTool(), h.handleAnalyzeClipboard)
}

func analyzeImageTool() mcp.Tool {
	return mcp.NewTool("analyze_image",
		mcp.WithDescription("Ask a custom question about an image (identify objects, read text, count items, compare elements, etc). Provide an image via URL, local file path, or base64 data URI."),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("What to ask about the image. Be specific (e.g. 'What objects are in this image?', 'Read all text visible', 'How many people?', 'Describe the colors')."),
		),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("The image to analyze: URL (http/https), absolute local file path, or data:image/...;base64,... URI"),
		),
	)
}

func describeImageTool() mcp.Tool {
	return mcp.NewTool("describe_image",
		mcp.WithDescription("Get a full natural-language description of what an image shows: objects, people, text, colors, layout, and scene. Use this when you just want to know what's in the image, not ask a specific question. Provide the image via URL, local file path, or base64 data URI."),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("The image to describe: URL (http/https), absolute local file path, or data:image/...;base64,... URI"),
		),
		mcp.WithString("detail",
			mcp.Description("Level of detail: 'brief' (1-2 sentences) or 'detailed' (full description). Defaults to detailed."),
		),
	)
}

func describeClipboardTool() mcp.Tool {
	return mcp.NewTool("describe_clipboard",
		mcp.WithDescription("Describe the image currently in your system clipboard. No image parameter needed — it reads the clipboard automatically. Use this when the user asks about an image they just copied."),
		mcp.WithString("detail",
			mcp.Description("Level of detail: 'brief' (1-2 sentences) or 'detailed' (full description). Defaults to detailed."),
		),
	)
}

func analyzeClipboardTool() mcp.Tool {
	return mcp.NewTool("analyze_clipboard",
		mcp.WithDescription("Ask a custom question about the image currently in your system clipboard. No image parameter needed — it reads the clipboard automatically. Use this when the user asks a specific question about an image they just copied (e.g. 'what model is this?', 'read the text')."),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("What to ask about the clipboard image. Be specific and direct (e.g. 'List all car models in this image', 'What does the sign say?', 'Describe the person')."),
		),
	)
}

func clipboardImageDataURI() (string, error) {
	return clipboardImageDataURIImpl()
}

func (h *ToolHandler) handleDescribeClipboard(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.trackActivity()
	if err := h.waitReady(ctx); err != nil {
		return mcp.NewToolResultError("llama-server not ready yet"), nil
	}

	detail := request.GetString("detail", "detailed")

	dataURI, err := clipboardImageDataURI()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Clipboard error: %v", err)), nil
	}

	prompt := "Describe this image in detail, including all objects, text, colors, layout, and any notable elements."
	if detail == "brief" {
		prompt = "Briefly describe this image in 1-2 sentences."
	}

	response, err := h.chatCompletion(ctx, prompt, dataURI)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Vision model error: %v", err)), nil
	}

	return mcp.NewToolResultText(response), nil
}

func (h *ToolHandler) handleAnalyzeClipboard(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.trackActivity()
	prompt, _ := request.RequireString("prompt")

	if prompt == "" {
		return mcp.NewToolResultError("prompt is required"), nil
	}

	if err := h.waitReady(ctx); err != nil {
		return mcp.NewToolResultError("llama-server not ready yet"), nil
	}

	dataURI, err := clipboardImageDataURI()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Clipboard error: %v", err)), nil
	}

	response, err := h.chatCompletion(ctx, prompt, dataURI)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Vision model error: %v", err)), nil
	}

	return mcp.NewToolResultText(response), nil
}

func (h *ToolHandler) handleAnalyzeImage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.trackActivity()
	prompt, _ := request.RequireString("prompt")
	imageRef, _ := request.RequireString("image")

	if prompt == "" || imageRef == "" {
		return mcp.NewToolResultError("prompt and image are required"), nil
	}

	if err := h.waitReady(ctx); err != nil {
		return mcp.NewToolResultError("llama-server not ready yet"), nil
	}

	dataURI, err := image.ResolveToDataURI(imageRef)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve image: %v", err)), nil
	}

	response, err := h.chatCompletion(ctx, prompt, dataURI)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Vision model error: %v", err)), nil
	}

	return mcp.NewToolResultText(response), nil
}

func (h *ToolHandler) handleDescribeImage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.trackActivity()
	imageRef, _ := request.RequireString("image")
	detail := request.GetString("detail", "detailed")

	if imageRef == "" {
		return mcp.NewToolResultError("image is required"), nil
	}

	if err := h.waitReady(ctx); err != nil {
		return mcp.NewToolResultError("llama-server not ready yet"), nil
	}

	dataURI, err := image.ResolveToDataURI(imageRef)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve image: %v", err)), nil
	}

	prompt := "Describe this image in detail, including all objects, text, colors, layout, and any notable elements."
	if detail == "brief" {
		prompt = "Briefly describe this image in 1-2 sentences."
	}

	response, err := h.chatCompletion(ctx, prompt, dataURI)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Vision model error: %v", err)), nil
	}

	return mcp.NewToolResultText(response), nil
}

type chatRequest struct {
	Messages       []chatMessage          `json:"messages"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content []chatContent `json:"content"`
}

type chatContent struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (h *ToolHandler) chatCompletion(ctx context.Context, prompt, dataURI string) (string, error) {
	h.mu.Lock()
	if !h.loaded && h.restartFunc != nil {
		fn := h.restartFunc
		h.mu.Unlock()
		log.Printf("llama-server not loaded, starting...")
		if err := fn(context.Background()); err != nil {
			return "", fmt.Errorf("start llama-server: %w", err)
		}
	} else {
		h.mu.Unlock()
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

	for attempt := 0; attempt < 2; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, "POST", h.llamaURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := h.httpClient.Do(httpReq)
		if err != nil {
			if attempt == 0 {
				h.mu.Lock()
				fn := h.restartFunc
				h.mu.Unlock()
				if fn != nil {
					log.Printf("llama-server unreachable, restarting...")
					if restartErr := fn(context.Background()); restartErr != nil {
						return "", fmt.Errorf("llama-server request: %w (restart failed: %v)", err, restartErr)
					}
					continue
				}
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
