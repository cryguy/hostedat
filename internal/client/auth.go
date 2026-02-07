package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

type APIKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	CreatedAt string `json:"created_at"`
}

// BrowserLogin performs the full CLI login flow:
// 1. Start a local HTTP server on a random port
// 2. Open the browser to the server's CLI login page
// 3. Wait for the callback with a JWT
// 4. Use the JWT to create an API key
// 5. Return the API key
func BrowserLogin(serverURL string) (apiKey string, err error) {
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

	var token string
	var callbackErr error
	done := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			callbackErr = fmt.Errorf("state mismatch (possible CSRF)")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			close(done)
			return
		}
		token = r.URL.Query().Get("token")
		if token == "" {
			callbackErr = fmt.Errorf("no token in callback")
			http.Error(w, "Missing token", http.StatusBadRequest)
			close(done)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>hostedat</title><style>
body{font-family:system-ui;background:#0a0a0a;color:#e5e5e5;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#171717;border:1px solid #262626;border-radius:12px;padding:2rem;text-align:center}
</style></head><body><div class="card"><h2>Authenticated!</h2><p>You can close this tab.</p></div></body></html>`)

		close(done)
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	// Open browser
	loginURL := fmt.Sprintf("%s/api/v1/auth/cli?port=%d&state=%s", serverURL, port, state)
	fmt.Printf("Opening browser to %s\n", loginURL)
	fmt.Println("If the browser doesn't open, visit the URL manually.")
	openBrowser(loginURL)

	// Wait for callback with timeout
	select {
	case <-done:
	case <-time.After(2 * time.Minute):
		srv.Shutdown(context.Background())
		return "", fmt.Errorf("login timed out (2 minutes)")
	}

	srv.Shutdown(context.Background())

	if callbackErr != nil {
		return "", callbackErr
	}

	// Use JWT to create an API key
	c := New(serverURL, token)
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
	cmd.Start()
}
