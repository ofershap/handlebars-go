package handlebars

// Token kinds produced by the lexer. Handlebars syntax is small enough that a
// hand-rolled scanner beats a generated one: we control error positions and
// keep the stdlib-only promise.
type tokenKind int

const (
	tokEOF           tokenKind = iota
	tokContent                 // raw text between mustaches
	tokOpen                    // {{
	tokOpenRaw                 // {{{ (unescaped)
	tokOpenAmp                 // {{& (unescaped)
	tokOpenBlock               // {{#
	tokOpenInverse             // {{^
	tokOpenPartial             // {{>
	tokOpenComment             // {{! or {{!--
	tokOpenRawBlock            // {{{{  (raw block: {{{{helper}}}} ... {{{{/helper}}}})
	tokClose                   // }}
	tokCloseRaw                // }}}
	tokCloseRawBlock           // }}}}
	tokIdent                   // path, helper name, @data var, literal
	tokString                  // "..." or '...'
	tokNumber                  // 123, -1.5
	tokBool                    // true / false (as param literal)
	tokNull                    // null / undefined (as param literal)
	tokEquals                  // = (hash args)
	tokOpenSexpr               // (
	tokCloseSexpr              // )
	tokOpenBracket             // [ segment-literal open in a path
	tokCloseBracket            // ]
	tokSlash                   // / (path separator and closing-tag marker)
	tokDot                     // . (this / ../ handled at path level)
	tokElse                    // {{else}} or {{^}} inside a block
	tokAs                      // | (block param delimiter: |x y|)
	tokStar                    // * (decorator reference, e.g. {{* foo}})
)

type token struct {
	kind tokenKind
	val  string
	line int
	col  int
}
