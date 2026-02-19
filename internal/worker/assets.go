package worker

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cryguy/hostedat/internal/storage"
	v8 "github.com/tommie/v8go"
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
// Fetch is local file I/O.
func buildAssetsBinding(iso *v8.Isolate, ctx *v8.Context, fetcher AssetsFetcher) (*v8.Value, error) {
	assets, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating assets object: %w", err)
	}

	assets.Set("fetch", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			return throwError(iso, "ASSETS.fetch requires a request argument")
		}

		// Extract request data via JS.
		ctx.Global().Set("__tmp_assets_req", args[0])
		result, err := ctx.RunScript(`(function() {
			var r = globalThis.__tmp_assets_req;
			delete globalThis.__tmp_assets_req;
			var headers = {};
			if (r.headers && r.headers._map) {
				var m = r.headers._map;
				for (var k in m) { if (m.hasOwnProperty(k)) headers[k] = String(m[k]); }
			}
			var body = r._body != null ? String(r._body) : null;
			return JSON.stringify({url: r.url || '', method: r.method || 'GET', headers: headers, body: body});
		})()`, "assets_extract_req.js")
		if err != nil {
			return throwError(iso, fmt.Sprintf("ASSETS.fetch: extracting request: %s", err.Error()))
		}

		var reqData struct {
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
			Body    *string           `json:"body"`
		}
		if err := json.Unmarshal([]byte(result.String()), &reqData); err != nil {
			return throwError(iso, fmt.Sprintf("ASSETS.fetch: parsing request: %s", err.Error()))
		}

		goReq := &WorkerRequest{
			URL:     reqData.URL,
			Method:  reqData.Method,
			Headers: reqData.Headers,
		}
		if reqData.Body != nil {
			goReq.Body = []byte(*reqData.Body)
		}

		resp, err := fetcher.Fetch(goReq)
		if err != nil {
			return throwError(iso, err.Error())
		}

		// Build Response via JS constructor.
		headersJSON, _ := json.Marshal(resp.Headers)
		if resp.Body != nil {
			bodyVal, _ := v8.NewValue(iso, string(resp.Body))
			ctx.Global().Set("__tmp_assets_body", bodyVal)
		} else {
			ctx.Global().Set("__tmp_assets_body", v8.Null(iso))
		}
		statusVal, _ := v8.NewValue(iso, int32(resp.StatusCode))
		ctx.Global().Set("__tmp_assets_status", statusVal)
		hdrsVal, _ := v8.NewValue(iso, string(headersJSON))
		ctx.Global().Set("__tmp_assets_headers", hdrsVal)

		jsResp, err := ctx.RunScript(`(function() {
			var body = globalThis.__tmp_assets_body;
			var status = globalThis.__tmp_assets_status;
			var hdrs = JSON.parse(globalThis.__tmp_assets_headers);
			delete globalThis.__tmp_assets_body;
			delete globalThis.__tmp_assets_status;
			delete globalThis.__tmp_assets_headers;
			return new Response(body, {status: status, headers: hdrs});
		})()`, "assets_build_resp.js")
		if err != nil {
			return throwError(iso, fmt.Sprintf("ASSETS.fetch: creating Response: %s", err.Error()))
		}

		return jsResp
	}).GetFunction(ctx))

	return assets.Value, nil
}

// buildEnvObject creates the full env JS object passed to the worker's
// fetch handler as the second argument.
func buildEnvObject(iso *v8.Isolate, ctx *v8.Context, env *Env, db *gorm.DB, minioClient interface{}, presignClient interface{}, publicS3URL string) (*v8.Value, error) {
	envObj, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating env object: %w", err)
	}

	// Plain environment variables.
	if env.Vars != nil {
		for k, v := range env.Vars {
			val, _ := v8.NewValue(iso, v)
			envObj.Set(k, val)
		}
	}

	// Secrets (same shape, just from a different source).
	if env.Secrets != nil {
		for k, v := range env.Secrets {
			val, _ := v8.NewValue(iso, v)
			envObj.Set(k, val)
		}
	}

	// KV namespace bindings.
	if env.KVBindings != nil && db != nil {
		for name, nsID := range env.KVBindings {
			bridge := &KVBridge{DB: db, NamespaceID: nsID}
			kvVal, err := buildKVBinding(iso, ctx, bridge)
			if err != nil {
				return nil, fmt.Errorf("building KV binding %q: %w", name, err)
			}
			envObj.Set(name, kvVal)
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
				storVal, err := buildStorageBinding(iso, ctx, bridge)
				if err != nil {
					return nil, fmt.Errorf("building storage binding %q: %w", name, err)
				}
				envObj.Set(name, storVal)
			}
		}
	}

	// ASSETS binding.
	if env.Assets != nil {
		assetsVal, err := buildAssetsBinding(iso, ctx, env.Assets)
		if err != nil {
			return nil, fmt.Errorf("building assets binding: %w", err)
		}
		envObj.Set("ASSETS", assetsVal)
	}

	return envObj.Value, nil
}

// buildExecContext creates the ctx JS object (third argument to fetch handler).
// waitUntil(promise) collects promises into globalThis.__waitUntilPromises
// which are drained after the response is returned.
func buildExecContext(iso *v8.Isolate, ctx *v8.Context) (*v8.Value, error) {
	execCtx, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating exec context: %w", err)
	}

	// Initialize the promises array for this request.
	if _, err := ctx.RunScript("globalThis.__waitUntilPromises = [];", "waituntil_init.js"); err != nil {
		return nil, fmt.Errorf("initializing waitUntil array: %w", err)
	}

	waitUntilFT := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) > 0 {
			ctx.Global().Set("__tmp_wu_promise", args[0])
			ctx.RunScript("globalThis.__waitUntilPromises.push(Promise.resolve(globalThis.__tmp_wu_promise)); delete globalThis.__tmp_wu_promise;", "waituntil_push.js")
		}
		return v8.Undefined(iso)
	})
	execCtx.Set("waitUntil", waitUntilFT.GetFunction(ctx))

	passThroughFT := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		return v8.Undefined(iso)
	})
	execCtx.Set("passThroughOnException", passThroughFT.GetFunction(ctx))

	return execCtx.Value, nil
}

// drainWaitUntil awaits all promises collected by ctx.waitUntil().
// It runs Promise.allSettled on the array so that rejections don't break
// the response. Must be called on the isolate's goroutine.
func drainWaitUntil(ctx *v8.Context, deadline time.Time) {
	drainScript := `(async function() {
		var promises = globalThis.__waitUntilPromises || [];
		globalThis.__waitUntilPromises = [];
		if (promises.length > 0) {
			await Promise.allSettled(promises);
		}
	})()`
	wuVal, err := ctx.RunScript(drainScript, "waituntil_drain.js")
	if err != nil {
		return
	}
	if wuVal != nil && wuVal.IsPromise() {
		awaitValue(ctx, wuVal, deadline)
	}
}
