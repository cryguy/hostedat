package worker

import (
	"encoding/json"
	"fmt"

	v8 "github.com/tommie/v8go"
)

// ServiceBindingConfig identifies the target worker for a service binding.
type ServiceBindingConfig struct {
	TargetSiteID    string
	TargetDeployKey string
}

// ServiceBindingBridge provides Go methods that back the service binding JS bindings.
type ServiceBindingBridge struct {
	Engine *Engine
	Env    *Env
}

// Fetch calls the target worker's fetch handler with the given request.
func (sb *ServiceBindingBridge) Fetch(config ServiceBindingConfig, req *WorkerRequest) (*WorkerResponse, error) {
	result := sb.Engine.Execute(config.TargetSiteID, config.TargetDeployKey, sb.Env, req)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.Response == nil {
		return nil, fmt.Errorf("target worker returned no response")
	}
	return result.Response, nil
}

// buildServiceBinding creates a JS object with an async fetch() method
// that calls the target worker via the engine.
func buildServiceBinding(iso *v8.Isolate, ctx *v8.Context, bridge *ServiceBindingBridge, config ServiceBindingConfig) (*v8.Value, error) {
	sbObj, err := newJSObject(iso, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating service binding object: %w", err)
	}

	// fetch(urlOrRequest, init?) -> Promise<Response>
	sbObj.Set("fetch", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		resolver, _ := v8.NewPromiseResolver(ctx)
		args := info.Args()
		if len(args) == 0 {
			errVal, _ := v8.NewValue(iso, "service binding fetch() requires at least one argument")
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		// Extract request details via JS.
		ctx.Global().Set("__tmp_sb_arg", args[0])
		var initArg string
		if len(args) > 1 && args[1].IsObject() {
			ctx.Global().Set("__tmp_sb_init", args[1])
			initArg = "globalThis.__tmp_sb_init"
		} else {
			initArg = "null"
		}

		extractScript := fmt.Sprintf(`(function() {
			var arg = globalThis.__tmp_sb_arg;
			var init = %s;
			delete globalThis.__tmp_sb_arg;
			delete globalThis.__tmp_sb_init;

			var url, method, headers, body;

			if (typeof arg === 'string') {
				url = arg;
			} else if (arg && typeof arg === 'object') {
				url = arg.url || '';
				method = arg.method;
				headers = {};
				if (arg.headers) {
					if (typeof arg.headers.forEach === 'function') {
						arg.headers.forEach(function(v, k) { headers[k] = v; });
					} else {
						for (var k in arg.headers) headers[k] = arg.headers[k];
					}
				}
				body = arg._bodyStr || null;
			}

			if (init) {
				if (init.method) method = init.method;
				if (init.headers) {
					if (!headers) headers = {};
					if (typeof init.headers.forEach === 'function') {
						init.headers.forEach(function(v, k) { headers[k] = v; });
					} else {
						for (var k in init.headers) headers[k] = init.headers[k];
					}
				}
				if (init.body !== undefined) body = String(init.body);
			}

			return JSON.stringify({
				url: url || 'https://fake-host/',
				method: method || 'GET',
				headers: headers || {},
				body: body || null
			});
		})()`, initArg)

		reqResult, err := ctx.RunScript(extractScript, "sb_extract_req.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, "failed to parse request: "+err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		var reqData struct {
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
			Body    *string           `json:"body"`
		}
		if err := json.Unmarshal([]byte(reqResult.String()), &reqData); err != nil {
			errVal, _ := v8.NewValue(iso, "failed to unmarshal request: "+err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		workerReq := &WorkerRequest{
			Method:  reqData.Method,
			URL:     reqData.URL,
			Headers: reqData.Headers,
		}
		if reqData.Body != nil {
			workerReq.Body = []byte(*reqData.Body)
		}

		resp, err := bridge.Fetch(config, workerReq)
		if err != nil {
			errVal, _ := v8.NewValue(iso, "service binding fetch failed: "+err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		// Build a JS Response from the Go response.
		respJSON, _ := json.Marshal(map[string]interface{}{
			"status":  resp.StatusCode,
			"headers": resp.Headers,
			"body":    string(resp.Body),
		})
		jsResp, err := ctx.RunScript(fmt.Sprintf(`(function() {
			var d = JSON.parse(%q);
			var h = new Headers();
			if (d.headers) {
				for (var k in d.headers) h.set(k, d.headers[k]);
			}
			return new Response(d.body, { status: d.status, headers: h });
		})()`, string(respJSON)), "sb_build_response.js")
		if err != nil {
			errVal, _ := v8.NewValue(iso, "failed to build response: "+err.Error())
			resolver.Reject(errVal)
			return resolver.GetPromise().Value
		}

		resolver.Resolve(jsResp)
		return resolver.GetPromise().Value
	}).GetFunction(ctx))

	return sbObj.Value, nil
}

// setupServiceBindings is a placeholder setup function (currently a no-op).
func setupServiceBindings(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	return nil
}
