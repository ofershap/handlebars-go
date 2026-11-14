package handlebars

import "fmt"

// TemplateError names the line and column of a lex/parse failure and quotes
// the offending text. It is the only error type Parse returns.
type TemplateError struct {
	Line int
	Col  int
	Text string
	Msg  string
}

func (e *TemplateError) Error() string {
	return fmt.Sprintf("handlebars: %d:%d: %s (near %q)", e.Line, e.Col, e.Msg, e.Text)
}
