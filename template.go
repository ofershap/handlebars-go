package handlebars

import "strings"

// SafeString marks a helper return value as already-safe HTML, bypassing
// escaping - the same contract as Handlebars.SafeString in handlebars.js.
// Only wrap content you have already sanitized.
type SafeString string

// Template is a compiled handlebars template. Safe for concurrent Execute.
type Template struct {
	src  string
	prog *Program
}

// Options mirror the handlebars.js 4.x compile/runtime options.
type Options struct {
	// Strict: missing fields are an error instead of "".
	Strict bool
	// NoEscape: disable HTML escaping globally.
	NoEscape bool
	// Compat: enable recursive field lookup (deprecated handlebars.js mode).
	Compat bool
	// KnownHelpers: names known to exist at execution time.
	KnownHelpers map[string]bool
	// KnownHelpersOnly: resolve only KnownHelpers as helpers.
	KnownHelpersOnly bool
	// AssumeObjects: skip existence checks when traversing paths.
	AssumeObjects bool
	// PreventIndent: don't auto-indent nested partials.
	PreventIndent bool
	// StringParams: unresolved helper params resolve to their name string.
	StringParams bool
	// TrackIds: record id names used to resolve params (exposed via Frame).
	TrackIds bool
	// Helpers: helper registry for this render.
	Helpers map[string]Helper
	// Partials: partial registry for this render.
	Partials map[string]*Template
	// Data: @-rooted data variables.
	Data map[string]any
}

// Render parses and executes src against ctx in one call.
func Render(src string, ctx any, opts *Options) (string, error) {
	t, err := Parse(src)
	if err != nil {
		return "", err
	}
	return t.Execute(ctx, opts)
}

// MustRender is Render that panics on error (the one intentional panic).
func MustRender(src string, ctx any, opts *Options) string {
	s, err := Render(src, ctx, opts)
	if err != nil {
		panic(err)
	}
	return s
}

// Execute renders the template against ctx.
func (t *Template) Execute(ctx any, opts *Options) (string, error) {
	if opts == nil {
		opts = &Options{}
	}
	st := &state{
		opts:     opts,
		helpers:  map[string]Helper{},
		partials: map[string]*Template{},
	}
	for k, v := range builtinHelpers {
		st.helpers[k] = v
	}
	for k, v := range opts.Helpers {
		st.helpers[k] = v
	}
	for k, v := range opts.Partials {
		st.partials[k] = v
	}
	data := map[string]any{"root": ctx}
	for k, v := range opts.Data {
		data[k] = v
	}
	root := &Frame{Value: ctx, Data: data, Parent: nil}
	var b strings.Builder
	if err := st.execProgram(&b, t.prog, root); err != nil {
		return "", err
	}
	return b.String(), nil
}
