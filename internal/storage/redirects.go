package storage

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type RedirectRule struct {
	From       string
	To         string
	StatusCode int
}

func ParseRedirects(filePath string) ([]RedirectRule, error) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var rules []RedirectRule
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		rule := RedirectRule{
			From:       parts[0],
			To:         parts[1],
			StatusCode: 301, // default
		}

		if len(parts) >= 3 {
			if code, err := strconv.Atoi(parts[2]); err == nil {
				rule.StatusCode = code
			}
		}

		rules = append(rules, rule)
	}

	return rules, scanner.Err()
}

// MatchRedirect checks if the request path matches any redirect rule.
// Returns the matched rule, the resolved target path, and whether a match was found.
func MatchRedirect(rules []RedirectRule, path string) (*RedirectRule, string, bool) {
	for i := range rules {
		rule := &rules[i]
		matched, resolved := matchPattern(rule.From, path, rule.To)
		if matched {
			return rule, resolved, true
		}
	}
	return nil, "", false
}

// matchPattern matches a path against a pattern with optional * wildcard.
// Returns whether it matched and the resolved target with :splat substitution.
func matchPattern(pattern, path, target string) (bool, string) {
	// Exact match
	if pattern == path {
		resolved := strings.ReplaceAll(target, ":splat", "")
		return true, resolved
	}

	// Wildcard match: /foo/* matches /foo/anything
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		if strings.HasPrefix(path, prefix+"/") || path == prefix {
			splat := strings.TrimPrefix(path, prefix+"/")
			resolved := strings.ReplaceAll(target, ":splat", splat)
			return true, resolved
		}
	}

	// Root wildcard: /* matches everything
	if pattern == "/*" {
		splat := strings.TrimPrefix(path, "/")
		resolved := strings.ReplaceAll(target, ":splat", splat)
		return true, resolved
	}

	return false, ""
}
