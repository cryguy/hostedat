package worker

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestScheduledEventProperties(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled(event, env, ctx) {
    console.log(JSON.stringify({
      type: event.type,
      hasCron: typeof event.cron === 'string',
      cron: event.cron,
      hasScheduledTime: typeof event.scheduledTime === 'number',
      isEvent: event instanceof Event,
      isScheduledEvent: event instanceof ScheduledEvent,
    }));
  },
};`
	siteID := "test-sched-props"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "*/10 * * * *")
	if result.Error != nil {
		t.Fatalf("ExecuteScheduled: %v", result.Error)
	}
	if len(result.Logs) == 0 {
		t.Fatal("expected logs from scheduled handler")
	}

	var data struct {
		Type             string `json:"type"`
		HasCron          bool   `json:"hasCron"`
		Cron             string `json:"cron"`
		HasScheduledTime bool   `json:"hasScheduledTime"`
		IsEvent          bool   `json:"isEvent"`
		IsScheduledEvent bool   `json:"isScheduledEvent"`
	}
	if err := json.Unmarshal([]byte(result.Logs[0].Message), &data); err != nil {
		t.Fatalf("unmarshal log: %v", err)
	}
	if data.Type != "scheduled" {
		t.Errorf("type = %q, want 'scheduled'", data.Type)
	}
	if !data.HasCron {
		t.Error("cron should be a string")
	}
	if data.Cron != "*/10 * * * *" {
		t.Errorf("cron = %q, want '*/10 * * * *'", data.Cron)
	}
	if !data.HasScheduledTime {
		t.Error("scheduledTime should be a number")
	}
	if !data.IsEvent {
		t.Error("ScheduledEvent should be instanceof Event")
	}
	if !data.IsScheduledEvent {
		t.Error("event should be instanceof ScheduledEvent")
	}
}

func TestScheduledEventWaitUntil(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  async scheduled(event, env, ctx) {
    event.waitUntil(Promise.resolve("background work"));
    event.waitUntil(new Promise(r => setTimeout(r, 10)));
    console.log("promises: " + event._waitUntilPromises.length);
  },
};`
	siteID := "test-sched-waituntil"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "0 * * * *")
	if result.Error != nil {
		t.Fatalf("ExecuteScheduled: %v", result.Error)
	}
	if len(result.Logs) == 0 {
		t.Fatal("expected logs")
	}
	if result.Logs[0].Message != "promises: 2" {
		t.Errorf("log = %q, want 'promises: 2'", result.Logs[0].Message)
	}
}

func TestNavigatorScheduling(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    return Response.json({
      hasScheduling: typeof navigator.scheduling === 'object',
      hasIsInputPending: typeof navigator.scheduling.isInputPending === 'function',
      result: navigator.scheduling.isInputPending(),
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		HasScheduling     bool `json:"hasScheduling"`
		HasIsInputPending bool `json:"hasIsInputPending"`
		Result            bool `json:"result"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !data.HasScheduling {
		t.Error("navigator.scheduling should be an object")
	}
	if !data.HasIsInputPending {
		t.Error("navigator.scheduling.isInputPending should be a function")
	}
	if data.Result != false {
		t.Error("isInputPending() should return false")
	}
}

func TestTailHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  tail(events, env, ctx) {
    console.log("tail called with " + events.length + " events");
    console.log("first script: " + events[0].scriptName);
    console.log("first outcome: " + events[0].outcome);
  },
};`
	siteID := "test-tail-handler"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	events := []TailEvent{
		{
			ScriptName: "my-worker",
			Logs:       []LogEntry{{Level: "log", Message: "hello", Time: time.Now()}},
			Exceptions: nil,
			Outcome:    "ok",
			Timestamp:  time.Now(),
		},
	}

	result := e.ExecuteTail(siteID, deployKey, defaultEnv(), events)
	if result.Error != nil {
		t.Fatalf("ExecuteTail: %v", result.Error)
	}
	if len(result.Logs) < 3 {
		t.Fatalf("expected at least 3 logs, got %d", len(result.Logs))
	}
	if !strings.Contains(result.Logs[0].Message, "1 events") {
		t.Errorf("log[0] = %q, want '1 events'", result.Logs[0].Message)
	}
	if !strings.Contains(result.Logs[1].Message, "my-worker") {
		t.Errorf("log[1] = %q, want 'my-worker'", result.Logs[1].Message)
	}
	if !strings.Contains(result.Logs[2].Message, "ok") {
		t.Errorf("log[2] = %q, want 'ok'", result.Logs[2].Message)
	}
}

func TestTailHandlerReceivesLogs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  tail(events, env, ctx) {
    const event = events[0];
    const logMessages = event.logs.map(l => l.level + ":" + l.message);
    console.log(JSON.stringify({
      scriptName: event.scriptName,
      logCount: event.logs.length,
      logMessages: logMessages,
      exceptionCount: event.exceptions.length,
      exceptions: event.exceptions,
      outcome: event.outcome,
      hasTimestamp: typeof event.timestamp === 'string',
    }));
  },
};`
	siteID := "test-tail-logs"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	events := []TailEvent{
		{
			ScriptName: "worker-abc",
			Logs: []LogEntry{
				{Level: "log", Message: "request started", Time: time.Now()},
				{Level: "error", Message: "something failed", Time: time.Now()},
			},
			Exceptions: []string{"TypeError: x is not a function"},
			Outcome:    "exception",
			Timestamp:  time.Now(),
		},
	}

	result := e.ExecuteTail(siteID, deployKey, defaultEnv(), events)
	if result.Error != nil {
		t.Fatalf("ExecuteTail: %v", result.Error)
	}
	if len(result.Logs) == 0 {
		t.Fatal("expected logs from tail handler")
	}

	var data struct {
		ScriptName     string   `json:"scriptName"`
		LogCount       int      `json:"logCount"`
		LogMessages    []string `json:"logMessages"`
		ExceptionCount int      `json:"exceptionCount"`
		Exceptions     []string `json:"exceptions"`
		Outcome        string   `json:"outcome"`
		HasTimestamp   bool     `json:"hasTimestamp"`
	}
	if err := json.Unmarshal([]byte(result.Logs[0].Message), &data); err != nil {
		t.Fatalf("unmarshal log: %v", err)
	}
	if data.ScriptName != "worker-abc" {
		t.Errorf("scriptName = %q, want 'worker-abc'", data.ScriptName)
	}
	if data.LogCount != 2 {
		t.Errorf("logCount = %d, want 2", data.LogCount)
	}
	if len(data.LogMessages) != 2 {
		t.Fatalf("logMessages len = %d, want 2", len(data.LogMessages))
	}
	if data.LogMessages[0] != "log:request started" {
		t.Errorf("logMessages[0] = %q", data.LogMessages[0])
	}
	if data.LogMessages[1] != "error:something failed" {
		t.Errorf("logMessages[1] = %q", data.LogMessages[1])
	}
	if data.ExceptionCount != 1 {
		t.Errorf("exceptionCount = %d, want 1", data.ExceptionCount)
	}
	if data.Exceptions[0] != "TypeError: x is not a function" {
		t.Errorf("exceptions[0] = %q", data.Exceptions[0])
	}
	if data.Outcome != "exception" {
		t.Errorf("outcome = %q, want 'exception'", data.Outcome)
	}
	if !data.HasTimestamp {
		t.Error("timestamp should be present as a string")
	}
}

func TestTailHandler_NoHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
};`
	siteID := "test-tail-noh"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	events := []TailEvent{
		{ScriptName: "test", Outcome: "ok", Timestamp: time.Now()},
	}

	result := e.ExecuteTail(siteID, deployKey, defaultEnv(), events)
	if result.Error == nil {
		t.Fatal("ExecuteTail should fail with no tail handler")
	}
	if !strings.Contains(result.Error.Error(), "no tail handler") {
		t.Errorf("error = %q, should mention 'no tail handler'", result.Error)
	}
}

func TestScheduledEvent_TimestampRange(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled(event, env, ctx) {
    const now = Date.now();
    const diff = Math.abs(now - event.scheduledTime);
    console.log(JSON.stringify({ diff, scheduledTime: event.scheduledTime }));
  },
};`
	siteID := "test-sched-ts"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "* * * * *")
	if result.Error != nil {
		t.Fatalf("ExecuteScheduled: %v", result.Error)
	}
	if len(result.Logs) == 0 {
		t.Fatal("expected logs")
	}

	var data struct {
		Diff          float64 `json:"diff"`
		ScheduledTime float64 `json:"scheduledTime"`
	}
	if err := json.Unmarshal([]byte(result.Logs[0].Message), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.ScheduledTime == 0 {
		t.Error("scheduledTime should not be zero")
	}
	// scheduledTime should be within 5 seconds of now
	if data.Diff > 5000 {
		t.Errorf("scheduledTime is %fms away from now, should be < 5000ms", data.Diff)
	}
}

func TestScheduledEvent_AsyncWaitUntil(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  async scheduled(event, env, ctx) {
    let sideEffect = 0;
    event.waitUntil(new Promise(resolve => {
      setTimeout(() => { sideEffect = 42; resolve(); }, 10);
    }));
    // Wait for all waitUntil promises
    await Promise.all(event._waitUntilPromises);
    console.log(JSON.stringify({ sideEffect }));
  },
};`
	siteID := "test-sched-async-wu"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.ExecuteScheduled(siteID, deployKey, defaultEnv(), "0 * * * *")
	if result.Error != nil {
		t.Fatalf("ExecuteScheduled: %v", result.Error)
	}
	if len(result.Logs) == 0 {
		t.Fatal("expected logs")
	}

	var data struct {
		SideEffect int `json:"sideEffect"`
	}
	if err := json.Unmarshal([]byte(result.Logs[0].Message), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.SideEffect != 42 {
		t.Errorf("sideEffect = %d, want 42", data.SideEffect)
	}
}

func TestTailHandler_MultipleEvents(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  tail(events, env, ctx) {
    console.log(JSON.stringify({
      count: events.length,
      names: events.map(e => e.scriptName),
      outcomes: events.map(e => e.outcome),
    }));
  },
};`
	siteID := "test-tail-multi"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	events := []TailEvent{
		{ScriptName: "worker-a", Outcome: "ok", Timestamp: time.Now()},
		{ScriptName: "worker-b", Outcome: "exception", Timestamp: time.Now()},
		{ScriptName: "worker-c", Outcome: "ok", Timestamp: time.Now()},
	}

	result := e.ExecuteTail(siteID, deployKey, defaultEnv(), events)
	if result.Error != nil {
		t.Fatalf("ExecuteTail: %v", result.Error)
	}
	if len(result.Logs) == 0 {
		t.Fatal("expected logs")
	}

	var data struct {
		Count    int      `json:"count"`
		Names    []string `json:"names"`
		Outcomes []string `json:"outcomes"`
	}
	if err := json.Unmarshal([]byte(result.Logs[0].Message), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Count != 3 {
		t.Errorf("count = %d, want 3", data.Count)
	}
	if len(data.Names) != 3 || data.Names[0] != "worker-a" || data.Names[1] != "worker-b" || data.Names[2] != "worker-c" {
		t.Errorf("names = %v, want [worker-a worker-b worker-c]", data.Names)
	}
	if data.Outcomes[1] != "exception" {
		t.Errorf("outcomes[1] = %q, want 'exception'", data.Outcomes[1])
	}
}

func TestTailHandler_AsyncHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  async tail(events, env, ctx) {
    await new Promise(resolve => setTimeout(resolve, 10));
    console.log("async tail done with " + events.length + " events");
  },
};`
	siteID := "test-tail-async"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	events := []TailEvent{
		{ScriptName: "worker-x", Outcome: "ok", Timestamp: time.Now()},
	}

	result := e.ExecuteTail(siteID, deployKey, defaultEnv(), events)
	if result.Error != nil {
		t.Fatalf("ExecuteTail: %v", result.Error)
	}
	if len(result.Logs) == 0 {
		t.Fatal("expected logs from async tail handler")
	}
	if !strings.Contains(result.Logs[0].Message, "async tail done") {
		t.Errorf("log = %q, should contain 'async tail done'", result.Logs[0].Message)
	}
}

func TestTailHandler_EmptyEvents(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch() { return new Response("ok"); },
  tail(events, env, ctx) {
    console.log(JSON.stringify({ count: events.length, isArray: Array.isArray(events) }));
  },
};`
	siteID := "test-tail-empty"
	deployKey := "deploy1"

	if _, err := e.CompileAndCache(siteID, deployKey, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	result := e.ExecuteTail(siteID, deployKey, defaultEnv(), []TailEvent{})
	if result.Error != nil {
		t.Fatalf("ExecuteTail: %v", result.Error)
	}
	if len(result.Logs) == 0 {
		t.Fatal("expected logs")
	}

	var data struct {
		Count   int  `json:"count"`
		IsArray bool `json:"isArray"`
	}
	if err := json.Unmarshal([]byte(result.Logs[0].Message), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Count != 0 {
		t.Errorf("count = %d, want 0", data.Count)
	}
	if !data.IsArray {
		t.Error("events should be an array")
	}
}
