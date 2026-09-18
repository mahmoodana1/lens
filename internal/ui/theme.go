package ui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

// tokyonight-moon, the same palette the editor next to this panel uses, so a
// diff here and the file open in nvim read as the same code.
const (
	tnBg          = "#222436"
	tnBgHighlight = "#2f334d"
	tnFg          = "#c8d3f5"
	tnFgDark      = "#828bb8"
	tnComment     = "#636da6"
	tnBlue        = "#82aaff"
	tnBlue1       = "#65bcff"
	tnBlue5       = "#89ddff"
	tnCyan        = "#86e1fc"
	tnGreen       = "#c3e88d"
	tnMagenta     = "#c099ff"
	tnOrange      = "#ff966c"
	tnRed         = "#ff757f"
	tnYellow      = "#ffc777"
	tnTeal        = "#4fd6be"
	tnGitAdd      = "#b8db87"
	tnGitDelete   = "#e26a75"
)

// Changed lines get a wash of their diff colour rather than a solid block, so
// the syntax colouring stays readable on top of it.
var (
	bgAdd    = blend(tnGitAdd, tnBg, 0.22)
	bgDel    = blend(tnGitDelete, tnBg, 0.22)
	bgCursor = tnBgHighlight
)

var (
	colAdd     = lipgloss.Color(tnGitAdd)
	colDel     = lipgloss.Color(tnGitDelete)
	colDim     = lipgloss.Color(tnComment)
	colAccent  = lipgloss.Color(tnBlue)
	colHeading = lipgloss.Color(tnCyan)

	styHeading  = lipgloss.NewStyle().Foreground(colHeading).Bold(true)
	styIntent   = lipgloss.NewStyle().Foreground(colAccent).Italic(true)
	styAdd      = lipgloss.NewStyle().Foreground(colAdd)
	styDel      = lipgloss.NewStyle().Foreground(colDel)
	styContext  = lipgloss.NewStyle().Foreground(lipgloss.NoColor{})
	styGutter   = lipgloss.NewStyle().Foreground(colDim)
	styDim      = lipgloss.NewStyle().Foreground(colDim)
	stySelected = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styRow      = lipgloss.NewStyle()
	styHelp     = lipgloss.NewStyle().Foreground(colDim)
	styBorder   = lipgloss.NewStyle().Foreground(colDim)
)

// colours reports whether the terminal takes colour at all. Backgrounds are
// written by hand rather than through lipgloss, so they need the same gate.
var colours = lipgloss.NewStyle().Foreground(lipgloss.Color(tnRed)).Render("x") != "x"

// syntaxStyle is tokyonight-moon expressed as a chroma style.
func syntaxStyle() *chroma.Style {
	st, err := chroma.NewStyle("tokyonight-moon", chroma.StyleEntries{
		chroma.Background:      tnFg + " bg:" + tnBg,
		chroma.Comment:         "italic " + tnComment,
		chroma.CommentPreproc:  tnMagenta,
		chroma.Keyword:         tnMagenta,
		chroma.KeywordType:     tnBlue1,
		chroma.KeywordConstant: tnOrange,
		chroma.Operator:        tnBlue5,
		chroma.Punctuation:     tnFgDark,
		chroma.Name:            tnFg,
		chroma.NameBuiltin:     tnBlue1,
		chroma.NameClass:       tnBlue1,
		chroma.NameFunction:    tnBlue,
		chroma.NameTag:         tnRed,
		chroma.NameAttribute:   tnTeal,
		chroma.NameConstant:    tnOrange,
		chroma.NameDecorator:   tnYellow,
		chroma.LiteralString:   tnGreen,
		chroma.LiteralNumber:   tnOrange,
		chroma.GenericInserted: tnGitAdd,
		chroma.GenericDeleted:  tnGitDelete,
		chroma.Error:           tnRed,
	})
	if err != nil {
		return styles.Fallback
	}
	return st
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

// lipglossColour turns a chroma hex colour into a lipgloss style.
func lipglossColour(hex string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
}

// StripANSIForTest exposes stripANSI so tests can assert on plain text.
func StripANSIForTest(s string) string { return stripANSI(s) }
