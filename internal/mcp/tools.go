package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/vision-mcp/internal/image"
)

type ToolHandler struct {
	llamaURL     string
	customPrompt string
	httpClient   *http.Client
}

func NewToolHandler(llamaURL, customPrompt string) *ToolHandler {
	return &ToolHandler{
		llamaURL:     llamaURL,
		customPrompt: customPrompt,
		httpClient:   &http.Client{},
	}
}

func (h *ToolHandler) RegisterTools(s *server.MCPServer) {
	s.AddTool(analyzeImageTool(), h.handleAnalyzeImage)
	s.AddTool(describeImageTool(), h.handleDescribeImage)
}

func analyzeImageTool() mcp.Tool {
	return mcp.NewTool("analyze_image",
		mcp.WithDescription("Analyze an image with a custom prompt using the local vision model."),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("Question or instruction about the image"),
		),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("URL (http/https), local file path, or base64 data URI"),
		),
	)
}

func describeImageTool() mcp.Tool {
	return mcp.NewTool("describe_image",
		mcp.WithDescription("Get a visual description of an image."),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("URL, file path, or data URI"),
		),
		mcp.WithString("detail",
			mcp.Description("Level of detail: brief or detailed"),
		),
	)
}

func (h *ToolHandler) handleAnalyzeImage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt, _ := request.RequireString("prompt")
	imageRef, _ := request.RequireString("image")

	if prompt == "" || imageRef == "" {
		return mcp.NewToolResultError("prompt and image are required"), nil
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
	imageRef, _ := request.RequireString("image")
	detail := request.GetString("detail", "detailed")

	if imageRef == "" {
		return mcp.NewToolResultError("image is required"), nil
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
	Messages []chatMessage `json:"messages"`
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
	req := chatRequest{
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", h.llamaURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(httpReq)
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
