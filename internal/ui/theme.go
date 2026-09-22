package ui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/mahmood/lens/internal/editor"
	"github.com/rivo/uniseg"
)

// tokyonight-moon, the colourscheme the editor next to this panel wears, so a
// diff here and the file open in nvim read as the same code.
//
// These are not a transcription of the theme's source: they are what a running
// nvim answered when asked what it draws each group with, group by group. That
// is the only account that cannot be out of date.
const (
	tnBg           = "#222436" // Normal bg
	tnFg           = "#c8d3f5" // Normal fg, @variable
	tnFgDark       = "#828bb8" // @punctuation.bracket
	tnComment      = "#636da6" // Comment, italic
	tnMagenta      = "#c099ff" // @keyword.function/.conditional/.repeat, @constructor
	tnPink         = "#fca7ea" // @keyword, @keyword.return — italic
	tnBlue         = "#82aaff" // Function, Title
	tnBlue1        = "#65bcff" // Type, @function.builtin, @constant.builtin
	tnBlue5        = "#89ddff" // Operator, @punctuation.delimiter
	tnBlue7        = "#589ed7" // @type.builtin
	tnCyan         = "#86e1fc" // Keyword, PreProc, @module, @keyword.import
	tnTeal         = "#4fd6be" // @property, @variable.member
	tnGreen        = "#c3e88d" // String, Character
	tnOrange       = "#ff966c" // Number, Boolean, Constant
	tnYellow       = "#ffc777" // @string.documentation
	tnRed          = "#ff757f" // @variable.builtin
	tnError        = "#c53b53" // Error
	tnRegex        = "#b4f9f8" // @string.regexp
	tnLineNr       = "#3b4261" // LineNr
	tnCursorLine   = "#2f334d" // CursorLine bg
	tnCursorLineNr = "#ff966c" // CursorLineNr, bold
	tnDiffAdd      = "#2a4556" // DiffAdd bg
	tnDiffDelete   = "#4b2a3d" // DiffDelete bg
	tnDiffText     = "#394b70" // DiffText bg
)

// palette is the colours actually in use: the built-in scheme above, with
// anything the reader's own editor told us overlaid on top.
type palette struct {
	fg, bg                                       string
	comment, keyword, function, typ, str, number string
	constant, operator, punct, property, module  string
	builtin, selfRef, regex, doc, errCol, label  string
	lineNr, cursorLine, diffAdd, diffDelete      string
	commentItalic                                bool
}

// builtIn is the scheme to fall back on when there is no editor to ask.
func builtIn() palette {
	return palette{
		fg: tnFg, bg: tnBg,
		comment: tnComment, keyword: tnMagenta, function: tnBlue, typ: tnBlue1,
		str: tnGreen, number: tnOrange, constant: tnBlue1, operator: tnBlue5,
		punct: tnFgDark, property: tnTeal, module: tnCyan, builtin: tnBlue1,
		selfRef: tnRed, regex: tnRegex, doc: tnYellow, errCol: tnError, label: tnBlue,
		lineNr: tnLineNr, cursorLine: tnCursorLine,
		diffAdd: tnDiffAdd, diffDelete: tnDiffDelete,
		commentItalic: true,
	}
}

var pal = builtIn()

// Adopt takes on the colours the reader's editor draws code with, keeping the
// built-in value for anything the editor says nothing about.
//
// A colourscheme written down here is right for exactly one reader. The editor
// is asked instead, so the diff matches whatever they actually use — and with
// no editor to ask, nothing changes at all.
func Adopt(c editor.Colours) {
	pal = builtIn()

	for dst, got := range map[*string]string{
		&pal.fg: c.Fg, &pal.bg: c.Bg,
		&pal.comment: c.Comment, &pal.keyword: c.Keyword,
		&pal.function: c.Function, &pal.typ: c.Type,
		&pal.str: c.String, &pal.number: c.Number,
		&pal.constant: c.Constant, &pal.operator: c.Operator,
		&pal.punct: c.Punct, &pal.property: c.Property,
		&pal.module: c.Module, &pal.builtin: c.Builtin,
		&pal.selfRef: c.SelfRef, &pal.regex: c.Regex,
		&pal.doc: c.Doc, &pal.errCol: c.Error, &pal.label: c.Label,
		&pal.lineNr: c.LineNr, &pal.cursorLine: c.CursorLine,
		&pal.diffAdd: c.DiffAdd, &pal.diffDelete: c.DiffDelete,
	} {
		if got != "" {
			*dst = got
		}
	}
	// Follow the editor on italics only if it answered about comments at all.
	if c.Comment != "" {
		pal.commentItalic = c.CommentItalic
	}
	apply()
}

// The washes behind changed lines are the editor's own DiffAdd, DiffDelete and
// CursorLine — taken whole rather than mixed here, so a changed line in the
// panel is the colour a changed line is in the editor.
var bgAdd, bgDel, bgCursor string

var (
	colAdd, colDel, colDim, colHeading lipgloss.Color

	styHeading, styIntent, styAdd, styDel   lipgloss.Style
	styGutter, styHunk, styDim, stySelected lipgloss.Style
	styRow, styHelp, styBorder              lipgloss.Style
)

// apply rebuilds everything drawn from the palette.
func apply() {
	bgAdd, bgDel, bgCursor = pal.diffAdd, pal.diffDelete, pal.cursorLine

	colAdd = lipgloss.Color(pal.str)
	colDel = lipgloss.Color(pal.selfRef)
	colDim = lipgloss.Color(pal.comment)
	colHeading = lipgloss.Color(pal.function)

	styHeading = lipgloss.NewStyle().Foreground(colHeading).Bold(true) // Title
	styIntent = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.property)).Italic(true)
	styAdd = lipgloss.NewStyle().Foreground(colAdd)
	styDel = lipgloss.NewStyle().Foreground(colDel)
	// LineNr and CursorLineNr: the numbers recede, the selection does not.
	styGutter = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.lineNr))
	styHunk = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.operator))
	styDim = lipgloss.NewStyle().Foreground(colDim)
	stySelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.number))
	styRow = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.fg))
	styHelp = lipgloss.NewStyle().Foreground(colDim)
	styBorder = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.lineNr))

	editorStyle = syntaxStyle()
}

func init() { apply() }

// colours reports whether the terminal takes colour at all. Backgrounds are
// written by hand rather than through lipgloss, so they need the same gate.
var colours = lipgloss.NewStyle().Foreground(lipgloss.Color(tnRed)).Render("x") != "x"

// syntaxStyle is tokyonight-moon expressed as a chroma style.
//
// chroma splits code into finer token types than vim's syntax groups, so each
// one is mapped to the group the editor would colour that text with. Anything
// left out inherits from its parent token, which is why the broad entries —
// Name, Literal, Keyword — are set as well as the narrow ones.
//
// One thing cannot be carried across: the editor draws `if` and `for` in
// magenta but a bare `defer` or `return` in italic pink, and chroma has no
// token that separates them. Keywords take the magenta, which is what most of
// them are.
func syntaxStyle() *chroma.Style {
	st, err := chroma.NewStyle("tokyonight-moon", chroma.StyleEntries{
		chroma.Background: pal.fg + " bg:" + pal.bg,
		chroma.Text:       pal.fg,

		chroma.Comment:        italicIf(pal.commentItalic) + pal.comment,
		chroma.CommentPreproc: pal.module, // PreProc
		chroma.CommentSpecial: italicIf(pal.commentItalic) + pal.comment,

		chroma.Keyword:            pal.keyword,  // @keyword.function, .conditional, .repeat
		chroma.KeywordConstant:    pal.constant, // @constant.builtin: true, false, nil
		chroma.KeywordDeclaration: pal.keyword,
		chroma.KeywordNamespace:   pal.module, // @keyword.import
		chroma.KeywordPseudo:      pal.constant,
		chroma.KeywordReserved:    pal.keyword,
		chroma.KeywordType:        pal.typ, // @type.builtin sits close to Type

		chroma.Name:                 pal.fg, // @variable
		chroma.NameAttribute:        pal.module,
		chroma.NameBuiltin:          pal.builtin, // @function.builtin
		chroma.NameBuiltinPseudo:    pal.selfRef, // @variable.builtin: self, this
		chroma.NameClass:            pal.typ,     // Type
		chroma.NameConstant:         pal.number,
		chroma.NameDecorator:        pal.module, // @attribute
		chroma.NameEntity:           pal.module,
		chroma.NameException:        pal.keyword,
		chroma.NameFunction:         pal.function,
		chroma.NameFunctionMagic:    pal.constant,
		chroma.NameLabel:            pal.label,
		chroma.NameNamespace:        pal.module, // @module
		chroma.NameOther:            pal.fg,
		chroma.NameProperty:         pal.property, // @property
		chroma.NameTag:              pal.constant,
		chroma.NameVariable:         pal.fg,
		chroma.NameVariableClass:    pal.property,
		chroma.NameVariableGlobal:   pal.property,
		chroma.NameVariableInstance: pal.property, // @variable.member
		chroma.NameVariableMagic:    pal.selfRef,

		chroma.Literal:                pal.number,
		chroma.LiteralDate:            pal.number,
		chroma.LiteralNumber:          pal.number,
		chroma.LiteralString:          pal.str,
		chroma.LiteralStringAffix:     pal.keyword,
		chroma.LiteralStringChar:      pal.str, // Character
		chroma.LiteralStringDoc:       "italic " + pal.doc,
		chroma.LiteralStringEscape:    pal.keyword, // @string.escape
		chroma.LiteralStringInterpol:  pal.keyword,
		chroma.LiteralStringRegex:     pal.regex,
		chroma.LiteralStringSymbol:    pal.str,
		chroma.LiteralStringDelimiter: pal.str,
		chroma.LiteralStringBacktick:  pal.str,
		chroma.LiteralStringDouble:    pal.str,
		chroma.LiteralStringSingle:    pal.str,
		chroma.LiteralStringHeredoc:   pal.str,
		chroma.LiteralStringOther:     pal.str,

		chroma.Operator:     pal.operator,
		chroma.OperatorWord: pal.operator, // @keyword.operator
		chroma.Punctuation:  pal.punct,    // @punctuation.bracket

		chroma.GenericDeleted:    pal.selfRef,
		chroma.GenericInserted:   pal.str,
		chroma.GenericEmph:       "italic",
		chroma.GenericStrong:     "bold",
		chroma.GenericHeading:    "bold " + pal.function,
		chroma.GenericSubheading: "bold " + pal.constant,

		chroma.Error: pal.errCol,
	})
	if err != nil {
		return styles.Fallback
	}
	return st
}

// italicIf is the chroma style prefix for a group the editor italicises.
func italicIf(yes bool) string {
	if yes {
		return "italic "
	}
	return ""
}

// entryStyle turns one of the style's entries into the way it is drawn. Colour
// alone is not the whole of it: catppuccin leans on italics for comments,
// strings of documentation and module names, and dropping them flattens code
// into a wall of coloured text.
func entryStyle(e chroma.StyleEntry) (lipgloss.Style, bool) {
	st, set := lipgloss.NewStyle(), false
	if e.Colour.IsSet() {
		st, set = st.Foreground(lipgloss.Color(e.Colour.String())), true
	}
	if e.Bold == chroma.Yes {
		st, set = st.Bold(true), true
	}
	if e.Italic == chroma.Yes {
		st, set = st.Italic(true), true
	}
	if e.Underline == chroma.Yes {
		st, set = st.Underline(true), true
	}
	return st, set
}

// blend mixes a colour into a background: t=0 is all background, t=1 all colour.
func blend(fg, bg string, t float64) string {
	fr, fg2, fb := hexToRGB(fg)
	br, bg2, bb := hexToRGB(bg)
	mix := func(a, b int) int { return int(float64(b) + (float64(a)-float64(b))*t + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", mix(fr, br), mix(fg2, bg2), mix(fb, bb))
}

func hexToRGB(hex string) (int, int, int) {
	var r, g, b int
	fmt.Sscanf(strings.TrimPrefix(hex, "#"), "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

// bgEscape is the SGR sequence that sets a truecolour background.
func bgEscape(hex string) string {
	r, g, b := hexToRGB(hex)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

// withBackground washes a colour across a line that already carries foreground
// styling, re-asserting it after every reset the styling emits — otherwise the
// wash would stop at the first token boundary.
func withBackground(line, hex string) string {
	if !colours || hex == "" {
		return line
	}
	return paintBackground(line, hex)
}

// paintBackground is the wash itself, terminal support aside.
func paintBackground(line, hex string) string {
	set := bgEscape(hex)
	return set + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+set) + "\x1b[0m"
}

// stripBackground drops background colours but keeps foregrounds, so the cursor
// band can replace the added/removed wash without losing the syntax colours.
func stripBackground(line string) string {
	var b strings.Builder
	for {
		i := strings.Index(line, "\x1b[48;2;")
		if i < 0 {
			b.WriteString(line)
			return b.String()
		}
		b.WriteString(line[:i])
		rest := line[i:]
		end := strings.IndexByte(rest, 'm')
		if end < 0 {
			return b.String()
		}
		line = rest[end+1:]
	}
}

// VisibleWidth is the rendered cell width of a string, ignoring ANSI styling.
func VisibleWidth(s string) int {
	return uniseg.StringWidth(stripANSI(s))
}

// stripANSI removes escape sequences so widths can be measured and strings cut.
func stripANSI(s string) string {
	out := make([]rune, 0, len(s))
	inEscape := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEscape = true
		case inEscape && (r == 'm' || r == 'K'):
			inEscape = false
		case !inEscape:
			out = append(out, r)
		}
	}
	return string(out)
}

// truncateVisible cuts a plain string to at most width cells, marking the cut.
func truncateVisible(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if uniseg.StringWidth(s) <= width {
		return s
	}
	// The marker is not always one cell: the unicode ellipsis is one and the
	// ASCII one is three, and reserving a single cell for either would overflow
	// the width this was given.
	mark := gl.Ellipsis
	mw := uniseg.StringWidth(mark)
	if width <= mw {
		return cutToWidth(mark, width)
	}
	out := make([]rune, 0, len(s))
	w := 0
	for _, r := range s {
		rw := uniseg.StringWidth(string(r))
		if w+rw > width-mw {
			break
		}
		out = append(out, r)
		w += rw
	}
	return string(out) + mark
}

// cutToWidth takes as many whole runes as fit, with nothing to mark the cut:
// it is for the marker itself, when even that does not fit.
func cutToWidth(s string, width int) string {
	out := make([]rune, 0, len(s))
	w := 0
	for _, r := range s {
		rw := uniseg.StringWidth(string(r))
		if w+rw > width {
			break
		}
		out = append(out, r)
		w += rw
	}
	return string(out)
}

// StripANSIForTest exposes stripANSI so tests can assert on plain text.
func StripANSIForTest(s string) string { return stripANSI(s) }
