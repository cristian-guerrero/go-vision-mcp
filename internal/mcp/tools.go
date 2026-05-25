// Package mcp implements the MCP tool handlers. It defines the
// vision-mcp tools (analyze_image, analyze_clipboard, clipboard
// history) and manages communication with llama-server via the
// OpenAI-compatible /v1/chat/completions endpoint.
package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/cristian-guerrero/go-vision-mcp/internal/clipboard"
	"github.com/cristian-guerrero/go-vision-mcp/internal/image"
)

// HTTPClient defines the interface for sending HTTP requests.
// *http.Client satisfies this interface, making the MCP handler
// testable without real network calls.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ToolHandler manages MCP tool registration, activity tracking for
// idle timeout, clipboard monitor integration, and delegates
// communication to either LlamaClient (local backend) or
// GeminiClient (Gemini API backend).
type ToolHandler struct {
	customPrompt string
	ready        chan struct{}
	clipboardMon clipboard.MonitorInterface
	client       *LlamaClient
	geminiClient geminiClient

	mu           sync.Mutex
	lastActivity time.Time
	stopFunc     func()
}

// geminiClient defines the interface used by ToolHandler for Gemini.
type geminiClient interface {
	ChatCompletion(ctx context.Context, prompt, dataURI string) (string, error)
	ChatCompletionMulti(ctx context.Context, prompt string, dataURIs []string) (string, error)
}

// NewToolHandler creates a ToolHandler. llamaURL may be empty and set
// later; customPrompt is a fmt template like "Analyze: %s".
func NewToolHandler(llamaURL, customPrompt string) *ToolHandler {
	return &ToolHandler{
		customPrompt: customPrompt,
		ready:        make(chan struct{}),
		lastActivity: time.Now(),
		client:       NewLlamaClient(llamaURL, nil),
	}
}

// SetLoaded marks whether llama-server is currently running and ready.
func (h *ToolHandler) SetLoaded(v bool) { h.client.SetLoaded(v) }

// IsLoaded returns true if llama-server is currently running.
func (h *ToolHandler) IsLoaded() bool { return h.client.IsLoaded() }

// IdleTime returns the duration since the last tool call activity.
func (h *ToolHandler) IdleTime() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Since(h.lastActivity)
}

// SetRestartFunc registers a function that downloads models (if needed)
// and starts/restarts llama-server. Called on-demand from chatCompletion.
func (h *ToolHandler) SetRestartFunc(f func(context.Context) error) { h.client.SetRestartFunc(f) }

// SetStopFunc registers a cleanup function that stops llama-server
// and the clipboard monitor.
func (h *ToolHandler) SetStopFunc(f func()) {
	h.mu.Lock()
	h.stopFunc = f
	h.mu.Unlock()
}

// SetClipboardMonitor attaches the clipboard history monitor to the
// handler so clipboard tools can access historical images.
func (h *ToolHandler) SetClipboardMonitor(m clipboard.MonitorInterface) {
	h.clipboardMon = m
}

// Stop calls the registered stop function (if any) to tear down
// llama-server and clipboard monitor.
func (h *ToolHandler) Stop() {
	h.mu.Lock()
	f := h.stopFunc
	h.mu.Unlock()
	if f != nil {
		f()
	}
}

// trackActivity records the current time so the idle timeout monitor
// can detect inactivity.
func (h *ToolHandler) trackActivity() {
	h.mu.Lock()
	h.lastActivity = time.Now()
	h.mu.Unlock()
}

// SetGeminiClient sets the Gemini API client. When set, all tool
// calls are routed to Gemini instead of llama-server.
func (h *ToolHandler) SetGeminiClient(c geminiClient) { h.geminiClient = c }

// SetLlamaURL sets the base URL for the llama-server API endpoint.
func (h *ToolHandler) SetLlamaURL(url string) { h.client.SetLlamaURL(url) }

// SetReady signals that the handler is initialized and tools can
// proceed (closes the ready channel).
func (h *ToolHandler) SetReady() {
	close(h.ready)
}

// waitReady blocks until SetReady has been called or the context is
// cancelled. Tools call this before sending requests to llama-server.
func (h *ToolHandler) waitReady(ctx context.Context) error {
	select {
	case <-h.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RegisterTools registers all vision-mcp MCP tools on the given
// server: analyze_image, analyze_clipboard, list_clipboard_history,
// analyze_clipboard_image, analyze_clipboard_images.
func (h *ToolHandler) RegisterTools(s *server.MCPServer) {
	s.AddTool(analyzeImageTool(), h.handleAnalyzeImage)
	s.AddTool(analyzeClipboardTool(), h.handleAnalyzeClipboard)
	s.AddTool(listClipboardHistoryTool(), h.handleListClipboardHistory)
	s.AddTool(analyzeClipboardImageTool(), h.handleAnalyzeClipboardImage)
	s.AddTool(analyzeClipboardImagesTool(), h.handleAnalyzeClipboardImages)
}

// analyzeImageTool defines the "analyze_image" MCP tool with
// required "prompt" (string) and "image" (string) parameters.
func analyzeImageTool() mcp.Tool {
	return mcp.NewTool("analyze_image",
		mcp.WithDescription("Ask a custom question about an image (identify objects, read text, count items, compare elements, etc). Provide an image via URL, local file path, or base64 data URI."),
		mcp.WithString("question",
			mcp.Required(),
			mcp.Description("What to ask about the image. Be specific (e.g. 'What objects are in this image?', 'Read all text visible', 'How many people?', 'Describe the colors')."),
		),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("The image to analyze: URL (http/https), absolute local file path, or data:image/...;base64,... URI"),
		),
	)
}

// analyzeClipboardTool defines the "analyze_clipboard" MCP tool with
// a required "prompt" parameter (reads image from system clipboard).
func analyzeClipboardTool() mcp.Tool {
	return mcp.NewTool("analyze_clipboard",
		mcp.WithDescription("Ask a custom question about the image currently in your system clipboard. No image parameter needed — it reads the clipboard automatically. Use this when the user asks a specific question about an image they just copied (e.g. 'what model is this?', 'read the text')."),
		mcp.WithString("question",
			mcp.Required(),
			mcp.Description("What to ask about the clipboard image. Be specific and direct (e.g. 'List all car models in this image', 'What does the sign say?', 'Describe the person')."),
		),
	)
}

// listClipboardHistoryTool defines a no-parameter tool that lists
// all clipboard monitor entries.
func listClipboardHistoryTool() mcp.Tool {
	return mcp.NewTool("list_clipboard_history",
		mcp.WithDescription("List all images captured by the clipboard monitor with their indices and timestamps. Use this to see what images have been copied to the clipboard since the monitor started. Requires clipboard_monitor_enabled=true in config."),
	)
}

// analyzeClipboardImageTool defines a tool that analyzes a specific
// image from clipboard history by index.
func analyzeClipboardImageTool() mcp.Tool {
	return mcp.NewTool("analyze_clipboard_image",
		mcp.WithDescription("Ask a custom question about a specific image from the clipboard history, referenced by its index. Use list_clipboard_history first to see available images. Requires clipboard_monitor_enabled=true in config."),
		mcp.WithNumber("index",
			mcp.Required(),
			mcp.Description("The index of the image in the clipboard history (e.g. 1 for the first image captured, 2 for the second)."),
		),
		mcp.WithString("question",
			mcp.Required(),
			mcp.Description("What to ask about the image. Be specific."),
		),
	)
}

// analyzeClipboardImagesTool defines a tool that analyzes multiple
// images from clipboard history by comma-separated indices.
func analyzeClipboardImagesTool() mcp.Tool {
	return mcp.NewTool("analyze_clipboard_images",
		mcp.WithDescription("Ask a question about multiple images from the clipboard history at once. Provide comma-separated indices. Useful for comparing images (e.g. 'la primera imagen es el antes y la segunda el después'). Requires clipboard_monitor_enabled=true in config."),
		mcp.WithString("indices",
			mcp.Required(),
			mcp.Description("Comma-separated list of image indices from clipboard history, e.g. '1,2,3'."),
		),
		mcp.WithString("question",
			mcp.Required(),
			mcp.Description("What to ask about the images. You can reference them by position (e.g. 'Image 1 is the BEFORE, Image 2 is the AFTER. What changed?')."),
		),
	)
}

// clipboardImageDataURI returns the current clipboard image as a
// data:image/png;base64,... URI. Dispatches to platform-specific impl.
func clipboardImageDataURI() (string, error) {
	return clipboardImageDataURIImpl()
}

// handleAnalyzeClipboard reads the current clipboard image, falls back
// to clipboard monitor history, and sends it to llama-server with the
// user's prompt.
func (h *ToolHandler) handleAnalyzeClipboard(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.trackActivity()
	prompt, _ := request.RequireString("question")

	if prompt == "" {
		return mcp.NewToolResultError("question is required"), nil
	}

	dataURI, err := clipboardImageDataURI()
	if err != nil {
		if h.clipboardMon != nil {
			latest, histErr := h.clipboardMon.GetLatestImage()
			if histErr == nil {
				dataURI = latest
			}
		}
	}

	if dataURI == "" {
		return mcp.NewToolResultError("No image found in clipboard or clipboard history. Copy an image first."), nil
	}

	if err := h.waitReady(ctx); err != nil {
		return mcp.NewToolResultError("llama-server not ready yet"), nil
	}

	response, err := h.chatCompletion(ctx, prompt, dataURI)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Vision model error: %v", err)), nil
	}

	return mcp.NewToolResultText(response), nil
}

// handleListClipboardHistory returns a text listing of all clipboard
// history entries with their index, timestamp, and source.
func (h *ToolHandler) handleListClipboardHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.trackActivity()
	if h.clipboardMon == nil {
		return mcp.NewToolResultError("Clipboard monitor not enabled. Set clipboard_monitor_enabled=true in config."), nil
	}
	entries := h.clipboardMon.ListHistory()
	if len(entries) == 0 {
		return mcp.NewToolResultText("Clipboard history is empty. Copy an image to the clipboard first."), nil
	}
	var lines []string
	for _, e := range entries {
		source := "clipboard"
		if e.OriginalPath != "" {
			source = e.OriginalPath
		}
		lines = append(lines, fmt.Sprintf("#%d — %s — %s", e.Index, e.Timestamp.Format("15:04:05"), source))
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

// handleAnalyzeClipboardImage retrieves a single clipboard history
// image by index and analyzes it with the given prompt.
func (h *ToolHandler) handleAnalyzeClipboardImage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.trackActivity()
	if h.clipboardMon == nil {
		return mcp.NewToolResultError("Clipboard monitor not enabled. Set clipboard_monitor_enabled=true in config."), nil
	}

	index, err := requestInt(request, "index")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid index: %v", err)), nil
	}

	prompt, _ := request.RequireString("question")
	if prompt == "" {
		return mcp.NewToolResultError("question is required"), nil
	}

	if err := h.waitReady(ctx); err != nil {
		return mcp.NewToolResultError("llama-server not ready yet"), nil
	}

	dataURI, err := h.clipboardMon.GetImage(index)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	response, err := h.chatCompletion(ctx, prompt, dataURI)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Vision model error: %v", err)), nil
	}

	return mcp.NewToolResultText(response), nil
}

// handleAnalyzeClipboardImages retrieves multiple clipboard history
// images by comma-separated indices and sends them all to the vision
// model in a single request (multi-image analysis).
func (h *ToolHandler) handleAnalyzeClipboardImages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.trackActivity()
	if h.clipboardMon == nil {
		return mcp.NewToolResultError("Clipboard monitor not enabled. Set clipboard_monitor_enabled=true in config."), nil
	}

	indicesStr, _ := request.RequireString("indices")
	if indicesStr == "" {
		return mcp.NewToolResultError("indices is required (comma-separated, e.g. '1,2,3')"), nil
	}

	prompt, _ := request.RequireString("question")
	if prompt == "" {
		return mcp.NewToolResultError("question is required"), nil
	}

	var indices []int
	for _, s := range strings.Split(indicesStr, ",") {
		i, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid index '%s': %v", s, err)), nil
		}
		indices = append(indices, i)
	}

	if len(indices) == 0 {
		return mcp.NewToolResultError("No valid indices provided"), nil
	}

	if err := h.waitReady(ctx); err != nil {
		return mcp.NewToolResultError("llama-server not ready yet"), nil
	}

	var dataURIs []string
	for _, idx := range indices {
		dataURI, err := h.clipboardMon.GetImage(idx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dataURIs = append(dataURIs, dataURI)
	}

	response, err := h.chatCompletionMulti(ctx, prompt, dataURIs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Vision model error: %v", err)), nil
	}

	return mcp.NewToolResultText(response), nil
}

// requestInt extracts an integer parameter from an MCP call request,
// handling the fact that mcp-go v0.52 stores Arguments as any and
// numbers may arrive as json.Number or float64.
func requestInt(request mcp.CallToolRequest, key string) (int, error) {
	raw := request.Params.Arguments
	if raw == nil {
		return 0, fmt.Errorf("no arguments")
	}
	switch v := raw.(type) {
	case map[string]interface{}:
		if val, ok := v[key]; ok {
			switch n := val.(type) {
			case float64:
				return int(n), nil
			case int:
				return n, nil
			case int64:
				return int(n), nil
			}
		}
	}
	args := request.GetArguments()
	if args == nil {
		return 0, fmt.Errorf("no arguments")
	}
	if val, ok := args[key]; ok {
		switch n := val.(type) {
		case float64:
			return int(n), nil
		case int:
			return n, nil
		case int64:
			return int(n), nil
		}
	}
	return 0, fmt.Errorf("missing or invalid '%s'", key)
}

// handleAnalyzeImage resolves the image reference (URL, file path,
// or data URI) to a base64 data URI, then sends it to llama-server
// with the user's prompt.
func (h *ToolHandler) handleAnalyzeImage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.trackActivity()
	prompt, _ := request.RequireString("question")
	imageRef, _ := request.RequireString("image")

	if prompt == "" || imageRef == "" {
		return mcp.NewToolResultError("question and image are required"), nil
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

// chatCompletion sends a single-image vision request via the active
// backend (GeminiClient or LlamaClient).
func (h *ToolHandler) chatCompletion(ctx context.Context, prompt, dataURI string) (string, error) {
	if h.geminiClient != nil {
		return h.geminiClient.ChatCompletion(ctx, prompt, dataURI)
	}
	return h.client.ChatCompletion(ctx, prompt, dataURI)
}

// chatCompletionMulti sends a multi-image vision request via the
// active backend (GeminiClient or LlamaClient).
func (h *ToolHandler) chatCompletionMulti(ctx context.Context, prompt string, dataURIs []string) (string, error) {
	if h.geminiClient != nil {
		return h.geminiClient.ChatCompletionMulti(ctx, prompt, dataURIs)
	}
	return h.client.ChatCompletionMulti(ctx, prompt, dataURIs)
}
