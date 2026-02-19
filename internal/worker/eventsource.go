package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	v8 "github.com/tommie/v8go"
)

// sseEvent represents a single Server-Sent Event.
type sseEvent struct {
	Type string `json:"type"`
	Data string `json:"data"`
	ID   string `json:"id,omitempty"`
}

// eventSourceJS defines the EventSource class as a JS polyfill that delegates
// network I/O to Go-backed helpers.
const eventSourceJS = `
(function() {
	const CONNECTING = 0;
	const OPEN = 1;
	const CLOSED = 2;

	class EventSource extends EventTarget {
		constructor(url, options) {
			super();
			if (typeof url !== 'string' && !(url instanceof URL)) {
				throw new TypeError('EventSource: url must be a string or URL');
			}
			this._url = typeof url === 'string' ? url : url.toString();
			this._withCredentials = !!(options && options.withCredentials);
			this._readyState = CONNECTING;
			this._sourceID = null;
			this._pollTimer = null;

			const reqID = globalThis.__requestID;
			const self = this;
			try {
				this._sourceID = __eventSourceConnect(reqID, this._url);
				this._readyState = OPEN;
				this.dispatchEvent(new Event('open'));
				// Start polling for events.
				this._startPolling(reqID);
			} catch(e) {
				this._readyState = CLOSED;
				const errEvt = new Event('error');
				errEvt.message = e.message || String(e);
				this.dispatchEvent(errEvt);
			}
		}

		_startPolling(reqID) {
			const self = this;
			function poll() {
				if (self._readyState === CLOSED || !self._sourceID) return;
				try {
					const eventsJSON = __eventSourcePoll(reqID, self._sourceID);
					if (eventsJSON && eventsJSON !== '[]') {
						const events = JSON.parse(eventsJSON);
						for (const evt of events) {
							if (evt.type === '__error__') {
								const errEvt = new Event('error');
								errEvt.message = evt.data;
								self.dispatchEvent(errEvt);
								self._readyState = CLOSED;
								return;
							}
							if (evt.type === '__done__') {
								self._readyState = CLOSED;
								return;
							}
							const me = new Event(evt.type || 'message');
							me.data = evt.data;
							me.lastEventId = evt.id || '';
							self.dispatchEvent(me);
						}
					}
				} catch(e) {
					// poll error, ignore
				}
				if (self._readyState !== CLOSED) {
					self._pollTimer = setTimeout(poll, 50);
				}
			}
			self._pollTimer = setTimeout(poll, 10);
		}

		get url() { return this._url; }
		get readyState() { return this._readyState; }
		get withCredentials() { return this._withCredentials; }

		set onopen(fn) {
			if (this._onopen) this.removeEventListener('open', this._onopen);
			this._onopen = fn;
			if (fn) this.addEventListener('open', fn);
		}
		get onopen() { return this._onopen || null; }

		set onmessage(fn) {
			if (this._onmessage) this.removeEventListener('message', this._onmessage);
			this._onmessage = fn;
			if (fn) this.addEventListener('message', fn);
		}
		get onmessage() { return this._onmessage || null; }

		set onerror(fn) {
			if (this._onerror) this.removeEventListener('error', this._onerror);
			this._onerror = fn;
			if (fn) this.addEventListener('error', fn);
		}
		get onerror() { return this._onerror || null; }

		close() {
			if (this._readyState === CLOSED) return;
			this._readyState = CLOSED;
			if (this._pollTimer) {
				clearTimeout(this._pollTimer);
				this._pollTimer = null;
			}
			if (this._sourceID) {
				try {
					const reqID = globalThis.__requestID;
					__eventSourceClose(reqID, this._sourceID);
				} catch(e) {}
			}
		}
	}

	EventSource.CONNECTING = CONNECTING;
	EventSource.OPEN = OPEN;
	EventSource.CLOSED = CLOSED;
	EventSource.prototype.CONNECTING = CONNECTING;
	EventSource.prototype.OPEN = OPEN;
	EventSource.prototype.CLOSED = CLOSED;

	globalThis.EventSource = EventSource;
})();
`

// setupEventSource registers Go-backed helpers for EventSource SSE and
// evaluates the JS wrapper.
func setupEventSource(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	// __eventSourceConnect(requestID, url) -> sourceID
	ctx.Global().Set("__eventSourceConnect", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("__eventSourceConnect", 2).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		rawURL := args[1].String()

		// SSRF protection: block private IPs.
		if eventSourceSSRFEnabled && isPrivateHostname(rawURL) {
			return throwError(iso, "EventSource: connection to private IP addresses is not allowed")
		}

		state := getRequestState(reqID)
		if state == nil {
			return throwError(iso, "EventSource: invalid request state")
		}

		if state.eventSources == nil {
			state.eventSources = make(map[string]*eventSourceState)
		}
		state.nextSourceID++
		sourceID := strconv.FormatInt(state.nextSourceID, 10)

		es := &eventSourceState{
			url: rawURL,
		}
		state.eventSources[sourceID] = es

		// Start the SSE connection in a goroutine.
		connCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		es.cancelFunc = cancel

		go func() {
			defer cancel()
			req, err := http.NewRequestWithContext(connCtx, "GET", rawURL, nil)
			if err != nil {
				es.mu.Lock()
				es.events = append(es.events, sseEvent{Type: "__error__", Data: err.Error()})
				es.closed = true
				es.mu.Unlock()
				return
			}
			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Cache-Control", "no-cache")

			client := &http.Client{
				Timeout:   30 * time.Second,
				Transport: eventSourceTransport,
			}
			resp, err := client.Do(req)
			if err != nil {
				es.mu.Lock()
				es.events = append(es.events, sseEvent{Type: "__error__", Data: err.Error()})
				es.closed = true
				es.mu.Unlock()
				return
			}

			es.mu.Lock()
			es.resp = resp
			es.body = resp.Body
			es.connected = true
			es.mu.Unlock()

			// Parse SSE stream.
			parseSSEStream(es, resp)
		}()

		// Wait briefly for connection to establish or fail.
		time.Sleep(50 * time.Millisecond)

		es.mu.Lock()
		hasError := false
		for _, evt := range es.events {
			if evt.Type == "__error__" {
				hasError = true
				break
			}
		}
		es.mu.Unlock()

		if hasError {
			return throwError(iso, "EventSource: connection failed")
		}

		val, _ := v8.NewValue(iso, sourceID)
		return val
	}).GetFunction(ctx))

	// __eventSourcePoll(requestID, sourceID) -> JSON array of events
	ctx.Global().Set("__eventSourcePoll", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("__eventSourcePoll", 2).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		sourceID := args[1].String()

		state := getRequestState(reqID)
		if state == nil || state.eventSources == nil {
			val, _ := v8.NewValue(iso, "[]")
			return val
		}

		es, ok := state.eventSources[sourceID]
		if !ok {
			val, _ := v8.NewValue(iso, "[]")
			return val
		}

		es.mu.Lock()
		events := es.events
		es.events = nil
		closed := es.closed
		es.mu.Unlock()

		if len(events) == 0 && closed {
			events = append(events, sseEvent{Type: "__done__"})
		}

		data, _ := json.Marshal(events)
		val, _ := v8.NewValue(iso, string(data))
		return val
	}).GetFunction(ctx))

	// __eventSourceClose(requestID, sourceID)
	ctx.Global().Set("__eventSourceClose", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			return throwError(iso, errMissingArg("__eventSourceClose", 2).Error())
		}
		reqID, _ := strconv.ParseUint(args[0].String(), 10, 64)
		sourceID := args[1].String()

		state := getRequestState(reqID)
		if state == nil || state.eventSources == nil {
			undef, _ := v8.NewValue(iso, true)
			return undef
		}

		es, ok := state.eventSources[sourceID]
		if !ok {
			undef, _ := v8.NewValue(iso, true)
			return undef
		}

		closeEventSource(es)
		delete(state.eventSources, sourceID)

		undef, _ := v8.NewValue(iso, true)
		return undef
	}).GetFunction(ctx))

	// Evaluate the JS wrapper.
	if _, err := ctx.RunScript(eventSourceJS, "eventsource.js"); err != nil {
		return fmt.Errorf("evaluating eventsource.js: %w", err)
	}

	return nil
}

// parseSSEStream reads SSE events from an HTTP response and queues them.
func parseSSEStream(es *eventSourceState, resp *http.Response) {
	defer func() {
		resp.Body.Close()
		es.mu.Lock()
		es.closed = true
		es.mu.Unlock()
	}()

	scanner := bufio.NewScanner(resp.Body)
	var eventType, data, lastID string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Dispatch event.
			if len(dataLines) > 0 {
				data = strings.Join(dataLines, "\n")
				if eventType == "" {
					eventType = "message"
				}
				evt := sseEvent{
					Type: eventType,
					Data: data,
					ID:   lastID,
				}
				es.mu.Lock()
				es.events = append(es.events, evt)
				es.mu.Unlock()
			}
			eventType = ""
			dataLines = nil
			continue
		}

		if strings.HasPrefix(line, ":") {
			// Comment, ignore.
			continue
		}

		var field, value string
		if idx := strings.Index(line, ":"); idx >= 0 {
			field = line[:idx]
			value = line[idx+1:]
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}
		} else {
			field = line
		}

		switch field {
		case "event":
			eventType = value
		case "data":
			dataLines = append(dataLines, value)
		case "id":
			lastID = value
		case "retry":
			// Ignore retry field.
		}
	}

	// Flush any remaining data.
	if len(dataLines) > 0 {
		data = strings.Join(dataLines, "\n")
		if eventType == "" {
			eventType = "message"
		}
		evt := sseEvent{
			Type: eventType,
			Data: data,
			ID:   lastID,
		}
		es.mu.Lock()
		es.events = append(es.events, evt)
		es.mu.Unlock()
	}
}

// closeEventSource closes an SSE connection and cleans up resources.
func closeEventSource(es *eventSourceState) {
	es.mu.Lock()
	defer es.mu.Unlock()

	es.closed = true
	if es.cancelFunc != nil {
		es.cancelFunc()
	}
	if es.body != nil {
		es.body.Close()
	}
}

// cleanupEventSources closes all open event sources for a request. This is
// a safety measure; not wired in yet but available for the engine to call
// on request cleanup.
var _ = cleanupEventSources // prevent unused warning

func cleanupEventSources(reqID uint64) {
	state := getRequestState(reqID)
	if state == nil || state.eventSources == nil {
		return
	}
	for id, es := range state.eventSources {
		closeEventSource(es)
		delete(state.eventSources, id)
	}
}

// eventSourceSSRFEnabled controls SSRF protection for EventSource connections.
// Tests can set this to false (along with eventSourceTransport) to allow
// connections to httptest servers on 127.0.0.1.
var eventSourceSSRFEnabled = true

// eventSourceTransport is the HTTP transport used for EventSource connections.
// Tests can override this to http.DefaultTransport to bypass SSRF dial checks.
var eventSourceTransport http.RoundTripper = &http.Transport{
	DialContext: ssrfSafeDialContext,
}

// Ensure sync import is used (eventSourceState has sync.Mutex).
var _ sync.Mutex
