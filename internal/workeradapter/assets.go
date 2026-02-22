package workeradapter

import (
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/worker"
)

// Compile-time interface check.
var _ worker.AssetsFetcher = (*StaticAssetsFetcher)(nil)

// StaticAssetsFetcher implements worker.AssetsFetcher by replicating the static
// file serving pipeline (redirects, headers, rewrites, SPA fallback, 404).
type StaticAssetsFetcher struct {
	Store     *storage.Manager
	Cache     *storage.SiteRulesCache
	SiteID    string
	DeployKey string
	SPAMode   bool
	Domain    string
}

// Fetch processes a WorkerRequest through the static pipeline and returns
// the appropriate response, just as the main server would.
func (f *StaticAssetsFetcher) Fetch(req *worker.WorkerRequest) (*worker.WorkerResponse, error) {
	deployPath := f.Store.GetDeploymentPath(f.SiteID, f.DeployKey)

	// Load or cache rules.
	rules := f.loadRules(deployPath)

	// Parse request path.
	u, err := url.Parse(req.URL)
	if err != nil {
		return &worker.WorkerResponse{StatusCode: 400, Headers: map[string]string{}, Body: []byte("Bad Request")}, nil
	}
	reqPath := u.Path
	if reqPath == "" {
		reqPath = "/"
	}

	// Collect matching custom headers.
	respHeaders := make(map[string]string)
	if rules != nil {
		for k, v := range storage.MatchHeaders(rules.Headers, reqPath) {
			respHeaders[k] = v
		}
	}

	// Check redirect rules (301/302 only).
	if rules != nil {
		redirectRules := filterRedirects(rules.Redirects)
		if rule, target, ok := storage.MatchRedirect(redirectRules, reqPath); ok {
			respHeaders["location"] = target
			return &worker.WorkerResponse{
				StatusCode: rule.StatusCode,
				Headers:    respHeaders,
				Body:       nil,
			}, nil
		}
	}

	// Try static file.
	if filePath, found := f.Store.ResolveFile(deployPath, reqPath); found {
		body, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", filePath, err)
		}
		ct := guessContentType(filePath)
		if ct != "" {
			respHeaders["content-type"] = ct
		}
		return &worker.WorkerResponse{
			StatusCode: 200,
			Headers:    respHeaders,
			Body:       body,
		}, nil
	}

	// Check rewrite rules (200).
	if rules != nil {
		rewriteRules := filterRewrites(rules.Redirects)
		if _, target, ok := storage.MatchRedirect(rewriteRules, reqPath); ok {
			if filePath, found := f.Store.ResolveFile(deployPath, target); found {
				body, err := os.ReadFile(filePath)
				if err != nil {
					return nil, fmt.Errorf("reading file %s: %w", filePath, err)
				}
				ct := guessContentType(filePath)
				if ct != "" {
					respHeaders["content-type"] = ct
				}
				return &worker.WorkerResponse{
					StatusCode: 200,
					Headers:    respHeaders,
					Body:       body,
				}, nil
			}
		}
	}

	// SPA fallback: serve /index.html if SPA mode is enabled.
	if f.SPAMode {
		if filePath, found := f.Store.ResolveFile(deployPath, "/index.html"); found {
			body, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("reading index.html: %w", err)
			}
			respHeaders["content-type"] = "text/html; charset=utf-8"
			return &worker.WorkerResponse{
				StatusCode: 200,
				Headers:    respHeaders,
				Body:       body,
			}, nil
		}
	}

	// Custom 404.html.
	notFoundPath := filepath.Join(deployPath, "404.html")
	if info, err := os.Stat(notFoundPath); err == nil && !info.IsDir() {
		body, err := os.ReadFile(notFoundPath)
		if err == nil {
			respHeaders["content-type"] = "text/html; charset=utf-8"
			return &worker.WorkerResponse{
				StatusCode: 404,
				Headers:    respHeaders,
				Body:       body,
			}, nil
		}
	}

	// Default 404.
	return &worker.WorkerResponse{
		StatusCode: 404,
		Headers:    respHeaders,
		Body:       []byte("Not Found"),
	}, nil
}

// loadRules loads redirect and header rules, using the cache when available.
func (f *StaticAssetsFetcher) loadRules(deployPath string) *storage.SiteRules {
	if cached, ok := f.Cache.Get(f.SiteID, f.DeployKey); ok {
		return cached
	}

	redirects, _ := storage.ParseRedirects(filepath.Join(deployPath, "_redirects"))
	headers, _ := storage.ParseHeaders(filepath.Join(deployPath, "_headers"))
	rules := &storage.SiteRules{
		Redirects: redirects,
		Headers:   headers,
	}
	f.Cache.Set(f.SiteID, f.DeployKey, rules)
	return rules
}

// filterRedirects returns only rules with status 301 or 302.
func filterRedirects(rules []storage.RedirectRule) []storage.RedirectRule {
	var out []storage.RedirectRule
	for _, r := range rules {
		if r.StatusCode == 301 || r.StatusCode == 302 {
			out = append(out, r)
		}
	}
	return out
}

// filterRewrites returns only rules with status 200.
func filterRewrites(rules []storage.RedirectRule) []storage.RedirectRule {
	var out []storage.RedirectRule
	for _, r := range rules {
		if r.StatusCode == 200 {
			out = append(out, r)
		}
	}
	return out
}

// guessContentType guesses the MIME type from the file extension.
func guessContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return "application/octet-stream"
	}
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
}
