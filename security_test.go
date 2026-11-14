package handlebars

import (
	"strings"
	"testing"
)

// Trust model under test: templates are trusted-author artifacts (whoever
// writes the template controls helper calls); template DATA may be hostile.
// Parse+fuzz covers hostile template text; these tests cover the runtime
// boundary: recursion, amplification, escaping, dangerous keys, helpers.

func TestRecursivePartialBounded(t *testing.T) {
	self, err := Parse("x{{> self}}")
	if err != nil {
		t.Fatal(err)
	}
	opts := &Options{Partials: map[string]*Template{"self": self}}
	_, err = Render("{{> self}}", nil, opts)
	if err == nil || !strings.Contains(err.Error(), "recursion") {
		t.Fatalf("want recursion error, got %v", err)
	}
}

func TestDeepNestedBlocks(t *testing.T) {
	// 2000-deep nesting: must not crash; parse recursion is stack-safe.
	var b strings.Builder
	for i := 0; i < 2000; i++ {
		b.WriteString("{{#if ok}}")
	}
	b.WriteString("deep")
	for i := 0; i < 2000; i++ {
		b.WriteString("{{/if}}")
	}
	out, err := Render(b.String(), map[string]any{"ok": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "deep" {
		t.Fatal(out)
	}
}

func TestAmplificationBoundedByData(t *testing.T) {
	// 100k-iteration each: completes; output is proportional to data size.
	// There is no built-in output cap - callers rendering hostile data must
	// bound the data (documented in the safety FAQ).
	items := make([]string, 100000)
	for i := range items {
		items[i] = "x"
	}
	out, err := Render("{{#each items}}x{{/each}}", map[string]any{"items": items}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 100000 {
		t.Fatal(len(out))
	}
}

func TestReservedKeysNotAccessible(t *testing.T) {
	ctx := map[string]any{"__proto__": "polluted", "safe": "ok"}
	if got := render(t, "[{{__proto__}}][{{safe}}]", ctx, nil); got != "[][ok]" {
		t.Fatal(got)
	}
}

func TestEscapeByDefaultAndBypasses(t *testing.T) {
	ctx := map[string]any{"h": "<script>alert(1)</script>"}
	if got := render(t, "{{h}}", ctx, nil); !strings.Contains(got, "&lt;script&gt;") {
		t.Fatal(got)
	}
	// triple-stache bypasses (documented, matches handlebars.js)
	if got := render(t, "{{{h}}}", ctx, nil); got != "<script>alert(1)</script>" {
		t.Fatal(got)
	}
	// SafeString bypasses (documented)
	opts := &Options{Helpers: map[string]Helper{
		"raw": func(c *HelperCall) (any, error) { return SafeString("<b>ok</b>"), nil },
	}}
	if got := render(t, "{{raw}}", ctx, opts); got != "<b>ok</b>" {
		t.Fatal(got)
	}
}

func TestMissingHelperErrors(t *testing.T) {
	// call-shaped unknown name errors (never silently empty)
	if _, err := Render("{{ghost 1}}", map[string]any{}, nil); err == nil {
		t.Fatal("want unknown-helper error")
	}
	// strict mode: missing field errors
	if _, err := Render("{{missing}}", map[string]any{}, &Options{Strict: true}); err == nil {
		t.Fatal("want strict error")
	}
	// knownHelpersOnly: unlisted helper errors even if registered
	opts := &Options{
		KnownHelpersOnly: true,
		KnownHelpers:     map[string]bool{"other": true},
		Helpers:          map[string]Helper{"h": func(c *HelperCall) (any, error) { return "x", nil }},
	}
	if _, err := Render("{{h 1}}", map[string]any{}, opts); err == nil {
		t.Fatal("want knownHelpersOnly error")
	}
}

func TestHelperRunsWithCallerPrivileges(t *testing.T) {
	// Documents the boundary: a helper CAN run arbitrary code (here: records
	// a call). Templates choose helpers; treat template text as code-author
	// trust, exactly like handlebars.js.
	called := false
	opts := &Options{Helpers: map[string]Helper{
		"sideEffect": func(c *HelperCall) (any, error) { called = true; return nil, nil },
	}}
	if _, err := Render("{{sideEffect}}", map[string]any{}, opts); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("helper should have run")
	}
}

type methodDoc struct{ secret string }

func (m methodDoc) Secret() string { return m.secret }

func TestStructMethodResolution(t *testing.T) {
	// Documents that exported zero-arg methods on data structs are callable
	// from templates (raymond-compatible behavior). Pass only data you are
	// comfortable exposing; do not hand templates structs with dangerous
	// methods.
	out := render(t, "{{Secret}}", methodDoc{secret: "visible"}, nil)
	if out != "visible" {
		t.Fatal(out)
	}
}
