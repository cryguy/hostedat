package worker

import (
	"encoding/json"
	"testing"
)

func TestFormData_AppendAndGet(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const fd = new FormData();
    fd.append("name", "alice");
    fd.append("age", "30");
    fd.append("name", "bob");
    return Response.json({
      name: fd.get("name"),
      age: fd.get("age"),
      allNames: fd.getAll("name"),
      has: fd.has("name"),
      missing: fd.has("missing"),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Name     string   `json:"name"`
		Age      string   `json:"age"`
		AllNames []string `json:"allNames"`
		Has      bool     `json:"has"`
		Missing  bool     `json:"missing"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Name != "alice" {
		t.Errorf("name = %q, want alice (first value)", data.Name)
	}
	if data.Age != "30" {
		t.Errorf("age = %q", data.Age)
	}
	if len(data.AllNames) != 2 {
		t.Errorf("allNames length = %d, want 2", len(data.AllNames))
	}
	if !data.Has {
		t.Error("has('name') should be true")
	}
	if data.Missing {
		t.Error("has('missing') should be false")
	}
}

func TestFormData_SetAndDelete(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const fd = new FormData();
    fd.append("key", "v1");
    fd.append("key", "v2");
    fd.set("key", "v3");
    const afterSet = fd.getAll("key");
    fd.delete("key");
    const afterDelete = fd.has("key");
    return Response.json({ afterSet, afterDelete });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		AfterSet    []string `json:"afterSet"`
		AfterDelete bool     `json:"afterDelete"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(data.AfterSet) != 1 || data.AfterSet[0] != "v3" {
		t.Errorf("afterSet = %v, want [v3]", data.AfterSet)
	}
	if data.AfterDelete {
		t.Error("has() should be false after delete()")
	}
}

func TestFormData_Iteration(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const fd = new FormData();
    fd.append("a", "1");
    fd.append("b", "2");
    fd.append("c", "3");

    const keys = [];
    const values = [];
    fd.forEach((value, key) => {
      keys.push(key);
      values.push(value);
    });
    return Response.json({ keys, values });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Keys   []string `json:"keys"`
		Values []string `json:"values"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(data.Keys) != 3 {
		t.Errorf("keys length = %d, want 3", len(data.Keys))
	}
	if len(data.Values) != 3 {
		t.Errorf("values length = %d, want 3", len(data.Values))
	}
}

func TestFormData_BlobBasics(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const blob = new Blob(["hello", " ", "world"], { type: "text/plain" });
    const text = await blob.text();
    return Response.json({
      text,
      size: blob.size,
      type: blob.type,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
		Size int    `json:"size"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Text != "hello world" {
		t.Errorf("text = %q", data.Text)
	}
	if data.Size != 11 {
		t.Errorf("size = %d, want 11", data.Size)
	}
	if data.Type != "text/plain" {
		t.Errorf("type = %q", data.Type)
	}
}

func TestFormData_BlobSlice(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const blob = new Blob(["0123456789"]);
    const sliced = blob.slice(2, 5);
    const text = await sliced.text();
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

	if data.Text != "234" {
		t.Errorf("sliced text = %q, want '234'", data.Text)
	}
}

func TestFormData_FileBasics(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const file = new File(["file content"], "test.txt", { type: "text/plain" });
    const text = await file.text();
    return Response.json({
      text,
      name: file.name,
      size: file.size,
      type: file.type,
      hasLastModified: typeof file.lastModified === 'number',
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text            string `json:"text"`
		Name            string `json:"name"`
		Size            int    `json:"size"`
		Type            string `json:"type"`
		HasLastModified bool   `json:"hasLastModified"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Text != "file content" {
		t.Errorf("text = %q", data.Text)
	}
	if data.Name != "test.txt" {
		t.Errorf("name = %q", data.Name)
	}
	if data.Type != "text/plain" {
		t.Errorf("type = %q", data.Type)
	}
	if !data.HasLastModified {
		t.Error("File should have lastModified property")
	}
}

func TestFormData_BlobArrayBuffer(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const blob = new Blob(["hello"]);
    const buf = await blob.arrayBuffer();
    const arr = new Uint8Array(buf);
    return Response.json({
      length: arr.length,
      first: arr[0],
      last: arr[arr.length - 1],
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Length int `json:"length"`
		First  int `json:"first"`
		Last   int `json:"last"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Length != 5 {
		t.Errorf("length = %d, want 5", data.Length)
	}
	if data.First != 104 { // 'h'
		t.Errorf("first = %d, want 104 ('h')", data.First)
	}
}

func TestFormData_Entries(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const fd = new FormData();
    fd.append("x", "1");
    fd.append("y", "2");
    const entries = [];
    for (const [key, value] of fd) {
      entries.push(key + "=" + value);
    }
    return Response.json({ entries });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Entries []string `json:"entries"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if len(data.Entries) != 2 {
		t.Errorf("entries length = %d, want 2", len(data.Entries))
	}
}

func TestFormData_Keys(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const fd = new FormData();
    fd.append("a", "1");
    fd.append("b", "2");
    fd.append("c", "3");
    const keys = [...fd.keys()];
    const values = [...fd.values()];
    return Response.json({ keys, values });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Keys   []string `json:"keys"`
		Values []string `json:"values"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if len(data.Keys) != 3 {
		t.Errorf("keys = %v", data.Keys)
	}
	if len(data.Values) != 3 {
		t.Errorf("values = %v", data.Values)
	}
}

func TestFormData_BlobNoType(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const blob = new Blob(["data"]);
    const text = await blob.text();
    return Response.json({
      text,
      type: blob.type,
      size: blob.size,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
		Type string `json:"type"`
		Size int    `json:"size"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Text != "data" {
		t.Errorf("text = %q", data.Text)
	}
	if data.Type != "" {
		t.Errorf("type should be empty, got %q", data.Type)
	}
	if data.Size != 4 {
		t.Errorf("size = %d, want 4", data.Size)
	}
}

func TestFormData_FileSlice(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const file = new File(["0123456789"], "numbers.txt");
    const sliced = file.slice(3, 7);
    const text = await sliced.text();
    return Response.json({ text, size: sliced.size });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Text string `json:"text"`
		Size int    `json:"size"`
	}
	json.Unmarshal(r.Response.Body, &data)
	if data.Text != "3456" {
		t.Errorf("sliced text = %q, want '3456'", data.Text)
	}
	if data.Size != 4 {
		t.Errorf("size = %d, want 4", data.Size)
	}
}
