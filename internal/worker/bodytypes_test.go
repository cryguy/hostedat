package worker

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// Request body types (Gap 5)
// ---------------------------------------------------------------------------

func TestBodyTypes_ArrayBufferBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const buf = new TextEncoder().encode("binary data").buffer;
    const req = new Request("https://example.com", { method: "POST", body: buf });
    const text = await req.text();
    return Response.json({ text });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Text != "binary data" {
		t.Errorf("ArrayBuffer body: got %q, want %q", data.Text, "binary data")
	}
}

func TestBodyTypes_TypedArrayBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const arr = new TextEncoder().encode("typed array body");
    const req = new Request("https://example.com", { method: "POST", body: arr });
    const text = await req.text();
    return Response.json({ text });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Text != "typed array body" {
		t.Errorf("TypedArray body: got %q, want %q", data.Text, "typed array body")
	}
}

func TestBodyTypes_URLSearchParamsBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const usp = new URLSearchParams();
    usp.append("q", "search term");
    const req = new Request("https://example.com", { method: "POST", body: usp });
    const text = await req.text();
    return Response.json({ text });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Text != "q=search%20term" && data.Text != "q=search+term" {
		t.Errorf("URLSearchParams body text = %q", data.Text)
	}
}

func TestBodyTypes_BlobBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const blob = new Blob(["blob body content"], { type: "text/plain" });
    const req = new Request("https://example.com", { method: "POST", body: blob });
    const text = await req.text();
    return Response.json({ text });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Text != "blob body content" {
		t.Errorf("Blob body text = %q, want %q", data.Text, "blob body content")
	}
}

func TestBodyTypes_ReadableStreamBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("hello "));
        controller.enqueue(new TextEncoder().encode("world"));
        controller.close();
      }
    });
    const resp = new Response(stream);
    const text = await resp.text();
    return Response.json({ text });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Text != "hello world" {
		t.Errorf("ReadableStream body: got %q, want %q", data.Text, "hello world")
	}
}

func TestBodyTypes_ResponseBlobBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const blob = new Blob(["response blob"], { type: "application/octet-stream" });
    const resp = new Response(blob);
    const text = await resp.text();
    return Response.json({ text });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Text != "response blob" {
		t.Errorf("Response Blob body: got %q, want %q", data.Text, "response blob")
	}
}

func TestBodyTypes_ArrayBufferRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const buf = new TextEncoder().encode("binary roundtrip").buffer;
    const req = new Request("https://example.com", { method: "POST", body: buf });
    const ab = await req.arrayBuffer();
    const result = new TextDecoder().decode(ab);
    return Response.json({ result });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Result != "binary roundtrip" {
		t.Errorf("ArrayBuffer roundtrip: got %q, want %q", data.Result, "binary roundtrip")
	}
}

// ---------------------------------------------------------------------------
// formData() parsing (Gap 5)
// ---------------------------------------------------------------------------

func TestBodyTypes_FormDataParsing_URLEncoded(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const fd = await request.formData();
    return Response.json({
      name: fd.get("name"),
      age: fd.get("age"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/",
		Headers: map[string]string{"content-type": "application/x-www-form-urlencoded"},
		Body:    []byte("name=Alice&age=30"),
	})
	assertOK(t, r)

	var data struct {
		Name string `json:"name"`
		Age  string `json:"age"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Name != "Alice" {
		t.Errorf("name = %q, want Alice", data.Name)
	}
	if data.Age != "30" {
		t.Errorf("age = %q, want 30", data.Age)
	}
}

func TestBodyTypes_FormDataParsing_Multipart(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const fd = await request.formData();
    return Response.json({
      field1: fd.get("field1"),
      field2: fd.get("field2"),
    });
  },
};`

	body := "--boundary123\r\n" +
		"Content-Disposition: form-data; name=\"field1\"\r\n\r\n" +
		"value1\r\n" +
		"--boundary123\r\n" +
		"Content-Disposition: form-data; name=\"field2\"\r\n\r\n" +
		"value2\r\n" +
		"--boundary123--\r\n"

	r := execJS(t, e, source, defaultEnv(), &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/",
		Headers: map[string]string{"content-type": "multipart/form-data; boundary=boundary123"},
		Body:    []byte(body),
	})
	assertOK(t, r)

	var data struct {
		Field1 string `json:"field1"`
		Field2 string `json:"field2"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Field1 != "value1" {
		t.Errorf("field1 = %q, want value1", data.Field1)
	}
	if data.Field2 != "value2" {
		t.Errorf("field2 = %q, want value2", data.Field2)
	}
}

func TestBodyTypes_FormDataRejectsNonFormContentType(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await request.formData();
      return Response.json({ error: false });
    } catch(e) {
      return Response.json({ error: true, message: e.message });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    []byte(`{"key":"value"}`),
	})
	assertOK(t, r)

	var data struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Error {
		t.Error("formData() on JSON content-type should throw TypeError")
	}
}

// ---------------------------------------------------------------------------
// Streams getReader protocol (Gap 3)
// ---------------------------------------------------------------------------

func TestBodyTypes_StreamGetReaderProtocol(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue("chunk1");
        controller.enqueue("chunk2");
        controller.close();
      }
    });
    const reader = stream.getReader();
    const r1 = await reader.read();
    const r2 = await reader.read();
    const r3 = await reader.read();
    return Response.json({
      r1Value: r1.value, r1Done: r1.done,
      r2Value: r2.value, r2Done: r2.done,
      r3Done: r3.done, r3ValueUndef: r3.value === undefined,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		R1Value    string `json:"r1Value"`
		R1Done     bool   `json:"r1Done"`
		R2Value    string `json:"r2Value"`
		R2Done     bool   `json:"r2Done"`
		R3Done     bool   `json:"r3Done"`
		R3ValUndef bool   `json:"r3ValueUndef"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.R1Value != "chunk1" || data.R1Done {
		t.Errorf("read 1: value=%q done=%v", data.R1Value, data.R1Done)
	}
	if data.R2Value != "chunk2" || data.R2Done {
		t.Errorf("read 2: value=%q done=%v", data.R2Value, data.R2Done)
	}
	if !data.R3Done {
		t.Error("read 3 should be done")
	}
	if !data.R3ValUndef {
		t.Error("read 3 value should be undefined")
	}
}

func TestBodyTypes_ResponseJsonParsing(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const resp = new Response('{"key":"value","num":42}');
    const data = await resp.json();
    return Response.json({ key: data.key, num: data.num });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Key string `json:"key"`
		Num int    `json:"num"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Key != "value" {
		t.Errorf("key = %q, want value", data.Key)
	}
	if data.Num != 42 {
		t.Errorf("num = %d, want 42", data.Num)
	}
}

func TestBodyTypes_RequestJsonParsing(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const data = await request.json();
    return Response.json({ name: data.name, age: data.age });
  },
};`

	r := execJS(t, e, source, defaultEnv(), &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    []byte(`{"name":"Alice","age":30}`),
	})
	assertOK(t, r)

	var data struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Name != "Alice" {
		t.Errorf("name = %q, want Alice", data.Name)
	}
	if data.Age != 30 {
		t.Errorf("age = %d, want 30", data.Age)
	}
}

func TestBodyTypes_ResponseArrayBufferFromArrayBuffer(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const buf = new TextEncoder().encode("hello ab").buffer;
    const resp = new Response(buf);
    const ab = await resp.arrayBuffer();
    const decoded = new TextDecoder().decode(ab);
    return Response.json({ decoded, isAB: ab instanceof ArrayBuffer });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Decoded string `json:"decoded"`
		IsAB    bool   `json:"isAB"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Decoded != "hello ab" {
		t.Errorf("decoded = %q, want 'hello ab'", data.Decoded)
	}
	if !data.IsAB {
		t.Error("arrayBuffer() should return an ArrayBuffer")
	}
}

func TestBodyTypes_FormDataWithFileUpload(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const fd = await request.formData();
    const file = fd.get("myfile");
    const text = fd.get("field");
    return Response.json({
      fileName: file ? file.name : "",
      fileContent: file ? await file.text() : "",
      field: text,
    });
  },
};`

	body := "--myboundary\r\n" +
		"Content-Disposition: form-data; name=\"field\"\r\n\r\n" +
		"hello\r\n" +
		"--myboundary\r\n" +
		"Content-Disposition: form-data; name=\"myfile\"; filename=\"test.txt\"\r\n" +
		"Content-Type: text/plain\r\n\r\n" +
		"file contents here\r\n" +
		"--myboundary--\r\n"

	r := execJS(t, e, source, defaultEnv(), &WorkerRequest{
		Method:  "POST",
		URL:     "http://localhost/",
		Headers: map[string]string{"content-type": "multipart/form-data; boundary=myboundary"},
		Body:    []byte(body),
	})
	assertOK(t, r)

	var data struct {
		FileName    string `json:"fileName"`
		FileContent string `json:"fileContent"`
		Field       string `json:"field"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.FileName != "test.txt" {
		t.Errorf("fileName = %q, want test.txt", data.FileName)
	}
	if data.FileContent != "file contents here" {
		t.Errorf("fileContent = %q, want 'file contents here'", data.FileContent)
	}
	if data.Field != "hello" {
		t.Errorf("field = %q, want hello", data.Field)
	}
}

func TestBodyTypes_FormDataBodySerialization(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const fd = new FormData();
    fd.append("key", "value");
    fd.append("file", new File(["content"], "doc.txt", { type: "text/plain" }));
    const req = new Request("https://example.com", { method: "POST", body: fd });
    const text = await req.text();
    return Response.json({
      hasKey: text.indexOf("key") !== -1,
      hasValue: text.indexOf("value") !== -1,
      hasFilename: text.indexOf("doc.txt") !== -1,
      hasContent: text.indexOf("content") !== -1,
      hasBoundary: text.indexOf("----FormDataBoundary") !== -1,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasKey      bool `json:"hasKey"`
		HasValue    bool `json:"hasValue"`
		HasFilename bool `json:"hasFilename"`
		HasContent  bool `json:"hasContent"`
		HasBoundary bool `json:"hasBoundary"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.HasKey {
		t.Error("serialized FormData should contain key name")
	}
	if !data.HasValue {
		t.Error("serialized FormData should contain field value")
	}
	if !data.HasFilename {
		t.Error("serialized FormData should contain filename")
	}
	if !data.HasContent {
		t.Error("serialized FormData should contain file content")
	}
	if !data.HasBoundary {
		t.Error("serialized FormData should contain boundary")
	}
}

func TestBodyTypes_NullBodyReturnsEmpty(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const resp = new Response(null);
    const text = await resp.text();
    const ab = await new Response(null).arrayBuffer();
    return Response.json({
      text: text,
      abLen: ab.byteLength,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text  string `json:"text"`
		ABLen int    `json:"abLen"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Text != "" {
		t.Errorf("null body text = %q, want empty", data.Text)
	}
	if data.ABLen != 0 {
		t.Errorf("null body arrayBuffer length = %d, want 0", data.ABLen)
	}
}
