package worker

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// Go-level D1Bridge tests
// ---------------------------------------------------------------------------

func TestD1Bridge_CreateTableAndInsert(t *testing.T) {
	bridge, err := NewD1BridgeMemory("test1")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	// Create table
	_, err = bridge.Exec("CREATE TABLE d1_users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)", nil)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// Insert
	result, err := bridge.Exec("INSERT INTO d1_users (name, age) VALUES (?, ?)", []interface{}{"alice", 30})
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if result.Meta.Changes != 1 {
		t.Errorf("changes = %d, want 1", result.Meta.Changes)
	}
	if !result.Meta.ChangedDB {
		t.Error("ChangedDB should be true after INSERT")
	}
}

func TestD1Bridge_SelectQuery(t *testing.T) {
	bridge, err := NewD1BridgeMemory("test2")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	_, _ = bridge.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)", nil)
	_, _ = bridge.Exec("INSERT INTO items (name) VALUES (?)", []interface{}{"apple"})
	_, _ = bridge.Exec("INSERT INTO items (name) VALUES (?)", []interface{}{"banana"})

	result, err := bridge.Exec("SELECT id, name FROM items ORDER BY name", nil)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if len(result.Columns) != 2 {
		t.Fatalf("columns = %d, want 2", len(result.Columns))
	}
	if result.Columns[0] != "id" || result.Columns[1] != "name" {
		t.Errorf("columns = %v, want [id, name]", result.Columns)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(result.Rows))
	}
	if result.Meta.RowsRead != 2 {
		t.Errorf("rows_read = %d, want 2", result.Meta.RowsRead)
	}
}

func TestD1Bridge_SelectWithBindings(t *testing.T) {
	bridge, err := NewD1BridgeMemory("test3")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	_, _ = bridge.Exec("CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, price REAL)", nil)
	_, _ = bridge.Exec("INSERT INTO products (name, price) VALUES (?, ?)", []interface{}{"widget", 9.99})
	_, _ = bridge.Exec("INSERT INTO products (name, price) VALUES (?, ?)", []interface{}{"gadget", 19.99})

	result, err := bridge.Exec("SELECT name, price FROM products WHERE price > ?", []interface{}{10.0})
	if err != nil {
		t.Fatalf("SELECT with binding: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(result.Rows))
	}
}

func TestD1Bridge_UpdateAndDelete(t *testing.T) {
	bridge, err := NewD1BridgeMemory("test4")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	_, _ = bridge.Exec("CREATE TABLE kv (key TEXT PRIMARY KEY, value TEXT)", nil)
	_, _ = bridge.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []interface{}{"a", "1"})
	_, _ = bridge.Exec("INSERT INTO kv (key, value) VALUES (?, ?)", []interface{}{"b", "2"})

	// Update
	result, err := bridge.Exec("UPDATE kv SET value = ? WHERE key = ?", []interface{}{"updated", "a"})
	if err != nil {
		t.Fatalf("UPDATE: %v", err)
	}
	if result.Meta.Changes != 1 {
		t.Errorf("update changes = %d, want 1", result.Meta.Changes)
	}

	// Delete
	result, err = bridge.Exec("DELETE FROM kv WHERE key = ?", []interface{}{"b"})
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if result.Meta.Changes != 1 {
		t.Errorf("delete changes = %d, want 1", result.Meta.Changes)
	}

	// Verify only "a" remains with updated value
	result, err = bridge.Exec("SELECT key, value FROM kv", nil)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(result.Rows))
	}
}

func TestD1Bridge_EmptyResult(t *testing.T) {
	bridge, err := NewD1BridgeMemory("test5")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	_, _ = bridge.Exec("CREATE TABLE empty_tbl (id INTEGER PRIMARY KEY)", nil)

	result, err := bridge.Exec("SELECT * FROM empty_tbl", nil)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("rows = %d, want 0", len(result.Rows))
	}
}

func TestD1Bridge_SQLError(t *testing.T) {
	bridge, err := NewD1BridgeMemory("test6")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	_, err = bridge.Exec("SELECT * FROM nonexistent_table", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}
}

// ---------------------------------------------------------------------------
// JS-level D1 binding tests
// ---------------------------------------------------------------------------

// d1Env creates an Env with a D1 database binding.
func d1Env(dbID string) *Env {
	return &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		D1Bindings: map[string]string{"DB": dbID},
	}
}

func TestD1_JSPrepareAndAll(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE d1_users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)");
    const insert = env.DB.prepare("INSERT INTO d1_users (name, email) VALUES (?, ?)");
    await insert.bind("alice", "alice@example.com").run();
    await insert.bind("bob", "bob@example.com").run();

    const result = await env.DB.prepare("SELECT name, email FROM d1_users ORDER BY name").all();
    return Response.json({
      count: result.results.length,
      success: result.success,
      first: result.results[0],
      second: result.results[1],
    });
  },
};`

	env := d1Env("js-test-1")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count   int                    `json:"count"`
		Success bool                   `json:"success"`
		First   map[string]interface{} `json:"first"`
		Second  map[string]interface{} `json:"second"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Count != 2 {
		t.Errorf("count = %d, want 2", data.Count)
	}
	if !data.Success {
		t.Error("success should be true")
	}
	if data.First["name"] != "alice" {
		t.Errorf("first name = %v, want alice", data.First["name"])
	}
	if data.Second["email"] != "bob@example.com" {
		t.Errorf("second email = %v", data.Second["email"])
	}
}

func TestD1_JSFirst(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)");
    await env.DB.prepare("INSERT INTO items (name) VALUES (?)").bind("widget").run();
    await env.DB.prepare("INSERT INTO items (name) VALUES (?)").bind("gadget").run();

    const row = await env.DB.prepare("SELECT name FROM items ORDER BY name LIMIT 1").first();
    const name = await env.DB.prepare("SELECT name FROM items ORDER BY name LIMIT 1").first("name");
    const missing = await env.DB.prepare("SELECT name FROM items WHERE id = 999").first();

    return Response.json({ row, name, missing });
  },
};`

	env := d1Env("js-test-2")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Row     map[string]interface{} `json:"row"`
		Name    string                 `json:"name"`
		Missing interface{}            `json:"missing"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Row["name"] != "gadget" {
		t.Errorf("row.name = %v, want gadget", data.Row["name"])
	}
	if data.Name != "gadget" {
		t.Errorf("name = %q, want gadget", data.Name)
	}
	if data.Missing != nil {
		t.Errorf("missing = %v, want null", data.Missing)
	}
}

func TestD1_JSRaw(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE data (id INTEGER PRIMARY KEY, val TEXT)");
    await env.DB.prepare("INSERT INTO data (val) VALUES (?)").bind("x").run();
    await env.DB.prepare("INSERT INTO data (val) VALUES (?)").bind("y").run();

    const raw = await env.DB.prepare("SELECT id, val FROM data ORDER BY val").raw();
    const rawWithCols = await env.DB.prepare("SELECT id, val FROM data ORDER BY val")
      .raw({ columnNames: true });

    return Response.json({
      rawCount: raw.length,
      rawFirst: raw[0],
      withColsCount: rawWithCols.length,
      header: rawWithCols[0],
    });
  },
};`

	env := d1Env("js-test-3")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		RawCount      int           `json:"rawCount"`
		RawFirst      []interface{} `json:"rawFirst"`
		WithColsCount int           `json:"withColsCount"`
		Header        []interface{} `json:"header"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.RawCount != 2 {
		t.Errorf("rawCount = %d, want 2", data.RawCount)
	}
	if data.WithColsCount != 3 { // 1 header + 2 data rows
		t.Errorf("withColsCount = %d, want 3", data.WithColsCount)
	}
	if len(data.Header) < 2 || data.Header[0] != "id" || data.Header[1] != "val" {
		t.Errorf("header = %v, want [id, val]", data.Header)
	}
}

func TestD1_JSRun(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE counter (id INTEGER PRIMARY KEY, n INTEGER)");
    const result = await env.DB.prepare("INSERT INTO counter (n) VALUES (?)").bind(42).run();
    return Response.json({
      success: result.success,
      hasMeta: result.meta !== undefined,
    });
  },
};`

	env := d1Env("js-test-4")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Success bool `json:"success"`
		HasMeta bool `json:"hasMeta"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Success {
		t.Error("success should be true")
	}
	if !data.HasMeta {
		t.Error("should have meta")
	}
}

func TestD1_JSBatch(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE batch_test (id INTEGER PRIMARY KEY, name TEXT)");

    const results = await env.DB.batch([
      env.DB.prepare("INSERT INTO batch_test (name) VALUES (?)").bind("a"),
      env.DB.prepare("INSERT INTO batch_test (name) VALUES (?)").bind("b"),
      env.DB.prepare("INSERT INTO batch_test (name) VALUES (?)").bind("c"),
      env.DB.prepare("SELECT name FROM batch_test ORDER BY name"),
    ]);

    return Response.json({
      batchLen: results.length,
      lastSuccess: results[3].success,
      selectCount: results[3].results.length,
      names: results[3].results.map(r => r.name),
    });
  },
};`

	env := d1Env("js-test-5")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		BatchLen    int      `json:"batchLen"`
		LastSuccess bool     `json:"lastSuccess"`
		SelectCount int      `json:"selectCount"`
		Names       []string `json:"names"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.BatchLen != 4 {
		t.Errorf("batchLen = %d, want 4", data.BatchLen)
	}
	if !data.LastSuccess {
		t.Error("last batch result should be success")
	}
	if data.SelectCount != 3 {
		t.Errorf("selectCount = %d, want 3", data.SelectCount)
	}
}

func TestD1_JSExecMultiStatement(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    const result = await env.DB.exec(
      "CREATE TABLE multi (id INTEGER PRIMARY KEY, v TEXT); " +
      "INSERT INTO multi (v) VALUES ('x'); " +
      "INSERT INTO multi (v) VALUES ('y')"
    );

    const rows = await env.DB.prepare("SELECT v FROM multi ORDER BY v").all();

    return Response.json({
      execCount: result.count,
      rowCount: rows.results.length,
      values: rows.results.map(r => r.v),
    });
  },
};`

	env := d1Env("js-test-6")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		ExecCount int      `json:"execCount"`
		RowCount  int      `json:"rowCount"`
		Values    []string `json:"values"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.ExecCount != 3 {
		t.Errorf("execCount = %d, want 3", data.ExecCount)
	}
	if data.RowCount != 2 {
		t.Errorf("rowCount = %d, want 2", data.RowCount)
	}
}

func TestD1_JSSQLError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await env.DB.prepare("SELECT * FROM nonexistent").all();
      return Response.json({ caught: false });
    } catch(e) {
      return Response.json({ caught: true, msg: e.message });
    }
  },
};`

	env := d1Env("js-test-7")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught bool   `json:"caught"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Caught {
		t.Error("SQL error should be caught")
	}
	if data.Msg == "" {
		t.Error("error message should not be empty")
	}
}

func TestD1_JSBindTypes(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE typed (id INTEGER PRIMARY KEY, txt TEXT, num REAL, flag INTEGER)");
    await env.DB.prepare("INSERT INTO typed (txt, num, flag) VALUES (?, ?, ?)")
      .bind("hello", 3.14, 1)
      .run();

    const row = await env.DB.prepare("SELECT txt, num, flag FROM typed").first();
    return Response.json({
      txt: row.txt,
      num: row.num,
      flag: row.flag,
    });
  },
};`

	env := d1Env("js-test-8")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Txt  string  `json:"txt"`
		Num  float64 `json:"num"`
		Flag int     `json:"flag"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Txt != "hello" {
		t.Errorf("txt = %q, want hello", data.Txt)
	}
	if data.Num != 3.14 {
		t.Errorf("num = %f, want 3.14", data.Num)
	}
	if data.Flag != 1 {
		t.Errorf("flag = %d, want 1", data.Flag)
	}
}

func TestD1_JSNullHandling(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE nullable (id INTEGER PRIMARY KEY, val TEXT)");
    await env.DB.prepare("INSERT INTO nullable (val) VALUES (?)").bind(null).run();

    const row = await env.DB.prepare("SELECT val FROM nullable").first();
    return Response.json({ val: row.val, isNull: row.val === null });
  },
};`

	env := d1Env("js-test-9")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Val    interface{} `json:"val"`
		IsNull bool        `json:"isNull"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsNull {
		t.Errorf("val should be null, got %v", data.Val)
	}
}

func TestD1_JSMultipleDatabases(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB1.exec("CREATE TABLE t1 (val TEXT)");
    await env.DB2.exec("CREATE TABLE t2 (val TEXT)");

    await env.DB1.prepare("INSERT INTO t1 (val) VALUES (?)").bind("from-db1").run();
    await env.DB2.prepare("INSERT INTO t2 (val) VALUES (?)").bind("from-db2").run();

    const r1 = await env.DB1.prepare("SELECT val FROM t1").first();
    const r2 = await env.DB2.prepare("SELECT val FROM t2").first();

    return Response.json({ v1: r1.val, v2: r2.val });
  },
};`

	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
		D1Bindings: map[string]string{"DB1": "multi-db1", "DB2": "multi-db2"},
	}
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		V1 string `json:"v1"`
		V2 string `json:"v2"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.V1 != "from-db1" {
		t.Errorf("v1 = %q, want from-db1", data.V1)
	}
	if data.V2 != "from-db2" {
		t.Errorf("v2 = %q, want from-db2", data.V2)
	}
}

func TestD1_JSPrepareReuse(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE reuse (id INTEGER PRIMARY KEY, name TEXT)");
    const stmt = env.DB.prepare("INSERT INTO reuse (name) VALUES (?)");

    // Each .bind() should return a new statement, not mutate the original.
    await stmt.bind("alice").run();
    await stmt.bind("bob").run();
    await stmt.bind("charlie").run();

    const result = await env.DB.prepare("SELECT name FROM reuse ORDER BY name").all();
    return Response.json({
      count: result.results.length,
      names: result.results.map(r => r.name),
    });
  },
};`

	env := d1Env("js-test-reuse")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count int      `json:"count"`
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Count != 3 {
		t.Errorf("count = %d, want 3", data.Count)
	}
}

func TestD1_JSMetaFields(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE meta_test (id INTEGER PRIMARY KEY, v TEXT)");
    const insertResult = await env.DB.prepare("INSERT INTO meta_test (v) VALUES (?)").bind("x").run();
    const selectResult = await env.DB.prepare("SELECT * FROM meta_test").all();

    return Response.json({
      insertMeta: insertResult.meta,
      selectMeta: selectResult.meta,
    });
  },
};`

	env := d1Env("js-test-meta")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		InsertMeta D1Meta `json:"insertMeta"`
		SelectMeta D1Meta `json:"selectMeta"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.InsertMeta.ChangedDB {
		t.Error("insert meta.changed_db should be true")
	}
	if data.InsertMeta.Changes != 1 {
		t.Errorf("insert meta.changes = %d, want 1", data.InsertMeta.Changes)
	}
	if data.SelectMeta.RowsRead != 1 {
		t.Errorf("select meta.rows_read = %d, want 1", data.SelectMeta.RowsRead)
	}
}

func TestD1_JSDumpRejected(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await env.DB.dump();
      return Response.json({ caught: false, msg: "" });
    } catch(e) {
      return Response.json({ caught: true, msg: e.message });
    }
  },
};`

	env := d1Env("js-test-dump")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught bool   `json:"caught"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Caught {
		t.Error("dump() should reject")
	}
	if data.Msg != "D1 dump() is not supported in this runtime" {
		t.Errorf("dump() message = %q, want \"D1 dump() is not supported in this runtime\"", data.Msg)
	}
}

func TestD1_JSBatchFailingStatement(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE batch_fail (id INTEGER PRIMARY KEY, name TEXT)");
    try {
      await env.DB.batch([
        env.DB.prepare("INSERT INTO batch_fail (name) VALUES (?)").bind("a"),
        env.DB.prepare("SELECT * FROM nonexistent_table_xyz"),
        env.DB.prepare("INSERT INTO batch_fail (name) VALUES (?)").bind("c"),
      ]);
      return Response.json({ caught: false, msg: "" });
    } catch(e) {
      return Response.json({ caught: true, msg: e.message });
    }
  },
};`

	env := d1Env("js-test-batch-fail")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Caught bool   `json:"caught"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Caught {
		t.Error("batch with failing statement should reject")
	}
	if data.Msg == "" {
		t.Error("batch error message should not be empty")
	}
}

func TestD1_JSFirstNonExistentColumn(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE first_col (id INTEGER PRIMARY KEY, name TEXT)");
    await env.DB.prepare("INSERT INTO first_col (name) VALUES (?)").bind("hello").run();

    const val = await env.DB.prepare("SELECT name FROM first_col").first("nonexistent");
    return Response.json({ val: val, isNull: val === null });
  },
};`

	env := d1Env("js-test-first-col")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Val    interface{} `json:"val"`
		IsNull bool        `json:"isNull"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.IsNull {
		t.Errorf("first(\"nonexistent\") should return null, got %v", data.Val)
	}
}

func TestD1_JSCTEQuery(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE cte_src (id INTEGER PRIMARY KEY, val INTEGER)");
    await env.DB.prepare("INSERT INTO cte_src (val) VALUES (?)").bind(10).run();
    await env.DB.prepare("INSERT INTO cte_src (val) VALUES (?)").bind(20).run();
    await env.DB.prepare("INSERT INTO cte_src (val) VALUES (?)").bind(30).run();

    const result = await env.DB.prepare(
      "WITH doubled AS (SELECT id, val * 2 AS dval FROM cte_src) SELECT dval FROM doubled ORDER BY dval"
    ).all();

    return Response.json({
      count: result.results.length,
      success: result.success,
      values: result.results.map(r => r.dval),
    });
  },
};`

	env := d1Env("js-test-cte")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count   int     `json:"count"`
		Success bool    `json:"success"`
		Values  []int64 `json:"values"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Count != 3 {
		t.Errorf("count = %d, want 3", data.Count)
	}
	if !data.Success {
		t.Error("success should be true")
	}
	if len(data.Values) != 3 || data.Values[0] != 20 || data.Values[1] != 40 || data.Values[2] != 60 {
		t.Errorf("values = %v, want [20 40 60]", data.Values)
	}
}

func TestD1_JSBindZeroArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    await env.DB.exec("CREATE TABLE bind_zero (id INTEGER PRIMARY KEY, name TEXT)");
    await env.DB.prepare("INSERT INTO bind_zero (name) VALUES ('static')").bind().run();

    const result = await env.DB.prepare("SELECT name FROM bind_zero").bind().all();
    return Response.json({
      count: result.results.length,
      success: result.success,
      name: result.results[0].name,
    });
  },
};`

	env := d1Env("js-test-bind-zero")
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Count   int    `json:"count"`
		Success bool   `json:"success"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if data.Count != 1 {
		t.Errorf("count = %d, want 1", data.Count)
	}
	if !data.Success {
		t.Error("success should be true")
	}
	if data.Name != "static" {
		t.Errorf("name = %q, want static", data.Name)
	}
}
