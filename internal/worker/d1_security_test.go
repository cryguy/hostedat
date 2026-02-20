package worker

import (
	"strings"
	"testing"
)

// Standalone security tests for D1 SQL command blocking
func TestD1Security_BlockATTACH(t *testing.T) {
	bridge, err := NewD1BridgeMemory("security-attach")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer bridge.Close()

	_, err = bridge.Exec("ATTACH DATABASE '/tmp/evil.db' AS evil", nil)
	if err == nil {
		t.Error("ATTACH should be blocked")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("error should mention 'not allowed', got: %v", err)
	}
}

func TestD1Security_BlockDETACH(t *testing.T) {
	bridge, err := NewD1BridgeMemory("security-detach")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer bridge.Close()

	_, err = bridge.Exec("DETACH DATABASE main", nil)
	if err == nil {
		t.Error("DETACH should be blocked")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("error should mention 'not allowed', got: %v", err)
	}
}

func TestD1Security_BlockDangerousPRAGMA(t *testing.T) {
	bridge, err := NewD1BridgeMemory("security-pragma")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer bridge.Close()

	_, err = bridge.Exec("PRAGMA database_list", nil)
	if err == nil {
		t.Error("PRAGMA database_list should be blocked")
	}
}

func TestD1Security_AllowSafePRAGMA(t *testing.T) {
	bridge, err := NewD1BridgeMemory("security-safe-pragma")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer bridge.Close()

	_, err = bridge.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)", nil)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = bridge.Exec("PRAGMA TABLE_INFO(test)", nil)
	if err != nil {
		t.Errorf("PRAGMA TABLE_INFO should be allowed, got: %v", err)
	}
}

func TestD1Security_NormalSQLWorks(t *testing.T) {
	bridge, err := NewD1BridgeMemory("security-normal")
	if err != nil {
		t.Fatalf("NewD1BridgeMemory: %v", err)
	}
	defer bridge.Close()

	_, err = bridge.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)", nil)
	if err != nil {
		t.Fatalf("CREATE: %v", err)
	}

	_, err = bridge.Exec("INSERT INTO users (name) VALUES (?)", []interface{}{"alice"})
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	result, err := bridge.Exec("SELECT name FROM users", nil)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(result.Rows))
	}
}
