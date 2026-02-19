package worker

import "testing"

func TestParseSelector_Tag(t *testing.T) {
	sel := parseSelector("div")
	if sel.Tag != "div" {
		t.Errorf("Tag = %q, want 'div'", sel.Tag)
	}
}

func TestParseSelector_Wildcard(t *testing.T) {
	sel := parseSelector("*")
	if sel.Tag != "*" {
		t.Errorf("Tag = %q, want '*'", sel.Tag)
	}
}

func TestParseSelector_ID(t *testing.T) {
	sel := parseSelector("#main")
	if sel.ID != "main" {
		t.Errorf("ID = %q, want 'main'", sel.ID)
	}
}

func TestParseSelector_Class(t *testing.T) {
	sel := parseSelector(".active")
	if len(sel.Classes) != 1 || sel.Classes[0] != "active" {
		t.Errorf("Classes = %v, want ['active']", sel.Classes)
	}
}

func TestParseSelector_TagWithIDAndClass(t *testing.T) {
	sel := parseSelector("div#main.active")
	if sel.Tag != "div" {
		t.Errorf("Tag = %q", sel.Tag)
	}
	if sel.ID != "main" {
		t.Errorf("ID = %q", sel.ID)
	}
	if len(sel.Classes) != 1 || sel.Classes[0] != "active" {
		t.Errorf("Classes = %v", sel.Classes)
	}
}

func TestParseSelector_MultipleClasses(t *testing.T) {
	sel := parseSelector("p.foo.bar")
	if sel.Tag != "p" {
		t.Errorf("Tag = %q", sel.Tag)
	}
	if len(sel.Classes) != 2 {
		t.Fatalf("Classes len = %d, want 2", len(sel.Classes))
	}
	if sel.Classes[0] != "foo" || sel.Classes[1] != "bar" {
		t.Errorf("Classes = %v", sel.Classes)
	}
}

func TestParseSelector_AttributeExists(t *testing.T) {
	sel := parseSelector("[href]")
	if len(sel.Attributes) != 1 {
		t.Fatalf("Attributes len = %d", len(sel.Attributes))
	}
	if sel.Attributes[0].Name != "href" || sel.Attributes[0].Op != "" {
		t.Errorf("attr = %+v", sel.Attributes[0])
	}
}

func TestParseSelector_AttributeEquals(t *testing.T) {
	sel := parseSelector(`[type="text"]`)
	if len(sel.Attributes) != 1 {
		t.Fatalf("Attributes len = %d", len(sel.Attributes))
	}
	a := sel.Attributes[0]
	if a.Name != "type" || a.Op != "=" || a.Value != "text" {
		t.Errorf("attr = %+v", a)
	}
}

func TestParseSelector_AttributeContains(t *testing.T) {
	sel := parseSelector(`[class*="btn"]`)
	if len(sel.Attributes) != 1 {
		t.Fatalf("Attributes len = %d", len(sel.Attributes))
	}
	a := sel.Attributes[0]
	if a.Op != "*=" || a.Value != "btn" {
		t.Errorf("attr = %+v", a)
	}
}

func TestParseSelector_AttributeStartsWith(t *testing.T) {
	sel := parseSelector(`[href^="https"]`)
	a := sel.Attributes[0]
	if a.Op != "^=" || a.Value != "https" {
		t.Errorf("attr = %+v", a)
	}
}

func TestParseSelector_AttributeEndsWith(t *testing.T) {
	sel := parseSelector(`[src$=".png"]`)
	a := sel.Attributes[0]
	if a.Op != "$=" || a.Value != ".png" {
		t.Errorf("attr = %+v", a)
	}
}

func TestParseSelector_AttributeWordMatch(t *testing.T) {
	sel := parseSelector(`[class~="active"]`)
	a := sel.Attributes[0]
	if a.Op != "~=" || a.Value != "active" {
		t.Errorf("attr = %+v", a)
	}
}

func TestParseSelector_Empty(t *testing.T) {
	sel := parseSelector("")
	if sel.Tag != "*" {
		t.Errorf("Tag = %q, want '*' for empty selector", sel.Tag)
	}
}

func TestParseSelector_Complex(t *testing.T) {
	sel := parseSelector(`div.card#hero[data-x="1"]`)
	if sel.Tag != "div" {
		t.Errorf("Tag = %q", sel.Tag)
	}
	if sel.ID != "hero" {
		t.Errorf("ID = %q", sel.ID)
	}
	if len(sel.Classes) != 1 || sel.Classes[0] != "card" {
		t.Errorf("Classes = %v", sel.Classes)
	}
	if len(sel.Attributes) != 1 {
		t.Fatalf("Attributes len = %d", len(sel.Attributes))
	}
	a := sel.Attributes[0]
	if a.Name != "data-x" || a.Op != "=" || a.Value != "1" {
		t.Errorf("attr = %+v", a)
	}
}

func TestCssSelector_Matches_Tag(t *testing.T) {
	sel := parseSelector("div")
	if !sel.matches("div", nil) {
		t.Error("should match div")
	}
	if sel.matches("span", nil) {
		t.Error("should not match span")
	}
}

func TestCssSelector_Matches_Wildcard(t *testing.T) {
	sel := parseSelector("*")
	if !sel.matches("div", nil) {
		t.Error("wildcard should match any tag")
	}
	if !sel.matches("span", nil) {
		t.Error("wildcard should match any tag")
	}
}

func TestCssSelector_Matches_ID(t *testing.T) {
	sel := parseSelector("#main")
	if !sel.matches("div", map[string]string{"id": "main"}) {
		t.Error("should match id=main")
	}
	if sel.matches("div", map[string]string{"id": "other"}) {
		t.Error("should not match id=other")
	}
}

func TestCssSelector_Matches_Class(t *testing.T) {
	sel := parseSelector(".active")
	if !sel.matches("div", map[string]string{"class": "foo active bar"}) {
		t.Error("should match class containing active")
	}
	if sel.matches("div", map[string]string{"class": "foo bar"}) {
		t.Error("should not match class without active")
	}
}

func TestCssSelector_Matches_AttributeExists(t *testing.T) {
	sel := parseSelector("[href]")
	if !sel.matches("a", map[string]string{"href": "/foo"}) {
		t.Error("should match element with href")
	}
	if sel.matches("a", map[string]string{}) {
		t.Error("should not match element without href")
	}
}

func TestCssSelector_Matches_AttributeEquals(t *testing.T) {
	sel := parseSelector(`[type="text"]`)
	if !sel.matches("input", map[string]string{"type": "text"}) {
		t.Error("should match type=text")
	}
	if sel.matches("input", map[string]string{"type": "number"}) {
		t.Error("should not match type=number")
	}
}

func TestCssSelector_Matches_AttributeContains(t *testing.T) {
	sel := parseSelector(`[class*="btn"]`)
	if !sel.matches("div", map[string]string{"class": "btn-primary"}) {
		t.Error("should match class containing 'btn'")
	}
	if sel.matches("div", map[string]string{"class": "link"}) {
		t.Error("should not match class without 'btn'")
	}
}

func TestCssSelector_Matches_AttributeStartsWith(t *testing.T) {
	sel := parseSelector(`[href^="https"]`)
	if !sel.matches("a", map[string]string{"href": "https://example.com"}) {
		t.Error("should match href starting with https")
	}
	if sel.matches("a", map[string]string{"href": "http://example.com"}) {
		t.Error("should not match href starting with http")
	}
}

func TestCssSelector_Matches_AttributeEndsWith(t *testing.T) {
	sel := parseSelector(`[src$=".png"]`)
	if !sel.matches("img", map[string]string{"src": "photo.png"}) {
		t.Error("should match src ending with .png")
	}
	if sel.matches("img", map[string]string{"src": "photo.jpg"}) {
		t.Error("should not match src ending with .jpg")
	}
}

func TestCssSelector_Matches_AttributeWordMatch(t *testing.T) {
	sel := parseSelector(`[class~="foo"]`)
	if !sel.matches("div", map[string]string{"class": "foo bar baz"}) {
		t.Error("should match class with word 'foo'")
	}
	if sel.matches("div", map[string]string{"class": "foobar baz"}) {
		t.Error("should not match class without exact word 'foo'")
	}
}

func TestCssSelector_Matches_CaseInsensitiveTag(t *testing.T) {
	sel := parseSelector("DIV")
	if !sel.matches("div", nil) {
		t.Error("tag matching should be case insensitive")
	}
}

func TestCssSelector_Matches_Combined(t *testing.T) {
	sel := parseSelector(`div.card[data-visible="true"]`)
	attrs := map[string]string{
		"class":        "card wide",
		"data-visible": "true",
	}
	if !sel.matches("div", attrs) {
		t.Error("should match combined selector")
	}

	// Wrong tag
	if sel.matches("span", attrs) {
		t.Error("should not match wrong tag")
	}

	// Missing class
	if sel.matches("div", map[string]string{"data-visible": "true"}) {
		t.Error("should not match without class")
	}

	// Wrong attribute value
	attrs2 := map[string]string{
		"class":        "card",
		"data-visible": "false",
	}
	if sel.matches("div", attrs2) {
		t.Error("should not match wrong attribute value")
	}
}

// ---------------------------------------------------------------------------
// Compound selector (combinator) parsing tests
// ---------------------------------------------------------------------------

func TestParseCompoundSelector_Simple(t *testing.T) {
	cs := parseCompoundSelector("div")
	if !cs.isSimple() {
		t.Error("single selector should be simple")
	}
	if cs.subject().Tag != "div" {
		t.Errorf("subject tag = %q, want 'div'", cs.subject().Tag)
	}
}

func TestParseCompoundSelector_ChildCombinator(t *testing.T) {
	cs := parseCompoundSelector("div > p")
	if cs.isSimple() {
		t.Error("child combinator should not be simple")
	}
	if len(cs.parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(cs.parts))
	}
	if cs.parts[0].sel.Tag != "div" {
		t.Errorf("parts[0] tag = %q, want 'div'", cs.parts[0].sel.Tag)
	}
	if cs.parts[0].combinator != combinatorChild {
		t.Errorf("combinator = %d, want child", cs.parts[0].combinator)
	}
	if cs.parts[1].sel.Tag != "p" {
		t.Errorf("parts[1] tag = %q, want 'p'", cs.parts[1].sel.Tag)
	}
}

func TestParseCompoundSelector_DescendantCombinator(t *testing.T) {
	cs := parseCompoundSelector("div p")
	if cs.isSimple() {
		t.Error("descendant combinator should not be simple")
	}
	if len(cs.parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(cs.parts))
	}
	if cs.parts[0].combinator != combinatorDescendant {
		t.Errorf("combinator = %d, want descendant", cs.parts[0].combinator)
	}
}

func TestParseCompoundSelector_AdjacentSibling(t *testing.T) {
	cs := parseCompoundSelector("h1 + p")
	if len(cs.parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(cs.parts))
	}
	if cs.parts[0].sel.Tag != "h1" {
		t.Errorf("parts[0] tag = %q", cs.parts[0].sel.Tag)
	}
	if cs.parts[0].combinator != combinatorAdjacentSibling {
		t.Errorf("combinator = %d, want adjacent sibling", cs.parts[0].combinator)
	}
	if cs.parts[1].sel.Tag != "p" {
		t.Errorf("parts[1] tag = %q", cs.parts[1].sel.Tag)
	}
}

func TestParseCompoundSelector_GeneralSibling(t *testing.T) {
	cs := parseCompoundSelector("h1 ~ p")
	if len(cs.parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(cs.parts))
	}
	if cs.parts[0].combinator != combinatorGeneralSibling {
		t.Errorf("combinator = %d, want general sibling", cs.parts[0].combinator)
	}
}

func TestParseCompoundSelector_ThreeParts(t *testing.T) {
	cs := parseCompoundSelector("div > ul > li")
	if len(cs.parts) != 3 {
		t.Fatalf("parts len = %d, want 3", len(cs.parts))
	}
	if cs.parts[0].sel.Tag != "div" || cs.parts[0].combinator != combinatorChild {
		t.Errorf("parts[0] = %+v", cs.parts[0])
	}
	if cs.parts[1].sel.Tag != "ul" || cs.parts[1].combinator != combinatorChild {
		t.Errorf("parts[1] = %+v", cs.parts[1])
	}
	if cs.parts[2].sel.Tag != "li" {
		t.Errorf("parts[2] tag = %q", cs.parts[2].sel.Tag)
	}
}

func TestParseCompoundSelector_MixedCombinators(t *testing.T) {
	cs := parseCompoundSelector("div p > span")
	if len(cs.parts) != 3 {
		t.Fatalf("parts len = %d, want 3", len(cs.parts))
	}
	if cs.parts[0].combinator != combinatorDescendant {
		t.Errorf("parts[0] combinator = %d, want descendant", cs.parts[0].combinator)
	}
	if cs.parts[1].combinator != combinatorChild {
		t.Errorf("parts[1] combinator = %d, want child", cs.parts[1].combinator)
	}
}

func TestParseCompoundSelector_WithClassAndID(t *testing.T) {
	cs := parseCompoundSelector("div.container > p#intro")
	if len(cs.parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(cs.parts))
	}
	if cs.parts[0].sel.Tag != "div" || len(cs.parts[0].sel.Classes) != 1 || cs.parts[0].sel.Classes[0] != "container" {
		t.Errorf("parts[0] = tag=%q classes=%v", cs.parts[0].sel.Tag, cs.parts[0].sel.Classes)
	}
	if cs.parts[1].sel.Tag != "p" || cs.parts[1].sel.ID != "intro" {
		t.Errorf("parts[1] = tag=%q id=%q", cs.parts[1].sel.Tag, cs.parts[1].sel.ID)
	}
}

// ---------------------------------------------------------------------------
// Compound selector context matching tests
// ---------------------------------------------------------------------------

func TestCompoundSelector_MatchesChild(t *testing.T) {
	cs := parseCompoundSelector("div > p")
	ancestors := []elementInfo{
		{tagName: "div", attrs: nil, depth: 1},
	}
	if !cs.matchesWithContext("p", nil, ancestors, nil) {
		t.Error("p should match 'div > p' with div parent")
	}
	// Wrong parent.
	ancestors2 := []elementInfo{
		{tagName: "span", attrs: nil, depth: 1},
	}
	if cs.matchesWithContext("p", nil, ancestors2, nil) {
		t.Error("p should not match 'div > p' with span parent")
	}
}

func TestCompoundSelector_MatchesDescendant(t *testing.T) {
	cs := parseCompoundSelector("div p")
	ancestors := []elementInfo{
		{tagName: "div", attrs: nil, depth: 1},
		{tagName: "section", attrs: nil, depth: 2},
	}
	if !cs.matchesWithContext("p", nil, ancestors, nil) {
		t.Error("p should match 'div p' with div as grandparent")
	}
}

func TestCompoundSelector_MatchesAdjacentSibling(t *testing.T) {
	cs := parseCompoundSelector("h1 + p")
	siblings := []elementInfo{
		{tagName: "h1", attrs: nil, depth: 1},
	}
	if !cs.matchesWithContext("p", nil, nil, siblings) {
		t.Error("p should match 'h1 + p' with h1 as previous sibling")
	}
	// Non-adjacent sibling.
	siblings2 := []elementInfo{
		{tagName: "h1", attrs: nil, depth: 1},
		{tagName: "span", attrs: nil, depth: 1},
	}
	if cs.matchesWithContext("p", nil, nil, siblings2) {
		t.Error("p should not match 'h1 + p' when h1 is not immediately preceding")
	}
}

func TestCompoundSelector_MatchesGeneralSibling(t *testing.T) {
	cs := parseCompoundSelector("h1 ~ p")
	siblings := []elementInfo{
		{tagName: "h1", attrs: nil, depth: 1},
		{tagName: "span", attrs: nil, depth: 1},
	}
	if !cs.matchesWithContext("p", nil, nil, siblings) {
		t.Error("p should match 'h1 ~ p' with h1 as any preceding sibling")
	}
}

func TestCompoundSelector_NoAncestors(t *testing.T) {
	cs := parseCompoundSelector("div > p")
	if cs.matchesWithContext("p", nil, nil, nil) {
		t.Error("p should not match 'div > p' with no ancestors")
	}
}

func TestCompoundSelector_DeepChain(t *testing.T) {
	cs := parseCompoundSelector("div > ul > li")
	ancestors := []elementInfo{
		{tagName: "div", attrs: nil, depth: 1},
		{tagName: "ul", attrs: nil, depth: 2},
	}
	if !cs.matchesWithContext("li", nil, ancestors, nil) {
		t.Error("li should match 'div > ul > li' with correct ancestor chain")
	}
	// Wrong intermediate.
	ancestors2 := []elementInfo{
		{tagName: "div", attrs: nil, depth: 1},
		{tagName: "ol", attrs: nil, depth: 2},
	}
	if cs.matchesWithContext("li", nil, ancestors2, nil) {
		t.Error("li should not match 'div > ul > li' with ol instead of ul")
	}
}

func TestCompoundSelector_DescendantWithAttributes(t *testing.T) {
	cs := parseCompoundSelector(".container span")
	ancestors := []elementInfo{
		{tagName: "div", attrs: map[string]string{"class": "container wide"}, depth: 1},
		{tagName: "p", attrs: nil, depth: 2},
	}
	if !cs.matchesWithContext("span", nil, ancestors, nil) {
		t.Error("span should match '.container span' with .container ancestor")
	}
}
