package worker

import (
	"encoding/json"
	"testing"
)

func queueTestDB(t *testing.T) *QueueBridge {
	t.Helper()
	db := testDB(t)
	db.AutoMigrate(&QueueMessage{})
	return &QueueBridge{DB: db}
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

func queueEnv(t *testing.T, bridge *QueueBridge) *Env {
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
	db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db}

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
	json.Unmarshal(r.Response.Body, &data)
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
	db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db}

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
	db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db}

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
	db.AutoMigrate(&QueueMessage{})
	e := newTestEngine(t, db)
	bridge := &QueueBridge{DB: db}

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
	json.Unmarshal(r.Response.Body, &data)
	if !data.HasSend {
		t.Error("env.MY_QUEUE.send should be a function")
	}
	if !data.HasSendBatch {
		t.Error("env.MY_QUEUE.sendBatch should be a function")
	}
}
