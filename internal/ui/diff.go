package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/mahmood/lens/internal/capture"
)

// Rendered is a laid-out diff together with the line offsets of each hunk,
// which is what n and N jump between.
type Rendered struct {
	Lines      []string
	HunkStarts []int
}

// RenderDiff lays out one edit: a header, the request that caused it, then each
// hunk with ctx lines of surrounding code. Lines are truncated to width so the
// two-pane layout never wraps.
func RenderDiff(e capture.Event, prompt string, ctx int, width int) []string {
	return RenderDiffFull(e, prompt, ctx, width).Lines
}

// RenderDiffFull renders a diff and reports where each hunk begins.
func RenderDiffFull(e capture.Event, prompt string, ctx int, width int) Rendered {
	if width < 20 {
		width = 20
	}
	var out []string
	var starts []int

	head := fmt.Sprintf("%s  %s", e.Rel, counts(e.Added, e.Removed))
	out = append(out, styHeading.Render(truncateVisible(head, width)))

	sub := fmt.Sprintf("%s · %s", orDash(e.Tool), e.Time.Format("15:04:05"))
	if e.Kind == "create" {
		sub = "new file · " + sub
	}
	out = append(out, styDim.Render(truncateVisible(sub, width)))

	if prompt != "" {
		out = append(out, "")
		for _, line := range wrap(collapse(prompt), width-2) {
			out = append(out, styIntent.Render("❯ "+line))
		}
	}

	hl := newHighlighter(e.Rel)
	for _, h := range e.Hunks {
		out = append(out, "")
		starts = append(starts, len(out))
		out = append(out, styGutter.Render(truncateVisible(hunkHeader(h), width)))
		out = append(out, renderHunk(h, ctx, width, hl)...)
	}
	return Rendered{Lines: out, HunkStarts: starts}
}

func renderHunk(h capture.Hunk, ctx, width int, hl *highlighter) []string {
	var out []string

	// Leading context: the lines nearest the hunk, so widening grows outward.
	above := tail(h.Above, ctx)
	oldNo := h.OldStart - len(above)
	newNo := h.NewStart - len(above)
	for _, ln := range above {
		out = append(out, gutterLine(oldNo, newNo, " ", ln, width, hl))
		oldNo++
		newNo++
	}

	for _, raw := range h.Lines {
		if raw == "" {
			out = append(out, gutterLine(oldNo, newNo, " ", "", width, hl))
			oldNo++
			newNo++
			continue
		}
		mark, body := raw[:1], raw[1:]
		switch mark {
		case "\\":
			continue // "no newline at end of file" is bookkeeping, not code
		case "+":
			out = append(out, gutterLine(0, newNo, "+", body, width, hl))
			newNo++
		case "-":
			out = append(out, gutterLine(oldNo, 0, "-", body, width, hl))
			oldNo++
		default:
			out = append(out, gutterLine(oldNo, newNo, " ", body, width, hl))
			oldNo++
			newNo++
		}
	}

	for _, ln := range head(h.Below, ctx) {
		out = append(out, gutterLine(oldNo, newNo, " ", ln, width, hl))
		oldNo++
		newNo++
	}
	return out
}

// gutterLine renders one source line: line number, change marker, then the code.
//
// Numbers are the file's current ones. A removed line has no number because it
// no longer exists — mixing old and new numbering in one column reads as noise.
func gutterLine(oldNo, newNo int, mark, body string, width int, hl *highlighter) string {
	num := "    "
	if mark != "-" && newNo > 0 {
		num = fmt.Sprintf("%4d", newNo)
	}

	const gutter = 7 // "NNNN"+space+mark+space
	body = strings.ReplaceAll(body, "\t", "    ")
	body = truncateVisible(body, width-gutter)

	var code string
	switch mark {
	case "+":
		code = styAdd.Render(body)
	case "-":
		code = styDel.Render(body)
	default:
		code = hl.render(body)
	}

	marker := mark
	switch mark {
	case "+":
		marker = styAdd.Render("+")
	case "-":
		marker = styDel.Render("-")
	default:
		marker = " "
	}
	return styGutter.Render(num) + " " + marker + " " + code
}

func hunkHeader(h capture.Hunk) string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
}

func counts(added, removed int) string {
	parts := []string{}
	if added > 0 {
		parts = append(parts, styAdd.Render(fmt.Sprintf("+%d", added)))
	}
	if removed > 0 {
		parts = append(parts, styDel.Render(fmt.Sprintf("-%d", removed)))
	}
	if len(parts) == 0 {
		return styDim.Render("no change")
	}
	return strings.Join(parts, " ")
}

// highlighter colours unchanged context lines by language. Added and removed
// lines keep their diff colour, which matters more than their syntax.
type highlighter struct {
	lexer chroma.Lexer
	style *chroma.Style
}

func newHighlighter(path string) *highlighter {
	lx := lexers.Match(filepath.Base(path))
	if lx == nil {
		return &highlighter{}
	}
	return &highlighter{lexer: chroma.Coalesce(lx), style: styles.Get("nord")}
}

func (h *highlighter) render(s string) string {
	if h.lexer == nil || s == "" {
		return s
	}
	it, err := h.lexer.Tokenise(nil, s)
	if err != nil {
		return s
	}
	var b strings.Builder
	for _, tok := range it.Tokens() {
		entry := h.style.Get(tok.Type)
		if entry.Colour.IsSet() {
			b.WriteString(lipglossColour(entry.Colour.String()).Render(tok.Value))
		} else {
			b.WriteString(tok.Value)
		}
	}
	return b.String()
}

func tail(s []string, n int) []string {
	if n <= 0 || len(s) == 0 {
		return nil
	}
	if n >= len(s) {
		return s
	}
	return s[len(s)-n:]
}

func head(s []string, n int) []string {
	if n <= 0 || len(s) == 0 {
		return nil
	}
	if n >= len(s) {
		return s
	}
	return s[:n]
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// collapse flattens a multi-line prompt so the intent line stays compact.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func wrap(s string, width int) []string {
	if width < 10 {
		width = 10
	}
	var out []string
	for len(s) > 0 {
		if len(s) <= width {
			out = append(out, s)
			break
		}
		cut := strings.LastIndex(s[:width], " ")
		if cut <= 0 {
			cut = width
		}
		out = append(out, strings.TrimSpace(s[:cut]))
		s = strings.TrimSpace(s[cut:])
		if len(out) == 3 { // an intent line is a reminder, not the whole prompt
			out[2] = truncateVisible(out[2], width)
			break
		}
	}
	return out
}
