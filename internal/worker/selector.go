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

// combinatorType represents a CSS combinator between two simple selectors.
type combinatorType int

const (
	combinatorNone            combinatorType = iota
	combinatorDescendant                     // "A B" — any descendant
	combinatorChild                          // "A > B" — direct child
	combinatorAdjacentSibling                // "A + B" — immediately following sibling
	combinatorGeneralSibling                 // "A ~ B" — any following sibling
)

// selectorPart represents one segment of a compound selector chain.
// For "div > p.active", the chain is:
//
//	[{sel: div, combinator: child}, {sel: p.active, combinator: none}]
type selectorPart struct {
	sel        *cssSelector
	combinator combinatorType // combinator AFTER this part (toward the subject)
}

// compoundSelector represents a full selector that may contain combinators.
// Parts are ordered left-to-right: parts[0] is the leftmost (ancestor),
// parts[len-1] is the subject (the element being matched).
type compoundSelector struct {
	parts []selectorPart
}

// isSimple returns true if this compound selector has no combinators
// (i.e., it is a single simple selector).
func (cs *compoundSelector) isSimple() bool {
	return len(cs.parts) <= 1
}

// subject returns the rightmost (subject) selector that the element must match.
func (cs *compoundSelector) subject() *cssSelector {
	if len(cs.parts) == 0 {
		return &cssSelector{Tag: "*"}
	}
	return cs.parts[len(cs.parts)-1].sel
}

// elementInfo captures the tag name and attributes of an element in the DOM
// context, used for ancestor/sibling matching.
type elementInfo struct {
	tagName string
	attrs   map[string]string
	depth   int
}

// matchesWithContext checks whether the compound selector matches the given
// element considering the DOM context (ancestors and siblings).
// ancestors is ordered from outermost to innermost (the immediate parent is last).
// prevSiblings is ordered from first to last sibling at the same depth.
func (cs *compoundSelector) matchesWithContext(tagName string, attrs map[string]string, ancestors []elementInfo, prevSiblings []elementInfo) bool {
	if len(cs.parts) == 0 {
		return false
	}

	// The subject (rightmost part) must match the current element.
	subject := cs.parts[len(cs.parts)-1].sel
	if !subject.matches(tagName, attrs) {
		return false
	}

	// If only one part, no combinator to check.
	if len(cs.parts) == 1 {
		return true
	}

	// Walk the chain from right to left, verifying each combinator.
	// Start from the part just before the subject.
	ancIdx := len(ancestors) - 1 // index into ancestors (innermost first)
	sibIdx := len(prevSiblings) - 1

	for i := len(cs.parts) - 2; i >= 0; i-- {
		part := cs.parts[i]
		comb := part.combinator

		switch comb {
		case combinatorChild:
			// The part must match the immediate parent.
			if ancIdx < 0 {
				return false
			}
			parent := ancestors[ancIdx]
			if !part.sel.matches(parent.tagName, parent.attrs) {
				return false
			}
			ancIdx--

		case combinatorDescendant:
			// The part must match some ancestor.
			found := false
			for ancIdx >= 0 {
				anc := ancestors[ancIdx]
				ancIdx--
				if part.sel.matches(anc.tagName, anc.attrs) {
					found = true
					break
				}
			}
			if !found {
				return false
			}

		case combinatorAdjacentSibling:
			// The part must match the immediately preceding sibling.
			if sibIdx < 0 {
				return false
			}
			sib := prevSiblings[sibIdx]
			if !part.sel.matches(sib.tagName, sib.attrs) {
				return false
			}
			sibIdx--

		case combinatorGeneralSibling:
			// The part must match any preceding sibling.
			found := false
			for sibIdx >= 0 {
				sib := prevSiblings[sibIdx]
				sibIdx--
				if part.sel.matches(sib.tagName, sib.attrs) {
					found = true
					break
				}
			}
			if !found {
				return false
			}

		default:
			return false
		}
	}

	return true
}

// parseCompoundSelector parses a CSS selector string that may contain
// combinators (>, +, ~, or whitespace for descendant).
func parseCompoundSelector(s string) *compoundSelector {
	s = strings.TrimSpace(s)
	if s == "" {
		return &compoundSelector{parts: []selectorPart{{sel: &cssSelector{Tag: "*"}}}}
	}

	tokens := tokenizeSelectorParts(s)
	if len(tokens) == 0 {
		return &compoundSelector{parts: []selectorPart{{sel: &cssSelector{Tag: "*"}}}}
	}

	// If there's only one token, it's a simple selector.
	if len(tokens) == 1 {
		return &compoundSelector{parts: []selectorPart{{sel: parseSelector(tokens[0])}}}
	}

	// Process tokens: alternating between selector strings and combinator tokens.
	var parts []selectorPart
	i := 0
	for i < len(tokens) {
		sel := parseSelector(tokens[i])
		i++

		comb := combinatorNone
		if i < len(tokens) {
			// Next token should be a combinator or implicit descendant.
			switch tokens[i] {
			case ">":
				comb = combinatorChild
			case "+":
				comb = combinatorAdjacentSibling
			case "~":
				comb = combinatorGeneralSibling
			case " ":
				comb = combinatorDescendant
			default:
				// Implicit descendant (two selectors side by side after tokenization).
				comb = combinatorDescendant
				// Don't consume the token; it's the next selector.
				parts = append(parts, selectorPart{sel: sel, combinator: comb})
				continue
			}
			i++ // consume the combinator token
		}

		parts = append(parts, selectorPart{sel: sel, combinator: comb})
	}

	return &compoundSelector{parts: parts}
}

// tokenizeSelectorParts splits a selector string into alternating tokens of
// simple-selector strings and combinator strings (">", "+", "~", " ").
func tokenizeSelectorParts(s string) []string {
	var tokens []string
	n := len(s)
	i := 0

	for i < n {
		// Skip leading whitespace.
		wsStart := i
		for i < n && s[i] == ' ' || i < n && s[i] == '\t' {
			i++
		}
		if i >= n {
			break
		}

		// Check for explicit combinator characters.
		if s[i] == '>' || s[i] == '+' || s[i] == '~' {
			combChar := string(s[i])
			i++
			// Skip trailing whitespace after combinator.
			for i < n && (s[i] == ' ' || s[i] == '\t') {
				i++
			}
			tokens = append(tokens, combChar)
			continue
		}

		// If we skipped whitespace and there are already tokens, this is
		// an implicit descendant combinator.
		if wsStart > 0 && i > wsStart && len(tokens) > 0 {
			lastToken := tokens[len(tokens)-1]
			if lastToken != ">" && lastToken != "+" && lastToken != "~" && lastToken != " " {
				tokens = append(tokens, " ")
			}
		}

		// Parse a simple selector (until whitespace or combinator).
		start := i
		for i < n && s[i] != ' ' && s[i] != '\t' && s[i] != '>' && s[i] != '+' && s[i] != '~' {
			if s[i] == '[' {
				// Skip to closing bracket.
				i++
				for i < n && s[i] != ']' {
					i++
				}
				if i < n {
					i++ // skip ]
				}
			} else {
				i++
			}
		}
		if i > start {
			tokens = append(tokens, s[start:i])
		}
	}

	return tokens
}

// parseSelector parses a simple CSS selector string into a cssSelector.
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
