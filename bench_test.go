package handlebars

import "testing"

var benchCtx = map[string]any{
	"user": map[string]any{"name": "Dana", "tags": []string{"a", "b", "c"}},
}

func BenchmarkExecute(b *testing.B) {
	tpl, err := Parse("Hello {{user.name}}! {{#each user.tags}}#{{this}} {{/each}}")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := tpl.Execute(benchCtx, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParse(b *testing.B) {
	src := "Hello {{user.name}}! {{#each user.tags}}#{{this}} {{/each}}"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}
