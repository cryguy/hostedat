package worker

import (
	"context"
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

		// Block obviously private hostnames before even attempting a connection.
		if isPrivateHostname(fetchURL) {
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

// isPrivateHostname performs a fast, non-resolving pre-check for obviously
// private hostnames and literal IP addresses. It does NOT resolve DNS — the
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
