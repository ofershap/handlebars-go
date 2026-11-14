package handlebars

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// lex scans a template into tokens. Whitespace inside mustaches is skipped;
// whitespace control ({{~ and ~}}) is recorded by trimming the adjacent
// content token, matching handlebars.js behavior.
func lex(src string) ([]token, error) {
	l := &lexer{src: src}
	var toks []token
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		toks = append(toks, t)
		if t.kind == tokEOF {
			return toks, nil
		}
	}
}

type lexer struct {
	src    string
	pos    int
	line   int
	col    int
	inMust bool // inside a {{ ... }} expression
}

func (l *lexer) errf(msg, text string) error {
	return &TemplateError{Line: l.line + 1, Col: l.col + 1, Text: text, Msg: msg}
}

// peek returns the upcoming substring of length n, or less at EOF.
func (l *lexer) peek(n int) string {
	end := l.pos + n
	if end > len(l.src) {
		end = len(l.src)
	}
	return l.src[l.pos:end]
}

func (l *lexer) advance(n int) {
	for i := 0; i < n && l.pos < len(l.src); i++ {
		if l.src[l.pos] == '\n' {
			l.line++
			l.col = 0
		} else {
			l.col++
		}
		l.pos++
	}
}

func (l *lexer) next() (token, error) {
	if l.pos >= len(l.src) {
		return token{kind: tokEOF, line: l.line + 1, col: l.col + 1}, nil
	}
	if !l.inMust {
		if strings.HasPrefix(l.peek(2), "{{") {
			return l.mustache()
		}
		return l.content()
	}
	return l.mustache()
}

// content consumes raw text up to the next mustache opener.
func (l *lexer) content() (token, error) {
	start, line, col := l.pos, l.line+1, l.col+1
	i := strings.Index(l.src[l.pos:], "{{")
	if i < 0 {
		l.advance(len(l.src) - l.pos)
		return token{kind: tokContent, val: l.src[start:], line: line, col: col}, nil
	}
	l.advance(i)
	tok := token{kind: tokContent, val: l.src[start:l.pos], line: line, col: col}
	return tok, nil
}

var openers = []struct {
	pat  string
	kind tokenKind
}{
	{"{{{{", tokOpenRawBlock},
	{"{{{", tokOpenRaw},
	{"{{!--", tokOpenComment},
	{"{{!", tokOpenComment},
	{"{{#", tokOpenBlock},
	{"{{^", tokOpenInverse},
	{"{{>", tokOpenPartial},
	{"{{&", tokOpenAmp},
	{"{{~", tokOpen}, // whitespace-control open: treat as tokOpen, trim handled by parser via val
	{"{{", tokOpen},
}

func (l *lexer) mustache() (token, error) {
	line, col := l.line+1, l.col+1
	if !l.inMust {
		// at a "{{" boundary
		for _, o := range openers {
			if strings.HasPrefix(l.peek(len(o.pat)), o.pat) {
				l.advance(len(o.pat))
				t := token{kind: o.kind, line: line, col: col}
				if o.pat == "{{~" {
					t.val = "~"
				}
				if o.kind == tokOpenComment {
					if o.pat == "{{!--" {
						t.val = "long"
					}
					return l.comment(t)
				}
				l.inMust = true
				if o.kind == tokOpenRawBlock {
					l.inMust = true
				}
				return t, nil
			}
		}
		// lone "{" inside content mode shouldn't reach here
	}
	// inside mustache: skip spaces
	for l.pos < len(l.src) {
		r, n := utf8.DecodeRuneInString(l.src[l.pos:])
		if !unicode.IsSpace(r) {
			break
		}
		l.advance(n)
	}
	line, col = l.line+1, l.col+1
	if l.pos >= len(l.src) {
		return token{}, l.errf("unclosed expression", "{{")
	}
	switch {
	case strings.HasPrefix(l.peek(4), "}}}}"):
		l.advance(4)
		l.inMust = false
		return token{kind: tokCloseRawBlock, line: line, col: col}, nil
	case strings.HasPrefix(l.peek(3), "}}}"):
		l.advance(3)
		l.inMust = false
		return token{kind: tokCloseRaw, line: line, col: col}, nil
	case strings.HasPrefix(l.peek(3), "~}}"):
		l.advance(3)
		l.inMust = false
		return token{kind: tokClose, val: "~", line: line, col: col}, nil
	case strings.HasPrefix(l.peek(2), "}}"):
		l.advance(2)
		l.inMust = false
		return token{kind: tokClose, line: line, col: col}, nil
	}
	ch := l.src[l.pos]
	switch ch {
	case '(':
		l.advance(1)
		return token{kind: tokOpenSexpr, line: line, col: col}, nil
	case ')':
		l.advance(1)
		return token{kind: tokCloseSexpr, line: line, col: col}, nil
	case '=':
		if strings.HasPrefix(l.peek(2), "=>") {
			return token{}, l.errf("unexpected =>", "=>")
		}
		l.advance(1)
		return token{kind: tokEquals, line: line, col: col}, nil
	case '[':
		l.advance(1)
		return token{kind: tokOpenBracket, line: line, col: col}, nil
	case ']':
		l.advance(1)
		return token{kind: tokCloseBracket, line: line, col: col}, nil
	case '/':
		l.advance(1)
		return token{kind: tokSlash, line: line, col: col}, nil
	case '|':
		l.advance(1)
		return token{kind: tokAs, line: line, col: col}, nil
	case '*':
		l.advance(1)
		return token{kind: tokStar, line: line, col: col}, nil
	case '"', '\'':
		return l.stringLit(ch)
	}
	// number?
	if ch == '-' || (ch >= '0' && ch <= '9') {
		if t, ok := l.number(); ok {
			return t, nil
		}
	}
	// identifier / path (may include . and / inside, handled by parser);
	// booleans/null recognized by value.
	start := l.pos
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if unicode.IsSpace(rune(c)) || strings.ContainsRune("()=[]{}<>|*'\"=", rune(c)) {
			break
		}
		// "~}}" handled above; stop ident at "~}}" or "}}"
		if c == '~' && strings.HasPrefix(l.peek(3), "~}}") {
			break
		}
		if c == '}' && strings.HasPrefix(l.peek(2), "}}") {
			break
		}
		l.advance(1)
	}
	if l.pos == start {
		return token{}, l.errf("unexpected character", string(ch))
	}
	word := l.src[start:l.pos]
	switch word {
	case "true", "false":
		return token{kind: tokBool, val: word, line: line, col: col}, nil
	case "null", "undefined":
		return token{kind: tokNull, val: word, line: line, col: col}, nil
	case "else":
		return token{kind: tokElse, line: line, col: col}, nil
	}
	return token{kind: tokIdent, val: word, line: line, col: col}, nil
}

// comment consumes through the comment closer. {{!-- --}} may contain }}.
func (l *lexer) comment(t token) (token, error) {
	if t.val == "long" {
		i := strings.Index(l.src[l.pos:], "--}}")
		if i < 0 {
			return token{}, l.errf("unclosed comment", "{{!--")
		}
		l.advance(i + 4)
		return t, nil
	}
	i := strings.Index(l.src[l.pos:], "}}")
	if i < 0 {
		return token{}, l.errf("unclosed comment", "{{!")
	}
	l.advance(i + 2)
	return t, nil
}

func (l *lexer) stringLit(quote byte) (token, error) {
	line, col := l.line+1, l.col+1
	l.advance(1) // opening quote
	var b strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\\' && l.pos+1 < len(l.src) {
			l.advance(1)
			b.WriteByte(l.src[l.pos])
			l.advance(1)
			continue
		}
		if c == quote {
			l.advance(1)
			return token{kind: tokString, val: b.String(), line: line, col: col}, nil
		}
		b.WriteByte(c)
		l.advance(1)
	}
	return token{}, l.errf("unclosed string literal", string(quote))
}

func (l *lexer) number() (token, bool) {
	line, col := l.line+1, l.col+1
	start := l.pos
	if l.src[l.pos] == '-' {
		l.advance(1)
	}
	digits := 0
	for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
		l.advance(1)
		digits++
	}
	if l.pos < len(l.src) && l.src[l.pos] == '.' && l.pos+1 < len(l.src) && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9' {
		l.advance(1)
		for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
			l.advance(1)
			digits++
		}
	}
	if digits == 0 {
		l.pos, l.line, l.col = start, line-1, col-1
		return token{}, false
	}
	return token{kind: tokNumber, val: l.src[start:l.pos], line: line, col: col}, true
}
