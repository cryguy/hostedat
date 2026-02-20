package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cryguy/hostedat/internal/config"
	v8 "github.com/tommie/v8go"
)

// fetchSSRFEnabled controls whether the SSRF-safe dialer is used for fetch.
// Tests set this to false so httptest servers on 127.0.0.1 are reachable.
var fetchSSRFEnabled = true

// forbiddenFetchHeaders is the blocklist of headers that workers cannot set.
// These headers are controlled by the HTTP transport or could be used for
// header smuggling attacks.
var forbiddenFetchHeaders = map[string]bool{
	"host":                 true,
	"transfer-encoding":    true,
	"connection":           true,
	"keep-alive":           true,
	"upgrade":              true,
	"proxy-authorization":  true,
	"proxy-connection":     true,
	"te":                   true,
	"trailer":              true,
	"x-forwarded-for":      true,
	"x-forwarded-host":     true,
	"x-forwarded-proto":    true,
	"x-real-ip":            true,
}

// fetchTransport is the http.RoundTripper used by fetch. Tests can override it.
var fetchTransport http.RoundTripper = &http.Transport{
	DialContext: ssrfSafeDialContext,
}

// fetchResult carries the outcome of an async HTTP request.
type fetchResult struct {
	resp *http.Response
	err  error
}

// setupFetch registers the global fetch() function as a Go-backed function
// backed by a PromiseResolver. It enforces per-request rate limits and blocks
// requests to private/loopback addresses.
//
// The HTTP request runs in a goroutine so that an AbortSignal can cancel it
// while the Go callback is waiting for the result. V8 is not accessed from
// the goroutine — only the context.CancelFunc is called, which is safe.
func setupFetch(iso *v8.Isolate, ctx *v8.Context, cfg config.WorkerConfig) error {
	// __fetchAbort(reqID, fetchID) — called from JS abort listener to cancel
	// an in-flight HTTP request. Safe to call from the JS thread at any time.
	abortFT := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return nil
		}
		reqID := uint64(args[0].Integer())
		fetchID := args[1].String()
		callFetchCancel(reqID, fetchID)
		return nil
	})
	_ = ctx.Global().Set("__fetchAbort", abortFT.GetFunction(ctx))

	fetchFT := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()

		// Rate limit check.
		reqID := getReqIDFromJS(ctx)
		state := getRequestState(reqID)
		if state != nil && state.fetchCount >= state.maxFetches {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("exceeded maximum fetch requests (%d)", state.maxFetches))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		if state != nil {
			state.fetchCount++
		}

		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "fetch requires at least 1 argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		// Set arguments as temp globals and extract via JS.
		_ = ctx.Global().Set("__tmp_fetch_arg0", args[0])
		if len(args) > 1 {
			_ = ctx.Global().Set("__tmp_fetch_arg1", args[1])
		}

		extractResult, err := ctx.RunScript(`(function() {
			var a0 = globalThis.__tmp_fetch_arg0;
			var a1 = globalThis.__tmp_fetch_arg1;
			delete globalThis.__tmp_fetch_arg0;
			delete globalThis.__tmp_fetch_arg1;
			var url = '', method = 'GET', headers = {}, body = null, bodyIsBase64 = false;
			var redirect = 'follow', signalAborted = false, signal = null;
			function extractBody(b) {
				if (b == null) return;
				if (b instanceof ArrayBuffer) {
					body = __bufferSourceToB64(b);
					bodyIsBase64 = true;
				} else if (ArrayBuffer.isView(b)) {
					body = __bufferSourceToB64(b);
					bodyIsBase64 = true;
				} else if (b instanceof ReadableStream) {
					var chunks = [];
					for (var i = 0; i < b._queue.length; i++) {
						var c = b._queue[i];
						if (typeof c === 'string') {
							var enc = new TextEncoder();
							var bytes = enc.encode(c);
							for (var j = 0; j < bytes.length; j++) chunks.push(bytes[j]);
						} else if (c instanceof Uint8Array || ArrayBuffer.isView(c)) {
							var arr = new Uint8Array(c.buffer || c, c.byteOffset || 0, c.byteLength || c.length);
							for (var j2 = 0; j2 < arr.length; j2++) chunks.push(arr[j2]);
						} else if (c instanceof ArrayBuffer) {
							var arr2 = new Uint8Array(c);
							for (var j3 = 0; j3 < arr2.length; j3++) chunks.push(arr2[j3]);
						} else {
							var s = String(c);
							for (var j4 = 0; j4 < s.length; j4++) chunks.push(s.charCodeAt(j4) & 0xFF);
						}
					}
					b._queue = [];
					if (chunks.length > 0) {
						body = __bufferSourceToB64(new Uint8Array(chunks));
						bodyIsBase64 = true;
					}
				} else {
					body = String(b);
				}
			}
			if (typeof a0 === 'string') {
				url = a0;
			} else if (a0 && typeof a0 === 'object') {
				url = a0.url || '';
				method = a0.method || 'GET';
				if (a0.headers && a0.headers._map) {
					var m = a0.headers._map;
					for (var k in m) { if (m.hasOwnProperty(k)) headers[k] = String(m[k]); }
				}
				if (a0._body != null) extractBody(a0._body);
				if (a0.redirect !== undefined) redirect = String(a0.redirect);
				if (a0.signal) { signal = a0.signal; if (a0.signal.aborted) signalAborted = true; }
			}
			if (a1 && typeof a1 === 'object') {
				if (a1.method !== undefined) method = String(a1.method).toUpperCase();
				if (a1.headers) {
					var src = a1.headers._map || a1.headers;
					if (typeof src === 'object') {
						for (var k in src) { if (src.hasOwnProperty(k)) headers[k.toLowerCase()] = String(src[k]); }
					}
				}
				if (a1.body != null) extractBody(a1.body);
				if (a1.redirect !== undefined) redirect = String(a1.redirect);
				if (a1.signal) { signal = a1.signal; if (a1.signal.aborted) signalAborted = true; }
			}
			if (!method) method = 'GET';
			globalThis.__tmp_fetch_signal = signal;
			return JSON.stringify({url: url, method: method, headers: headers, body: body, bodyIsBase64: bodyIsBase64, redirect: redirect, signalAborted: signalAborted});
		})()`, "fetch_extract.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: extracting arguments: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		var fetchArgs struct {
			URL           string            `json:"url"`
			Method        string            `json:"method"`
			Headers       map[string]string `json:"headers"`
			Body          *string           `json:"body"`
			BodyIsBase64  bool              `json:"bodyIsBase64"`
			Redirect      string            `json:"redirect"`
			SignalAborted bool              `json:"signalAborted"`
		}
		if err := json.Unmarshal([]byte(extractResult.String()), &fetchArgs); err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: parsing arguments: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		// If the signal was already aborted, reject immediately (fast path).
		if fetchArgs.SignalAborted {
			ctx.Global().Delete("__tmp_fetch_signal")
			abortErr, _ := ctx.RunScript(
				`new DOMException("The operation was aborted.", "AbortError")`,
				"fetch_abort.js")
			resolver.Reject(abortErr)
			return resolver.GetPromise().Value
		}

		// Block private hostnames before connecting.
		if fetchSSRFEnabled && isPrivateHostname(fetchArgs.URL) {
			ctx.Global().Delete("__tmp_fetch_signal")
			errVal, _ := v8.NewValue(iso, "fetch to private IP addresses is not allowed")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		timeout := time.Duration(cfg.FetchTimeoutSec) * time.Second
		maxBytes := int64(cfg.MaxResponseBytes)

		var bodyReader io.Reader
		if fetchArgs.Body != nil && *fetchArgs.Body != "" {
			if fetchArgs.BodyIsBase64 {
				decoded, decErr := base64.StdEncoding.DecodeString(*fetchArgs.Body)
				if decErr != nil {
					errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: decoding binary body: %s", decErr.Error()))
					resolver.Reject(errVal)
					return resolver.GetPromise().Value
				}
				bodyReader = strings.NewReader(string(decoded))
			} else {
				bodyReader = strings.NewReader(*fetchArgs.Body)
			}
		}

		// Create a cancellable context for this fetch. The cancel function is
		// stored in requestState so that an AbortSignal listener can trigger it
		// from the JS thread via __fetchAbort(reqID, fetchID).
		reqID = getReqIDFromJS(ctx)
		fetchCtx, fetchCancel := context.WithCancel(context.Background())
		fetchID := registerFetchCancel(reqID, fetchCancel)

		// Wire the abort listener: if a signal was provided and is not yet
		// aborted, register a JS abort event listener that calls __fetchAbort.
		// We stored the signal in __tmp_fetch_signal during extraction.
		if fetchID != "" {
			fetchIDVal, _ := v8.NewValue(iso, fetchID)
			_ = ctx.Global().Set("__tmp_fetch_id", fetchIDVal)
			_, _ = ctx.RunScript(`(function() {
				var sig = globalThis.__tmp_fetch_signal;
				var fid = globalThis.__tmp_fetch_id;
				delete globalThis.__tmp_fetch_signal;
				delete globalThis.__tmp_fetch_id;
				if (sig && !sig.aborted) {
					var reqID = globalThis.__requestID;
					var onAbort = function() {
						sig.removeEventListener('abort', onAbort);
						__fetchAbort(reqID, fid);
					};
					sig.addEventListener('abort', onAbort, {once: true});
					globalThis.__tmp_fetch_cleanup = function() {
						sig.removeEventListener('abort', onAbort);
					};
				}
			})()`, "fetch_wire_abort.js")
		}

		httpReq, err := http.NewRequestWithContext(fetchCtx, fetchArgs.Method, fetchArgs.URL, bodyReader)
		if err != nil {
			fetchCancel()
			removeFetchCancel(reqID, fetchID)
			_, _ = ctx.RunScript(`if (typeof globalThis.__tmp_fetch_cleanup === 'function') { globalThis.__tmp_fetch_cleanup(); delete globalThis.__tmp_fetch_cleanup; }`, "fetch_cleanup.js")
			ctx.Global().Delete("__tmp_fetch_signal")
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		for k, v := range fetchArgs.Headers {
			if forbiddenFetchHeaders[strings.ToLower(k)] {
				continue
			}
			httpReq.Header.Set(k, v)
		}

		// Build the redirect policy based on the redirect option.
		redirectMode := fetchArgs.Redirect
		if redirectMode == "" {
			redirectMode = "follow"
		}
		var checkRedirect func(req *http.Request, via []*http.Request) error
		switch redirectMode {
		case "manual":
			checkRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
		case "error":
			checkRedirect = func(req *http.Request, via []*http.Request) error {
				return fmt.Errorf("fetch failed: redirect mode is 'error'")
			}
		default: // "follow"
			checkRedirect = func(req *http.Request, via []*http.Request) error {
				if len(via) >= 20 {
					return fmt.Errorf("too many redirects")
				}
				if fetchSSRFEnabled && isPrivateHostname(req.URL.String()) {
					return fmt.Errorf("redirect to private IP address is not allowed")
				}
				return nil
			}
		}

		// Run the HTTP request in a goroutine so the Go context cancellation
		// (triggered by the JS abort listener) can interrupt it.
		// Note: V8 is NOT accessed from the goroutine — only pure Go operations.
		client := &http.Client{
			Timeout:       timeout,
			Transport:     fetchTransport,
			CheckRedirect: checkRedirect,
		}
		resultCh := make(chan fetchResult, 1)
		go func() {
			resp, err := client.Do(httpReq)
			resultCh <- fetchResult{resp: resp, err: err}
		}()
		result := <-resultCh

		// Cleanup: remove cancel from state and run JS listener cleanup.
		// Note: we check fetchCtx.Err() BEFORE calling fetchCancel() to
		// distinguish abort-signal cancellation from normal cleanup.
		abortedBySignal := fetchCtx.Err() != nil
		removeFetchCancel(reqID, fetchID)
		fetchCancel()
		_, _ = ctx.RunScript(`if (typeof globalThis.__tmp_fetch_cleanup === 'function') { globalThis.__tmp_fetch_cleanup(); delete globalThis.__tmp_fetch_cleanup; }`, "fetch_cleanup.js")

		resp, err := result.resp, result.err
		if err != nil {
			// For redirect mode "error", reject with a TypeError (check
			// before abort so redirect errors aren't misattributed).
			if redirectMode == "error" {
				typeErr, _ := ctx.RunScript(
					`new TypeError("fetch failed: redirect mode is 'error'")`,
					"fetch_redirect_err.js")
				resolver.Reject(typeErr)
				return resolver.GetPromise().Value
			}
			// If the context was cancelled by the abort signal, reject with
			// a DOMException AbortError to match the Web Fetch specification.
			if abortedBySignal {
				abortErr, _ := ctx.RunScript(
					`new DOMException("The operation was aborted.", "AbortError")`,
					"fetch_abort_inflight.js")
				resolver.Reject(abortErr)
				return resolver.GetPromise().Value
			}
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		defer func() { _ = resp.Body.Close() }()

		limitedReader := io.LimitReader(resp.Body, maxBytes+1)
		respBody, err := io.ReadAll(limitedReader)
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: reading body: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		if int64(len(respBody)) > maxBytes {
			respBody = respBody[:maxBytes]
		}

		// Build response headers as JSON.
		respHeaders := make(map[string]string)
		for k, vals := range resp.Header {
			respHeaders[strings.ToLower(k)] = strings.Join(vals, ", ")
		}
		headersJSON, _ := json.Marshal(respHeaders)

		// Determine the final URL — for "follow" mode this is the URL after redirects.
		finalURL := fetchArgs.URL
		if resp.Request != nil && resp.Request.URL != nil {
			finalURL = resp.Request.URL.String()
		}
		redirected := finalURL != fetchArgs.URL

		// Base64-encode the response body to preserve binary data integrity.
		bodyB64 := base64.StdEncoding.EncodeToString(respBody)
		bodyVal, _ := v8.NewValue(iso, bodyB64)
		_ = ctx.Global().Set("__tmp_fetch_resp_body", bodyVal)
		statusVal, _ := v8.NewValue(iso, int32(resp.StatusCode))
		_ = ctx.Global().Set("__tmp_fetch_resp_status", statusVal)
		statusTextVal, _ := v8.NewValue(iso, resp.Status)
		_ = ctx.Global().Set("__tmp_fetch_resp_statusText", statusTextVal)
		headersJSONVal, _ := v8.NewValue(iso, string(headersJSON))
		_ = ctx.Global().Set("__tmp_fetch_resp_headers", headersJSONVal)
		fetchURLVal, _ := v8.NewValue(iso, finalURL)
		_ = ctx.Global().Set("__tmp_fetch_resp_url", fetchURLVal)
		redirectedVal, _ := v8.NewValue(iso, redirected)
		_ = ctx.Global().Set("__tmp_fetch_resp_redirected", redirectedVal)

		jsResp, err := ctx.RunScript(`(function() {
			var b64Body = globalThis.__tmp_fetch_resp_body;
			var status = globalThis.__tmp_fetch_resp_status;
			var statusText = globalThis.__tmp_fetch_resp_statusText;
			var hdrs = JSON.parse(globalThis.__tmp_fetch_resp_headers);
			var url = globalThis.__tmp_fetch_resp_url;
			var redirected = globalThis.__tmp_fetch_resp_redirected;
			delete globalThis.__tmp_fetch_resp_body;
			delete globalThis.__tmp_fetch_resp_status;
			delete globalThis.__tmp_fetch_resp_statusText;
			delete globalThis.__tmp_fetch_resp_headers;
			delete globalThis.__tmp_fetch_resp_url;
			delete globalThis.__tmp_fetch_resp_redirected;
			var body = null;
			if (b64Body && b64Body.length > 0) {
				var buf = __b64ToBuffer(b64Body);
				var ct = (hdrs['content-type'] || '').toLowerCase();
				if (ct.indexOf('text/') === 0 || ct.indexOf('application/json') !== -1 ||
				    ct.indexOf('application/xml') !== -1 || ct.indexOf('application/javascript') !== -1 ||
				    ct.indexOf('application/x-www-form-urlencoded') !== -1) {
					body = new TextDecoder().decode(buf);
				} else {
					body = buf;
				}
			}
			var r = new Response(body, {status: status, statusText: statusText, headers: hdrs, url: url});
			if (redirected) {
				Object.defineProperty(r, 'redirected', {value: true, writable: false});
			}
			return r;
		})()`, "fetch_response.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: building response: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		resolver.Resolve(jsResp)
		return resolver.GetPromise().Value
	})

	_ = ctx.Global().Set("fetch", fetchFT.GetFunction(ctx))
	return nil
}

// isPrivateHostname performs a fast, non-resolving pre-check for obviously
// private hostnames and literal IP addresses. It does NOT resolve DNS  Ethe
// actual SSRF protection happens in ssrfSafeDialContext at connect time.
func isPrivateHostname(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true // block unparseable URLs
	}

	hostname := u.Hostname()
	if hostname == "" {
		return true
	}

	// Block known private hostnames.
	lower := strings.ToLower(hostname)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}

	// Block literal private IPs (no DNS resolution).
	if ip := net.ParseIP(hostname); ip != nil {
		return isPrivateIP(ip)
	}

	return false
}

// ssrfSafeDialContext is a custom DialContext that resolves DNS and validates
// the resolved IP against private ranges at actual connect time, preventing
// DNS rebinding / TOCTOU attacks.
func ssrfSafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", addr, err)
	}

	// Resolve DNS.
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed for %s: %w", host, err)
	}

	// Filter out private IPs.
	var safeIP net.IPAddr
	found := false
	for _, ip := range ips {
		if !isPrivateIP(ip.IP) {
			safeIP = ip
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("fetch to private IP addresses is not allowed")
	}

	// Connect to the validated IP directly.
	dialer := &net.Dialer{}
	return dialer.DialContext(ctx, network, net.JoinHostPort(safeIP.IP.String(), port))
}

// privateRanges is parsed once at init time to avoid repeated allocations
// on every isPrivateIP call.
var privateRanges []*net.IPNet

func init() {
	for _, cidr := range []string{
		// IPv4 private and special-use ranges
		"0.0.0.0/8",       // "This" network (RFC 1122)
		"10.0.0.0/8",      // Private (RFC 1918)
		"100.64.0.0/10",   // Carrier-grade NAT (RFC 6598)
		"127.0.0.0/8",     // Loopback (RFC 1122)
		"169.254.0.0/16",  // Link-local (RFC 3927)
		"172.16.0.0/12",   // Private (RFC 1918)
		"192.0.0.0/24",    // IETF protocol assignments (RFC 6890)
		"192.0.2.0/24",    // Documentation TEST-NET-1 (RFC 5737)
		"192.168.0.0/16",  // Private (RFC 1918)
		"198.18.0.0/15",   // Benchmarking (RFC 2544)
		"198.51.100.0/24", // Documentation TEST-NET-2 (RFC 5737)
		"203.0.113.0/24",  // Documentation TEST-NET-3 (RFC 5737)
		"240.0.0.0/4",     // Reserved for future use (RFC 1112)
		// IPv6 private and special-use ranges
		"::1/128",   // Loopback
		"fc00::/7",  // Unique local address
		"fe80::/10", // Link-local
	} {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("invalid CIDR: " + cidr)
		}
		privateRanges = append(privateRanges, n)
	}
}

// isPrivateIP returns true if the IP is in a private, loopback, or
// link-local range.
func isPrivateIP(ip net.IP) bool {
	for _, n := range privateRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
