package worker

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Integration: Hono-style router using all new Web APIs
// ---------------------------------------------------------------------------

// honoStyleWorker is a realistic Cloudflare Workers script that uses a
// minimal Hono-like router pattern and exercises every new Web API.
const honoStyleWorker = `
// --- Minimal Hono-like router ---
class Router {
  constructor() { this.routes = []; }
  get(path, handler) { this.routes.push({ method: 'GET', path, handler }); }
  post(path, handler) { this.routes.push({ method: 'POST', path, handler }); }
  all(path, handler) { this.routes.push({ method: '*', path, handler }); }
  async handle(request) {
    const url = new URL(request.url);
    for (const route of this.routes) {
      if (route.method !== '*' && route.method !== request.method) continue;
      if (route.path instanceof RegExp) {
        if (route.path.test(url.pathname)) return route.handler(request);
      } else if (url.pathname === route.path) {
        return route.handler(request);
      }
    }
    return new Response('Not Found', { status: 404 });
  }
}

const app = new Router();

// --- Route: GET / --- smoke test
app.get('/', (req) => {
  return new Response('Hello from Hono-style Worker!', {
    headers: { 'content-type': 'text/plain' },
  });
});

// --- Route: GET /api/health --- uses performance.now, navigator, structuredClone
app.get('/api/health', (req) => {
  const start = performance.now();
  const info = {
    status: 'ok',
    runtime: navigator.userAgent,
    apis: ['crypto', 'atob', 'btoa', 'AbortController', 'ReadableStream',
           'FormData', 'Blob', 'setTimeout', 'structuredClone', 'performance'],
  };
  const cloned = structuredClone(info);
  cloned.elapsed = performance.now() - start;
  cloned.cloneWorks = cloned.status === 'ok' && cloned !== info;
  return Response.json(cloned);
});

// --- Route: GET /api/crypto/hash --- SHA-256 digest
app.get('/api/crypto/hash', async (req) => {
  const url = new URL(req.url);
  const input = url.searchParams.get('input') || 'hello';
  const data = new TextEncoder().encode(input);
  const hash = await crypto.subtle.digest('SHA-256', data);
  const arr = new Uint8Array(hash);
  let hex = '';
  for (let i = 0; i < arr.length; i++) hex += arr[i].toString(16).padStart(2, '0');
  return Response.json({ input, sha256: hex, bytes: arr.length });
});

// --- Route: GET /api/crypto/uuid --- random UUID
app.get('/api/crypto/uuid', (req) => {
  const uuid = crypto.randomUUID();
  return Response.json({ uuid, valid: /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(uuid) });
});

// --- Route: POST /api/crypto/hmac --- HMAC sign + verify
app.post('/api/crypto/hmac', async (req) => {
  const body = await req.json();
  const keyData = new TextEncoder().encode(body.key || 'default-secret-key!!');
  const key = await crypto.subtle.importKey('raw', keyData, { name: 'HMAC', hash: 'SHA-256' }, false, ['sign', 'verify']);
  const msgData = new TextEncoder().encode(body.message || 'test');
  const sig = await crypto.subtle.sign('HMAC', key, msgData);
  const valid = await crypto.subtle.verify('HMAC', key, sig, msgData);
  return Response.json({ signatureBytes: new Uint8Array(sig).length, verified: valid });
});

// --- Route: POST /api/crypto/aes --- AES-GCM encrypt + decrypt round-trip
app.post('/api/crypto/aes', async (req) => {
  const body = await req.json();
  const keyBytes = new Uint8Array(16);
  crypto.getRandomValues(keyBytes);
  const key = await crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, ['encrypt', 'decrypt']);
  const iv = new Uint8Array(12);
  crypto.getRandomValues(iv);
  const plaintext = new TextEncoder().encode(body.data || 'secret');
  const ciphertext = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, plaintext);
  const decrypted = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ciphertext);
  const result = new TextDecoder().decode(decrypted);
  return Response.json({ original: body.data || 'secret', decrypted: result, match: result === (body.data || 'secret') });
});

// --- Route: GET /api/encoding --- atob/btoa round-trip
app.get('/api/encoding', (req) => {
  const url = new URL(req.url);
  const input = url.searchParams.get('input') || 'Hello, Workers!';
  const encoded = btoa(input);
  const decoded = atob(encoded);
  return Response.json({ input, encoded, decoded, match: input === decoded });
});

// --- Route: GET /api/abort --- AbortController
app.get('/api/abort', (req) => {
  const controller = new AbortController();
  const events = [];

  controller.signal.addEventListener('abort', () => {
    events.push('abort-fired');
  });

  events.push('before:' + controller.signal.aborted);
  controller.abort('test-reason');
  events.push('after:' + controller.signal.aborted);
  events.push('reason:' + controller.signal.reason);

  // Test AbortSignal.abort() static method
  const preAborted = AbortSignal.abort('pre');
  events.push('static:' + preAborted.aborted);

  // Test throwIfAborted
  try {
    controller.signal.throwIfAborted();
    events.push('no-throw');
  } catch (e) {
    events.push('threw');
  }

  return Response.json({ events });
});

// --- Route: GET /api/streams --- ReadableStream + TransformStream
app.get('/api/streams', async (req) => {
  // Create a readable stream of chunks
  const readable = new ReadableStream({
    start(controller) {
      controller.enqueue('Hello');
      controller.enqueue(' ');
      controller.enqueue('Streams');
      controller.enqueue('!');
      controller.close();
    }
  });

  // Pipe through an uppercase transform
  const transform = new TransformStream({
    transform(chunk, controller) {
      controller.enqueue(chunk.toUpperCase());
    }
  });

  // Read from source, write through transform, read result
  const reader = readable.getReader();
  const writer = transform.writable.getWriter();
  const resultReader = transform.readable.getReader();

  // Pump source into transform
  while (true) {
    const { value, done } = await reader.read();
    if (done) { writer.close(); break; }
    writer.write(value);
  }

  // Read all output
  let result = '';
  while (true) {
    const { value, done } = await resultReader.read();
    if (done) break;
    result += value;
  }

  return new Response(result);
});

// --- Route: POST /api/formdata --- FormData + Blob + File
app.post('/api/formdata', async (req) => {
  const fd = new FormData();
  fd.append('name', 'Alice');
  fd.append('name', 'Bob');
  fd.append('age', '30');

  const blob = new Blob(['file content here'], { type: 'text/plain' });
  const file = new File(['test data'], 'test.txt', { type: 'text/plain' });

  fd.append('document', file);

  const names = fd.getAll('name');
  const age = fd.get('age');
  const doc = fd.get('document');

  const blobText = await blob.text();
  const blobSlice = blob.slice(0, 4);
  const sliceText = await blobSlice.text();

  return Response.json({
    names,
    age,
    docName: doc ? doc.name : null,
    docType: doc ? doc.type : null,
    blobText,
    blobSize: blob.size,
    sliceText,
    hasFile: doc instanceof File,
    hasBlob: blob instanceof Blob,
    fdHas: fd.has('name'),
    fdDelete: (() => { fd.delete('age'); return fd.has('age'); })(),
    fdSet: (() => { fd.set('name', 'Charlie'); return fd.get('name'); })(),
  });
});

// --- Route: GET /api/timers --- setTimeout + clearTimeout
app.get('/api/timers', async (req) => {
  const events = [];

  // setTimeout fires
  await new Promise(resolve => {
    setTimeout(() => { events.push('timeout-fired'); resolve(); }, 0);
  });

  // clearTimeout prevents firing
  let cleared = true;
  const id = setTimeout(() => { cleared = false; }, 0);
  clearTimeout(id);
  await new Promise(r => setTimeout(r, 0));
  events.push('clearTimeout-works:' + cleared);

  // queueMicrotask
  await new Promise(resolve => {
    queueMicrotask(() => { events.push('microtask-fired'); resolve(); });
  });

  return Response.json({ events });
});

// --- Route: GET /api/events --- Event + EventTarget
app.get('/api/events', (req) => {
  const target = new EventTarget();
  const events = [];

  const handler = (e) => { events.push('event:' + e.type); };
  target.addEventListener('custom', handler);
  target.addEventListener('custom', () => { events.push('event2'); }, { once: true });

  target.dispatchEvent(new Event('custom'));
  target.dispatchEvent(new Event('custom')); // 'once' handler should not fire again

  target.removeEventListener('custom', handler);
  target.dispatchEvent(new Event('custom')); // nothing should fire

  const e = new Event('test', { cancelable: true });
  events.push('cancelable:' + e.cancelable);

  return Response.json({ events });
});

// --- Route: GET /api/domexception --- DOMException
app.get('/api/domexception', (req) => {
  const err = new DOMException('test error', 'AbortError');
  return Response.json({
    name: err.name,
    message: err.message,
    isError: err instanceof Error,
  });
});

// --- Route: GET /api/all --- combined check for all APIs
app.get('/api/all', async (req) => {
  const checks = {};

  // atob/btoa
  checks.encoding = btoa('test') === 'dGVzdA==' && atob('dGVzdA==') === 'test';

  // structuredClone
  const obj = { a: [1, 2] };
  const clone = structuredClone(obj);
  clone.a.push(3);
  checks.structuredClone = obj.a.length === 2 && clone.a.length === 3;

  // performance.now
  checks.performance = typeof performance.now() === 'number' && performance.now() >= 0;

  // navigator
  checks.navigator = typeof navigator === 'object' && typeof navigator.userAgent === 'string';

  // crypto.getRandomValues
  const arr = new Uint8Array(8);
  crypto.getRandomValues(arr);
  checks.getRandomValues = arr.some(b => b !== 0);

  // crypto.randomUUID
  checks.randomUUID = /^[0-9a-f-]{36}$/.test(crypto.randomUUID());

  // crypto.subtle.digest
  const hash = await crypto.subtle.digest('SHA-256', new TextEncoder().encode('test'));
  checks.subtleDigest = new Uint8Array(hash).length === 32;

  // AbortController
  const ac = new AbortController();
  ac.abort();
  checks.abortController = ac.signal.aborted === true;

  // Event + EventTarget
  const et = new EventTarget();
  let fired = false;
  et.addEventListener('x', () => { fired = true; });
  et.dispatchEvent(new Event('x'));
  checks.eventTarget = fired;

  // DOMException
  checks.domException = new DOMException('msg', 'TestError').name === 'TestError';

  // setTimeout
  checks.setTimeout = typeof setTimeout === 'function';

  // queueMicrotask
  checks.queueMicrotask = typeof queueMicrotask === 'function';

  // ReadableStream
  checks.readableStream = typeof ReadableStream === 'function';

  // WritableStream
  checks.writableStream = typeof WritableStream === 'function';

  // TransformStream
  checks.transformStream = typeof TransformStream === 'function';

  // FormData
  const fd = new FormData();
  fd.append('k', 'v');
  checks.formData = fd.get('k') === 'v';

  // Blob
  const blob = new Blob(['hi']);
  checks.blob = blob.size === 2;

  // File
  const file = new File(['data'], 'f.txt');
  checks.file = file.name === 'f.txt';

  const allPassed = Object.values(checks).every(v => v === true);
  return Response.json({ allPassed, checks, count: Object.keys(checks).length });
});

export default { fetch: (req, env, ctx) => app.handle(req) };
`

// TestIntegration_HonoStyleRouter exercises all new Web APIs through a
// realistic Hono-like router worker — the kind of script users deploy on
// Cloudflare Workers.
func TestIntegration_HonoStyleRouter(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)
	env := defaultEnv()

	// --- GET / ---
	t.Run("root", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/"))
		assertOK(t, r)
		if string(r.Response.Body) != "Hello from Hono-style Worker!" {
			t.Errorf("body = %q", r.Response.Body)
		}
	})

	// --- GET /api/health ---
	t.Run("health", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/health"))
		assertOK(t, r)
		var data struct {
			Status     string   `json:"status"`
			Runtime    string   `json:"runtime"`
			APIs       []string `json:"apis"`
			CloneWorks bool     `json:"cloneWorks"`
			Elapsed    float64  `json:"elapsed"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if data.Status != "ok" {
			t.Errorf("status = %q", data.Status)
		}
		if data.Runtime != "hostedat-worker/1.0" {
			t.Errorf("runtime = %q", data.Runtime)
		}
		if !data.CloneWorks {
			t.Error("structuredClone did not produce independent copy")
		}
		if data.Elapsed < 0 {
			t.Error("performance.now elapsed should be >= 0")
		}
		if len(data.APIs) == 0 {
			t.Error("API list should not be empty")
		}
	})

	// --- GET /api/crypto/hash ---
	t.Run("crypto/hash", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/crypto/hash?input=hello"))
		assertOK(t, r)
		var data struct {
			Input  string `json:"input"`
			SHA256 string `json:"sha256"`
			Bytes  int    `json:"bytes"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
		if data.SHA256 != expected {
			t.Errorf("sha256 = %q, want %q", data.SHA256, expected)
		}
		if data.Bytes != 32 {
			t.Errorf("bytes = %d, want 32", data.Bytes)
		}
	})

	// --- GET /api/crypto/uuid ---
	t.Run("crypto/uuid", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/crypto/uuid"))
		assertOK(t, r)
		var data struct {
			UUID  string `json:"uuid"`
			Valid bool   `json:"valid"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !data.Valid {
			t.Errorf("UUID %q did not match v4 pattern", data.UUID)
		}
	})

	// --- POST /api/crypto/hmac ---
	t.Run("crypto/hmac", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, &WorkerRequest{
			Method:  "POST",
			URL:     "http://localhost/api/crypto/hmac",
			Headers: map[string]string{"content-type": "application/json"},
			Body:    []byte(`{"key":"my-secret","message":"sign this"}`),
		})
		assertOK(t, r)
		var data struct {
			SignatureBytes int  `json:"signatureBytes"`
			Verified       bool `json:"verified"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if data.SignatureBytes != 32 {
			t.Errorf("signatureBytes = %d, want 32", data.SignatureBytes)
		}
		if !data.Verified {
			t.Error("HMAC verification failed")
		}
	})

	// --- POST /api/crypto/aes ---
	t.Run("crypto/aes", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, &WorkerRequest{
			Method:  "POST",
			URL:     "http://localhost/api/crypto/aes",
			Headers: map[string]string{"content-type": "application/json"},
			Body:    []byte(`{"data":"top secret message"}`),
		})
		assertOK(t, r)
		var data struct {
			Original  string `json:"original"`
			Decrypted string `json:"decrypted"`
			Match     bool   `json:"match"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !data.Match {
			t.Errorf("AES round-trip failed: original=%q decrypted=%q", data.Original, data.Decrypted)
		}
	})

	// --- GET /api/encoding ---
	t.Run("encoding", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/encoding?input=Workers+Rock"))
		assertOK(t, r)
		var data struct {
			Input   string `json:"input"`
			Encoded string `json:"encoded"`
			Decoded string `json:"decoded"`
			Match   bool   `json:"match"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !data.Match {
			t.Errorf("encoding round-trip mismatch: input=%q decoded=%q", data.Input, data.Decoded)
		}
	})

	// --- GET /api/abort ---
	t.Run("abort", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/abort"))
		assertOK(t, r)
		var data struct {
			Events []string `json:"events"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		expected := []string{"before:false", "abort-fired", "after:true", "reason:test-reason", "static:true", "threw"}
		if len(data.Events) != len(expected) {
			t.Fatalf("events = %v, want %v", data.Events, expected)
		}
		for i, ev := range expected {
			if data.Events[i] != ev {
				t.Errorf("events[%d] = %q, want %q", i, data.Events[i], ev)
			}
		}
	})

	// --- GET /api/streams ---
	t.Run("streams", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/streams"))
		assertOK(t, r)
		if string(r.Response.Body) != "HELLO STREAMS!" {
			t.Errorf("body = %q, want 'HELLO STREAMS!'", r.Response.Body)
		}
	})

	// --- POST /api/formdata ---
	t.Run("formdata", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, &WorkerRequest{
			Method:  "POST",
			URL:     "http://localhost/api/formdata",
			Headers: map[string]string{},
		})
		assertOK(t, r)
		var data struct {
			Names     []string `json:"names"`
			Age       string   `json:"age"`
			DocName   string   `json:"docName"`
			DocType   string   `json:"docType"`
			BlobText  string   `json:"blobText"`
			BlobSize  int      `json:"blobSize"`
			SliceText string   `json:"sliceText"`
			HasFile   bool     `json:"hasFile"`
			HasBlob   bool     `json:"hasBlob"`
			FdHas     bool     `json:"fdHas"`
			FdDelete  bool     `json:"fdDelete"`
			FdSet     string   `json:"fdSet"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(data.Names) != 2 || data.Names[0] != "Alice" || data.Names[1] != "Bob" {
			t.Errorf("names = %v", data.Names)
		}
		if data.Age != "30" {
			t.Errorf("age = %q", data.Age)
		}
		if data.DocName != "test.txt" {
			t.Errorf("docName = %q", data.DocName)
		}
		if data.BlobText != "file content here" {
			t.Errorf("blobText = %q", data.BlobText)
		}
		if data.BlobSize != 17 {
			t.Errorf("blobSize = %d", data.BlobSize)
		}
		if data.SliceText != "file" {
			t.Errorf("sliceText = %q", data.SliceText)
		}
		if !data.HasFile {
			t.Error("document should be instanceof File")
		}
		if !data.HasBlob {
			t.Error("blob should be instanceof Blob")
		}
		if !data.FdHas {
			t.Error("FormData.has should be true")
		}
		if data.FdDelete {
			t.Error("FormData.has should be false after delete")
		}
		if data.FdSet != "Charlie" {
			t.Errorf("fdSet = %q, want Charlie", data.FdSet)
		}
	})

	// --- GET /api/timers ---
	t.Run("timers", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/timers"))
		assertOK(t, r)
		var data struct {
			Events []string `json:"events"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(data.Events) != 3 {
			t.Fatalf("events = %v, want 3 entries", data.Events)
		}
		if data.Events[0] != "timeout-fired" {
			t.Errorf("events[0] = %q", data.Events[0])
		}
		if data.Events[1] != "clearTimeout-works:true" {
			t.Errorf("events[1] = %q", data.Events[1])
		}
		if data.Events[2] != "microtask-fired" {
			t.Errorf("events[2] = %q", data.Events[2])
		}
	})

	// --- GET /api/events ---
	t.Run("events", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/events"))
		assertOK(t, r)
		var data struct {
			Events []string `json:"events"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// First dispatch: handler + once handler = 2 events
		// Second dispatch: handler only (once removed) = 1 event
		// Third dispatch: handler removed = 0 events
		// + cancelable check
		expected := []string{"event:custom", "event2", "event:custom", "cancelable:true"}
		if len(data.Events) != len(expected) {
			t.Fatalf("events = %v, want %v", data.Events, expected)
		}
		for i, ev := range expected {
			if data.Events[i] != ev {
				t.Errorf("events[%d] = %q, want %q", i, data.Events[i], ev)
			}
		}
	})

	// --- GET /api/domexception ---
	t.Run("domexception", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/domexception"))
		assertOK(t, r)
		var data struct {
			Name    string `json:"name"`
			Message string `json:"message"`
			IsError bool   `json:"isError"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if data.Name != "AbortError" {
			t.Errorf("name = %q", data.Name)
		}
		if data.Message != "test error" {
			t.Errorf("message = %q", data.Message)
		}
		if !data.IsError {
			t.Error("DOMException should be instanceof Error")
		}
	})

	// --- GET /api/all --- comprehensive check of all APIs at once
	t.Run("all-apis", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/api/all"))
		assertOK(t, r)
		var data struct {
			AllPassed bool            `json:"allPassed"`
			Checks    map[string]bool `json:"checks"`
			Count     int             `json:"count"`
		}
		if err := json.Unmarshal(r.Response.Body, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if !data.AllPassed {
			var failed []string
			for name, passed := range data.Checks {
				if !passed {
					failed = append(failed, name)
				}
			}
			t.Errorf("API checks failed: %s", strings.Join(failed, ", "))
		}
		t.Logf("all %d API checks passed: %v", data.Count, data.AllPassed)
	})

	// --- GET /unknown --- 404 fallback
	t.Run("404", func(t *testing.T) {
		r := execJS(t, e, honoStyleWorker, env, getReq("http://localhost/unknown"))
		assertOK(t, r)
		if r.Response.StatusCode != 404 {
			t.Errorf("status = %d, want 404", r.Response.StatusCode)
		}
	})
}
