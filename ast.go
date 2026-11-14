package handlebars

// AST nodes. A Template is a Program; blocks hold two programs (body and
// inverse). Mustaches carry one Expression.

type Program struct {
	Body []Node
}

type Node interface{ node() }

type ContentNode struct{ Text string }

type CommentNode struct{}

// MustacheNode is {{expr}} / {{{expr}}} / {{& expr}}.
type MustacheNode struct {
	Expr      *Expression
	Unescaped bool
}

// BlockNode is {{#expr}}...{{else}}...{{/expr}} (and ^ variants).
type BlockNode struct {
	Expr        *Expression
	Program     *Program
	Inverse     *Program
	BlockParams []string
	RawBlock    bool // {{{{helper}}}} form: body passed as raw string param
	RawBody     string
}

// PartialNode is {{> name context hash=...}} or {{> @partial-block}}.
type PartialNode struct {
	Name        string // literal name or dynamic via DynamicName
	DynamicName *Expression
	Context     *Expression // optional context argument
	Hash        map[string]*Expression
	Indent      string   // standalone-line indentation, for partial indenting
	Inline      *Program // non-nil for {{#* inline}}...{{/inline}} decorators-as-partials
	Fn          *Program // {{> @partial-block}} fallback content
}

type Expression struct {
	Path    *Path
	Literal *Literal
	Sub     *Expression // subexpression (a b c)
	Params  []*Expression
	Hash    map[string]*Expression
}

type PathKind int

const (
	PathPlain PathKind = iota // foo, foo.bar, ../foo, this
	PathData                  // @foo, @../foo
	PathThis                  // . or this
)

type Path struct {
	Kind     PathKind
	Parts    []string
	Depth    int  // number of leading ../
	Rooted   bool // leading this. or .
	Original string
}

type LitKind int

const (
	LitString LitKind = iota
	LitNumber
	LitBool
	LitNull // null or undefined
)

type Literal struct {
	Kind LitKind
	Str  string
	Num  float64
	Bool bool
}

func (*ContentNode) node()  {}
func (*CommentNode) node()  {}
func (*MustacheNode) node() {}
func (*BlockNode) node()    {}
func (*PartialNode) node()  {}
