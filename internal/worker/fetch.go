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

// setupFetch registers the global fetch() function as a Go-backed function
// backed by a PromiseResolver. It enforces per-request rate limits and blocks
// requests to private/loopback addresses.
//
// The HTTP request runs synchronously on the JS thread. V8 is not thread-safe,
// and each worker serves one request at a time, so blocking during the HTTP
// call is acceptable.
func setupFetch(iso *v8.Isolate, ctx *v8.Context, el *eventLoop, cfg config.WorkerConfig) error {
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
		ctx.Global().Set("__tmp_fetch_arg0", args[0])
		if len(args) > 1 {
			ctx.Global().Set("__tmp_fetch_arg1", args[1])
		}

		extractResult, err := ctx.RunScript(`(function() {
			var a0 = globalThis.__tmp_fetch_arg0;
			var a1 = globalThis.__tmp_fetch_arg1;
			delete globalThis.__tmp_fetch_arg0;
			delete globalThis.__tmp_fetch_arg1;
			var url = '', method = 'GET', headers = {}, body = null, bodyIsBase64 = false;
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
			}
			if (!method) method = 'GET';
			return JSON.stringify({url: url, method: method, headers: headers, body: body, bodyIsBase64: bodyIsBase64});
		})()`, "fetch_extract.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: extracting arguments: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		var fetchArgs struct {
			URL          string            `json:"url"`
			Method       string            `json:"method"`
			Headers      map[string]string `json:"headers"`
			Body         *string           `json:"body"`
			BodyIsBase64 bool              `json:"bodyIsBase64"`
		}
		if err := json.Unmarshal([]byte(extractResult.String()), &fetchArgs); err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: parsing arguments: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		// Block private hostnames before connecting.
		if isPrivateHostname(fetchArgs.URL) {
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

		httpReq, err := http.NewRequest(fetchArgs.Method, fetchArgs.URL, bodyReader)
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		for k, v := range fetchArgs.Headers {
			httpReq.Header.Set(k, v)
		}

		// Use a custom transport that validates resolved IPs at connect time,
		// preventing DNS rebinding/TOCTOU attacks.
		client := &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext: ssrfSafeDialContext,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				// Redirect hostname pre-check (the dialer also validates at connect time).
				if isPrivateHostname(req.URL.String()) {
					return fmt.Errorf("redirect to private IP address is not allowed")
				}
				return nil
			},
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}
		defer resp.Body.Close()

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

		// Base64-encode the response body to preserve binary data integrity.
		bodyB64 := base64.StdEncoding.EncodeToString(respBody)
		bodyVal, _ := v8.NewValue(iso, bodyB64)
		ctx.Global().Set("__tmp_fetch_resp_body", bodyVal)
		statusVal, _ := v8.NewValue(iso, int32(resp.StatusCode))
		ctx.Global().Set("__tmp_fetch_resp_status", statusVal)
		statusTextVal, _ := v8.NewValue(iso, resp.Status)
		ctx.Global().Set("__tmp_fetch_resp_statusText", statusTextVal)
		headersJSONVal, _ := v8.NewValue(iso, string(headersJSON))
		ctx.Global().Set("__tmp_fetch_resp_headers", headersJSONVal)
		fetchURLVal, _ := v8.NewValue(iso, fetchArgs.URL)
		ctx.Global().Set("__tmp_fetch_resp_url", fetchURLVal)

		jsResp, err := ctx.RunScript(`(function() {
			var b64Body = globalThis.__tmp_fetch_resp_body;
			var status = globalThis.__tmp_fetch_resp_status;
			var statusText = globalThis.__tmp_fetch_resp_statusText;
			var hdrs = JSON.parse(globalThis.__tmp_fetch_resp_headers);
			var url = globalThis.__tmp_fetch_resp_url;
			delete globalThis.__tmp_fetch_resp_body;
			delete globalThis.__tmp_fetch_resp_status;
			delete globalThis.__tmp_fetch_resp_statusText;
			delete globalThis.__tmp_fetch_resp_headers;
			delete globalThis.__tmp_fetch_resp_url;
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
			return new Response(body, {status: status, statusText: statusText, headers: hdrs, url: url});
		})()`, "fetch_response.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, fmt.Sprintf("fetch: building response: %s", err.Error()))
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		resolver.Resolve(jsResp)
		return resolver.GetPromise().Value
	})

	ctx.Global().Set("fetch", fetchFT.GetFunction(ctx))
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
		"0.0.0.0/8",          // "This" network (RFC 1122)
		"10.0.0.0/8",         // Private (RFC 1918)
		"100.64.0.0/10",      // Carrier-grade NAT (RFC 6598)
		"127.0.0.0/8",        // Loopback (RFC 1122)
		"169.254.0.0/16",     // Link-local (RFC 3927)
		"172.16.0.0/12",      // Private (RFC 1918)
		"192.0.0.0/24",       // IETF protocol assignments (RFC 6890)
		"192.0.2.0/24",       // Documentation TEST-NET-1 (RFC 5737)
		"192.168.0.0/16",     // Private (RFC 1918)
		"198.18.0.0/15",      // Benchmarking (RFC 2544)
		"198.51.100.0/24",    // Documentation TEST-NET-2 (RFC 5737)
		"203.0.113.0/24",     // Documentation TEST-NET-3 (RFC 5737)
		"240.0.0.0/4",        // Reserved for future use (RFC 1112)
		// IPv6 private and special-use ranges
		"::1/128",            // Loopback
		"fc00::/7",           // Unique local address
		"fe80::/10",          // Link-local
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
