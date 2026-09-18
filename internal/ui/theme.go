package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

// The palette leans on the terminal's own colours where it can, so the panel
// sits comfortably next to Claude Code in the same window.
var (
	colAdd     = lipgloss.Color("2")
	colDel     = lipgloss.Color("1")
	colDim     = lipgloss.Color("8")
	colAccent  = lipgloss.Color("4")
	colHeading = lipgloss.Color("6")

	styHeading  = lipgloss.NewStyle().Foreground(colHeading).Bold(true)
	styIntent   = lipgloss.NewStyle().Foreground(colAccent).Italic(true)
	styAdd      = lipgloss.NewStyle().Foreground(colAdd)
	styDel      = lipgloss.NewStyle().Foreground(colDel)
	styContext  = lipgloss.NewStyle().Foreground(lipgloss.NoColor{})
	styGutter   = lipgloss.NewStyle().Foreground(colDim)
	styDim      = lipgloss.NewStyle().Foreground(colDim)
	stySelected = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styHelp     = lipgloss.NewStyle().Foreground(colDim)
	styBorder   = lipgloss.NewStyle().Foreground(colDim)
)

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
