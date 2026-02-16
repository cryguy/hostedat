package worker

import (
	"encoding/json"
	"testing"

	"github.com/cryguy/hostedat/internal/models"
)

// ---------------------------------------------------------------------------
// STORAGE binding integration tests — exercises the full JS → Go bridge
// ---------------------------------------------------------------------------

func storageTestSetup(t *testing.T) (*Engine, *Env) {
	t.Helper()
	db := testDB(t)
	// Also migrate StorageObject for the bridge.
	if err := db.AutoMigrate(&models.StorageObject{}); err != nil {
		t.Fatalf("auto-migrate StorageObject: %v", err)
	}
	e := newTestEngine(t, db)

	bridge := &StorageBridge{
		DB:          db,
		SiteID:      "storage-site",
		StoragePath: t.TempDir(),
	}
	env := &Env{
		Vars:          make(map[string]string),
		Secrets:       make(map[string]string),
		KVBindings:    make(map[string]string),
		StorageBridge: bridge,
	}
	return e, env
}

func TestStorageBinding_PutAndGet(t *testing.T) {
	e, env := storageTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.STORAGE.put("greeting.txt", "hello storage");
    const obj = await env.STORAGE.get("greeting.txt");
    const text = await obj.text();
    return Response.json({ text, key: obj.key, size: obj.size });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	json.Unmarshal(r.Response.Body, &data)
	if data["text"] != "hello storage" {
		t.Errorf("text = %v", data["text"])
	}
	if data["key"] != "greeting.txt" {
		t.Errorf("key = %v", data["key"])
	}
}

func TestStorageBinding_GetNull(t *testing.T) {
	e, env := storageTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    const obj = await env.STORAGE.get("missing.txt");
    return Response.json({ isNull: obj === null });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	json.Unmarshal(r.Response.Body, &data)
	if data["isNull"] != true {
		t.Errorf("isNull = %v, want true", data["isNull"])
	}
}

func TestStorageBinding_HeadReturnsMetadata(t *testing.T) {
	e, env := storageTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.STORAGE.put("meta.txt", "data", {
      httpMetadata: { contentType: "text/plain" },
      customMetadata: { author: "alice" },
    });
    const info = await env.STORAGE.head("meta.txt");
    return Response.json({
      key: info.key,
      size: info.size,
      contentType: info.httpMetadata.contentType,
      author: info.customMetadata.author,
    });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	json.Unmarshal(r.Response.Body, &data)
	if data["key"] != "meta.txt" {
		t.Errorf("key = %v", data["key"])
	}
	if data["contentType"] != "text/plain" {
		t.Errorf("contentType = %v", data["contentType"])
	}
	if data["author"] != "alice" {
		t.Errorf("author = %v", data["author"])
	}
}

func TestStorageBinding_HeadNull(t *testing.T) {
	e, env := storageTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    const info = await env.STORAGE.head("nope.txt");
    return Response.json({ isNull: info === null });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	json.Unmarshal(r.Response.Body, &data)
	if data["isNull"] != true {
		t.Errorf("isNull = %v", data["isNull"])
	}
}

func TestStorageBinding_Delete(t *testing.T) {
	e, env := storageTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.STORAGE.put("to-delete.txt", "bye");
    await env.STORAGE.delete("to-delete.txt");
    const obj = await env.STORAGE.get("to-delete.txt");
    return Response.json({ isNull: obj === null });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	json.Unmarshal(r.Response.Body, &data)
	if data["isNull"] != true {
		t.Errorf("isNull = %v, want true", data["isNull"])
	}
}

func TestStorageBinding_ListObjects(t *testing.T) {
	e, env := storageTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.STORAGE.put("docs/a.txt", "a");
    await env.STORAGE.put("docs/b.txt", "b");
    await env.STORAGE.put("images/c.png", "c");
    const result = await env.STORAGE.list({ prefix: "docs/" });
    return Response.json({
      count: result.objects.length,
      truncated: result.truncated,
    });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	json.Unmarshal(r.Response.Body, &data)
	if data["count"] != float64(2) {
		t.Errorf("count = %v, want 2", data["count"])
	}
	if data["truncated"] != false {
		t.Errorf("truncated = %v", data["truncated"])
	}
}

func TestStorageBinding_ListWithDelimiter(t *testing.T) {
	e, env := storageTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.STORAGE.put("photos/2024/a.jpg", "a");
    await env.STORAGE.put("photos/2024/b.jpg", "b");
    await env.STORAGE.put("photos/2025/c.jpg", "c");
    await env.STORAGE.put("photos/root.jpg", "d");
    const result = await env.STORAGE.list({ prefix: "photos/", delimiter: "/" });
    return Response.json({
      objectCount: result.objects ? result.objects.length : 0,
      prefixCount: result.delimitedPrefixes ? result.delimitedPrefixes.length : 0,
    });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	json.Unmarshal(r.Response.Body, &data)
	if data["objectCount"] != float64(1) {
		t.Errorf("objectCount = %v, want 1", data["objectCount"])
	}
	if data["prefixCount"] != float64(2) {
		t.Errorf("prefixCount = %v, want 2", data["prefixCount"])
	}
}

func TestStorageBinding_Overwrite(t *testing.T) {
	e, env := storageTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.STORAGE.put("key.txt", "v1");
    await env.STORAGE.put("key.txt", "v2-updated");
    const obj = await env.STORAGE.get("key.txt");
    const text = await obj.text();
    return Response.json({ text, size: obj.size });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	json.Unmarshal(r.Response.Body, &data)
	if data["text"] != "v2-updated" {
		t.Errorf("text = %v", data["text"])
	}
}

func TestStorageBinding_CustomContentType(t *testing.T) {
	e, env := storageTestSetup(t)

	source := `export default {
  async fetch(request, env) {
    await env.STORAGE.put("data.json", '{"ok":true}', {
      httpMetadata: { contentType: "application/json" },
    });
    const info = await env.STORAGE.head("data.json");
    return Response.json({ ct: info.httpMetadata.contentType });
  },
};`

	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data map[string]interface{}
	json.Unmarshal(r.Response.Body, &data)
	if data["ct"] != "application/json" {
		t.Errorf("contentType = %v", data["ct"])
	}
}
