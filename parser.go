package handlebars

import (
	"strconv"
	"strings"
)

// Parse compiles a template source into a Template.
func Parse(src string) (*Template, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	prog, err := p.program()
	if err != nil {
		return nil, err
	}
	prog = stripStandalone(prog)
	return &Template{src: src, prog: prog}, nil
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) cur() token  { return p.toks[p.pos] }
func (p *parser) bump() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) errf(t token, msg string) error {
	return &TemplateError{Line: t.line, Col: t.col, Text: t.val, Msg: msg}
}

// program parses until EOF or a block closer/inverse, which it leaves for the
// caller to consume.
func (p *parser) program() (*Program, error) {
	prog := &Program{}
	for {
		t := p.cur()
		switch t.kind {
		case tokEOF, tokSlash:
			return prog, nil
		case tokContent:
			p.bump()
			prog.Body = append(prog.Body, &ContentNode{Text: t.val})
		case tokOpenComment:
			p.bump()
			prog.Body = append(prog.Body, &CommentNode{})
		case tokOpen, tokOpenRaw, tokOpenAmp:
			if t.kind == tokOpen && p.pos+1 < len(p.toks) && p.toks[p.pos+1].kind == tokSlash {
				return prog, nil // closing tag {{/name}}: caller consumes
			}
			if t.kind == tokOpen && p.peekIsElse() {
				return prog, nil // {{else}}: caller consumes
			}
			n, err := p.simpleMustache()
			if err != nil {
				return nil, err
			}
			prog.Body = append(prog.Body, n)
		case tokOpenBlock:
			n, err := p.block(false)
			if err != nil {
				return nil, err
			}
			prog.Body = append(prog.Body, n)
		case tokOpenInverse:
			if p.pos+1 < len(p.toks) && p.toks[p.pos+1].kind == tokClose {
				return prog, nil // {{^}} inverse separator
			}
			n, err := p.block(false)
			if err != nil {
				return nil, err
			}
			prog.Body = append(prog.Body, n)
		case tokOpenRawBlock:
			n, err := p.block(true)
			if err != nil {
				return nil, err
			}
			prog.Body = append(prog.Body, n)
		case tokOpenPartial:
			n, err := p.partial()
			if err != nil {
				return nil, err
			}
			prog.Body = append(prog.Body, n)
		default:
			return nil, p.errf(t, "unexpected token")
		}
	}
}

func (p *parser) closeAfter(kind tokenKind) (token, error) {
	t := p.cur()
	if t.kind != kind {
		return t, p.errf(t, "expected closing }}")
	}
	return p.bump(), nil
}

func (p *parser) simpleMustache() (Node, error) {
	open := p.bump() // tokOpen / tokOpenRaw / tokOpenAmp
	// {{else}} at top level of a block is handled by block(); here it is an error
	e, err := p.expression(true)
	if err != nil {
		return nil, err
	}
	closeKind := tokClose
	if open.kind == tokOpenRaw {
		closeKind = tokCloseRaw
	}
	if _, err := p.closeAfter(closeKind); err != nil {
		return nil, err
	}
	return &MustacheNode{Expr: e, Unescaped: open.kind != tokOpen}, nil
}

// block parses {{#expr}} body {{else}} inverse {{/name}}.
func (p *parser) block(raw bool) (Node, error) {
	inverted := p.cur().kind == tokOpenInverse
	p.bump() // opener
	e, err := p.expression(true)
	if err != nil {
		return nil, err
	}
	// optional block params: as |x y|
	var blockParams []string
	if p.cur().kind == tokIdent && p.cur().val == "as" {
		p.bump()
		if p.cur().kind != tokAs {
			return nil, p.errf(p.cur(), "expected | after as")
		}
		p.bump()
		for p.cur().kind == tokIdent {
			blockParams = append(blockParams, p.bump().val)
		}
		if p.cur().kind != tokAs {
			return nil, p.errf(p.cur(), "expected closing |")
		}
		p.bump()
	}
	closeKind := tokClose
	if raw {
		closeKind = tokCloseRawBlock
	}
	if _, err := p.closeAfter(closeKind); err != nil {
		return nil, err
	}
	name := exprHead(e)
	if name == "" {
		return nil, p.errf(p.toks[p.pos-1], "block helper must start with a name")
	}

	if raw {
		// raw block: everything up to {{{{/name}}}} is passed verbatim
		body, err := p.rawBody(name)
		if err != nil {
			return nil, err
		}
		return &BlockNode{Expr: e, RawBlock: true, RawBody: body, BlockParams: blockParams}, nil
	}

	body, err := p.program()
	if err != nil {
		return nil, err
	}
	var inverse *Program
	if p.cur().kind == tokOpenInverse {
		// {{^}} or {{else}}-style inverse inside the block
		p.bump()
		if p.cur().kind == tokClose {
			p.bump()
		} else {
			// {{else}} form
			if p.cur().kind == tokElse {
				p.bump()
			}
			if _, err := p.closeAfter(tokClose); err != nil {
				return nil, err
			}
		}
		inverse, err = p.program()
		if err != nil {
			return nil, err
		}
	} else if p.cur().kind == tokOpen && p.peekIsElse() {
		p.bump() // {{
		p.bump() // else
		if _, err := p.closeAfter(tokClose); err != nil {
			return nil, err
		}
		inverse, err = p.program()
		if err != nil {
			return nil, err
		}
	}
	// closing {{/name}}
	if p.cur().kind != tokOpen {
		return nil, p.errf(p.cur(), "missing closing tag for block "+name)
	}
	p.bump()
	if p.cur().kind != tokSlash {
		return nil, p.errf(p.cur(), "missing closing tag for block "+name)
	}
	p.bump()
	ct := p.cur()
	if ct.kind != tokIdent || baseName(ct.val) != baseName(name) {
		return nil, p.errf(ct, "closing tag does not match "+name)
	}
	p.bump()
	if _, err := p.closeAfter(tokClose); err != nil {
		return nil, err
	}
	bn := &BlockNode{Expr: e, Program: body, Inverse: inverse, BlockParams: blockParams}
	if inverted {
		bn.Program, bn.Inverse = bn.Inverse, bn.Program
	}
	return bn, nil
}

func (p *parser) peekIsElse() bool {
	return p.pos+1 < len(p.toks) && p.toks[p.pos+1].kind == tokElse
}

// rawBody reads the verbatim body token emitted in raw mode plus its closer.
func (p *parser) rawBody(name string) (string, error) {
	t := p.cur()
	if t.kind == tokEOF {
		return "", p.errf(t, "unclosed raw block "+name)
	}
	var body string
	if t.kind == tokContent {
		body = t.val
		p.bump()
	}
	// closer: tokOpenRawBlock tokSlash tokIdent tokCloseRawBlock
	if p.cur().kind != tokOpenRawBlock {
		return "", p.errf(p.cur(), "expected raw block closer for "+name)
	}
	p.bump()
	if p.cur().kind != tokSlash {
		return "", p.errf(p.cur(), "expected raw block closer for "+name)
	}
	p.bump()
	ct := p.cur()
	if ct.kind != tokIdent || ct.val != name {
		return "", p.errf(ct, "raw block closer does not match "+name)
	}
	p.bump()
	if p.cur().kind != tokCloseRawBlock {
		return "", p.errf(p.cur(), "expected }}}} after raw block closer")
	}
	p.bump()
	return body, nil
}

func (p *parser) partial() (Node, error) {
	p.bump() // {{>
	n := &PartialNode{}
	// {{#* inline}} is handled elsewhere; here: name or (dynamic) or @partial-block
	switch p.cur().kind {
	case tokIdent:
		n.Name = p.bump().val
	case tokOpenSexpr:
		e, err := p.subexpr()
		if err != nil {
			return nil, err
		}
		n.DynamicName = e
	case tokStar:
		return nil, p.errf(p.cur(), "decorator partials not supported in {{>")
	default:
		return nil, p.errf(p.cur(), "expected partial name")
	}
	// optional context + hash
	for {
		t := p.cur()
		if t.kind == tokClose {
			p.bump()
			return n, nil
		}
		e, err := p.expression(false)
		if err != nil {
			return nil, err
		}
		if p.cur().kind == tokEquals {
			p.bump()
			v, err := p.expression(false)
			if err != nil {
				return nil, err
			}
			if n.Hash == nil {
				n.Hash = map[string]*Expression{}
			}
			n.Hash[exprHead(e)] = v
		} else {
			if n.Context != nil {
				return nil, p.errf(t, "partial takes at most one context argument")
			}
			n.Context = e
		}
	}
}

// expression parses one expression. head=true allows helper-name + params;
// head=false parses a single param value (path, literal, or subexpression).
func (p *parser) expression(head bool) (*Expression, error) {
	t := p.cur()
	switch t.kind {
	case tokString:
		p.bump()
		return &Expression{Literal: &Literal{Kind: LitString, Str: t.val}}, nil
	case tokNumber:
		p.bump()
		f, err := strconv.ParseFloat(t.val, 64)
		if err != nil {
			return nil, p.errf(t, "bad number")
		}
		return &Expression{Literal: &Literal{Kind: LitNumber, Num: f, Str: t.val}}, nil
	case tokBool:
		p.bump()
		return &Expression{Literal: &Literal{Kind: LitBool, Bool: t.val == "true", Str: t.val}}, nil
	case tokNull:
		p.bump()
		return &Expression{Literal: &Literal{Kind: LitNull, Str: t.val}}, nil
	case tokOpenSexpr:
		return p.subexpr()
	case tokIdent:
		p.bump()
		path := parsePath(t.val)
		e := &Expression{Path: path}
		if head {
			for {
				nt := p.cur()
				if nt.kind == tokClose || nt.kind == tokCloseRaw || nt.kind == tokCloseRawBlock || nt.kind == tokEOF || nt.kind == tokCloseSexpr || nt.kind == tokAs {
					break
				}
				// hash arg?
				if nt.kind == tokIdent && p.pos+1 < len(p.toks) && p.toks[p.pos+1].kind == tokEquals {
					key := p.bump().val
					p.bump() // =
					v, err := p.expression(false)
					if err != nil {
						return nil, err
					}
					if e.Hash == nil {
						e.Hash = map[string]*Expression{}
					}
					e.Hash[key] = v
					continue
				}
				if nt.kind == tokIdent && nt.val == "as" {
					break // block params, caller handles
				}
				sub, err := p.expression(false)
				if err != nil {
					return nil, err
				}
				e.Params = append(e.Params, sub)
			}
		}
		return e, nil
	}
	return nil, p.errf(t, "expected expression")
}

func (p *parser) subexpr() (*Expression, error) {
	p.bump() // (
	e, err := p.expression(true)
	if err != nil {
		return nil, err
	}
	if p.cur().kind != tokCloseSexpr {
		return nil, p.errf(p.cur(), "expected )")
	}
	p.bump()
	return &Expression{Sub: e}, nil
}

func parsePath(s string) *Path {
	p := &Path{Original: s}
	for strings.HasPrefix(s, "../") {
		p.Depth++
		s = s[3:]
	}
	if strings.HasPrefix(s, "@") {
		p.Kind = PathData
		s = s[1:]
	} else if s == "." || s == "this" {
		p.Kind = PathThis
		return p
	} else if strings.HasPrefix(s, "this/") || strings.HasPrefix(s, "this.") || strings.HasPrefix(s, "./") {
		p.Rooted = true
		s = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(s, "this/"), "this."), "./")
	}
	// segments on . or /
	f := func(r rune) bool { return r == '.' || r == '/' }
	for _, seg := range strings.FieldsFunc(s, f) {
		if strings.HasPrefix(seg, "[") && strings.HasSuffix(seg, "]") {
			seg = seg[1 : len(seg)-1]
		}
		if seg != "" {
			p.Parts = append(p.Parts, seg)
		}
	}
	return p
}

// exprHead returns the leading path string of a helper expression, or "".
func exprHead(e *Expression) string {
	if e == nil || e.Path == nil {
		return ""
	}
	return e.Path.Original
}

func baseName(s string) string {
	if i := strings.LastIndexAny(s, "./"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// stripStandalone removes whitespace-only lines around standalone block,
// inverse, comment and partial tags, per the handlebars spec. Conservative:
// only tags whose open and close sit alone on their line.
func stripStandalone(prog *Program) *Program {
	// Implemented as a post-pass over the body list; content adjacent to a
	// standalone tag loses the tag's line (indent before + newline after).
	out := &Program{Body: make([]Node, 0, len(prog.Body))}
	for i, n := range prog.Body {
		switch n.(type) {
		case *BlockNode, *CommentNode, *PartialNode:
			// trim trailing spaces of previous content (up to and incl. its newline boundary)
			if len(out.Body) > 0 {
				if c, ok := out.Body[len(out.Body)-1].(*ContentNode); ok {
					if trimmed, ok := trimLineTail(c.Text); ok {
						c.Text = trimmed
						// trim leading newline of next content
						if i+1 < len(prog.Body) {
							if nc, ok := prog.Body[i+1].(*ContentNode); ok {
								nc.Text = trimLineHead(nc.Text)
							}
						}
					}
				}
			}
		}
		out.Body = append(out.Body, n)
	}
	return out
}

func trimLineTail(s string) (string, bool) {
	i := strings.LastIndexByte(s, '\n')
	tail := s[i+1:]
	if strings.Trim(tail, " \t") == "" && (tail != "" || i >= 0) {
		return s[:i+1], true
	}
	return s, false
}

func trimLineHead(s string) string {
	j := 0
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	if j < len(s) && s[j] == '\n' {
		return s[j+1:]
	}
	if j < len(s) && s[j] == '\r' && j+1 < len(s) && s[j+1] == '\n' {
		return s[j+2:]
	}
	return s
}
