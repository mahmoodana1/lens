package ui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

// catppuccin mocha, the flavour the editor next to this panel wears, so a diff
// here and the file open in nvim read as the same code. The values are the
// flavour's own palette, and the mapping below is catppuccin's own — its syntax
// and treesitter highlight groups, transcribed onto chroma's token types.
const (
	ctpRosewater = "#f5e0dc"
	ctpFlamingo  = "#f2cdcd"
	ctpPink      = "#f5c2e7"
	ctpMauve     = "#cba6f7"
	ctpRed       = "#f38ba8"
	ctpMaroon    = "#eba0ac"
	ctpPeach     = "#fab387"
	ctpYellow    = "#f9e2af"
	ctpGreen     = "#a6e3a1"
	ctpTeal      = "#94e2d5"
	ctpSky       = "#89dceb"
	ctpSapphire  = "#74c7ec"
	ctpBlue      = "#89b4fa"
	ctpLavender  = "#b4befe"
	ctpText      = "#cdd6f4"
	ctpSubtext0  = "#a6adc8"
	ctpOverlay2  = "#9399b2"
	ctpOverlay1  = "#7f849c"
	ctpOverlay0  = "#6c7086"
	ctpSurface2  = "#585b70"
	ctpSurface1  = "#45475a"
	ctpSurface0  = "#313244"
	ctpBase      = "#1e1e2e"
)

// The washes are the editor's own DiffAdd, DiffDelete and CursorLine, which
// catppuccin builds by mixing a colour into the base rather than painting a
// solid block — which is what keeps the syntax colouring readable on top.
var (
	bgAdd    = blend(ctpGreen, ctpBase, 0.18)
	bgDel    = blend(ctpRed, ctpBase, 0.18)
	bgCursor = blend(ctpSurface0, ctpBase, 0.64)
)

var (
	colAdd     = lipgloss.Color(ctpGreen)
	colDel     = lipgloss.Color(ctpRed)
	colDim     = lipgloss.Color(ctpOverlay0)
	colAccent  = lipgloss.Color(ctpLavender)
	colHeading = lipgloss.Color(ctpBlue)

	styHeading = lipgloss.NewStyle().Foreground(colHeading).Bold(true) // Title
	styIntent  = lipgloss.NewStyle().Foreground(colAccent).Italic(true)
	styAdd     = lipgloss.NewStyle().Foreground(colAdd)
	styDel     = lipgloss.NewStyle().Foreground(colDel)
	// LineNr and CursorLineNr: the numbers recede, the selection does not.
	styGutter   = lipgloss.NewStyle().Foreground(lipgloss.Color(ctpSurface1))
	styHunk     = lipgloss.NewStyle().Foreground(lipgloss.Color(ctpSapphire))
	styDim      = lipgloss.NewStyle().Foreground(colDim)
	stySelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ctpLavender))
	styRow      = lipgloss.NewStyle().Foreground(lipgloss.Color(ctpText))
	styHelp     = lipgloss.NewStyle().Foreground(colDim)
	styBorder   = lipgloss.NewStyle().Foreground(lipgloss.Color(ctpSurface1))
)

// colours reports whether the terminal takes colour at all. Backgrounds are
// written by hand rather than through lipgloss, so they need the same gate.
var colours = lipgloss.NewStyle().Foreground(lipgloss.Color(ctpRed)).Render("x") != "x"

// syntaxStyle is catppuccin mocha expressed as a chroma style.
//
// chroma splits code into finer token types than vim's syntax groups, so this
// follows catppuccin's treesitter map where one exists and its syntax map
// otherwise. Anything left out inherits from its parent token, which is why the
// broad entries — Name, Literal, Keyword — are set as well as the narrow ones.
//
// One thing cannot be carried across: catppuccin italicises conditionals but
// not other keywords, and chroma has no token that separates `if` from `func`.
// Keywords are left upright rather than italicising all of them.
func syntaxStyle() *chroma.Style {
	st, err := chroma.NewStyle("catppuccin-mocha", chroma.StyleEntries{
		chroma.Background: ctpText + " bg:" + ctpBase,
		chroma.Text:       ctpText,

		chroma.Comment:        "italic " + ctpOverlay2,
		chroma.CommentPreproc: ctpPink, // PreProc
		chroma.CommentSpecial: "italic " + ctpPink,

		chroma.Keyword:            ctpMauve,
		chroma.KeywordConstant:    ctpPeach, // true, false, nil
		chroma.KeywordDeclaration: ctpMauve,
		chroma.KeywordNamespace:   ctpMauve, // Include: import, package
		chroma.KeywordPseudo:      ctpPeach,
		chroma.KeywordReserved:    ctpMauve,
		chroma.KeywordType:        ctpMauve, // @type.builtin: int, string

		chroma.Name:                 ctpText, // @variable
		chroma.NameAttribute:        ctpLavender,
		chroma.NameBuiltin:          ctpPeach, // @function.builtin
		chroma.NameBuiltinPseudo:    ctpRed,   // @variable.builtin: self, this
		chroma.NameClass:            ctpYellow,
		chroma.NameConstant:         ctpPeach,
		chroma.NameDecorator:        ctpPeach, // @attribute
		chroma.NameEntity:           ctpPink,
		chroma.NameException:        ctpMauve,
		chroma.NameFunction:         ctpBlue,
		chroma.NameFunctionMagic:    ctpPeach,
		chroma.NameLabel:            ctpSapphire,
		chroma.NameNamespace:        "italic " + ctpYellow, // @module
		chroma.NameOther:            ctpText,
		chroma.NameProperty:         ctpLavender, // @property
		chroma.NameTag:              ctpLavender,
		chroma.NameVariable:         ctpText,
		chroma.NameVariableClass:    ctpLavender,
		chroma.NameVariableGlobal:   ctpLavender,
		chroma.NameVariableInstance: ctpLavender, // @variable.member
		chroma.NameVariableMagic:    ctpRed,

		chroma.Literal:                ctpPeach,
		chroma.LiteralDate:            ctpPink,
		chroma.LiteralNumber:          ctpPeach,
		chroma.LiteralString:          ctpGreen,
		chroma.LiteralStringAffix:     ctpMauve,
		chroma.LiteralStringChar:      ctpTeal, // Character
		chroma.LiteralStringDoc:       "italic " + ctpTeal,
		chroma.LiteralStringEscape:    ctpPink,
		chroma.LiteralStringInterpol:  ctpPink,
		chroma.LiteralStringRegex:     ctpPink,
		chroma.LiteralStringSymbol:    ctpFlamingo,
		chroma.LiteralStringDelimiter: ctpGreen,
		chroma.LiteralStringBacktick:  ctpGreen,
		chroma.LiteralStringDouble:    ctpGreen,
		chroma.LiteralStringSingle:    ctpGreen,
		chroma.LiteralStringHeredoc:   ctpGreen,
		chroma.LiteralStringOther:     ctpGreen,

		chroma.Operator:     ctpSky,
		chroma.OperatorWord: ctpMauve, // @keyword.operator
		chroma.Punctuation:  ctpOverlay2,

		chroma.GenericDeleted:    ctpRed,
		chroma.GenericInserted:   ctpGreen,
		chroma.GenericEmph:       "italic",
		chroma.GenericStrong:     "bold",
		chroma.GenericHeading:    "bold " + ctpBlue,
		chroma.GenericSubheading: "bold " + ctpSapphire,

		chroma.Error: ctpRed,
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
