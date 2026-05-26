// Package gemini implements Google Gemini API authentication via
// Device OAuth flow and API key, plus the vision client for
// analyzing images using Gemini models through
// generativelanguage.googleapis.com.
package gemini

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// GeminiScope is the OAuth scope required for the Generative Language API.
const GeminiScope = "https://www.googleapis.com/auth/generative-language.retriever"

// googleEndpoint is the standard Google OAuth 2.0 endpoint with
// device code flow support.
var googleEndpoint = oauth2.Endpoint{
	AuthURL:       "https://accounts.google.com/o/oauth2/auth",
	TokenURL:      "https://oauth2.googleapis.com/token",
	DeviceAuthURL: "https://oauth2.googleapis.com/device/code",
}

// Token wraps oauth2.Token for JSON serialization into config.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

// toOAuth2 converts the internal Token to x/oauth2.Token.
func (t *Token) toOAuth2() *oauth2.Token {
	if t == nil {
		return nil
	}
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.Expiry,
	}
}

// tokenFromOAuth2 converts an x/oauth2.Token to the internal Token.
func tokenFromOAuth2(t *oauth2.Token) *Token {
	if t == nil {
		return nil
	}
	return &Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.Expiry,
	}
}

// Valid returns true if the token has a non-empty access token and
// has not expired (or has no expiry set).
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true
	}
	return time.Now().Before(t.Expiry)
}

// StartDeviceFlow initiates the Device OAuth flow and returns the
// DeviceAuthResponse containing the user code and verification URL.
// The caller should display these to the user and call PollForToken.
func StartDeviceFlow(ctx context.Context, clientID string) (*DeviceAuthResponse, error) {
	config := &oauth2.Config{
		ClientID: clientID,
		Scopes:   []string{GeminiScope},
		Endpoint: googleEndpoint,
	}

	da, err := config.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("device auth: %w", err)
	}

	return (*DeviceAuthResponse)(da), nil
}

// DeviceAuthResponse wraps the oauth2 DeviceAuthResponse with a
// simpler display interface.
type DeviceAuthResponse struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	Expiry                  time.Time
	Interval                int64
}

// DisplayMessage returns a human-readable string instructing the user
// to visit the verification URL and enter the code.
func (d *DeviceAuthResponse) DisplayMessage() string {
	var b strings.Builder
	b.WriteString("To authenticate with Google:\n\n")
	b.WriteString(fmt.Sprintf("  1. Open this URL in your browser:\n     %s\n", d.VerificationURI))
	b.WriteString(fmt.Sprintf("  2. Enter the code: %s\n", d.UserCode))
	b.WriteString(fmt.Sprintf("  3. Sign in and grant the requested permissions\n\n"))
	b.WriteString("Waiting for authorization...")
	return b.String()
}

// PollForToken polls the token endpoint until the user completes
// authorization. It blocks until success, cancellation, or timeout.
func PollForToken(ctx context.Context, clientID string, da *DeviceAuthResponse) (*Token, error) {
	config := &oauth2.Config{
		ClientID: clientID,
		Scopes:   []string{GeminiScope},
		Endpoint: googleEndpoint,
	}

	oauthDA := &oauth2.DeviceAuthResponse{
		DeviceCode:              da.DeviceCode,
		UserCode:                da.UserCode,
		VerificationURI:         da.VerificationURI,
		VerificationURIComplete: da.VerificationURIComplete,
		Expiry:                  da.Expiry,
		Interval:                da.Interval,
	}

	token, err := config.DeviceAccessToken(ctx, oauthDA)
	if err != nil {
		return nil, fmt.Errorf("device access token: %w", err)
	}

	return tokenFromOAuth2(token), nil
}

// RefreshToken uses a refresh token to obtain a new access token.
func RefreshToken(ctx context.Context, clientID, refreshToken string) (*Token, error) {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("refresh_token", refreshToken)
	v.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(v.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh returned HTTP %d", resp.StatusCode)
	}

	var raw struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}

	token := &Token{
		AccessToken:  raw.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    raw.TokenType,
		Expiry:       time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second),
	}

	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}

	return token, nil
}

// TokenSource creates an oauth2.TokenSource that automatically refreshes
// the token when it expires, using the provided HTTP client.
// RandomState generates a cryptographically random state string
// for CSRF protection in the OAuth flow.
func RandomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// FindAvailablePort finds a free TCP port on localhost.
func FindAvailablePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// LocalServerResult holds the result of a LocalServerFlow.
type LocalServerResult struct {
	Token *Token
	State string
	err   error
}

// RunLocalServerFlow starts a temporary HTTP server on localhost,
// constructs the Google OAuth authorization URL, and blocks until
// the user completes authorization in the browser or the context
// is cancelled.
//
// It returns the obtained Token and the authorization URL that the
// caller should open in the browser. The function opensBrowser is
// called with the authorization URL when the server is ready.
// If openBrowser is nil, the URL is printed to stdout.
//
// On success, the returned token includes a refresh_token (because
// access_type=offline is used) which can be used for future
// token refreshes without user interaction.
func RunLocalServerFlow(ctx context.Context, clientID string, openBrowser func(url string)) (*Token, error) {
	port, err := FindAvailablePort()
	if err != nil {
		return nil, err
	}

	state, err := RandomState()
	if err != nil {
		return nil, err
	}

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	oauthConfig := &oauth2.Config{
		ClientID:    clientID,
		RedirectURL: redirectURI,
		Scopes:      []string{GeminiScope},
		Endpoint:    googleEndpoint,
	}

	authURL := oauthConfig.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)

	resultCh := make(chan LocalServerResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		gotState := r.URL.Query().Get("state")
		errorParam := r.URL.Query().Get("error")

		if errorParam != "" {
			resultCh <- LocalServerResult{err: fmt.Errorf("user denied: %s", errorParam)}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(fmt.Sprintf(successPage, "Error", "Authorization denied.", "You can close this tab and return to the terminal.")))
			return
		}

		if gotState != state {
			resultCh <- LocalServerResult{err: fmt.Errorf("state mismatch: got %s, expected %s", gotState, state)}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(fmt.Sprintf(successPage, "Error", "Security mismatch. Please try again.", "Close this tab and return to the terminal.")))
			return
		}

		if code == "" {
			resultCh <- LocalServerResult{err: fmt.Errorf("no authorization code received")}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(fmt.Sprintf(successPage, "Error", "No authorization code received.", "Close this tab and return to the terminal.")))
			return
		}

		token, err := oauthConfig.Exchange(ctx, code)
		if err != nil {
			resultCh <- LocalServerResult{err: fmt.Errorf("exchange code: %w", err)}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(fmt.Sprintf(successPage, "Error", "Failed to exchange authorization code.", "Close this tab and return to the terminal.")))
			return
		}

		resultCh <- LocalServerResult{Token: tokenFromOAuth2(token)}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(fmt.Sprintf(successPage, "Success!", "You are now authenticated with Google Gemini API.", "You can close this tab and return to the terminal.")))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
		// Minimal timeouts to avoid hanging
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  5 * time.Second,
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Printf("Local OAuth server error: %v", serveErr)
		}
	}()

	shutdown := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}

	defer shutdown()

	if openBrowser != nil {
		openBrowser(authURL)
	} else {
		fmt.Printf("\n  Open this URL in your browser:\n  %s\n\n", authURL)
		fmt.Println("  Waiting for authorization...")
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			return nil, result.err
		}
		return result.Token, nil

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authorization timed out after 5 minutes")
	}
}

// successPage is a minimal HTML page shown in the browser after
// authorization completes (success or failure).
var successPage = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>%s</title>
<style>
  body { font-family: -apple-system, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f5f5f5; }
  .card { background: white; padding: 2em; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); text-align: center; max-width: 400px; }
  h1 { color: #1a73e8; margin: 0 0 0.5em; }
  p { color: #555; margin: 0.5em 0; line-height: 1.5; }
  .success { font-size: 3em; margin-bottom: 0.3em; }
</style></head>
<body><div class="card">
  <div class="success">&#10003;</div>
  <h1>%s</h1>
  <p>%s</p>
  <p style="font-size:0.85em;color:#999;">%s</p>
</div></body>
</html>`

// TokenSource creates an oauth2.TokenSource that automatically refreshes
// the token when it expires, using the provided HTTP client.
func TokenSource(ctx context.Context, clientID string, token *Token, httpClient *http.Client) oauth2.TokenSource {
	config := &oauth2.Config{
		ClientID: clientID,
		Scopes:   []string{GeminiScope},
		Endpoint: googleEndpoint,
	}

	if httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}

	return config.TokenSource(ctx, token.toOAuth2())
}
