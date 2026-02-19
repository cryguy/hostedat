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
