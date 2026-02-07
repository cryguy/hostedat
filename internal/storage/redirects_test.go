package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRedirects(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "_redirects")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseRedirects_Basic(t *testing.T) {
	rules, err := ParseRedirects(writeRedirects(t, `
# This is a comment
/old /new 301

/blog/* /blog/:splat 200
`))
	if err != nil {
		t.Fatalf("ParseRedirects: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	if rules[0].From != "/old" || rules[0].To != "/new" || rules[0].StatusCode != 301 {
		t.Errorf("rule 0: %+v", rules[0])
	}
	if rules[1].From != "/blog/*" || rules[1].To != "/blog/:splat" || rules[1].StatusCode != 200 {
		t.Errorf("rule 1: %+v", rules[1])
	}
}

func TestParseRedirects_DefaultStatus(t *testing.T) {
	rules, err := ParseRedirects(writeRedirects(t, `/a /b`))
	if err != nil {
		t.Fatalf("ParseRedirects: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].StatusCode != 301 {
		t.Errorf("default status = %d, want 301", rules[0].StatusCode)
	}
}

func TestParseRedirects_NonexistentFile(t *testing.T) {
	rules, err := ParseRedirects("/nonexistent/_redirects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rules != nil {
		t.Fatalf("expected nil rules, got %v", rules)
	}
}

func TestMatchRedirect_ExactMatch(t *testing.T) {
	rules := []RedirectRule{{From: "/old", To: "/new", StatusCode: 301}}
	rule, target, ok := MatchRedirect(rules, "/old")
	if !ok {
		t.Fatal("expected match")
	}
	if target != "/new" {
		t.Errorf("target = %q, want /new", target)
	}
	if rule.StatusCode != 301 {
		t.Errorf("status = %d, want 301", rule.StatusCode)
	}
}

func TestMatchRedirect_WildcardSplat(t *testing.T) {
	rules := []RedirectRule{{From: "/blog/*", To: "/blog/:splat", StatusCode: 200}}
	_, target, ok := MatchRedirect(rules, "/blog/hello-world")
	if !ok {
		t.Fatal("expected match")
	}
	if target != "/blog/hello-world" {
		t.Errorf("target = %q, want /blog/hello-world", target)
	}
}

func TestMatchRedirect_RootWildcard(t *testing.T) {
	rules := []RedirectRule{{From: "/*", To: "/index.html", StatusCode: 200}}
	_, target, ok := MatchRedirect(rules, "/any/path")
	if !ok {
		t.Fatal("expected match")
	}
	if target != "/index.html" {
		t.Errorf("target = %q, want /index.html", target)
	}
}

func TestMatchRedirect_NoMatch(t *testing.T) {
	rules := []RedirectRule{{From: "/specific", To: "/other", StatusCode: 301}}
	_, _, ok := MatchRedirect(rules, "/different")
	if ok {
		t.Fatal("expected no match")
	}
}

func TestMatchRedirect_FirstMatchWins(t *testing.T) {
	rules := []RedirectRule{
		{From: "/path", To: "/first", StatusCode: 301},
		{From: "/path", To: "/second", StatusCode: 302},
	}
	_, target, ok := MatchRedirect(rules, "/path")
	if !ok {
		t.Fatal("expected match")
	}
	if target != "/first" {
		t.Errorf("target = %q, want /first (first match wins)", target)
	}
}
