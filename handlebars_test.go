package handlebars

import (
	"strings"
	"testing"
)

func render(t *testing.T, src string, ctx any, opts *Options) string {
	t.Helper()
	out, err := Render(src, ctx, opts)
	if err != nil {
		t.Fatalf("Render(%q) error: %v", src, err)
	}
	return out
}

func TestContent(t *testing.T) {
	if got := render(t, "hello world", nil, nil); got != "hello world" {
		t.Fatal(got)
	}
}

func TestInterpolation(t *testing.T) {
	ctx := map[string]any{"name": "<Ofer>", "age": 40}
	if got := render(t, "hi {{name}}, {{age}}", ctx, nil); got != "hi &lt;Ofer&gt;, 40" {
		t.Fatal(got)
	}
	if got := render(t, "hi {{{name}}} {{& name}}", ctx, nil); got != "hi <Ofer> <Ofer>" {
		t.Fatal(got)
	}
}

func TestNestedPath(t *testing.T) {
	ctx := map[string]any{"user": map[string]any{"name": "dana"}}
	if got := render(t, "{{user.name}} {{user/missing}}", ctx, nil); got != "dana " {
		t.Fatal(got)
	}
}

func TestEach(t *testing.T) {
	ctx := map[string]any{"items": []string{"a", "b", "c"}}
	got := render(t, "{{#each items}}[{{@index}}:{{this}} first={{@first}} last={{@last}}]{{/each}}", ctx, nil)
	want := "[0:a first=true last=false][1:b first=false last=false][2:c first=false last=true]"
	if got != want {
		t.Fatal(got)
	}
}

func TestIfUnless(t *testing.T) {
	ctx := map[string]any{"ok": true, "no": false}
	if got := render(t, "{{#if ok}}Y{{else}}N{{/if}}{{#unless no}}U{{/unless}}", ctx, nil); got != "YU" {
		t.Fatal(got)
	}
	// includeZero
	if got := render(t, "{{#if n includeZero=true}}Z{{/if}}", map[string]any{"n": 0}, nil); got != "Z" {
		t.Fatal(got)
	}
}

func TestWithAndParent(t *testing.T) {
	ctx := map[string]any{"user": map[string]any{"name": "dana"}, "suffix": "!"}
	if got := render(t, "{{#with user}}{{name}}{{../suffix}}{{/with}}", ctx, nil); got != "dana!" {
		t.Fatal(got)
	}
}

func TestSection(t *testing.T) {
	ctx := map[string]any{"people": []map[string]any{{"n": "a"}, {"n": "b"}}}
	if got := render(t, "{{#people}}{{n}};{{/people}}", ctx, nil); got != "a;b;" {
		t.Fatal(got)
	}
	ctx2 := map[string]any{"people": []string{}}
	if got := render(t, "{{#people}}x{{else}}empty{{/people}}", ctx2, nil); got != "empty" {
		t.Fatal(got)
	}
}

func TestComment(t *testing.T) {
	if got := render(t, "a{{! gone }}b{{!-- also {{gone}} --}}c", nil, nil); got != "abc" {
		t.Fatal(got)
	}
}

func TestStrict(t *testing.T) {
	_, err := Render("{{missing}}", map[string]any{}, &Options{Strict: true})
	if err == nil {
		t.Fatal("strict should error on missing field")
	}
	_, err = Render("{{present}}", map[string]any{"present": 1}, &Options{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubexpression(t *testing.T) {
	opts := &Options{Helpers: map[string]Helper{
		"shout": func(c *HelperCall) (any, error) { return strings.ToUpper(stringify(c.Params[0])), nil },
		"excl":  func(c *HelperCall) (any, error) { return stringify(c.Params[0]) + "!", nil },
	}}
	if got := render(t, "{{shout (excl name)}}", map[string]any{"name": "hi"}, opts); got != "HI!" {
		t.Fatal(got)
	}
}

func TestPartials(t *testing.T) {
	pt := MustParsePartial(t, "P:{{v}}")
	opts := &Options{Partials: map[string]*Template{"p": pt}}
	if got := render(t, "{{> p}}", map[string]any{"v": 7}, opts); got != "P:7" {
		t.Fatal(got)
	}
	// hash overrides
	if got := render(t, "{{> p v=9}}", map[string]any{"v": 7}, opts); got != "P:9" {
		t.Fatal(got)
	}
}

func MustParsePartial(t *testing.T, src string) *Template {
	t.Helper()
	tpl, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return tpl
}

func TestBlockParams(t *testing.T) {
	ctx := map[string]any{"items": []string{"x", "y"}}
	got := render(t, "{{#each items as |it i|}}{{i}}={{it}};{{/each}}", ctx, nil)
	if got != "0=x;1=y;" {
		t.Fatal(got)
	}
}

func TestRawBlock(t *testing.T) {
	got := render(t, "{{{{raw}}}}{{not-executed}}{{{{/raw}}}}", nil, nil)
	if got != "{{not-executed}}" {
		t.Fatal(got)
	}
}

func TestDataVars(t *testing.T) {
	ctx := map[string]any{"items": []string{"only"}}
	opts := &Options{Data: map[string]any{"app": "demo"}}
	got := render(t, "{{@app}}:{{#each items}}{{@index}}{{/each}}", ctx, opts)
	if got != "demo:0" {
		t.Fatal(got)
	}
}

func TestStringParams(t *testing.T) {
	opts := &Options{StringParams: true, Helpers: map[string]Helper{
		"echo": func(c *HelperCall) (any, error) { return c.Params[0], nil },
	}}
	if got := render(t, "{{echo missing.path}}", map[string]any{}, opts); got != "missing.path" {
		t.Fatal(got)
	}
}

func TestParseErrorPosition(t *testing.T) {
	_, err := Parse("line1\nline2 {{unclosed")
	te, ok := err.(*TemplateError)
	if !ok {
		t.Fatalf("want TemplateError, got %v", err)
	}
	if te.Line != 2 {
		t.Fatalf("line: %d", te.Line)
	}
}
