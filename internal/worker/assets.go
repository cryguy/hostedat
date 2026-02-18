package worker

import (
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/cryguy/hostedat/internal/storage"
	"github.com/fastschema/qjs"
	minio "github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// StaticAssetsFetcher implements AssetsFetcher by replicating the static
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
func (f *StaticAssetsFetcher) Fetch(req *WorkerRequest) (*WorkerResponse, error) {
	deployPath := f.Store.GetDeploymentPath(f.SiteID, f.DeployKey)

	// Load or cache rules.
	rules := f.loadRules(deployPath)

	// Parse request path.
	u, err := url.Parse(req.URL)
	if err != nil {
		return &WorkerResponse{StatusCode: 400, Headers: map[string]string{}, Body: []byte("Bad Request")}, nil
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
			return &WorkerResponse{
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
		ct := contentType(filePath)
		if ct != "" {
			respHeaders["content-type"] = ct
		}
		return &WorkerResponse{
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
				ct := contentType(filePath)
				if ct != "" {
					respHeaders["content-type"] = ct
				}
				return &WorkerResponse{
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
			return &WorkerResponse{
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
			return &WorkerResponse{
				StatusCode: 404,
				Headers:    respHeaders,
				Body:       body,
			}, nil
		}
	}

	// Default 404.
	return &WorkerResponse{
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

// contentType guesses the MIME type from the file extension.
func contentType(filePath string) string {
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

// buildAssetsBinding creates a JS object with a fetch(request) method
// that delegates to the given AssetsFetcher. This is synchronous because
// Fetch is local file I/O, and calling the Response constructor from a
// goroutine causes WASM thread-safety crashes.
func buildAssetsBinding(ctx *qjs.Context, fetcher AssetsFetcher) *qjs.Value {
	assets := ctx.NewObject()

	fetchFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()

		if len(args) == 0 {
			return nil, fmt.Errorf("ASSETS.fetch requires a request argument")
		}

		// Convert JS Request to Go WorkerRequest.
		jsReq := args[0]
		urlVal := jsReq.GetPropertyStr("url")
		methodVal := jsReq.GetPropertyStr("method")

		goReq := &WorkerRequest{
			URL:     urlVal.String(),
			Method:  methodVal.String(),
			Headers: make(map[string]string),
		}
		urlVal.Free()
		methodVal.Free()

		headersVal := jsReq.GetPropertyStr("headers")
		if headersVal.IsObject() {
			mapVal := headersVal.GetPropertyStr("_map")
			if mapVal.IsObject() {
				names, err := mapVal.GetOwnPropertyNames()
				if err == nil {
					for _, name := range names {
						v := mapVal.GetPropertyStr(name)
						goReq.Headers[name] = v.String()
						v.Free()
					}
				}
			}
			mapVal.Free()
		}
		headersVal.Free()

		bodyVal := jsReq.GetPropertyStr("_body")
		if !bodyVal.IsNull() && !bodyVal.IsUndefined() {
			goReq.Body = []byte(bodyVal.String())
		}
		bodyVal.Free()

		resp, err := fetcher.Fetch(goReq)
		if err != nil {
			return nil, err
		}

		// Build JS Response from Go response on the JS thread.
		headersObj := c.NewObject()
		for k, v := range resp.Headers {
			headersObj.SetPropertyStr(k, c.NewString(v))
		}

		initObj := c.NewObject()
		initObj.SetPropertyStr("status", c.NewInt32(int32(resp.StatusCode)))
		initObj.SetPropertyStr("headers", headersObj)

		var bodyJS *qjs.Value
		if resp.Body != nil {
			bodyJS = c.NewString(string(resp.Body))
		} else {
			bodyJS = c.NewNull()
		}

		responseCtor := c.Global().GetPropertyStr("Response")
		jsResp := responseCtor.CallConstructor(bodyJS, initObj)
		responseCtor.Free()

		if jsResp.IsError() {
			defer jsResp.Free()
			return nil, fmt.Errorf("ASSETS.fetch: failed to create Response: %s", jsResp.String())
		}

		return jsResp, nil
	}, false)
	assets.SetPropertyStr("fetch", fetchFn)

	return assets
}

// buildEnvObject creates the full env JS object passed to the worker's
// fetch handler as the second argument.
func buildEnvObject(ctx *qjs.Context, env *Env, db *gorm.DB, minioClient interface{}, presignClient interface{}, publicS3URL string) *qjs.Value {
	envObj := ctx.NewObject()

	// Plain environment variables.
	if env.Vars != nil {
		for k, v := range env.Vars {
			envObj.SetPropertyStr(k, ctx.NewString(v))
		}
	}

	// Secrets (same shape, just from a different source).
	if env.Secrets != nil {
		for k, v := range env.Secrets {
			envObj.SetPropertyStr(k, ctx.NewString(v))
		}
	}

	// KV namespace bindings.
	if env.KVBindings != nil && db != nil {
		for name, nsID := range env.KVBindings {
			bridge := &KVBridge{DB: db, NamespaceID: nsID}
			envObj.SetPropertyStr(name, buildKVBinding(ctx, bridge))
		}
	}

	// Storage bucket bindings (R2-compatible).
	if env.StorageBindings != nil && minioClient != nil {
		mc, ok := minioClient.(*minio.Client)
		if ok {
			var pc *minio.Client
			if presignClient != nil {
				pc, _ = presignClient.(*minio.Client)
			}
			for name, bucketName := range env.StorageBindings {
				bridge := &StorageBridge{
					Client:        mc,
					PresignClient: pc,
					BucketName:    bucketName,
					PublicS3URL:   publicS3URL,
				}
				envObj.SetPropertyStr(name, buildStorageBinding(ctx, bridge))
			}
		}
	}

	// ASSETS binding.
	if env.Assets != nil {
		envObj.SetPropertyStr("ASSETS", buildAssetsBinding(ctx, env.Assets))
	}

	return envObj
}

// buildExecContext creates the ctx JS object (third argument to fetch handler).
// Currently provides a no-op waitUntil.
func buildExecContext(ctx *qjs.Context) *qjs.Value {
	execCtx := ctx.NewObject()

	waitUntilFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		// No-op: we don't support background tasks beyond the request lifetime.
		return this.Context().NewUndefined(), nil
	}, false)
	execCtx.SetPropertyStr("waitUntil", waitUntilFn)

	passThroughOnExceptionFn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
		return this.Context().NewUndefined(), nil
	}, false)
	execCtx.SetPropertyStr("passThroughOnException", passThroughOnExceptionFn)

	return execCtx
}
