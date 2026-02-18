package worker

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/fastschema/qjs"
)

// webAPIsJS defines the Web API classes (Headers, Request, Response, URL,
// TextEncoder, TextDecoder) in JavaScript. Go-backed helpers like __parseURL
// are registered separately and called from inside these classes.
const webAPIsJS = `
class Headers {
	constructor(init) {
		this._map = {};
		if (init) {
			if (init instanceof Headers) {
				for (const [k, v] of Object.entries(init._map)) this._map[k] = v;
			} else if (Array.isArray(init)) {
				for (const [k, v] of init) this._map[k.toLowerCase()] = String(v);
			} else {
				for (const [k, v] of Object.entries(init)) this._map[k.toLowerCase()] = String(v);
			}
		}
	}
	get(name) { return this._map[name.toLowerCase()] ?? null; }
	set(name, value) { this._map[name.toLowerCase()] = String(value); }
	has(name) { return name.toLowerCase() in this._map; }
	delete(name) { delete this._map[name.toLowerCase()]; }
	append(name, value) {
		const key = name.toLowerCase();
		this._map[key] = this._map[key] ? this._map[key] + ', ' + String(value) : String(value);
	}
	forEach(cb) { for (const [k, v] of Object.entries(this._map)) cb(v, k, this); }
	entries() { return Object.entries(this._map)[Symbol.iterator](); }
	keys() { return Object.keys(this._map)[Symbol.iterator](); }
	values() { return Object.values(this._map)[Symbol.iterator](); }
}

class URL {
	constructor(input, base) {
		const parsed = JSON.parse(__parseURL(input, base || ''));
		if (parsed.error) throw new TypeError(parsed.error);
		this.href = parsed.href;
		this.protocol = parsed.protocol;
		this.hostname = parsed.hostname;
		this.port = parsed.port;
		this.pathname = parsed.pathname;
		this.search = parsed.search;
		this.hash = parsed.hash;
		this.origin = parsed.origin;
		this.host = parsed.host;
		this.searchParams = new URLSearchParams(this.search);
		this.searchParams._url = this;
	}
	toString() { return this.href; }
}

class URLSearchParams {
	constructor(init) {
		this._entries = [];
		if (typeof init === 'string') {
			const s = init.startsWith('?') ? init.slice(1) : init;
			if (s) {
				for (const pair of s.split('&')) {
					const [k, ...rest] = pair.split('=');
					this._entries.push([decodeURIComponent(k), decodeURIComponent(rest.join('='))]);
				}
			}
		}
	}
	get(name) {
		const e = this._entries.find(([k]) => k === name);
		return e ? e[1] : null;
	}
	has(name) { return this._entries.some(([k]) => k === name); }
	toString() { return this._entries.map(([k, v]) => encodeURIComponent(k) + '=' + encodeURIComponent(v)).join('&'); }
	forEach(cb) { for (const [k, v] of this._entries) cb(v, k, this); }
	entries() { return this._entries[Symbol.iterator](); }
	keys() { return this._entries.map(([k]) => k)[Symbol.iterator](); }
	values() { return this._entries.map(([, v]) => v)[Symbol.iterator](); }
}

class Request {
	constructor(input, init) {
		init = init || {};
		if (input instanceof Request) {
			this.url = input.url;
			this.method = input.method;
			this.headers = new Headers(input.headers);
			this._body = input._body;
		} else {
			this.url = String(input);
			this.method = (init.method || 'GET').toUpperCase();
			this.headers = new Headers(init.headers);
			this._body = init.body !== undefined ? init.body : null;
		}
		if (init.method) this.method = init.method.toUpperCase();
		if (init.headers) this.headers = new Headers(init.headers);
		if (init.body !== undefined) this._body = init.body;
	}
	async text() { return this._body !== null && this._body !== undefined ? String(this._body) : ''; }
	async json() { return JSON.parse(await this.text()); }
	async arrayBuffer() {
		const t = await this.text();
		const enc = new TextEncoder();
		return enc.encode(t).buffer;
	}
	clone() { return new Request(this); }
}

class Response {
	constructor(body, init) {
		init = init || {};
		this._body = body !== undefined && body !== null ? body : null;
		this.status = init.status !== undefined ? init.status : 200;
		this.statusText = init.statusText || '';
		this.headers = new Headers(init.headers);
		this.ok = this.status >= 200 && this.status < 300;
		this.url = init.url || '';
	}
	async text() { return this._body !== null && this._body !== undefined ? String(this._body) : ''; }
	async json() { return JSON.parse(await this.text()); }
	async arrayBuffer() {
		const t = await this.text();
		const enc = new TextEncoder();
		return enc.encode(t).buffer;
	}
	clone() {
		return new Response(this._body, {
			status: this.status,
			statusText: this.statusText,
			headers: new Headers(this.headers),
		});
	}
	static json(data, init) {
		init = init || {};
		const body = JSON.stringify(data);
		const headers = new Headers(init.headers);
		if (!headers.has('content-type')) headers.set('content-type', 'application/json');
		return new Response(body, { ...init, headers });
	}
	static redirect(url, status) {
		status = status || 302;
		return new Response(null, { status, headers: { location: url } });
	}
}

if (typeof TextEncoder === 'undefined') {
	globalThis.TextEncoder = class TextEncoder {
		encode(str) {
			str = String(str);
			const buf = [];
			for (let i = 0; i < str.length; i++) {
				let c = str.charCodeAt(i);
				if (c < 0x80) {
					buf.push(c);
				} else if (c < 0x800) {
					buf.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f));
				} else if (c >= 0xd800 && c <= 0xdbff && i + 1 < str.length) {
					const next = str.charCodeAt(++i);
					const cp = ((c - 0xd800) << 10) + (next - 0xdc00) + 0x10000;
					buf.push(0xf0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3f), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
				} else {
					buf.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
				}
			}
			return new Uint8Array(buf);
		}
	};
}

if (typeof TextDecoder === 'undefined') {
	globalThis.TextDecoder = class TextDecoder {
		decode(buf) {
			if (!buf) return '';
			const bytes = new Uint8Array(buf.buffer || buf);
			let result = '';
			for (let i = 0; i < bytes.length;) {
				const b = bytes[i];
				if (b < 0x80) { result += String.fromCharCode(b); i++; }
				else if ((b & 0xe0) === 0xc0) { result += String.fromCharCode(((b & 0x1f) << 6) | (bytes[i+1] & 0x3f)); i += 2; }
				else if ((b & 0xf0) === 0xe0) { result += String.fromCharCode(((b & 0x0f) << 12) | ((bytes[i+1] & 0x3f) << 6) | (bytes[i+2] & 0x3f)); i += 3; }
				else if ((b & 0xf8) === 0xf0) {
					const cp = ((b & 0x07) << 18) | ((bytes[i+1] & 0x3f) << 12) | ((bytes[i+2] & 0x3f) << 6) | (bytes[i+3] & 0x3f);
					result += String.fromCodePoint(cp); i += 4;
				} else { result += '\ufffd'; i++; }
			}
			return result;
		}
	};
}

globalThis.Headers = Headers;
globalThis.URL = URL;
globalThis.URLSearchParams = URLSearchParams;
globalThis.Request = Request;
globalThis.Response = Response;
`

// urlSearchParamsExtJS patches URLSearchParams with mutation methods and URL sync.
// Must be evaluated after webAPIsJS so that URLSearchParams and URL are defined.
const urlSearchParamsExtJS = `
(function() {
var USP = URLSearchParams.prototype;

USP._sync = function() {
	if (this._url) {
		var s = this.toString();
		this._url.search = s ? '?' + s : '';
		this._url.href = this._url.origin + this._url.pathname + this._url.search + this._url.hash;
	}
};

USP.getAll = function(name) {
	return this._entries.filter(function(e) { return e[0] === name; }).map(function(e) { return e[1]; });
};

USP.set = function(name, value) {
	var s = String(value);
	var found = false;
	var filtered = [];
	for (var i = 0; i < this._entries.length; i++) {
		var entry = this._entries[i];
		if (entry[0] === name) {
			if (!found) {
				filtered.push([name, s]);
				found = true;
			}
		} else {
			filtered.push(entry);
		}
	}
	if (!found) filtered.push([name, s]);
	this._entries = filtered;
	this._sync();
};

USP.append = function(name, value) {
	this._entries.push([name, String(value)]);
	this._sync();
};

// Override delete to support sync
var origDelete = USP['delete'];
USP['delete'] = function(name) {
	this._entries = this._entries.filter(function(e) { return e[0] !== name; });
	this._sync();
};

USP.sort = function() {
	this._entries.sort(function(a, b) { return a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0; });
	this._sync();
};

})();
`

// setupURLSearchParamsExt evaluates the URLSearchParams extension polyfill.
func setupURLSearchParamsExt(rt *qjs.Runtime) error {
	if _, err := rt.Eval("urlsearchparams_ext.js", qjs.Code(urlSearchParamsExtJS)); err != nil {
		return fmt.Errorf("evaluating urlsearchparams_ext.js: %w", err)
	}
	return nil
}

// setupWebAPIs registers Go-backed helpers and evaluates the JS class
// definitions that form the Web API surface available to workers.
func setupWebAPIs(rt *qjs.Runtime) error {
	ctx := rt.Context()

	// Register Go-backed URL parser.
	ctx.SetFunc("__parseURL", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 1 {
			return c.NewString(`{"error":"URL constructor requires at least 1 argument"}`), nil
		}

		rawURL := args[0].String()
		var base string
		if len(args) > 1 {
			base = args[1].String()
		}

		parsed, err := parseURL(rawURL, base)
		if err != nil {
			errJSON := fmt.Sprintf(`{"error":%q}`, err.Error())
			return c.NewString(errJSON), nil
		}

		data, _ := json.Marshal(parsed)
		return c.NewString(string(data)), nil
	})

	// Evaluate the JS class definitions.
	_, err := rt.Eval("webapi.js", qjs.Code(webAPIsJS))
	return err
}

// urlParsed is the JSON structure returned by __parseURL.
type urlParsed struct {
	Href     string `json:"href"`
	Protocol string `json:"protocol"`
	Hostname string `json:"hostname"`
	Port     string `json:"port"`
	Pathname string `json:"pathname"`
	Search   string `json:"search"`
	Hash     string `json:"hash"`
	Origin   string `json:"origin"`
	Host     string `json:"host"`
}

func parseURL(rawURL, base string) (*urlParsed, error) {
	var u *url.URL
	var err error

	if base != "" {
		baseURL, berr := url.Parse(base)
		if berr != nil {
			return nil, fmt.Errorf("invalid base URL: %s", base)
		}
		ref, rerr := url.Parse(rawURL)
		if rerr != nil {
			return nil, fmt.Errorf("invalid URL: %s", rawURL)
		}
		u = baseURL.ResolveReference(ref)
	} else {
		u, err = url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %s", rawURL)
		}
	}

	if u.Scheme == "" {
		return nil, fmt.Errorf("invalid URL: %s", rawURL)
	}

	protocol := u.Scheme + ":"
	hostname := u.Hostname()
	port := u.Port()
	host := hostname
	if port != "" {
		host = hostname + ":" + port
	}
	origin := protocol + "//" + host
	search := ""
	if u.RawQuery != "" {
		search = "?" + u.RawQuery
	}
	hash := ""
	if u.Fragment != "" {
		hash = "#" + u.Fragment
	}

	return &urlParsed{
		Href:     u.String(),
		Protocol: protocol,
		Hostname: hostname,
		Port:     port,
		Pathname: u.Path,
		Search:   search,
		Hash:     hash,
		Origin:   origin,
		Host:     host,
	}, nil
}

// goRequestToJS converts a Go WorkerRequest into a JS Request object by
// invoking the Request constructor defined in webAPIsJS.
func goRequestToJS(ctx *qjs.Context, req *WorkerRequest) (*qjs.Value, error) {
	// Build the init object.
	init := ctx.NewObject()
	init.SetPropertyStr("method", ctx.NewString(req.Method))

	headersObj := ctx.NewObject()
	for k, v := range req.Headers {
		headersObj.SetPropertyStr(strings.ToLower(k), ctx.NewString(v))
	}
	init.SetPropertyStr("headers", headersObj)

	if req.Body != nil && len(req.Body) > 0 {
		init.SetPropertyStr("body", ctx.NewString(string(req.Body)))
	}

	// Call: new Request(url, init)
	requestCtor := ctx.Global().GetPropertyStr("Request")
	defer requestCtor.Free()

	jsReq := requestCtor.CallConstructor(ctx.NewString(req.URL), init)
	if jsReq.IsError() {
		return nil, fmt.Errorf("failed to create JS Request: %s", jsReq.String())
	}

	return jsReq, nil
}

// jsResponseToGo extracts a Go WorkerResponse from a JS Response value.
func jsResponseToGo(ctx *qjs.Context, val *qjs.Value) (*WorkerResponse, error) {
	if val.IsNull() || val.IsUndefined() {
		return nil, fmt.Errorf("worker returned null/undefined instead of Response")
	}

	status := val.GetPropertyStr("status")
	statusCode := int(status.Int32())
	status.Free()

	// Extract headers from headers._map
	headersVal := val.GetPropertyStr("headers")
	headersMap := headersVal.GetPropertyStr("_map")
	goHeaders := make(map[string]string)

	if headersMap.IsObject() {
		names, err := headersMap.GetOwnPropertyNames()
		if err == nil {
			for _, name := range names {
				v := headersMap.GetPropertyStr(name)
				goHeaders[name] = v.String()
				v.Free()
			}
		}
	}
	headersMap.Free()
	headersVal.Free()

	// Read body directly from _body property to avoid async .text() Promise
	// which causes WASM memory issues when freeing the awaited Promise.
	var body []byte
	bodyVal := val.GetPropertyStr("_body")
	if !bodyVal.IsNull() && !bodyVal.IsUndefined() {
		bodyStr := bodyVal.String()
		if bodyStr != "" {
			body = []byte(bodyStr)
		}
	}
	bodyVal.Free()

	return &WorkerResponse{
		StatusCode: statusCode,
		Headers:    goHeaders,
		Body:       body,
	}, nil
}
