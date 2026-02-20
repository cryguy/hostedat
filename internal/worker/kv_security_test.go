package worker

import (
	"encoding/json"
	"testing"
)

// TestKV_MetadataWithSpecialCharacters verifies that metadata with special
// characters (that could be JS injection) is safely interpolated.
func TestKV_MetadataWithSpecialCharacters(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    // Store with metadata containing characters that could be JS injection
    await env.MY_KV.put('test-key', 'hello', {
      metadata: { "key": "value\"}; console.log('injected'); //" }
    });
    const result = await env.MY_KV.getWithMetadata('test-key', { type: 'text' });
    return Response.json({
      value: result.value,
      metadata: result.metadata,
      metaType: typeof result.metadata,
    });
  },
};`

	env := kvEnv(t, db, "kv-meta-special")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Value    string      `json:"value"`
		Metadata interface{} `json:"metadata"`
		MetaType string      `json:"metaType"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Value != "hello" {
		t.Errorf("value = %q, want 'hello'", data.Value)
	}
	if data.MetaType != "object" {
		t.Errorf("metadata type = %q, want 'object'", data.MetaType)
	}

	// Verify the metadata is a proper object (not a string or corrupted)
	metaMap, ok := data.Metadata.(map[string]interface{})
	if !ok {
		t.Fatalf("metadata is not an object, got type %T", data.Metadata)
	}
	if metaMap["key"] != `value"}; console.log('injected'); //` {
		t.Errorf("metadata.key = %q, injection not properly escaped", metaMap["key"])
	}
}
