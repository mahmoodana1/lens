package ui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
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

// The washes behind changed lines are the editor's own DiffAdd, DiffDelete and
// CursorLine — taken whole rather than mixed here, so a changed line in the
// panel is the colour a changed line is in the editor.
var (
	bgAdd    = tnDiffAdd
	bgDel    = tnDiffDelete
	bgCursor = tnCursorLine
)

var (
	colAdd     = lipgloss.Color(tnGreen)
	colDel     = lipgloss.Color(tnRed)
	colDim     = lipgloss.Color(tnComment)
	colAccent  = lipgloss.Color(tnBlue)
	colHeading = lipgloss.Color(tnBlue)

	styHeading = lipgloss.NewStyle().Foreground(colHeading).Bold(true) // Title
	styIntent  = lipgloss.NewStyle().Foreground(lipgloss.Color(tnTeal)).Italic(true)
	styAdd     = lipgloss.NewStyle().Foreground(colAdd)
	styDel     = lipgloss.NewStyle().Foreground(colDel)
	// LineNr and CursorLineNr: the numbers recede, the selection does not.
	styGutter   = lipgloss.NewStyle().Foreground(lipgloss.Color(tnLineNr))
	styHunk     = lipgloss.NewStyle().Foreground(lipgloss.Color(tnBlue5))
	styDim      = lipgloss.NewStyle().Foreground(colDim)
	stySelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(tnCursorLineNr))
	styRow      = lipgloss.NewStyle().Foreground(lipgloss.Color(tnFg))
	styHelp     = lipgloss.NewStyle().Foreground(colDim)
	styBorder   = lipgloss.NewStyle().Foreground(lipgloss.Color(tnLineNr))
)

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
		chroma.Background: tnFg + " bg:" + tnBg,
		chroma.Text:       tnFg,

		chroma.Comment:        "italic " + tnComment,
		chroma.CommentPreproc: tnCyan, // PreProc
		chroma.CommentSpecial: "italic " + tnComment,

		chroma.Keyword:            tnMagenta, // @keyword.function, .conditional, .repeat
		chroma.KeywordConstant:    tnBlue1,   // @constant.builtin: true, false, nil
		chroma.KeywordDeclaration: tnMagenta,
		chroma.KeywordNamespace:   tnCyan, // @keyword.import
		chroma.KeywordPseudo:      tnBlue1,
		chroma.KeywordReserved:    tnMagenta,
		chroma.KeywordType:        tnBlue1, // @type.builtin sits close to Type

		chroma.Name:                 tnFg, // @variable
		chroma.NameAttribute:        tnCyan,
		chroma.NameBuiltin:          tnBlue1, // @function.builtin
		chroma.NameBuiltinPseudo:    tnRed,   // @variable.builtin: self, this
		chroma.NameClass:            tnBlue1, // Type
		chroma.NameConstant:         tnOrange,
		chroma.NameDecorator:        tnCyan, // @attribute
		chroma.NameEntity:           tnCyan,
		chroma.NameException:        tnMagenta,
		chroma.NameFunction:         tnBlue,
		chroma.NameFunctionMagic:    tnBlue1,
		chroma.NameLabel:            tnBlue,
		chroma.NameNamespace:        tnCyan, // @module
		chroma.NameOther:            tnFg,
		chroma.NameProperty:         tnTeal, // @property
		chroma.NameTag:              tnBlue1,
		chroma.NameVariable:         tnFg,
		chroma.NameVariableClass:    tnTeal,
		chroma.NameVariableGlobal:   tnTeal,
		chroma.NameVariableInstance: tnTeal, // @variable.member
		chroma.NameVariableMagic:    tnRed,

		chroma.Literal:                tnOrange,
		chroma.LiteralDate:            tnOrange,
		chroma.LiteralNumber:          tnOrange,
		chroma.LiteralString:          tnGreen,
		chroma.LiteralStringAffix:     tnMagenta,
		chroma.LiteralStringChar:      tnGreen, // Character
		chroma.LiteralStringDoc:       "italic " + tnYellow,
		chroma.LiteralStringEscape:    tnMagenta, // @string.escape
		chroma.LiteralStringInterpol:  tnMagenta,
		chroma.LiteralStringRegex:     tnRegex,
		chroma.LiteralStringSymbol:    tnGreen,
		chroma.LiteralStringDelimiter: tnGreen,
		chroma.LiteralStringBacktick:  tnGreen,
		chroma.LiteralStringDouble:    tnGreen,
		chroma.LiteralStringSingle:    tnGreen,
		chroma.LiteralStringHeredoc:   tnGreen,
		chroma.LiteralStringOther:     tnGreen,

		chroma.Operator:     tnBlue5,
		chroma.OperatorWord: tnBlue5,  // @keyword.operator
		chroma.Punctuation:  tnFgDark, // @punctuation.bracket

		chroma.GenericDeleted:    tnRed,
		chroma.GenericInserted:   tnGreen,
		chroma.GenericEmph:       "italic",
		chroma.GenericStrong:     "bold",
		chroma.GenericHeading:    "bold " + tnBlue,
		chroma.GenericSubheading: "bold " + tnBlue1,

		chroma.Error: tnError,
	})
	if err != nil {
		return styles.Fallback
	}
	return st
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
	if width == 1 {
		return "…"
	}
	out := make([]rune, 0, len(s))
	w := 0
	for _, r := range s {
		rw := uniseg.StringWidth(string(r))
		if w+rw > width-1 {
			break
		}
		out = append(out, r)
		w += rw
	}
	return string(out) + "…"
}

// StripANSIForTest exposes stripANSI so tests can assert on plain text.
func StripANSIForTest(s string) string { return stripANSI(s) }
