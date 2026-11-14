package handlebars

import "strings"

// escapeHTML mirrors handlebars.js' escapeExpression map exactly:
// & < > " ' ` =
var escReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#x27;",
	"`", "&#x60;",
	"=", "&#x3D;",
)

func escapeHTML(s string) string { return escReplacer.Replace(s) }
