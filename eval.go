package handlebars

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Helper is a handlebars helper. Returning BlockResult triggers block
// rendering; see HelperCall for the block body accessors.
type Helper func(call *HelperCall) (any, error)

// HelperCall carries everything a helper needs: params, hash, the current
// frame, block bodies (for block helpers), and option metadata.
type HelperCall struct {
	Params []any
	Hash   map[string]any
	Frame  *Frame
	// Fn renders the block body with an optional new context (nil = current).
	Fn func(ctx any) (string, error)
	// Inverse renders the {{else}} branch.
	Inverse func(ctx any) (string, error)
	// RawBody is the verbatim body of a {{{{raw}}}} block, if any.
	RawBody string
	// BlockParams holds the block-param values from `as |x y|`.
	BlockParams []any
	// Ids, when Options.TrackIds, records the id string each param resolved from.
	Ids []string
}

// Frame is one level of the context stack.
type Frame struct {
	Value  any
	Parent *Frame
	Data   map[string]any
	// BlockParamNames/Values shadow path lookups inside a block.
	BlockParamNames  []string
	BlockParamValues []any
}

type state struct {
	opts     *Options
	helpers  map[string]Helper
	partials map[string]*Template
	depth    int
}

func (s *state) execProgram(b *strings.Builder, prog *Program, f *Frame) error {
	if prog == nil {
		return nil
	}
	for _, n := range prog.Body {
		var err error
		switch n := n.(type) {
		case *ContentNode:
			b.WriteString(n.Text)
		case *CommentNode:
		case *MustacheNode:
			err = s.execMustache(b, n, f)
		case *BlockNode:
			err = s.execBlock(b, n, f)
		case *PartialNode:
			err = s.execPartial(b, n, f)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *state) execMustache(b *strings.Builder, n *MustacheNode, f *Frame) error {
	v, err := s.evalExpr(n.Expr, f)
	if err != nil {
		return err
	}
	out := stringify(v)
	if !n.Unescaped && !s.opts.NoEscape {
		out = escapeHTML(out)
	}
	b.WriteString(out)
	return nil
}

func (s *state) execBlock(b *strings.Builder, n *BlockNode, f *Frame) error {
	head := baseName(exprHead(n.Expr))
	// block helper?
	if h, ok := s.lookupHelper(head); ok {
		return s.execBlockHelper(b, n, f, h)
	}
	if s.opts.KnownHelpersOnly {
		return &TemplateError{Msg: fmt.Sprintf("unknown helper %q", head)}
	}
	// plain path section: #foo iterates arrays / renders truthy
	v, err := s.evalPathValue(n.Expr.Path, f)
	if err != nil {
		return err
	}
	return s.renderSection(b, n, v, f)
}

func (s *state) execBlockHelper(b *strings.Builder, n *BlockNode, f *Frame, h Helper) error {
	params, ids, err := s.evalParams(n.Expr.Params, f)
	if err != nil {
		return err
	}
	hash, err := s.evalHash(n.Expr.Hash, f)
	if err != nil {
		return err
	}
	call := &HelperCall{Params: params, Hash: hash, Frame: f, Ids: ids, RawBody: n.RawBody, BlockParams: make([]any, len(n.BlockParams))}
	call.Fn = func(ctx any) (string, error) {
		nf := &Frame{Value: pick(ctx, f.Value), Parent: f, Data: f.Data}
		if len(n.BlockParams) > 0 {
			nf.BlockParamNames = n.BlockParams
			nf.BlockParamValues = call.BlockParams
		}
		var sb strings.Builder
		if n.RawBlock {
			sb.WriteString(n.RawBody)
			return sb.String(), nil
		}
		if err := s.execProgram(&sb, n.Program, nf); err != nil {
			return "", err
		}
		return sb.String(), nil
	}
	call.Inverse = func(ctx any) (string, error) {
		nf := &Frame{Value: pick(ctx, f.Value), Parent: f, Data: f.Data}
		var sb strings.Builder
		if err := s.execProgram(&sb, n.Inverse, nf); err != nil {
			return "", err
		}
		return sb.String(), nil
	}
	v, err := h(call)
	if err != nil {
		return err
	}
	switch v := v.(type) {
	case nil:
		return nil
	case string:
		b.WriteString(v)
	default:
		b.WriteString(stringify(v))
	}
	return nil
}

func pick(v, fallback any) any {
	if v == nil {
		return fallback
	}
	return v
}

// renderSection handles {{#path}} without a helper: iterate or truthy-render.
func (s *state) renderSection(b *strings.Builder, n *BlockNode, v any, f *Frame) error {
	rv := indirect(reflect.ValueOf(v))
	if rv.IsValid() && rv.Kind() == reflect.Slice || rv.IsValid() && rv.Kind() == reflect.Array {
		if rv.Len() == 0 {
			return s.execProgram(b, n.Inverse, f)
		}
		for i := 0; i < rv.Len(); i++ {
			nf := &Frame{Value: rv.Index(i).Interface(), Parent: f, Data: f.Data}
			if len(n.BlockParams) > 0 {
				nf.BlockParamNames = n.BlockParams
				nf.BlockParamValues = []any{rv.Index(i).Interface(), i}
			}
			if err := s.execProgram(b, n.Program, nf); err != nil {
				return err
			}
		}
		return nil
	}
	if truthy(v) {
		nf := f
		if rv.IsValid() && rv.Kind() == reflect.Map || rv.IsValid() && rv.Kind() == reflect.Struct {
			nf = &Frame{Value: v, Parent: f, Data: f.Data}
		}
		return s.execProgram(b, n.Program, nf)
	}
	return s.execProgram(b, n.Inverse, f)
}

func (s *state) execPartial(b *strings.Builder, n *PartialNode, f *Frame) error {
	name := n.Name
	if n.DynamicName != nil {
		v, err := s.evalExpr(n.DynamicName, f)
		if err != nil {
			return err
		}
		name = stringify(v)
	}
	pt, ok := s.partials[name]
	if !ok {
		if name == "@partial-block" {
			return s.execProgram(b, n.Fn, f)
		}
		return &TemplateError{Msg: fmt.Sprintf("partial %q not registered", name)}
	}
	ctx := f.Value
	if n.Context != nil {
		v, err := s.evalExpr(n.Context, f)
		if err != nil {
			return err
		}
		ctx = v
	}
	if len(n.Hash) > 0 {
		h, err := s.evalHash(n.Hash, f)
		if err != nil {
			return err
		}
		ctx = mergeContext(ctx, h)
	}
	var pb strings.Builder
	if err := s.execProgram(&pb, pt.prog, &Frame{Value: ctx, Parent: nil, Data: f.Data}); err != nil {
		return err
	}
	out := pb.String()
	if !s.opts.PreventIndent && n.Indent != "" {
		out = indentPartial(out, n.Indent)
	}
	b.WriteString(out)
	return nil
}

func indentPartial(s, indent string) string {
	lines := strings.SplitAfter(s, "\n")
	for i, ln := range lines {
		if strings.TrimRight(ln, "\r\n") != "" && i > 0 || (i == 0 && false) {
			lines[i] = indent + ln
		}
		if i == 0 {
			continue
		}
		if strings.TrimRight(ln, "\r\n") != "" {
			lines[i] = indent + ln
		}
	}
	return strings.Join(lines, "")
}

func mergeContext(ctx any, hash map[string]any) any {
	m := map[string]any{}
	rv := indirect(reflect.ValueOf(ctx))
	if rv.IsValid() && rv.Kind() == reflect.Map {
		iter := rv.MapRange()
		for iter.Next() {
			m[fmt.Sprint(iter.Key().Interface())] = iter.Value().Interface()
		}
	} else if rv.IsValid() && rv.Kind() == reflect.Struct {
		t := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			if rv.Field(i).CanInterface() {
				m[t.Field(i).Name] = rv.Field(i).Interface()
			}
		}
	}
	for k, v := range hash {
		m[k] = v
	}
	return m
}

func (s *state) lookupHelper(name string) (Helper, bool) {
	if s.opts.KnownHelpersOnly && !s.opts.KnownHelpers[name] {
		return nil, false
	}
	h, ok := s.helpers[name]
	return h, ok
}

// evalExpr evaluates a mustache/subexpression. If the head is a known helper
// (or has params/hash and isn't a path), calls it; otherwise resolves the path.
func (s *state) evalExpr(e *Expression, f *Frame) (any, error) {
	if e == nil {
		return nil, nil
	}
	if e.Literal != nil {
		switch e.Literal.Kind {
		case LitString:
			return e.Literal.Str, nil
		case LitNumber:
			return e.Literal.Num, nil
		case LitBool:
			return e.Literal.Bool, nil
		case LitNull:
			return nil, nil
		}
	}
	if e.Sub != nil {
		return s.evalExpr(e.Sub, f)
	}
	head := baseName(e.Path.Original)
	if h, ok := s.lookupHelper(head); ok && (len(e.Params) > 0 || len(e.Hash) > 0 || !isContextPath(e.Path, f)) {
		params, ids, err := s.evalParams(e.Params, f)
		if err != nil {
			return nil, err
		}
		hash, err := s.evalHash(e.Hash, f)
		if err != nil {
			return nil, err
		}
		return h(&HelperCall{Params: params, Hash: hash, Frame: f, Ids: ids})
	}
	if s.opts.StringParams && (len(e.Params) > 0 || len(e.Hash) > 0) {
		return e.Path.Original, nil
	}
	if len(e.Params) > 0 || len(e.Hash) > 0 {
		if s.opts.KnownHelpersOnly {
			return nil, &TemplateError{Msg: fmt.Sprintf("unknown helper %q", head)}
		}
		if s.opts.Strict {
			return nil, &TemplateError{Msg: fmt.Sprintf("missing helper %q (strict)", head)}
		}
		// handlebars.js: helperMissing hook, else error on call-shaped path
		if hm, ok := s.helpers["helperMissing"]; ok {
			params, ids, err := s.evalParams(e.Params, f)
			if err != nil {
				return nil, err
			}
			hash, err := s.evalHash(e.Hash, f)
			if err != nil {
				return nil, err
			}
			return hm(&HelperCall{Params: params, Hash: hash, Frame: f, Ids: ids})
		}
		return nil, &TemplateError{Msg: fmt.Sprintf("unknown helper %q", head)}
	}
	return s.evalPathValue(e.Path, f)
}

func (s *state) evalParams(exprs []*Expression, f *Frame) ([]any, []string, error) {
	out := make([]any, len(exprs))
	ids := make([]string, len(exprs))
	for i, e := range exprs {
		v, err := s.evalExpr(e, f)
		if err != nil {
			return nil, nil, err
		}
		if v == nil && s.opts.StringParams && e.Path != nil {
			v = e.Path.Original
		}
		out[i] = v
		if s.opts.TrackIds && e.Path != nil {
			ids[i] = e.Path.Original
		}
	}
	return out, ids, nil
}

func (s *state) evalHash(m map[string]*Expression, f *Frame) (map[string]any, error) {
	if len(m) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(m))
	for k, e := range m {
		v, err := s.evalExpr(e, f)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// isContextPath reports whether the path resolves in the context stack.
func isContextPath(p *Path, f *Frame) bool {
	if len(p.Parts) == 0 {
		return true
	}
	_, ok := lookupPath(p, f, false)
	return ok
}

// evalPathValue resolves a path against the frame stack.
func (s *state) evalPathValue(p *Path, f *Frame) (any, error) {
	v, ok := lookupPath(p, f, s.opts.Compat)
	if ok {
		return v, nil
	}
	if s.opts.AssumeObjects {
		return nil, nil
	}
	if s.opts.Strict {
		return nil, &TemplateError{Msg: fmt.Sprintf("missing value for %q (strict)", p.Original)}
	}
	return nil, nil
}

// lookupPath walks the frame stack for p. Returns (value, found).
func lookupPath(p *Path, f *Frame, compat bool) (any, bool) {
	if p.Kind == PathData {
		d := f.Data
		for fr := f; fr != nil && d == nil; fr = fr.Parent {
			d = fr.Data
		}
		if d == nil {
			return nil, false
		}
		return resolveParts(d, p.Parts)
	}
	// block params shadow everything
	if len(p.Parts) > 0 {
		for fr := f; fr != nil; fr = fr.Parent {
			for i, name := range fr.BlockParamNames {
				if name == p.Parts[0] {
					if i < len(fr.BlockParamValues) {
						return resolveParts(fr.BlockParamValues[i], p.Parts[1:])
					}
				}
			}
		}
	}
	// depth walk
	fr := f
	for d := 0; d < p.Depth && fr != nil; d++ {
		fr = fr.Parent
	}
	if fr == nil {
		return nil, false
	}
	if p.Kind == PathThis {
		return fr.Value, true
	}
	for cur := fr; cur != nil; cur = cur.Parent {
		if v, ok := resolveParts(cur.Value, p.Parts); ok {
			return v, true
		}
		if !compat {
			break // handlebars.js default: current context only... plus root? no.
		}
	}
	// handlebars resolves against the current context only by default, but
	// @root and partials change that; keep current-context semantics.
	return nil, false
}

func resolveParts(v any, parts []string) (any, bool) {
	cur := v
	for _, part := range parts {
		rv := indirect(reflect.ValueOf(cur))
		if !rv.IsValid() {
			return nil, false
		}
		switch rv.Kind() {
		case reflect.Map:
			kv := reflect.ValueOf(part)
			if kv.Type().AssignableTo(rv.Type().Key()) {
				mv := rv.MapIndex(kv)
				if !mv.IsValid() {
					return nil, false
				}
				cur = mv.Interface()
			} else {
				// try numeric index into map with interface keys
				found := false
				iter := rv.MapRange()
				for iter.Next() {
					if fmt.Sprint(iter.Key().Interface()) == part {
						cur = iter.Value().Interface()
						found = true
						break
					}
				}
				if !found {
					return nil, false
				}
			}
		case reflect.Struct:
			t := rv.Type()
			fv := reflect.Value{}
			for i := 0; i < rv.NumField(); i++ {
				fld := t.Field(i)
				if strings.EqualFold(fld.Name, part) && fld.IsExported() {
					fv = rv.Field(i)
					break
				}
				if tag := fld.Tag.Get("handlebars"); tag == part && fld.IsExported() {
					fv = rv.Field(i)
					break
				}
			}
			if !fv.IsValid() {
				// method?
				m := rv.MethodByName(part)
				if m.IsValid() && m.Type().NumIn() == 0 && m.Type().NumOut() == 1 {
					cur = m.Call(nil)[0].Interface()
					continue
				}
				return nil, false
			}
			cur = fv.Interface()
		case reflect.Slice, reflect.Array:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= rv.Len() {
				return nil, false
			}
			cur = rv.Index(i).Interface()
		default:
			return nil, false
		}
	}
	return cur, true
}

func indirect(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

// truthy follows handlebars.js: false, 0, "", nil, empty array are falsy;
// everything else truthy. Empty object IS truthy in handlebars.
func truthy(v any) bool {
	rv := indirect(reflect.ValueOf(v))
	if !rv.IsValid() {
		return false
	}
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.String:
		return rv.Len() > 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.Slice, reflect.Array:
		return rv.Len() > 0
	}
	return true
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch v := v.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	rv := indirect(reflect.ValueOf(v))
	if rv.IsValid() {
		switch rv.Kind() {
		case reflect.Bool:
			return strconv.FormatBool(rv.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return strconv.FormatInt(rv.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return strconv.FormatUint(rv.Uint(), 10)
		case reflect.Float32, reflect.Float64:
			f := rv.Float()
			if f == float64(int64(f)) {
				return strconv.FormatInt(int64(f), 10)
			}
			return strconv.FormatFloat(f, 'f', -1, 64)
		}
	}
	return fmt.Sprint(v)
}
