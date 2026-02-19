package worker

import (
	"encoding/json"
	"strings"
	"testing"

	gohtml "golang.org/x/net/html"
)

func TestHTMLRewriter_SetAttribute(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div id="old">Hello</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    var rw = new HTMLRewriter()
      .on('div', {
        element: function(el) {
          el.setAttribute('id', 'new');
          el.setAttribute('class', 'modified');
        }
      });
    var transformed = rw.transform(res);
    return transformed;
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, `id="new"`) {
		t.Errorf("body should contain id='new', got %q", body)
	}
	if !strings.Contains(body, `class="modified"`) {
		t.Errorf("body should contain class='modified', got %q", body)
	}
}

func TestHTMLRewriter_Before(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>Hello</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          el.before('<span>Before</span>', { html: true });
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "<span>Before</span>") {
		t.Errorf("body should contain before content, got %q", body)
	}
	divIdx := strings.Index(body, "<div")
	beforeIdx := strings.Index(body, "<span>Before</span>")
	if beforeIdx >= divIdx {
		t.Errorf("before content should appear before div, got %q", body)
	}
}

func TestHTMLRewriter_After(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>Hello</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          el.after('<span>After</span>', { html: true });
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "<span>After</span>") {
		t.Errorf("body should contain after content, got %q", body)
	}
	endDivIdx := strings.Index(body, "</div>")
	afterIdx := strings.Index(body, "<span>After</span>")
	if afterIdx <= endDivIdx {
		t.Errorf("after content should appear after </div>, got %q", body)
	}
}

func TestHTMLRewriter_PrependAppend(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>Content</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          el.prepend('<b>Start</b>', { html: true });
          el.append('<b>End</b>', { html: true });
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "<b>Start</b>") {
		t.Errorf("body should contain prepend content, got %q", body)
	}
	if !strings.Contains(body, "<b>End</b>") {
		t.Errorf("body should contain append content, got %q", body)
	}
}

func TestHTMLRewriter_SetInnerContent(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><span>Old</span></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          el.setInnerContent('<b>New</b>', { html: true });
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "<b>New</b>") {
		t.Errorf("body should contain new inner content, got %q", body)
	}
	if strings.Contains(body, "<span>Old</span>") {
		t.Errorf("body should not contain old inner content, got %q", body)
	}
}

func TestHTMLRewriter_Remove(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><span class="remove">Bad</span><span>Keep</span></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('.remove', {
        element: function(el) {
          el.remove();
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "Bad") {
		t.Errorf("body should not contain removed element content, got %q", body)
	}
	if !strings.Contains(body, "Keep") {
		t.Errorf("body should still contain non-removed content, got %q", body)
	}
}

func TestHTMLRewriter_TextHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>Original Text</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        text: function(text) {
          if (text.text.trim()) {
            text.replace('Replaced Text');
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "Original") {
		t.Errorf("body should not contain original text, got %q", body)
	}
	if !strings.Contains(body, "Replaced Text") {
		t.Errorf("body should contain replacement text, got %q", body)
	}
}

func TestHTMLRewriter_CommentHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><!-- secret comment -->Visible</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        comments: function(comment) {
          comment.remove();
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "secret comment") {
		t.Errorf("body should not contain removed comment, got %q", body)
	}
	if !strings.Contains(body, "Visible") {
		t.Errorf("body should still contain visible text, got %q", body)
	}
}

func TestHTMLRewriter_DocumentEndHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>Body</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .onDocument({
        end: function(end) {
          end.append('<!-- footer -->', { html: true });
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.HasSuffix(body, "<!-- footer -->") {
		t.Errorf("body should end with footer comment, got %q", body)
	}
}

func TestHTMLRewriter_SelfClosingTag(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<img src="old.jpg" />';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('img', {
        element: function(el) {
          el.setAttribute('src', 'new.jpg');
          el.setAttribute('alt', 'Photo');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, `src="new.jpg"`) {
		t.Errorf("body should have new src, got %q", body)
	}
	if !strings.Contains(body, `alt="Photo"`) {
		t.Errorf("body should have alt attribute, got %q", body)
	}
}

func TestHTMLRewriter_TagNameChange(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<h1>Title</h1>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('h1', {
        element: function(el) {
          el.tagName = 'h2';
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "<h2") {
		t.Errorf("body should contain h2 tag, got %q", body)
	}
}

func TestHTMLRewriter_MultipleSelectors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<h1>Title</h1><p>Body</p>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    var tags = [];
    return new HTMLRewriter()
      .on('h1', {
        element: function(el) {
          el.setAttribute('class', 'title');
        }
      })
      .on('p', {
        element: function(el) {
          el.setAttribute('class', 'body');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, `class="title"`) {
		t.Errorf("h1 should have class=title, got %q", body)
	}
	if !strings.Contains(body, `class="body"`) {
		t.Errorf("p should have class=body, got %q", body)
	}
}

func TestHTMLRewriter_PreservesResponseMetadata(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>Hello</div>';
    var res = new Response(html, {
      status: 201,
      headers: { 'Content-Type': 'text/html', 'X-Custom': 'test' }
    });
    var transformed = new HTMLRewriter()
      .on('div', { element: function(el) { el.setAttribute('id', 'x'); } })
      .transform(res);
    return Response.json({
      status: transformed.status,
      hasCustom: transformed.headers.get('X-Custom') === 'test',
    });
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	var data struct {
		Status    int  `json:"status"`
		HasCustom bool `json:"hasCustom"`
	}
	if err := json.Unmarshal(r.Response.Body, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Status != 201 {
		t.Errorf("status = %d, want 201", data.Status)
	}
	if !data.HasCustom {
		t.Error("custom header should be preserved")
	}
}

// ---------------------------------------------------------------------------
// Pure Go helper tests (no V8)
// ---------------------------------------------------------------------------

func TestVoidElement(t *testing.T) {
	voids := []string{"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"}
	for _, tag := range voids {
		if !voidElement(tag) {
			t.Errorf("voidElement(%q) = false, want true", tag)
		}
	}
}

func TestVoidElement_CaseInsensitive(t *testing.T) {
	if !voidElement("BR") {
		t.Error("voidElement should be case-insensitive")
	}
	if !voidElement("Img") {
		t.Error("voidElement should handle mixed case")
	}
}

func TestVoidElement_NonVoid(t *testing.T) {
	nonVoids := []string{"div", "span", "p", "a", "h1", "table", "form", "script", "style"}
	for _, tag := range nonVoids {
		if voidElement(tag) {
			t.Errorf("voidElement(%q) = true, want false", tag)
		}
	}
}

func TestHtmlAttrsToMap_Empty(t *testing.T) {
	m := htmlAttrsToMap(nil)
	if len(m) != 0 {
		t.Errorf("htmlAttrsToMap(nil) = %v, want empty map", m)
	}
}

func TestHtmlAttrsToMap_Multiple(t *testing.T) {
	attrs := []gohtml.Attribute{
		{Key: "id", Val: "main"},
		{Key: "class", Val: "container wide"},
		{Key: "data-x", Val: ""},
	}
	m := htmlAttrsToMap(attrs)
	if m["id"] != "main" {
		t.Errorf("id = %q, want 'main'", m["id"])
	}
	if m["class"] != "container wide" {
		t.Errorf("class = %q, want 'container wide'", m["class"])
	}
	if v, ok := m["data-x"]; !ok || v != "" {
		t.Errorf("data-x = %q, ok=%v, want empty string", v, ok)
	}
}

func TestShouldSkipContent_EmptyStack(t *testing.T) {
	if shouldSkipContent(nil, 1) {
		t.Error("shouldSkipContent(nil, 1) should be false")
	}
}

func TestShouldSkipContent_NoSkip(t *testing.T) {
	stack := []*matchedElement{
		{handlerIdx: 0, depth: 1, skipContent: false},
	}
	if shouldSkipContent(stack, 2) {
		t.Error("shouldSkipContent should be false when no elements have skipContent")
	}
}

func TestShouldSkipContent_Skip(t *testing.T) {
	stack := []*matchedElement{
		{handlerIdx: 0, depth: 1, skipContent: true},
	}
	if !shouldSkipContent(stack, 2) {
		t.Error("shouldSkipContent should be true when inside a skipped element")
	}
}

func TestShouldSkipContent_DepthMismatch(t *testing.T) {
	stack := []*matchedElement{
		{handlerIdx: 0, depth: 5, skipContent: true},
	}
	// depth 3 is NOT inside depth 5
	if shouldSkipContent(stack, 3) {
		t.Error("shouldSkipContent should be false when depth is less than element depth")
	}
}

// ---------------------------------------------------------------------------
// Additional integration tests
// ---------------------------------------------------------------------------

func TestHTMLRewriter_RemoveVoidElement(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><img src="old.jpg" /><p>Text</p></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('img', {
        element: function(el) {
          el.remove();
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "img") {
		t.Errorf("body should not contain removed img, got %q", body)
	}
	if !strings.Contains(body, "Text") {
		t.Errorf("body should still contain text, got %q", body)
	}
}

func TestHTMLRewriter_DocumentTextHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<p>Hello World</p>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .onDocument({
        text: function(text) {
          if (text.text.includes('World')) {
            text.replace('Universe');
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "World") {
		t.Errorf("body should have replaced World, got %q", body)
	}
}

func TestHTMLRewriter_NullBody(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var res = new Response(null, { status: 204 });
    var transformed = new HTMLRewriter()
      .on('div', { element: function(el) { el.remove(); } })
      .transform(res);
    return new Response("ok");
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)
}

func TestHTMLRewriter_GetAttribute(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<a href="/old" class="link">Click</a>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('a', {
        element: function(el) {
          var href = el.getAttribute('href');
          el.setAttribute('href', href + '?v=2');
          if (el.hasAttribute('class')) {
            el.removeAttribute('class');
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, `href="/old?v=2"`) {
		t.Errorf("body should have updated href, got %q", body)
	}
	if strings.Contains(body, "class=") {
		t.Errorf("body should not have class attribute, got %q", body)
	}
}

func TestHTMLRewriter_Doctype(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<!DOCTYPE html><html><body><div>Hi</div></body></html>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          el.setAttribute('class', 'modified');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Errorf("doctype should be preserved, got %q", body)
	}
	if !strings.Contains(body, `class="modified"`) {
		t.Errorf("div should be modified, got %q", body)
	}
}

func TestHTMLRewriter_CommentReplace(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><!-- old comment -->Content</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        comments: function(comment) {
          comment.replace('replaced text');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "old comment") {
		t.Errorf("body should not contain old comment, got %q", body)
	}
	if !strings.Contains(body, "replaced text") {
		t.Errorf("body should contain replaced text, got %q", body)
	}
}

func TestHTMLRewriter_CommentBeforeAfter(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><!-- marker -->Content</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        comments: function(comment) {
          comment.before('<b>BEFORE</b>', { html: true });
          comment.after('<b>AFTER</b>', { html: true });
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "<b>BEFORE</b>") {
		t.Errorf("body should contain BEFORE, got %q", body)
	}
	if !strings.Contains(body, "<b>AFTER</b>") {
		t.Errorf("body should contain AFTER, got %q", body)
	}
}

func TestHTMLRewriter_TextBeforeAfter(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<p>Hello</p>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('p', {
        text: function(text) {
          if (text.text.trim()) {
            text.before('[', { html: false });
            text.after(']', { html: false });
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "[") || !strings.Contains(body, "]") {
		t.Errorf("body should contain brackets around text, got %q", body)
	}
}

func TestHTMLRewriter_CommentTextProperty(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><!-- hello --></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    var captured = "";
    return new HTMLRewriter()
      .on('div', {
        comments: function(comment) {
          captured = comment.text;
          comment.replace("captured: " + captured.trim());
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "captured: hello") {
		t.Errorf("body should contain captured comment text, got %q", body)
	}
}

func TestHTMLRewriter_ReplaceInnerContentHTML(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><p>old</p></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          el.setInnerContent('<b>replaced</b>', { html: true });
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "old") {
		t.Errorf("body should not contain old content, got %q", body)
	}
	if !strings.Contains(body, "<b>replaced</b>") {
		t.Errorf("body should contain replacement, got %q", body)
	}
}

func TestHTMLRewriter_NestedSelectors(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div class="outer"><div class="inner">Nested</div></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    var matched = [];
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          var cls = el.getAttribute('class');
          el.setAttribute('data-matched', 'true');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	// Both divs should be matched.
	count := strings.Count(body, `data-matched="true"`)
	if count != 2 {
		t.Errorf("expected 2 matched divs, got %d in %q", count, body)
	}
}

func TestHTMLRewriter_EscapesTextByDefault(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>Hello</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          el.before('<script>alert("xss")</script>');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	// Without {html: true}, the content should be HTML-escaped
	if strings.Contains(body, "<script>") {
		t.Errorf("injected content without html:true should be escaped, got %q", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("injected content should be HTML-escaped, got %q", body)
	}
}

func TestHTMLRewriter_DocumentEndAppend(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<html><body>Hello</body></html>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .onDocument({
        end: function(end) {
          end.append('<!-- appended -->', { html: true });
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "<!-- appended -->") {
		t.Errorf("document end append should add content, got %q", body)
	}
}

func TestHTMLRewriter_DocumentTextChunks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>first</div> between <div>second</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    var textParts = [];
    return new HTMLRewriter()
      .onDocument({
        text: function(text) {
          if (text.text.trim()) textParts.push(text.text.trim());
          if (text.lastInTextNode) {
            // set on global for retrieval
          }
        }
      })
      .on('div', {
        element: function(el) {
          el.setAttribute('data-seen', 'true');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "first") || !strings.Contains(body, "second") {
		t.Errorf("document text handler should preserve content, got %q", body)
	}
	if !strings.Contains(body, `data-seen="true"`) {
		t.Errorf("element handler should still work alongside document handler, got %q", body)
	}
}

func TestHTMLRewriter_RemoveAttribute(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div class="old" id="keep">text</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          el.removeAttribute('class');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "class=") {
		t.Errorf("class attribute should be removed, got %q", body)
	}
	if !strings.Contains(body, `id="keep"`) {
		t.Errorf("id attribute should be preserved, got %q", body)
	}
}

func TestHTMLRewriter_HasAttribute(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div data-x="1">content</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) {
          var hasX = el.hasAttribute('data-x');
          var hasY = el.hasAttribute('data-y');
          el.setAttribute('data-has-x', String(hasX));
          el.setAttribute('data-has-y', String(hasY));
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, `data-has-x="true"`) {
		t.Errorf("hasAttribute('data-x') should be true, got %q", body)
	}
	if !strings.Contains(body, `data-has-y="false"`) {
		t.Errorf("hasAttribute('data-y') should be false, got %q", body)
	}
}

func TestHTMLRewriter_AttributeSelector(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<a href="/link1">L1</a><a>L2</a><a href="/link3">L3</a>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('a[href]', {
        element: function(el) {
          el.setAttribute('class', 'has-href');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	// Count occurrences of class="has-href"
	count := strings.Count(body, `class="has-href"`)
	if count != 2 {
		t.Errorf("expected 2 elements with href matched, got %d in %q", count, body)
	}
}

func TestHTMLRewriter_CommentRemoveAll(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><!-- keep out -->Hello<!-- another --></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div', {
        comments: function(comment) {
          comment.remove();
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "<!--") {
		t.Errorf("all comments should be removed, got %q", body)
	}
	if !strings.Contains(body, "Hello") {
		t.Errorf("text content should be preserved, got %q", body)
	}
}

func TestHTMLRewriter_TextRemove(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<p>Remove this text</p>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('p', {
        text: function(text) {
          if (text.text.trim()) {
            text.remove();
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "Remove this text") {
		t.Errorf("text should be removed, got %q", body)
	}
}

func TestHTMLRewriter_DocumentTextBeforeAfter(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>Hello</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .onDocument({
        text: function(text) {
          if (text.text.trim()) {
            text.before('[START]');
            text.after('[END]');
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "[START]") {
		t.Errorf("body should contain [START], got %q", body)
	}
	if !strings.Contains(body, "[END]") {
		t.Errorf("body should contain [END], got %q", body)
	}
}

func TestHTMLRewriter_DocumentTextReplace(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<p>old content</p>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .onDocument({
        text: function(text) {
          if (text.text.includes('old')) {
            text.replace('new content');
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "old content") {
		t.Errorf("body should not have old content, got %q", body)
	}
	if !strings.Contains(body, "new content") {
		t.Errorf("body should have new content, got %q", body)
	}
}

func TestHTMLRewriter_DocumentTextRemove(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<p>remove me</p>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .onDocument({
        text: function(text) {
          if (text.text.trim()) {
            text.remove();
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if strings.Contains(body, "remove me") {
		t.Errorf("text should be removed, got %q", body)
	}
}

func TestHTMLRewriter_DocumentEndEscaped(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>Body</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .onDocument({
        end: function(end) {
          end.append('<script>alert("xss")</script>');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	// Without {html: true}, should be escaped
	if strings.Contains(body, "<script>") {
		t.Errorf("appended content without html:true should be escaped, got %q", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("appended content should be HTML-escaped, got %q", body)
	}
}

func TestHTMLRewriter_NoContentTypePassthrough(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var res = new Response('{"key":"value"}', {
      headers: { 'Content-Type': 'application/json' }
    });
    return new HTMLRewriter()
      .on('div', {
        element: function(el) { el.remove(); }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	// Non-HTML content should pass through unchanged
	body := string(r.Response.Body)
	if !strings.Contains(body, `"key"`) {
		t.Errorf("non-HTML body should pass through, got %q", body)
	}
}

func TestHTMLRewriter_MultipleTextChunks(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div>first</div><div>second</div><div>third</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    var count = 0;
    return new HTMLRewriter()
      .on('div', {
        text: function(text) {
          if (text.text.trim()) {
            count++;
            text.replace('item' + count);
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "item1") || !strings.Contains(body, "item2") || !strings.Contains(body, "item3") {
		t.Errorf("all text chunks should be replaced, got %q", body)
	}
}

// ---------------------------------------------------------------------------
// Combinator selector integration tests
// ---------------------------------------------------------------------------

func TestHTMLRewriter_ChildCombinator(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><p>Direct child</p><span><p>Nested grandchild</p></span></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div > p', {
        element: function(el) {
          el.setAttribute('class', 'matched');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	// Only the direct child <p> should be matched, not the grandchild.
	count := strings.Count(body, `class="matched"`)
	if count != 1 {
		t.Errorf("expected 1 matched p (direct child), got %d in %q", count, body)
	}
	if !strings.Contains(body, "Direct child") {
		t.Errorf("body should contain direct child text, got %q", body)
	}
}

func TestHTMLRewriter_DescendantCombinator(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><p>Child</p><section><p>Grandchild</p></section></div><p>Outside</p>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div p', {
        element: function(el) {
          el.setAttribute('class', 'inside-div');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	// Both p's inside div should match, but not the one outside.
	count := strings.Count(body, `class="inside-div"`)
	if count != 2 {
		t.Errorf("expected 2 descendant p's matched, got %d in %q", count, body)
	}
}

func TestHTMLRewriter_AdjacentSiblingCombinator(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><h1>Title</h1><p>First para</p><p>Second para</p></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('h1 + p', {
        element: function(el) {
          el.setAttribute('class', 'after-h1');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	// Only the first <p> immediately after <h1> should match.
	count := strings.Count(body, `class="after-h1"`)
	if count != 1 {
		t.Errorf("expected 1 adjacent sibling match, got %d in %q", count, body)
	}
	if !strings.Contains(body, "First para") {
		t.Errorf("body should contain first para text, got %q", body)
	}
}

func TestHTMLRewriter_GeneralSiblingCombinator(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><h1>Title</h1><span>Gap</span><p>Para 1</p><p>Para 2</p></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('h1 ~ p', {
        element: function(el) {
          el.setAttribute('class', 'sibling-of-h1');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	// Both p's follow h1 (not necessarily immediately), so both should match.
	count := strings.Count(body, `class="sibling-of-h1"`)
	if count != 2 {
		t.Errorf("expected 2 general sibling matches, got %d in %q", count, body)
	}
}

func TestHTMLRewriter_CombinatorWithTextHandler(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><p>Replace me</p></div><p>Leave me</p>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div > p', {
        text: function(text) {
          if (text.text.trim()) {
            text.replace('Replaced');
          }
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, "Replaced") {
		t.Errorf("text inside matched element should be replaced, got %q", body)
	}
	if !strings.Contains(body, "Leave me") {
		t.Errorf("text outside matched element should be preserved, got %q", body)
	}
}

func TestHTMLRewriter_NestedCombinatorChain(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div><ul><li>Item</li></ul></div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('div > ul > li', {
        element: function(el) {
          el.setAttribute('class', 'deep-match');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	if !strings.Contains(body, `class="deep-match"`) {
		t.Errorf("li should match 'div > ul > li', got %q", body)
	}
}

func TestHTMLRewriter_SimpleSelectorStillWorks(t *testing.T) {
	// Ensure simple selectors (no combinators) continue to work after refactor.
	db := testDB(t)
	e := newTestEngine(t, db)

	source := `export default {
  fetch(request, env) {
    var html = '<div class="target">Hello</div><div>World</div>';
    var res = new Response(html, { headers: { 'Content-Type': 'text/html' } });
    return new HTMLRewriter()
      .on('.target', {
        element: function(el) {
          el.setAttribute('data-found', 'yes');
        }
      })
      .transform(res);
  },
};`

	r := execJS(t, e, source, defaultEnv(), getReq("http://localhost/"))
	assertOK(t, r)

	body := string(r.Response.Body)
	count := strings.Count(body, `data-found="yes"`)
	if count != 1 {
		t.Errorf("expected 1 match for .target, got %d in %q", count, body)
	}
}
