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

	// Test full Latin-1 range 0-255 including null byte.
	source := `export default {
  fetch(request, env) {
    let bin = '';
    for (let i = 0; i < 256; i++) bin += String.fromCharCode(i);
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
	if data.Len != 256 {
		t.Errorf("len = %d, want 256", data.Len)
	}
	if !data.Match {
		t.Error("round-trip mismatch for binary string")
	}
}

// TestEncoding_NullByteRoundTrip is a deterministic regression test for the
// null-byte truncation bug. Strings with embedded 0x00 bytes must survive
// the btoa/atob round-trip without corruption.
func TestEncoding_NullByteRoundTrip(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    // Build a string with null bytes at specific positions.
    const s = String.fromCharCode(0x41, 0x00, 0x42, 0x00, 0x00, 0x43);
    const encoded = btoa(s);
    const decoded = atob(encoded);
    // Verify length and each code point individually.
    const codes = [];
    for (let i = 0; i < decoded.length; i++) codes.push(decoded.charCodeAt(i));
    return Response.json({
      inputLen: s.length,
      outputLen: decoded.length,
      match: decoded === s,
      encoded,
      codes,
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		InputLen  int    `json:"inputLen"`
		OutputLen int    `json:"outputLen"`
		Match     bool   `json:"match"`
		Encoded   string `json:"encoded"`
		Codes     []int  `json:"codes"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.InputLen != 6 {
		t.Errorf("inputLen = %d, want 6", data.InputLen)
	}
	if data.OutputLen != 6 {
		t.Errorf("outputLen = %d, want 6", data.OutputLen)
	}
	if !data.Match {
		t.Error("null byte round-trip mismatch")
	}
	// btoa("\x41\x00\x42\x00\x00\x43") should produce "QQBCAABD"
	if data.Encoded != "QQBCAABD" {
		t.Errorf("encoded = %q, want QQBCAABD", data.Encoded)
	}
	expected := []int{0x41, 0x00, 0x42, 0x00, 0x00, 0x43}
	for i, want := range expected {
		if i >= len(data.Codes) {
			t.Errorf("codes[%d]: missing (decoded too short)", i)
		} else if data.Codes[i] != want {
			t.Errorf("codes[%d] = 0x%02x, want 0x%02x", i, data.Codes[i], want)
		}
	}
}

func TestEncoding_AtobPaddingTolerance(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Browsers accept base64 without padding. "aGVsbG8" is "hello" without the trailing "=".
	source := `export default {
  fetch(request, env) {
    const withPad = atob("aGVsbG8=");
    const withoutPad = atob("aGVsbG8");
    return Response.json({
      withPad,
      withoutPad,
      match: withPad === withoutPad && withPad === "hello",
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		WithPad    string `json:"withPad"`
		WithoutPad string `json:"withoutPad"`
		Match      bool   `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Errorf("padding tolerance failed: withPad=%q withoutPad=%q", data.WithPad, data.WithoutPad)
	}
}

func TestEncoding_AtobWhitespaceTolerance(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    // Browsers strip ASCII whitespace before decoding.
    const decoded = atob("  aGVs\n\tbG8=\r\n");
    return Response.json({ decoded, match: decoded === "hello" });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Decoded string `json:"decoded"`
		Match   bool   `json:"match"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.Match {
		t.Errorf("whitespace tolerance failed: decoded=%q", data.Decoded)
	}
}

func TestEncoding_AtobRejectsInvalidLength(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// "A" has unpadded length 1 (mod 4 === 1), which is invalid per the HTML spec.
	source := `export default {
  fetch(request, env) {
    const results = [];
    const inputs = ["A", "AAAAA"];
    for (const input of inputs) {
      try {
        atob(input);
        results.push({ input, threw: false });
      } catch(e) {
        results.push({ input, threw: true });
      }
    }
    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results []struct {
		Input string `json:"input"`
		Threw bool   `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, r := range results {
		if !r.Threw {
			t.Errorf("atob(%q) should throw for invalid length", r.Input)
		}
	}
}

func TestEncoding_BtoaRejectsNonLatin1(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    const results = [];
    const inputs = ["\u0100", "\u4e16\u754c", "\ud83d\ude00"];
    for (const input of inputs) {
      try {
        btoa(input);
        results.push({ threw: false });
      } catch(e) {
        results.push({ threw: true });
      }
    }
    return Response.json(results);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var results []struct {
		Threw bool `json:"threw"`
	}
	if err := json.Unmarshal(r.Response.Body, &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for i, r := range results {
		if !r.Threw {
			t.Errorf("btoa(non-Latin1 input %d) should throw", i)
		}
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
