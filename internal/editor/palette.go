package editor

import (
	"fmt"
	"strings"
)

// Colours is what an editor draws code with: one value per highlight group the
// panel needs, taken from the editor itself.
//
// A field is empty when the colourscheme says nothing about that group. The
// panel keeps its own value for those, rather than showing a guess.
type Colours struct {
	Fg, Bg string

	Comment  string
	Keyword  string
	Function string
	Type     string
	String   string
	Number   string
	Constant string
	Operator string
	Punct    string
	Property string
	Module   string
	Builtin  string
	SelfRef  string
	Regex    string
	Doc      string
	Error    string
	Label    string

	LineNr     string
	CursorLine string
	DiffAdd    string
	DiffDelete string

	CommentItalic bool
}

// group is one highlight group to ask about, and where its answer goes.
type group struct {
	name  string
	attr  string // "fg" or "bg"
	field func(*Colours) *string
}

// groups maps the editor's highlight groups onto the panel's needs.
//
// The treesitter groups are asked for first where they exist, because they are
// what the editor actually draws code with; the older syntax groups stand
// behind them. Asking is the whole point: a palette transcribed from a theme's
// source goes stale the moment the reader changes colourscheme.
var groups = []group{
	{"Normal", "fg", func(c *Colours) *string { return &c.Fg }},
	{"Normal", "bg", func(c *Colours) *string { return &c.Bg }},

	{"Comment", "fg", func(c *Colours) *string { return &c.Comment }},
	{"@keyword.function", "fg", func(c *Colours) *string { return &c.Keyword }},
	{"Keyword", "fg", func(c *Colours) *string { return &c.Keyword }},
	{"Function", "fg", func(c *Colours) *string { return &c.Function }},
	{"Type", "fg", func(c *Colours) *string { return &c.Type }},
	{"String", "fg", func(c *Colours) *string { return &c.String }},
	{"Number", "fg", func(c *Colours) *string { return &c.Number }},
	{"@constant.builtin", "fg", func(c *Colours) *string { return &c.Constant }},
	{"Constant", "fg", func(c *Colours) *string { return &c.Constant }},
	{"Operator", "fg", func(c *Colours) *string { return &c.Operator }},
	{"@punctuation.bracket", "fg", func(c *Colours) *string { return &c.Punct }},
	{"Delimiter", "fg", func(c *Colours) *string { return &c.Punct }},
	{"@property", "fg", func(c *Colours) *string { return &c.Property }},
	{"@module", "fg", func(c *Colours) *string { return &c.Module }},
	{"@function.builtin", "fg", func(c *Colours) *string { return &c.Builtin }},
	{"@variable.builtin", "fg", func(c *Colours) *string { return &c.SelfRef }},
	{"@string.regexp", "fg", func(c *Colours) *string { return &c.Regex }},
	{"@string.documentation", "fg", func(c *Colours) *string { return &c.Doc }},
	{"Error", "fg", func(c *Colours) *string { return &c.Error }},
	{"Label", "fg", func(c *Colours) *string { return &c.Label }},

	{"LineNr", "fg", func(c *Colours) *string { return &c.LineNr }},
	{"CursorLine", "bg", func(c *Colours) *string { return &c.CursorLine }},
	{"DiffAdd", "bg", func(c *Colours) *string { return &c.DiffAdd }},
	{"DiffDelete", "bg", func(c *Colours) *string { return &c.DiffDelete }},
}

// Palette asks an editor what it draws each group with.
//
// It is one round trip: the whole question is built as a single expression, so
// a reader does not wait on twenty of them while the panel opens.
func Palette(s Server) (Colours, error) {
	var c Colours

	// A sentinel at each end: a group the scheme is silent about answers with
	// an empty string, and the reply is trimmed, so an empty answer at either
	// edge would be lost and the count would not line up.
	parts := make([]string, 0, len(groups)+3)
	parts = append(parts, `'.'`)
	for _, g := range groups {
		parts = append(parts, fmt.Sprintf(`synIDattr(synIDtrans(hlID(%s)),%s)`, vimString(g.name), vimString(g.attr+"#")))
	}
	parts = append(parts, `synIDattr(synIDtrans(hlID('Comment')),'italic')`)
	parts = append(parts, `'.'`)

	out, err := query(s.Addr, "join(["+strings.Join(parts, ",")+"],\"\\n\")")
	if err != nil {
		return c, fmt.Errorf("editor: reading the palette: %w", err)
	}

	answers := strings.Split(out, "\n")
	if want := len(groups) + 3; len(answers) != want {
		return c, fmt.Errorf("editor: the palette came back with %d answers, want %d", len(answers), want)
	}
	answers = answers[1:] // past the opening sentinel

	for i, g := range groups {
		value := strings.TrimSpace(answers[i])
		if !looksLikeColour(value) {
			continue
		}
		// First answer wins: the treesitter groups are listed before the
		// syntax groups that stand behind them.
		if field := g.field(&c); *field == "" {
			*field = strings.ToLower(value)
		}
	}
	c.CommentItalic = strings.TrimSpace(answers[len(groups)]) == "1"
	return c, nil
}

// looksLikeColour rejects the empty strings and names an editor gives for a
// group it has no colour for.
func looksLikeColour(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
