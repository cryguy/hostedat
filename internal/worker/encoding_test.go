package worker

import (
	"encoding/json"
	"testing"
)

func TestEncoding_BtoaAtobRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const encoded = btoa("Hello, World!");
    const decoded = atob(encoded);
    return Response.json({ encoded, decoded });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]string
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["encoded"] != "SGVsbG8sIFdvcmxkIQ==" {
		t.Errorf("encoded = %q, want SGVsbG8sIFdvcmxkIQ==", data["encoded"])
	}
	if data["decoded"] != "Hello, World!" {
		t.Errorf("decoded = %q, want Hello, World!", data["decoded"])
	}
}

func TestEncoding_BtoaBinaryString(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Test Latin-1 range 1-255 (skip 0 / null byte which truncates C strings
	// in the QuickJS WASM bridge).
	source := `export default {
  fetch(request, env) {
    let bin = '';
    for (let i = 1; i < 256; i++) bin += String.fromCharCode(i);
    const encoded = btoa(bin);
    const decoded = atob(encoded);
    return Response.json({ len: decoded.length, match: decoded === bin });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Len   int  `json:"len"`
		Match bool `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Len != 255 {
		t.Errorf("len = %d, want 255", data.Len)
	}
	if !data.Match {
		t.Error("round-trip mismatch for binary string")
	}
}

func TestEncoding_AtobInvalidInput(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    try {
      atob("not valid base64!@#$");
      return new Response("should not reach", { status: 200 });
    } catch(e) {
      return new Response("error: " + e.message, { status: 400 });
    }
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
	if r.Response.StatusCode != 400 {
		t.Errorf("status = %d, want 400", r.Response.StatusCode)
	}
}

func TestEncoding_BtoaEmptyString(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const encoded = btoa("");
    const decoded = atob(encoded);
    return Response.json({ encoded, decoded });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]string
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["encoded"] != "" {
		t.Errorf("encoded = %q, want empty", data["encoded"])
	}
	if data["decoded"] != "" {
		t.Errorf("decoded = %q, want empty", data["decoded"])
	}
}
