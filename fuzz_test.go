package handlebars

import "testing"

// go test -fuzz=FuzzParse -fuzztime=60s
// Seed corpus: the golden templates exercise every construct.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"Hello {{name}}!", "{{#each list}}{{@index}}{{/each}}", "{{> p}}",
		"{{{{raw}}}}x{{{{/raw}}}}", "{{~a~}}", "{{#if a}}{{else}}{{/if}}",
		"{{a.b.[1].c}}", "{{!-- c --}}", "{{helper a=1 b=\"s\"}}",
		"{{#with a}}{{../b}}{{/with}}", "{{(sub x)}}", "{{@root.x}}",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		tpl, err := Parse(src)
		if err != nil {
			return // parse errors are fine; panics are not
		}
		ctx := map[string]any{"a": 1, "list": []any{1, "x", nil}, "name": "n"}
		_, _ = tpl.Execute(ctx, nil) // execution errors fine; panics are not
	})
}
