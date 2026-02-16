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

func TestMatchPattern_WildcardPrefixExact(t *testing.T) {
	// Test /blog/* matching /blog exactly (path == prefix case)
	// When path is exactly the prefix, TrimPrefix(path, prefix+"/") returns the original path
	// because "/blog" doesn't start with "/blog/"
	matched, resolved := matchPattern("/blog/*", "/blog", "/articles/:splat")
	if !matched {
		t.Error("expected /blog/* to match /blog")
	}
	// splat = TrimPrefix("/blog", "/blog/") = "/blog" (unchanged)
	expected := "/articles//blog"
	if resolved != expected {
		t.Errorf("resolved = %q, want %q", resolved, expected)
	}
}

func TestMatchPattern_WildcardWithSplat(t *testing.T) {
	// Test /api/* with :splat substitution
	matched, resolved := matchPattern("/api/*", "/api/users/123", "/v2/api/:splat")
	if !matched {
		t.Error("expected match")
	}
	if resolved != "/v2/api/users/123" {
		t.Errorf("resolved = %q, want /v2/api/users/123", resolved)
	}
}

func TestMatchPattern_RootWildcardWithSplat(t *testing.T) {
	// Test /* with :splat replacement
	matched, resolved := matchPattern("/*", "/some/deep/path", "/app/:splat")
	if !matched {
		t.Error("expected match")
	}
	if resolved != "/app/some/deep/path" {
		t.Errorf("resolved = %q, want /app/some/deep/path", resolved)
	}
}

func TestMatchPattern_ExactMatchWithSplat(t *testing.T) {
	// Test exact match removes :splat entirely
	matched, resolved := matchPattern("/exact", "/exact", "/target/:splat/more")
	if !matched {
		t.Error("expected match")
	}
	if resolved != "/target//more" {
		t.Errorf("resolved = %q, want /target//more", resolved)
	}
}

func TestMatchPattern_WildcardNoMatch(t *testing.T) {
	// Test wildcard pattern that doesn't match
	matched, _ := matchPattern("/blog/*", "/about/page", "/articles/:splat")
	if matched {
		t.Error("expected no match for /blog/* against /about/page")
	}
}

func TestMatchPattern_NoMatchAtAll(t *testing.T) {
	// Test pattern that doesn't match (no exact, no wildcard)
	matched, resolved := matchPattern("/specific", "/different", "/target")
	if matched {
		t.Error("expected no match")
	}
	if resolved != "" {
		t.Errorf("resolved should be empty on no match, got %q", resolved)
	}
}

func TestMatchPattern_WildcardWithTrailingSlash(t *testing.T) {
	// Test /api/* matching /api/v1/users (path has prefix+"/")
	matched, resolved := matchPattern("/api/*", "/api/v1/users", "/backend/:splat")
	if !matched {
		t.Error("expected match")
	}
	if resolved != "/backend/v1/users" {
		t.Errorf("resolved = %q, want /backend/v1/users", resolved)
	}
}

func TestMatchPattern_RootWildcardDirect(t *testing.T) {
	// Test the /* pattern directly (note: this is handled by lines 80-86, not 90-94)
	// Lines 90-94 are unreachable because /* matches the HasSuffix check at line 80
	matched, resolved := matchPattern("/*", "/foo/bar", "/app/:splat")
	if !matched {
		t.Error("expected /* to match /foo/bar")
	}
	if resolved != "/app/foo/bar" {
		t.Errorf("resolved = %q, want /app/foo/bar", resolved)
	}
}
