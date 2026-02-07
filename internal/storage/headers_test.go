package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func writeHeaders(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "_headers")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseHeaders_Basic(t *testing.T) {
	rules, err := ParseHeaders(writeHeaders(t, `/*
  X-Frame-Options: DENY
  X-Content-Type-Options: nosniff
/*.js
  Cache-Control: public, max-age=31536000
`))
	if err != nil {
		t.Fatalf("ParseHeaders: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	if rules[0].Path != "/*" {
		t.Errorf("rule 0 path = %q", rules[0].Path)
	}
	if rules[0].Headers["X-Frame-Options"] != "DENY" {
		t.Errorf("X-Frame-Options = %q", rules[0].Headers["X-Frame-Options"])
	}
	if rules[1].Path != "/*.js" {
		t.Errorf("rule 1 path = %q", rules[1].Path)
	}
}

func TestParseHeaders_NonexistentFile(t *testing.T) {
	rules, err := ParseHeaders("/nonexistent/_headers")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rules != nil {
		t.Fatalf("expected nil, got %v", rules)
	}
}

func TestMatchHeaders_WildcardAll(t *testing.T) {
	rules := []HeaderRule{
		{Path: "/*", Headers: map[string]string{"X-Test": "yes"}},
	}
	h := MatchHeaders(rules, "/any/path")
	if h["X-Test"] != "yes" {
		t.Errorf("X-Test = %q", h["X-Test"])
	}
}

func TestMatchHeaders_ExtensionWildcard(t *testing.T) {
	rules := []HeaderRule{
		{Path: "/*.js", Headers: map[string]string{"Cache": "immutable"}},
	}
	h := MatchHeaders(rules, "/assets/app.js")
	if h["Cache"] != "immutable" {
		t.Error("expected match for .js file")
	}
	h = MatchHeaders(rules, "/style.css")
	if h["Cache"] != "" {
		t.Error("should not match .css file")
	}
}

func TestMatchHeaders_MultipleRules(t *testing.T) {
	rules := []HeaderRule{
		{Path: "/*", Headers: map[string]string{"X-Global": "yes"}},
		{Path: "/*.js", Headers: map[string]string{"X-JS": "yes"}},
	}
	h := MatchHeaders(rules, "/app.js")
	if h["X-Global"] != "yes" {
		t.Error("missing X-Global")
	}
	if h["X-JS"] != "yes" {
		t.Error("missing X-JS")
	}
}

func TestMatchHeaders_PathPrefix(t *testing.T) {
	rules := []HeaderRule{
		{Path: "/assets/*", Headers: map[string]string{"X-Assets": "yes"}},
	}
	h := MatchHeaders(rules, "/assets/img/logo.png")
	if h["X-Assets"] != "yes" {
		t.Error("expected match under /assets/")
	}
	h = MatchHeaders(rules, "/other/file.txt")
	if h["X-Assets"] != "" {
		t.Error("should not match outside /assets/")
	}
}
