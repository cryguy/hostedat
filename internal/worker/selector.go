package worker

import (
	"strings"
)

// cssSelector represents a parsed CSS selector for HTMLRewriter matching.
// Supports: element, #id, .class, [attr], [attr=val], [attr*=val],
// [attr^=val], [attr$=val], and combinations thereof.
type cssSelector struct {
	Tag        string
	ID         string
	Classes    []string
	Attributes []attrMatcher
}

type attrMatcher struct {
	Name  string
	Op    string // "" (exists), "=", "*=", "^=", "$=", "~="
	Value string
}

// parseSelector parses a CSS selector string into a cssSelector.
// Examples: "div", "#id", ".class", "[href]", "div.class#id[data-x=foo]", "*"
func parseSelector(s string) *cssSelector {
	s = strings.TrimSpace(s)
	if s == "" {
		return &cssSelector{Tag: "*"}
	}

	sel := &cssSelector{}
	i := 0
	n := len(s)

	// Parse tag name (everything before #, ., or [)
	start := i
	for i < n && s[i] != '#' && s[i] != '.' && s[i] != '[' {
		i++
	}
	if i > start {
		sel.Tag = s[start:i]
	}

	// Parse the rest: #id, .class, [attr...]
	for i < n {
		switch s[i] {
		case '#':
			i++ // skip #
			start = i
			for i < n && s[i] != '#' && s[i] != '.' && s[i] != '[' {
				i++
			}
			sel.ID = s[start:i]

		case '.':
			i++ // skip .
			start = i
			for i < n && s[i] != '#' && s[i] != '.' && s[i] != '[' {
				i++
			}
			sel.Classes = append(sel.Classes, s[start:i])

		case '[':
			i++ // skip [
			start = i
			for i < n && s[i] != ']' {
				i++
			}
			attrStr := s[start:i]
			if i < n {
				i++ // skip ]
			}
			sel.Attributes = append(sel.Attributes, parseAttrMatcher(attrStr))

		default:
			i++
		}
	}

	return sel
}

func parseAttrMatcher(s string) attrMatcher {
	// Check for operators: *=, ^=, $=, ~=, =
	for _, op := range []string{"*=", "^=", "$=", "~=", "="} {
		if idx := strings.Index(s, op); idx != -1 {
			name := strings.TrimSpace(s[:idx])
			value := strings.TrimSpace(s[idx+len(op):])
			// Strip quotes from value
			value = strings.Trim(value, `"'`)
			return attrMatcher{Name: name, Op: op, Value: value}
		}
	}
	// Existence check only
	return attrMatcher{Name: strings.TrimSpace(s)}
}

// matches returns true if the selector matches the given element.
func (sel *cssSelector) matches(tagName string, attrs map[string]string) bool {
	// Check tag name
	if sel.Tag != "" && sel.Tag != "*" && !strings.EqualFold(sel.Tag, tagName) {
		return false
	}

	// Check ID
	if sel.ID != "" && attrs["id"] != sel.ID {
		return false
	}

	// Check classes
	for _, cls := range sel.Classes {
		classes := strings.Fields(attrs["class"])
		found := false
		for _, c := range classes {
			if c == cls {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check attribute matchers
	for _, am := range sel.Attributes {
		val, exists := attrs[am.Name]
		if !exists {
			return false
		}
		switch am.Op {
		case "": // existence only
			// already checked
		case "=":
			if val != am.Value {
				return false
			}
		case "*=":
			if !strings.Contains(val, am.Value) {
				return false
			}
		case "^=":
			if !strings.HasPrefix(val, am.Value) {
				return false
			}
		case "$=":
			if !strings.HasSuffix(val, am.Value) {
				return false
			}
		case "~=":
			found := false
			for _, w := range strings.Fields(val) {
				if w == am.Value {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}
