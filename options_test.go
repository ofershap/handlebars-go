package handlebars

import "testing"

func TestTrackIds(t *testing.T) {
	var gotIds []string
	opts := &Options{
		TrackIds: true,
		Helpers: map[string]Helper{
			"cap": func(c *HelperCall) (any, error) {
				gotIds = c.Ids
				return nil, nil
			},
		},
	}
	if _, err := Render("{{cap user.name 42}}", map[string]any{"user": map[string]any{"name": "x"}}, opts); err != nil {
		t.Fatal(err)
	}
	if len(gotIds) != 2 || gotIds[0] != "user.name" || gotIds[1] != "" {
		t.Fatalf("ids: %v", gotIds)
	}
	// off by default
	opts.TrackIds = false
	gotIds = nil
	if _, err := Render("{{cap user.name}}", map[string]any{"user": map[string]any{"name": "x"}}, opts); err != nil {
		t.Fatal(err)
	}
	if gotIds != nil {
		t.Fatalf("ids should be nil without trackIds: %v", gotIds)
	}
}

func TestKnownHelpers(t *testing.T) {
	// helper listed in KnownHelpers wins over a same-named context field
	opts := &Options{
		KnownHelpers: map[string]bool{"up": true},
		Helpers: map[string]Helper{
			"up": func(c *HelperCall) (any, error) { return "HELPER", nil },
		},
	}
	if got := render(t, "{{up}}", map[string]any{"up": "field"}, opts); got != "HELPER" {
		t.Fatal(got)
	}
	// without KnownHelpers, the context field wins (matches handlebars.js)
	opts.KnownHelpers = nil
	if got := render(t, "{{up}}", map[string]any{"up": "field"}, opts); got != "field" {
		t.Fatal(got)
	}
}

func TestHelperMissingOverride(t *testing.T) {
	opts := &Options{Helpers: map[string]Helper{
		"helperMissing": func(c *HelperCall) (any, error) { return "MISSING!", nil },
	}}
	if got := render(t, "{{ghost 1}}", map[string]any{}, opts); got != "MISSING!" {
		t.Fatal(got)
	}
}

func TestErrorLineCol(t *testing.T) {
	_, err := Parse("ok\nok\n{{#if x}}unclosed")
	te, ok := err.(*TemplateError)
	if !ok {
		t.Fatalf("want TemplateError, got %v", err)
	}
	if te.Line != 3 {
		t.Fatalf("line: %d (err: %v)", te.Line, err)
	}
}
