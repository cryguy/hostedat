package worker

import (
	"encoding/json"
	"testing"
)

func queueTestDB(t *testing.T) *QueueBridge {
	t.Helper()
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	return &QueueBridge{DB: db, SiteID: "test-site"}
}

func TestQueueBridge_Send(t *testing.T) {
	bridge := queueTestDB(t)

	id, err := bridge.Send("my-queue", `{"key":"value"}`, "json")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id == "" {
		t.Fatal("Send returned empty ID")
	}

	msgs, err := bridge.Consume("my-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Consume count = %d, want 1", len(msgs))
	}
	if msgs[0].Body != `{"key":"value"}` {
		t.Errorf("body = %q, want %q", msgs[0].Body, `{"key":"value"}`)
	}
	if msgs[0].ContentType != "json" {
		t.Errorf("contentType = %q, want %q", msgs[0].ContentType, "json")
	}
}

func TestQueueBridge_SendBatch(t *testing.T) {
	bridge := queueTestDB(t)

	inputs := []QueueMessageInput{
		{Body: "msg1", ContentType: "text"},
		{Body: "msg2", ContentType: "text"},
		{Body: "msg3", ContentType: "json"},
	}

	ids, err := bridge.SendBatch("batch-queue", inputs)
	if err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("SendBatch returned %d IDs, want 3", len(ids))
	}

	msgs, err := bridge.Consume("batch-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("Consume count = %d, want 3", len(msgs))
	}
}

func TestQueueBridge_SendDefaultContentType(t *testing.T) {
	bridge := queueTestDB(t)

	_, err := bridge.Send("default-ct-queue", "hello", "")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	msgs, err := bridge.Consume("default-ct-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Consume count = %d, want 1", len(msgs))
	}
	if msgs[0].ContentType != "json" {
		t.Errorf("default contentType = %q, want %q", msgs[0].ContentType, "json")
	}
}

func TestQueueBridge_Ack(t *testing.T) {
	bridge := queueTestDB(t)

	id, err := bridge.Send("ack-queue", "data", "text")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if err := bridge.Ack(id); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	// Consume should return no unacked messages.
	msgs, err := bridge.Consume("ack-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Consume after ack = %d, want 0", len(msgs))
	}
}

// JS-level queue binding tests.

func queueEnv(t *testing.T, _ *QueueBridge) *Env {
	t.Helper()
	return &Env{
		Vars:          make(map[string]string),
		Secrets:       make(map[string]string),
		KVBindings:    make(map[string]string),
		QueueBindings: map[string]string{"MY_QUEUE": "test-queue"},
	}
}

func TestQueue_JSSend(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db, SiteID: "test-" + t.Name()}

	source := `export default {
  async fetch(request, env) {
    await env.MY_QUEUE.send("hello world", { contentType: "text" });
    return Response.json({ ok: true });
  },
};`

	env := queueEnv(t, bridge)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.OK {
		t.Error("expected ok: true")
	}

	// Verify the message was stored.
	msgs, err := bridge.Consume("test-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Body != "hello world" {
		t.Errorf("body = %q, want %q", msgs[0].Body, "hello world")
	}
	if msgs[0].ContentType != "text" {
		t.Errorf("contentType = %q, want %q", msgs[0].ContentType, "text")
	}
}

func TestQueue_JSSendBatch(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db, SiteID: "test-" + t.Name()}

	source := `export default {
  async fetch(request, env) {
    await env.MY_QUEUE.sendBatch([
      { body: "first", contentType: "text" },
      { body: "second", contentType: "text" },
    ]);
    return Response.json({ ok: true });
  },
};`

	env := queueEnv(t, bridge)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	msgs, err := bridge.Consume("test-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestQueue_JSDefaultContentType(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db, SiteID: "test-" + t.Name()}

	source := `export default {
  async fetch(request, env) {
    await env.MY_QUEUE.send("no-options");
    return Response.json({ ok: true });
  },
};`

	env := queueEnv(t, bridge)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	msgs, err := bridge.Consume("test-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].ContentType != "json" {
		t.Errorf("default contentType = %q, want %q", msgs[0].ContentType, "json")
	}
}

func TestQueue_JSBindingExists(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db, SiteID: "test-" + t.Name()}

	source := `export default {
  async fetch(request, env) {
    const hasSend = typeof env.MY_QUEUE.send === 'function';
    const hasSendBatch = typeof env.MY_QUEUE.sendBatch === 'function';
    return Response.json({ hasSend, hasSendBatch });
  },
};`

	env := queueEnv(t, bridge)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasSend      bool `json:"hasSend"`
		HasSendBatch bool `json:"hasSendBatch"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.HasSend {
		t.Error("env.MY_QUEUE.send should be a function")
	}
	if !data.HasSendBatch {
		t.Error("env.MY_QUEUE.sendBatch should be a function")
	}
}

// TestQueue_JSSendStringMessage verifies that Queue.send() works with a plain string body.
func TestQueue_JSSendStringMessage(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db, SiteID: "test-" + t.Name()}

	source := `export default {
  async fetch(request, env) {
    await env.MY_QUEUE.send("plain string message", { contentType: "text" });
    return Response.json({ ok: true });
  },
};`

	env := queueEnv(t, bridge)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	msgs, err := bridge.Consume("test-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Body != "plain string message" {
		t.Errorf("body = %q, want %q", msgs[0].Body, "plain string message")
	}
	if msgs[0].ContentType != "text" {
		t.Errorf("contentType = %q, want %q", msgs[0].ContentType, "text")
	}
}

// TestQueue_JSSendJSONObject verifies that Queue.send() works with a JSON object body.
func TestQueue_JSSendJSONObject(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db, SiteID: "test-" + t.Name()}

	source := `export default {
  async fetch(request, env) {
    await env.MY_QUEUE.send(JSON.stringify({ action: "process", id: 42 }));
    return Response.json({ ok: true });
  },
};`

	env := queueEnv(t, bridge)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	msgs, err := bridge.Consume("test-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var body struct {
		Action string `json:"action"`
		ID     int    `json:"id"`
	}
	if err := json.Unmarshal([]byte(msgs[0].Body), &body); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if body.Action != "process" {
		t.Errorf("action = %q, want %q", body.Action, "process")
	}
	if body.ID != 42 {
		t.Errorf("id = %d, want 42", body.ID)
	}
}

// TestQueue_JSSendBatchMultipleMessages verifies sendBatch with varied messages.
func TestQueue_JSSendBatchMultipleMessages(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db, SiteID: "test-" + t.Name()}

	source := `export default {
  async fetch(request, env) {
    await env.MY_QUEUE.sendBatch([
      { body: "msg-alpha", contentType: "text" },
      { body: "msg-beta", contentType: "text" },
      { body: "msg-gamma", contentType: "json" },
    ]);
    return Response.json({ ok: true });
  },
};`

	env := queueEnv(t, bridge)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	msgs, err := bridge.Consume("test-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Body != "msg-alpha" {
		t.Errorf("msg[0].body = %q, want %q", msgs[0].Body, "msg-alpha")
	}
	if msgs[1].Body != "msg-beta" {
		t.Errorf("msg[1].body = %q, want %q", msgs[1].Body, "msg-beta")
	}
	if msgs[2].Body != "msg-gamma" {
		t.Errorf("msg[2].body = %q, want %q", msgs[2].Body, "msg-gamma")
	}
}

// TestQueue_JSSendNoArgs verifies that Queue.send() with no arguments rejects.
func TestQueue_JSSendNoArgs(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await env.MY_QUEUE.send();
      return Response.json({ rejected: false });
    } catch (e) {
      return Response.json({ rejected: true, message: String(e) });
    }
  },
};`

	env := queueEnv(t, nil)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Rejected {
		t.Error("send() with no args should reject")
	}
}

// TestQueue_JSSendBatchNoArgs verifies that Queue.sendBatch() with no arguments rejects.
func TestQueue_JSSendBatchNoArgs(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    try {
      await env.MY_QUEUE.sendBatch();
      return Response.json({ rejected: false });
    } catch (e) {
      return Response.json({ rejected: true, message: String(e) });
    }
  },
};`

	env := queueEnv(t, nil)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Rejected bool   `json:"rejected"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Rejected {
		t.Error("sendBatch() with no args should reject")
	}
}

// TestQueue_JSSendBatchNonArray verifies that Queue.sendBatch() with a non-array input handles gracefully.
func TestQueue_JSSendBatchNonArray(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db, SiteID: "test-" + t.Name()}

	source := `export default {
  async fetch(request, env) {
    await env.MY_QUEUE.sendBatch("not-an-array");
    return Response.json({ ok: true });
  },
};`

	env := queueEnv(t, bridge)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	// Should succeed but with zero messages since the input is not an array
	msgs, err := bridge.Consume("test-queue", 10)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for non-array input, got %d", len(msgs))
	}
}

// TestQueue_ConsumeDefaultBatchSize verifies Consume with zero/negative batch size uses default.
func TestQueue_ConsumeDefaultBatchSize(t *testing.T) {
	bridge := queueTestDB(t)

	// Add 3 messages
	for i := 0; i < 3; i++ {
		_, _ = bridge.Send("default-batch", "msg", "text")
	}

	// batchSize 0 should default to 10
	msgs, err := bridge.Consume("default-batch", 0)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("Consume(0) returned %d messages, want 3", len(msgs))
	}

	// batchSize -1 should also default to 10
	msgs2, err := bridge.Consume("default-batch", -1)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	// All are still unacked from previous consume
	if len(msgs2) != 3 {
		t.Errorf("Consume(-1) returned %d messages, want 3", len(msgs2))
	}
}

// TestQueue_AccessibleFromWorkerEnv verifies the queue binding is accessible
// from env and has the correct shape.
func TestQueue_AccessibleFromWorkerEnv(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db, SiteID: "test-" + t.Name()}

	source := `export default {
  async fetch(request, env) {
    const exists = env.MY_QUEUE !== undefined;
    const isObj = typeof env.MY_QUEUE === 'object';
    const hasSend = typeof env.MY_QUEUE.send === 'function';
    const hasSendBatch = typeof env.MY_QUEUE.sendBatch === 'function';
    return Response.json({ exists, isObj, hasSend, hasSendBatch });
  },
};`

	env := queueEnv(t, bridge)
	r := execJS(t, e, source, env, getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Exists       bool `json:"exists"`
		IsObj        bool `json:"isObj"`
		HasSend      bool `json:"hasSend"`
		HasSendBatch bool `json:"hasSendBatch"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatal(err)
	}
	if !data.Exists {
		t.Error("env.MY_QUEUE should exist")
	}
	if !data.IsObj {
		t.Error("env.MY_QUEUE should be an object")
	}
	if !data.HasSend {
		t.Error("env.MY_QUEUE.send should be a function")
	}
	if !data.HasSendBatch {
		t.Error("env.MY_QUEUE.sendBatch should be a function")
	}
}
