package storage

import (
	"bufio"
	"os"
	"strings"
)

type HeaderRule struct {
	Path    string
	Headers map[string]string
}

func ParseHeaders(filePath string) ([]HeaderRule, error) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var rules []HeaderRule
	var current *HeaderRule

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Path line (no leading whitespace)
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if current != nil {
				rules = append(rules, *current)
			}
			current = &HeaderRule{
				Path:    trimmed,
				Headers: make(map[string]string),
			}
			continue
		}

		// Header line (indented)
		if current != nil {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				current.Headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	if current != nil {
		rules = append(rules, *current)
	}

	return rules, scanner.Err()
}

// MatchHeaders returns all headers that apply to the given path.
func MatchHeaders(rules []HeaderRule, path string) map[string]string {
	result := make(map[string]string)

	for _, rule := range rules {
		if matchHeaderPath(rule.Path, path) {
			for k, v := range rule.Headers {
				result[k] = v
			}
		}
	}

	return result
}

func matchHeaderPath(pattern, path string) bool {
	if pattern == path {
		return true
	}

	// /* matches everything
	if pattern == "/*" {
		return true
	}

	// /*.ext matches files with that extension
	if strings.HasPrefix(pattern, "/*") {
		suffix := strings.TrimPrefix(pattern, "/*")
		return strings.HasSuffix(path, suffix)
	}

	// /path/* matches everything under /path/
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(path, prefix+"/") || path == prefix
	}

	return false
}
