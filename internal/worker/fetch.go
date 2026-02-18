package worker

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cryguy/hostedat/internal/config"
	"github.com/fastschema/qjs"
)

// setupFetch registers the global fetch() function as a Go-backed async
// function. It enforces per-request rate limits and blocks requests to
// private/loopback addresses.
//
// The HTTP request runs synchronously on the JS thread. QuickJS/Wazero is
// not thread-safe — building JS objects from goroutines causes WASM
// out-of-bounds crashes. Each runtime serves one request at a time, so
// blocking during the HTTP call is acceptable.
func setupFetch(rt *qjs.Runtime, cfg config.WorkerConfig) error {
	ctx := rt.Context()

	ctx.SetAsyncFunc("fetch", func(this *qjs.This) {
		args := this.Args()
		promise := this.Promise()
		c := this.Context()

		// Read per-request state for rate limiting.
		reqIDVal := c.Global().GetPropertyStr("__requestID")
		reqID := uint64(reqIDVal.Int64())
		reqIDVal.Free()

		state := getRequestState(reqID)
		if state != nil && state.fetchCount >= state.maxFetches {
			promise.Reject(c.NewError(fmt.Errorf("exceeded maximum fetch requests (%d)", state.maxFetches)))
			return
		}
		if state != nil {
			state.fetchCount++
		}

		// Extract URL and options from arguments.
		var fetchURL, method string
		headers := make(map[string]string)
		var body string

		if len(args) == 0 {
			promise.Reject(c.NewError(fmt.Errorf("fetch requires at least 1 argument")))
			return
		}

		if args[0].IsString() {
			fetchURL = args[0].String()
			method = "GET"
		} else if args[0].IsObject() {
			// Request object
			urlVal := args[0].GetPropertyStr("url")
			fetchURL = urlVal.String()
			urlVal.Free()
			methodVal := args[0].GetPropertyStr("method")
			method = methodVal.String()
			methodVal.Free()

			headersVal := args[0].GetPropertyStr("headers")
			if headersVal.IsObject() {
				mapVal := headersVal.GetPropertyStr("_map")
				if mapVal.IsObject() {
					names, err := mapVal.GetOwnPropertyNames()
					if err == nil {
						for _, name := range names {
							v := mapVal.GetPropertyStr(name)
							headers[name] = v.String()
							v.Free()
						}
					}
				}
				mapVal.Free()
			}
			headersVal.Free()

			bodyVal := args[0].GetPropertyStr("_body")
			if !bodyVal.IsNull() && !bodyVal.IsUndefined() {
				body = bodyVal.String()
			}
			bodyVal.Free()
		}

		// Apply overrides from second argument (init object).
		if len(args) > 1 && args[1].IsObject() {
			init := args[1]
			mVal := init.GetPropertyStr("method")
			if !mVal.IsUndefined() {
				method = strings.ToUpper(mVal.String())
			}
			mVal.Free()

			hVal := init.GetPropertyStr("headers")
			if hVal.IsObject() {
				// Could be a Headers instance or a plain object.
				mapVal := hVal.GetPropertyStr("_map")
				source := hVal
				if mapVal.IsObject() {
					source = mapVal
				}
				names, err := source.GetOwnPropertyNames()
				if err == nil {
					for _, name := range names {
						v := source.GetPropertyStr(name)
						headers[strings.ToLower(name)] = v.String()
						v.Free()
					}
				}
				if mapVal.IsObject() {
					mapVal.Free()
				}
			}
			hVal.Free()

			bVal := init.GetPropertyStr("body")
			if !bVal.IsUndefined() && !bVal.IsNull() {
				body = bVal.String()
			}
			bVal.Free()
		}

		if method == "" {
			method = "GET"
		}

		// Block private IP addresses.
		if isPrivateURL(fetchURL) {
			promise.Reject(c.NewError(fmt.Errorf("fetch to private IP addresses is not allowed")))
			return
		}

		timeout := time.Duration(cfg.FetchTimeoutSec) * time.Second
		maxBytes := int64(cfg.MaxResponseBytes)

		var bodyReader io.Reader
		if body != "" {
			bodyReader = strings.NewReader(body)
		}

		httpReq, err := http.NewRequest(method, fetchURL, bodyReader)
		if err != nil {
			promise.Reject(c.NewError(fmt.Errorf("fetch: %w", err)))
			return
		}
		for k, v := range headers {
			httpReq.Header.Set(k, v)
		}

		client := &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				// Re-validate each redirect target against private IPs
				if isPrivateURL(req.URL.String()) {
					return fmt.Errorf("redirect to private IP address is not allowed")
				}
				return nil
			},
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			promise.Reject(c.NewError(fmt.Errorf("fetch: %w", err)))
			return
		}
		defer resp.Body.Close()

		limitedReader := io.LimitReader(resp.Body, maxBytes+1)
		respBody, err := io.ReadAll(limitedReader)
		if err != nil {
			promise.Reject(c.NewError(fmt.Errorf("fetch: reading body: %w", err)))
			return
		}
		if int64(len(respBody)) > maxBytes {
			respBody = respBody[:maxBytes]
		}

		// Build Response-compatible JS object via constructor.
		respHeaders := c.NewObject()
		for k, vals := range resp.Header {
			respHeaders.SetPropertyStr(strings.ToLower(k), c.NewString(strings.Join(vals, ", ")))
		}

		initObj := c.NewObject()
		initObj.SetPropertyStr("status", c.NewInt32(int32(resp.StatusCode)))
		initObj.SetPropertyStr("statusText", c.NewString(resp.Status))
		initObj.SetPropertyStr("headers", respHeaders)
		initObj.SetPropertyStr("url", c.NewString(fetchURL))

		responseCtor := c.Global().GetPropertyStr("Response")
		jsResp := responseCtor.CallConstructor(c.NewString(string(respBody)), initObj)
		responseCtor.Free()

		promise.Resolve(jsResp)
	})

	return nil
}

// isPrivateURL checks whether the URL targets a private/loopback address.
func isPrivateURL(rawURL string) bool {
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

	// Resolve and check IP ranges.
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If we can't resolve, check if it's a literal IP.
		ip := net.ParseIP(hostname)
		if ip == nil {
			return false // unresolvable hostname, let HTTP client handle
		}
		return isPrivateIP(ip)
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true
		}
	}

	return false
}

// isPrivateIP returns true if the IP is in a private, loopback, or
// link-local range.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network *net.IPNet
	}{
		{mustParseCIDR("127.0.0.0/8")},
		{mustParseCIDR("10.0.0.0/8")},
		{mustParseCIDR("172.16.0.0/12")},
		{mustParseCIDR("192.168.0.0/16")},
		{mustParseCIDR("169.254.0.0/16")},
		{mustParseCIDR("::1/128")},
		{mustParseCIDR("fc00::/7")},
		{mustParseCIDR("fe80::/10")},
	}

	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}

	return false
}

func mustParseCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic("invalid CIDR: " + s)
	}
	return n
}
