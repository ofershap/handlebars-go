package handlebars

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

var builtinHelpers = map[string]Helper{
	"if":                 helperIf,
	"unless":             helperUnless,
	"each":               helperEach,
	"with":               helperWith,
	"lookup":             helperLookup,
	"log":                helperLog,
	"blockHelperMissing": helperBlockMissing,
	"helperMissing":      helperMissingDefault,
}

func helperIf(c *HelperCall) (any, error) {
	if len(c.Params) < 1 {
		return nil, &TemplateError{Msg: "#if requires one parameter"}
	}
	includeZero, _ := c.Hash["includeZero"].(bool)
	v := c.Params[0]
	cond := truthy(v) || (includeZero && isZero(v))
	if cond {
		if c.Fn != nil {
			return c.Fn(nil)
		}
		return true, nil
	}
	if c.Inverse != nil {
		return c.Inverse(nil)
	}
	return nil, nil
}

func isZero(v any) bool {
	rv := indirect(reflect.ValueOf(v))
	if !rv.IsValid() {
		return false
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	}
	return false
}

func helperUnless(c *HelperCall) (any, error) {
	if len(c.Params) < 1 {
		return nil, &TemplateError{Msg: "#unless requires one parameter"}
	}
	if !truthy(c.Params[0]) {
		if c.Fn != nil {
			return c.Fn(nil)
		}
		return true, nil
	}
	if c.Inverse != nil {
		return c.Inverse(nil)
	}
	return nil, nil
}

func helperEach(c *HelperCall) (any, error) {
	if len(c.Params) < 1 {
		return nil, &TemplateError{Msg: "#each requires one parameter"}
	}
	rv := indirect(reflect.ValueOf(c.Params[0]))
	if !rv.IsValid() {
		if c.Inverse != nil {
			return c.Inverse(nil)
		}
		return nil, nil
	}
	var b strings.Builder
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		if rv.Len() == 0 {
			if c.Inverse != nil {
				return c.Inverse(nil)
			}
			return "", nil
		}
		for i := 0; i < rv.Len(); i++ {
			item := rv.Index(i).Interface()
			c.PendingData = map[string]any{"index": i, "first": i == 0, "last": i == rv.Len()-1, "key": i}
			if len(c.BlockParams) > 0 {
				c.BlockParams = []any{item, i}
			}
			out, err := c.Fn(item)
			if err != nil {
				return nil, err
			}
			b.WriteString(out)
		}
	case reflect.Map:
		// Go maps have no insertion order: keys are sorted for determinism.
		// (handlebars.js preserves object insertion order - a named dialect note.)
		keys := rv.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})
		for i, k := range keys {
			val := rv.MapIndex(k).Interface()
			c.PendingData = map[string]any{"key": fmt.Sprint(k.Interface()), "index": i, "first": i == 0, "last": i == len(keys)-1}
			if len(c.BlockParams) > 0 {
				c.BlockParams = []any{val, fmt.Sprint(k.Interface())}
			}
			out, err := c.Fn(val)
			if err != nil {
				return nil, err
			}
			b.WriteString(out)
		}
	default:
		if c.Inverse != nil {
			return c.Inverse(nil)
		}
	}
	return b.String(), nil
}

func helperWith(c *HelperCall) (any, error) {
	if len(c.Params) < 1 {
		return nil, &TemplateError{Msg: "#with requires one parameter"}
	}
	if !truthy(c.Params[0]) {
		if c.Inverse != nil {
			return c.Inverse(nil)
		}
		return nil, nil
	}
	if c.Fn == nil {
		return nil, nil
	}
	if len(c.BlockParams) > 0 {
		c.BlockParams = []any{c.Params[0]}
	}
	return c.Fn(c.Params[0])
}

func helperLookup(c *HelperCall) (any, error) {
	if len(c.Params) < 2 {
		return nil, &TemplateError{Msg: "lookup requires two parameters"}
	}
	v, ok := resolveParts(c.Params[0], []string{stringify(c.Params[1])})
	if !ok {
		return nil, nil
	}
	return v, nil
}

func helperLog(c *HelperCall) (any, error) {
	parts := make([]string, len(c.Params))
	for i, p := range c.Params {
		parts[i] = stringify(p)
	}
	fmt.Println("handlebars log:", strings.Join(parts, " "))
	return nil, nil
}

// RawHelper renders a {{{{raw}}}} block verbatim. handlebars.js ships no
// raw helper; register it explicitly: Options.Helpers["raw"] = handlebars.RawHelper.
func RawHelper(c *HelperCall) (any, error) {
	return c.RawBody, nil
}

// helperBlockMissing is the default {{#path}} behavior when path is a helper
// name-shaped block with no matching helper: render section semantics.
func helperBlockMissing(c *HelperCall) (any, error) {
	if c.Fn == nil {
		return nil, nil
	}
	if len(c.Params) > 0 && truthy(c.Params[0]) {
		return c.Fn(c.Params[0])
	}
	if c.Inverse != nil {
		return c.Inverse(nil)
	}
	return nil, nil
}

// helperMissingDefault is a no-op default; users override via Options.Helpers.
func helperMissingDefault(c *HelperCall) (any, error) {
	return nil, &TemplateError{Msg: "missing helper"}
}
