package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/cryguy/hostedat/internal/auth"
)

type APIKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	CreatedAt string `json:"created_at"`
}

// BrowserLogin performs the full CLI login flow using OAuth 2.0 PKCE:
// 1. Generate PKCE code verifier + challenge
// 2. Start a local HTTP server on a random port
// 3. Open the browser to the server's CLI login page with the code challenge
// 4. Wait for the callback with an authorization code
// 5. Exchange the code + verifier for a JWT
// 6. Use the JWT to create an API key
// 7. Return the API key
func BrowserLogin(serverURL, cliVersion string) (apiKey string, err error) {
	// Generate PKCE verifier and challenge
	codeVerifier, err := auth.GenerateCodeVerifier()
	if err != nil {
		return "", fmt.Errorf("failed to generate code verifier: %w", err)
	}
	codeChallenge := auth.ComputeCodeChallenge(codeVerifier)

	// Generate random state for CSRF protection
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	// Start local server on random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("failed to start local server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	var code string
	var callbackErr error
	done := make(chan struct{})
	var closeOnce sync.Once

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			callbackErr = fmt.Errorf("state mismatch (possible CSRF)")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			closeOnce.Do(func() { close(done) })
			return
		}
		code = r.URL.Query().Get("code")
		if code == "" {
			callbackErr = fmt.Errorf("no authorization code in callback")
			http.Error(w, "Missing code", http.StatusBadRequest)
			closeOnce.Do(func() { close(done) })
			return
		}

		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><head><title>hostedat</title><style>
body{font-family:system-ui;background:#0a0a0a;color:#e5e5e5;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#171717;border:1px solid #262626;border-radius:12px;padding:2rem;text-align:center}
</style></head><body><div class="card"><h2>Authenticated!</h2><p>You can close this tab.</p></div></body></html>`)

		closeOnce.Do(func() { close(done) })
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// non-fatal: server stops when Shutdown is called
			_ = err
		}
	}()

	// Open browser
	loginURL := fmt.Sprintf("%s/api/v1/auth/cli?port=%d&state=%s&code_challenge=%s&code_challenge_method=S256",
		serverURL, port, state, codeChallenge)
	fmt.Printf("Opening browser to %s\n", loginURL)
	fmt.Println("If the browser doesn't open, visit the URL manually.")
	openBrowser(loginURL)

	// Wait for callback with timeout
	select {
	case <-done:
	case <-time.After(2 * time.Minute):
		_ = srv.Shutdown(context.Background())
		return "", fmt.Errorf("login timed out (2 minutes)")
	}

	_ = srv.Shutdown(context.Background())

	if callbackErr != nil {
		return "", callbackErr
	}

	// Exchange authorization code + verifier for JWT
	tokenReqBody, _ := json.Marshal(map[string]string{
		"code":          code,
		"code_verifier": codeVerifier,
	})
	httpClient := &http.Client{Timeout: 30 * time.Second}
	tokenResp, err := httpClient.Post(serverURL+"/api/v1/auth/token", "application/json", bytes.NewReader(tokenReqBody))
	if err != nil {
		return "", fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer func() { _ = tokenResp.Body.Close() }()

	var tokenResult struct {
		Token string `json:"token"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}
	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed: %s", tokenResult.Error)
	}

	// Use JWT to create an API key
	c := New(serverURL, tokenResult.Token)
	c.Version = cliVersion
	var keyResp APIKeyResponse
	if err := c.post("/api/v1/keys", map[string]string{"name": "hostedat-cli"}, &keyResp); err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	return keyResp.Key, nil
}

func (c *Client) CreateAPIKey(name string) (*APIKeyResponse, error) {
	var resp APIKeyResponse
	err := c.post("/api/v1/keys", map[string]string{"name": name}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
