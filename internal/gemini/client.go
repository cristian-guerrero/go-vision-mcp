package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/oauth2"
)

// DefaultModel is the default Gemini model for vision tasks.
// gemini-3.5-flash is the latest stable mid-size multimodal model
// available on the free tier (as of 2026).
const DefaultModel = "gemini-3.5-flash"

// AvailableModels lists known Gemini vision models available on
// the free tier. Verify availability by calling ListModels.
var AvailableModels = []string{
	"gemini-2.5-flash",
	"gemini-2.5-pro",
	"gemini-2.0-flash",
	"gemini-2.0-flash-lite",
	"gemini-2.5-flash-lite",
	"gemini-3-flash-preview",
	"gemini-3-pro-preview",
}

// Client communicates with the Gemini API for vision tasks.
// It supports both API key and OAuth token authentication, and
// automatically refreshes tokens when they expire.
type Client struct {
	httpClient *http.Client
	apiKey     string
	model      string

	mu       sync.Mutex
	token    *Token
	tokenSrc oauth2.TokenSource
	clientID string
}

// NewClient creates a new Gemini API client.
//
//   - apiKey: set to use API key authentication (simpler).
//   - token + clientID: set to use OAuth token authentication.
//   - model: the Gemini model name (e.g. "gemini-2.0-flash-exp").
//   - httpClient: optional custom HTTP client (nil = http.DefaultClient).
func NewClient(apiKey string, token *Token, clientID, model string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if model == "" {
		model = DefaultModel
	}

	return &Client{
		httpClient: httpClient,
		apiKey:     apiKey,
		model:      model,
		token:      token,
		clientID:   clientID,
	}
}

// SetToken updates the OAuth token (used after refresh).
func (c *Client) SetToken(t *Token) {
	c.mu.Lock()
	c.token = t
	c.mu.Unlock()
}

// SetTokenSource sets an oauth2.TokenSource for automatic token refresh.
func (c *Client) SetTokenSource(ts oauth2.TokenSource) {
	c.mu.Lock()
	c.tokenSrc = ts
	c.mu.Unlock()
}

// Model returns the current Gemini model name.
func (c *Client) Model() string {
	return c.model
}

// SetModel changes the model name.
func (c *Client) SetModel(model string) {
	c.model = model
}

// ChatCompletion sends a single-image vision request to Gemini.
func (c *Client) ChatCompletion(ctx context.Context, prompt, dataURI string) (string, error) {
	return c.chatCompletion(ctx, prompt, []string{dataURI})
}

// ChatCompletionMulti sends a multi-image vision request to Gemini.
func (c *Client) ChatCompletionMulti(ctx context.Context, prompt string, dataURIs []string) (string, error) {
	return c.chatCompletion(ctx, prompt, dataURIs)
}

// chatCompletion builds and sends the Gemini API request.
func (c *Client) chatCompletion(ctx context.Context, prompt string, dataURIs []string) (string, error) {
	authHeader, err := c.buildAuthHeader(ctx)
	if err != nil {
		return "", fmt.Errorf("auth: %w", err)
	}

	geminiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", c.model)

	body := c.buildRequestBody(prompt, dataURIs)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", geminiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	if c.apiKey != "" {
		q := req.URL.Query()
		q.Set("key", c.apiKey)
		req.URL.RawQuery = q.Encode()
		req.Header.Del("Authorization")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		if geminiResp.PromptFeedback != nil {
			reason := geminiResp.PromptFeedback.BlockReason
			if reason != "" {
				return "", fmt.Errorf("request blocked: %s", reason)
			}
		}
		return "", fmt.Errorf("no response from model")
	}

	candidate := geminiResp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		if candidate.FinishReason != "" && candidate.FinishReason != "STOP" {
			return "", fmt.Errorf("response finished: %s", candidate.FinishReason)
		}
		return "", fmt.Errorf("empty response from model")
	}

	var textParts []string
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
	}

	return strings.Join(textParts, "\n"), nil
}

// buildAuthHeader returns the Authorization header value, attempting
// token refresh if the current token is expired.
func (c *Client) buildAuthHeader(ctx context.Context) (string, error) {
	c.mu.Lock()
	apiKey := c.apiKey
	token := c.token
	tokenSrc := c.tokenSrc
	clientID := c.clientID
	c.mu.Unlock()

	if apiKey != "" {
		return "", nil
	}

	if tokenSrc != nil {
		oauthToken, err := tokenSrc.Token()
		if err != nil {
			return "", fmt.Errorf("get token from source: %w", err)
		}
		return "Bearer " + oauthToken.AccessToken, nil
	}

	if token != nil && token.Valid() {
		return "Bearer " + token.AccessToken, nil
	}

	if token != nil && token.RefreshToken != "" && clientID != "" {
		log.Printf("Gemini token expired, refreshing...")
		newToken, err := RefreshToken(ctx, clientID, token.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("refresh token: %w", err)
		}

		c.mu.Lock()
		c.token = newToken
		c.mu.Unlock()

		log.Printf("Gemini token refreshed successfully")
		return "Bearer " + newToken.AccessToken, nil
	}

	return "", fmt.Errorf("no valid authentication method (API key or OAuth token)")
}

// buildRequestBody constructs the Gemini API JSON body for vision.
func (c *Client) buildRequestBody(prompt string, dataURIs []string) map[string]any {
	var parts []map[string]any

	for _, uri := range dataURIs {
		mimeType, b64 := parseDataURI(uri)
		parts = append(parts, map[string]any{
			"inline_data": map[string]any{
				"mime_type": mimeType,
				"data":      b64,
			},
		})
	}

	parts = append(parts, map[string]any{
		"text": prompt,
	})

	return map[string]any{
		"contents": []map[string]any{
			{
				"parts": parts,
			},
		},
	}
}

// parseDataURI extracts mime type and base64 data from a data URI.
// Input: "data:image/png;base64,iVBOR..."
// Output: "image/png", "iVBOR..."
func parseDataURI(uri string) (mimeType, b64 string) {
	if !strings.HasPrefix(uri, "data:") {
		return "image/png", uri
	}

	comma := strings.IndexByte(uri, ',')
	if comma < 0 {
		return "image/png", uri
	}

	header := uri[5:comma]
	b64 = uri[comma+1:]

	if strings.Contains(header, ";base64") {
		header = strings.TrimSuffix(header, ";base64")
	}
	if header == "" {
		header = "image/png"
	}

	return header, b64
}

// geminiResponse maps the Gemini API generateContent response.
type geminiResponse struct {
	Candidates []struct {
		Content      *geminiContent `json:"content,omitempty"`
		FinishReason string         `json:"finishReason,omitempty"`
	} `json:"candidates,omitempty"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason,omitempty"`
	} `json:"promptFeedback,omitempty"`
}

// geminiContent maps the content structure in the response.
type geminiContent struct {
	Parts []struct {
		Text string `json:"text,omitempty"`
	} `json:"parts,omitempty"`
	Role string `json:"role,omitempty"`
}
