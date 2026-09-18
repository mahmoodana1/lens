package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/mahmood/lens/internal/capture"
)

// LineKind says how a diff line should be washed.
type LineKind uint8

const (
	// LinePlain is anything unchanged: context, headers, the prompt.
	LinePlain LineKind = iota
	// LineAdd and LineDel are the lines the edit added and removed.
	LineAdd
	LineDel
)

// Rendered is a laid-out diff together with what each line is and the line
// offsets of each hunk, which is what n and N jump between.
type Rendered struct {
	Lines      []string
	Kinds      []LineKind
	HunkStarts []int
}

// add appends a line of the given kind.
func (r *Rendered) add(kind LineKind, line string) {
	r.Lines = append(r.Lines, line)
	r.Kinds = append(r.Kinds, kind)
}

// diffBackground picks the wash for a diff line. The cursor band beats the
// added and removed washes, and there is no band unless the diff has focus.
func diffBackground(kind LineKind, onCursor, focused bool) string {
	if focused && onCursor {
		return bgCursor
	}
	switch kind {
	case LineAdd:
		return bgAdd
	case LineDel:
		return bgDel
	}
	return ""
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
	var r Rendered

	head := fmt.Sprintf("%s  %s", e.Rel, counts(e.Added, e.Removed))
	r.add(LinePlain, styHeading.Render(truncateVisible(head, width)))

	sub := fmt.Sprintf("%s · %s", orDash(e.Tool), e.Time.Format("15:04:05"))
	if e.Kind == "create" {
		sub = "new file · " + sub
	}
	r.add(LinePlain, styDim.Render(truncateVisible(sub, width)))

	if prompt != "" {
		r.add(LinePlain, "")
		for _, line := range wrap(collapse(prompt), width-2) {
			r.add(LinePlain, styIntent.Render("❯ "+line))
		}
	}

	hl := newHighlighter(e.Rel)
	for _, h := range e.Hunks {
		r.add(LinePlain, "")
		r.HunkStarts = append(r.HunkStarts, len(r.Lines))
		r.add(LinePlain, styGutter.Render(truncateVisible(hunkHeader(h), width)))
		renderHunk(&r, h, ctx, width, hl)
	}
	return r
}

func renderHunk(r *Rendered, h capture.Hunk, ctx, width int, hl *highlighter) {
	// Leading context: the lines nearest the hunk, so widening grows outward.
	above := tail(h.Above, ctx)
	oldNo := h.OldStart - len(above)
	newNo := h.NewStart - len(above)
	for _, ln := range above {
		r.add(LinePlain, gutterLine(newNo, " ", ln, width, hl))
		oldNo++
		newNo++
	}

	for _, raw := range h.Lines {
		if raw == "" {
			r.add(LinePlain, gutterLine(newNo, " ", "", width, hl))
			oldNo++
			newNo++
			continue
		}
		mark, body := raw[:1], raw[1:]
		switch mark {
		case "\\":
			continue // "no newline at end of file" is bookkeeping, not code
		case "+":
			r.add(LineAdd, gutterLine(newNo, "+", body, width, hl))
			newNo++
		case "-":
			r.add(LineDel, gutterLine(0, "-", body, width, hl))
			oldNo++
		default:
			r.add(LinePlain, gutterLine(newNo, " ", body, width, hl))
			oldNo++
			newNo++
		}
	}

	for _, ln := range head(h.Below, ctx) {
		r.add(LinePlain, gutterLine(newNo, " ", ln, width, hl))
		oldNo++
		newNo++
	}
}

// gutterLine renders one source line: line number, change marker, then the code.
//
// Numbers are the file's current ones. A removed line has no number because it
// no longer exists — mixing old and new numbering in one column reads as noise.
func gutterLine(newNo int, mark, body string, width int, hl *highlighter) string {
	num := "    "
	if mark != "-" && newNo > 0 {
		num = fmt.Sprintf("%4d", newNo)
	}

	const gutter = 7 // "NNNN"+space+mark+space
	body = strings.ReplaceAll(body, "\t", "    ")
	body = truncateVisible(body, width-gutter)

	// Changed lines keep their syntax colours; the wash behind them is what says
	// they changed, so the code reads the same here as it does in the editor.
	marker := " "
	switch mark {
	case "+":
		marker = styAdd.Render("+")
	case "-":
		marker = styDel.Render("-")
	}
	return styGutter.Render(num) + " " + marker + " " + hl.render(body)
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

// editorStyle is built once: chroma styles are immutable and the panel renders
// a diff on every keystroke.
var editorStyle = syntaxStyle()

// highlighter colours code by language, in the editor's own palette.
type highlighter struct {
	lexer chroma.Lexer
	style *chroma.Style
}

func newHighlighter(path string) *highlighter {
	lx := lexers.Match(filepath.Base(path))
	if lx == nil {
		return &highlighter{}
	}
	return &highlighter{lexer: chroma.Coalesce(lx), style: editorStyle}
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
