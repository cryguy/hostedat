package worker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConsole_MultipleArguments(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log("hello", "world", 42);
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) == 0 {
		t.Fatal("no logs captured")
	}
	if r.Logs[0].Message != "hello world 42" {
		t.Errorf("message = %q, want 'hello world 42'", r.Logs[0].Message)
	}
}

func TestConsole_EmptyArgs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log();
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) == 0 {
		t.Fatal("no logs captured")
	}
	if r.Logs[0].Message != "" {
		t.Errorf("message = %q, want empty string", r.Logs[0].Message)
	}
}

func TestConsole_ObjectStringification(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log({ foo: "bar" });
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) == 0 {
		t.Fatal("no logs captured")
	}
	// V8's default toString for objects is [object Object]
	if r.Logs[0].Message != "[object Object]" {
		t.Errorf("message = %q, want '[object Object]'", r.Logs[0].Message)
	}
}

func TestConsole_AllLevels(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log("L");
    console.info("I");
    console.warn("W");
    console.error("E");
    console.debug("D");
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) != 5 {
		t.Fatalf("log count = %d, want 5", len(r.Logs))
	}

	levels := []string{"log", "info", "warn", "error", "debug"}
	msgs := []string{"L", "I", "W", "E", "D"}
	for i := range levels {
		if r.Logs[i].Level != levels[i] {
			t.Errorf("logs[%d].Level = %q, want %q", i, r.Logs[i].Level, levels[i])
		}
		if r.Logs[i].Message != msgs[i] {
			t.Errorf("logs[%d].Message = %q, want %q", i, r.Logs[i].Message, msgs[i])
		}
	}
}

func TestConsole_HighVolumeLogging(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    for (let i = 0; i < 100; i++) {
      console.log("msg-" + i);
    }
    return Response.json({ count: 100 });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) != 100 {
		t.Errorf("log count = %d, want 100", len(r.Logs))
	}
	if r.Logs[0].Message != "msg-0" {
		t.Errorf("first log = %q", r.Logs[0].Message)
	}
	if r.Logs[99].Message != "msg-99" {
		t.Errorf("last log = %q", r.Logs[99].Message)
	}
}

func TestConsole_LogsReturnedOnError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log("before error");
    console.warn("warning before error");
    throw new Error("intentional");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	if r.Error == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(r.Error.Error(), "intentional") {
		t.Errorf("error = %v", r.Error)
	}
	if len(r.Logs) < 2 {
		t.Errorf("should capture logs before error, got %d", len(r.Logs))
	}
}

func TestConsole_MixedTypes(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log("str", 42, true, null, undefined);
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) == 0 {
		t.Fatal("no logs")
	}
	// All args are stringified and joined with spaces
	msg := r.Logs[0].Message
	if !strings.Contains(msg, "str") || !strings.Contains(msg, "42") || !strings.Contains(msg, "true") {
		t.Errorf("message = %q, expected mixed types", msg)
	}
}

func TestConsole_LogTimestamp(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log("timestamped");
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) == 0 {
		t.Fatal("no logs")
	}
	if r.Logs[0].Time.IsZero() {
		t.Error("log time should not be zero")
	}
}

func TestConsole_JSONSerializableLogs(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    console.log("test message");
    console.error("error message");
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	// Logs should be JSON serializable (important for the worker log storage)
	data, err := json.Marshal(r.Logs)
	if err != nil {
		t.Fatalf("logs should be JSON serializable: %v", err)
	}

	var parsed []LogEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("logs should be JSON deserializable: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("parsed %d logs, want 2", len(parsed))
	}
}

// TestConsoleLog_MaxEntriesLimit verifies that logging stops after maxLogEntries.
func TestConsoleLog_MaxEntriesLimit(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    for (var i = 0; i < 1100; i++) {
      console.log('msg ' + i);
    }
    return new Response('ok');
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) > maxLogEntries {
		t.Errorf("log count = %d, want <= %d", len(r.Logs), maxLogEntries)
	}
	// Should capture exactly maxLogEntries, dropping subsequent logs.
	if len(r.Logs) != maxLogEntries {
		t.Errorf("log count = %d, want %d", len(r.Logs), maxLogEntries)
	}
}

// TestConsoleLog_MessageTruncation verifies that messages longer than
// maxLogMessageSize are truncated with "...(truncated)" suffix.
func TestConsoleLog_MessageTruncation(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  async fetch(request, env) {
    console.log('A'.repeat(10000));
    return new Response('ok');
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	if len(r.Logs) == 0 {
		t.Fatal("expected at least one log entry")
	}

	msg := r.Logs[0].Message
	expectedMaxLen := maxLogMessageSize + len("...(truncated)")
	if len(msg) > expectedMaxLen {
		t.Errorf("message length = %d, want <= %d", len(msg), expectedMaxLen)
	}
	if !strings.HasSuffix(msg, "...(truncated)") {
		t.Errorf("expected message to end with '...(truncated)', got: %q", msg[len(msg)-20:])
	}
}

// TestConsoleLog_LimitConstants verifies that the log limit constants are reasonable.
func TestConsoleLog_LimitConstants(t *testing.T) {
	if maxLogEntries < 100 || maxLogEntries > 10000 {
		t.Errorf("maxLogEntries = %d, want 100-10000", maxLogEntries)
	}
	if maxLogMessageSize < 1024 || maxLogMessageSize > 100*1024 {
		t.Errorf("maxLogMessageSize = %d, want 1KB-100KB", maxLogMessageSize)
	}
}
