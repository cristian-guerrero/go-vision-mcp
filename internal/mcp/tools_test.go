package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestToolDefinitions(t *testing.T) {
	s := server.NewMCPServer("test", "1.0.0")
	handler := NewToolHandler("http://localhost:8001", "test prompt: %s")
	handler.RegisterTools(s)
}

func TestAnalyzeImageMissingArgs(t *testing.T) {
	handler := NewToolHandler("http://localhost:8001", "test")
	req := mcp.CallToolRequest{}

	result, err := handler.handleAnalyzeImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing arguments")
	}
}

func TestDescribeImageMissingArgs(t *testing.T) {
	handler := NewToolHandler("http://localhost:8001", "test")
	req := mcp.CallToolRequest{}

	result, err := handler.handleDescribeImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing image argument")
	}
}

func TestChatCompletionWithMock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "This is a test image description."}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	handler := NewToolHandler(ts.URL, "test")
	result, err := handler.chatCompletion(context.Background(), "Describe this image", "data:image/png;base64,test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "test image") {
		t.Errorf("unexpected response: %s", result)
	}
}

func TestChatCompletionError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	handler := NewToolHandler(ts.URL, "test")
	_, err := handler.chatCompletion(context.Background(), "test", "data:image/png;base64,test")
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}
