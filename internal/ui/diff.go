package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/mahmood/lens/internal/capture"
)

// sampleBytes caps how much code is read to work out what language it is.
const sampleBytes = 4096

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
// offsets of each hunk, which is what n and N jump between. HunkIDs says which
// hunk of the edit each of those offsets belongs to, since a dismissed hunk is
// left out of the render but still numbered in the edit.
type Rendered struct {
	Lines      []string
	Kinds      []LineKind
	HunkStarts []int
	HunkIDs    []int
	// Nums is the line each rendered line shows in the file, or 0 for the
	// header, the prompt and removed lines that no longer have one. It is how
	// "open where I am looking" finds a place to open.
	Nums []int
}

// StartOf is the line a hunk of the edit begins on, and whether it is shown.
func (r Rendered) StartOf(hunk int) (int, bool) {
	for i, id := range r.HunkIDs {
		if id == hunk {
			return r.HunkStarts[i], true
		}
	}
	return 0, false
}

// add appends a line of the given kind, showing no line of the file.
func (r *Rendered) add(kind LineKind, line string) {
	r.addAt(kind, line, 0)
}

// addAt appends a line that shows line num of the file.
func (r *Rendered) addAt(kind LineKind, line string, num int) {
	r.Lines = append(r.Lines, line)
	r.Kinds = append(r.Kinds, kind)
	r.Nums = append(r.Nums, num)
}

// NumAt is the line of the file shown at a rendered line.
//
// Not every rendered line has one: a hunk header, the prompt, a removed line
// that the file no longer holds. The search runs forward first and only then
// back, because the lines without a number all sit above the code they belong
// to — resting on a hunk header means the hunk below, not the one before it.
func (r Rendered) NumAt(i int) int {
	if i < 0 || i >= len(r.Nums) {
		return 0
	}
	for j := i; j < len(r.Nums); j++ {
		if r.Nums[j] > 0 {
			return r.Nums[j]
		}
	}
	for j := i; j >= 0; j-- {
		if r.Nums[j] > 0 {
			return r.Nums[j]
		}
	}
	return 0
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
	return RenderDiffFull(e, prompt, ctx, width, nil).Lines
}

// RenderDiffFull renders a diff and reports where each hunk begins. Hunks the
// reader has dismissed are left out.
func RenderDiffFull(e capture.Event, prompt string, ctx int, width int, hidden Hidden) Rendered {
	if width < 20 {
		width = 20
	}
	var r Rendered

	head := fmt.Sprintf("%s  %s", e.Rel, counts(e.Added, e.Removed))
	r.add(LinePlain, styHeading.Render(truncateVisible(head, width)))

	sub := fmt.Sprintf("%s · %s", orDash(e.Tool), e.Time.Format("15:04:05"))
	switch e.Kind {
	case "create":
		sub = "new file · " + sub
	case "delete":
		// Not an edit that happened to remove a lot: the file is gone.
		sub = "deleted · " + sub
	}
	r.add(LinePlain, styDim.Render(truncateVisible(sub, width)))

	if prompt != "" {
		r.add(LinePlain, "")
		for _, line := range wrap(collapse(prompt), width-2) {
			r.add(LinePlain, styIntent.Render("❯ "+line))
		}
	}

	hl := newHighlighter(e.Rel, codeSample(e))
	for i, h := range e.Hunks {
		if hidden.Hunk(e.Rel, e.Seq, i) {
			continue
		}
		r.add(LinePlain, "")
		r.HunkStarts = append(r.HunkStarts, len(r.Lines))
		r.HunkIDs = append(r.HunkIDs, i)
		r.add(LinePlain, styHunk.Render(truncateVisible(hunkHeader(h), width)))
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
		r.addAt(LinePlain, gutterLine(newNo, " ", ln, width, hl), newNo)
		oldNo++
		newNo++
	}

	for _, raw := range h.Lines {
		if raw == "" {
			r.addAt(LinePlain, gutterLine(newNo, " ", "", width, hl), newNo)
			oldNo++
			newNo++
			continue
		}
		mark, body := raw[:1], raw[1:]
		switch mark {
		case "\\":
			continue // "no newline at end of file" is bookkeeping, not code
		case "+":
			r.addAt(LineAdd, gutterLine(newNo, "+", body, width, hl), newNo)
			newNo++
		case "-":
			r.add(LineDel, gutterLine(0, "-", body, width, hl))
			oldNo++
		default:
			r.addAt(LinePlain, gutterLine(newNo, " ", body, width, hl), newNo)
			oldNo++
			newNo++
		}
	}

	for _, ln := range head(h.Below, ctx) {
		r.addAt(LinePlain, gutterLine(newNo, " ", ln, width, hl), newNo)
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

func newHighlighter(path, code string) *highlighter {
	lx := lexerFor(path, code)
	if lx == nil {
		return &highlighter{}
	}
	return &highlighter{lexer: chroma.Coalesce(lx), style: editorStyle}
}

// standIn names a lexer for file types chroma has none of, where another
// language is close enough to read by. A .bats file is bash with a test
// harness, a .tmux config is shell-shaped, a justfile is a makefile — colouring
// them approximately beats handing back a wall of undifferentiated text.
var standIn = map[string]string{
	".bats":  "bash",
	".tmux":  "bash",
	".envrc": "bash",
	".just":  "make",
	".conf":  "ini",
	".astro": "html",
	".mdx":   "markdown",
}

// lexerFor picks the lexer to colour a file with: its name where chroma knows
// it, a stand-in where one reads close enough, and otherwise whatever the code
// itself gives away — a shebang on a file with no extension at all.
//
// Returning nil means the text is left exactly as it came.
func lexerFor(path, code string) chroma.Lexer {
	base := filepath.Base(path)
	if lx := lexers.Match(base); lx != nil {
		return lx
	}
	if name, ok := standIn[strings.ToLower(filepath.Ext(base))]; ok {
		if lx := lexers.Get(name); lx != nil {
			return lx
		}
	}
	if code != "" {
		return lexers.Analyse(code)
	}
	return nil
}

// codeSample is the code an edit touched, with the diff markers taken off, for
// working out what language it is when the name does not say.
func codeSample(e capture.Event) string {
	var b strings.Builder
	for _, h := range e.Hunks {
		for _, raw := range h.Lines {
			if raw == "" {
				b.WriteByte('\n')
				continue
			}
			switch raw[0] {
			case '\\': // the no-newline marker is bookkeeping, not code
				continue
			case '+', '-', ' ':
				b.WriteString(raw[1:])
			default:
				b.WriteString(raw)
			}
			b.WriteByte('\n')
		}
		if b.Len() > sampleBytes {
			break
		}
	}
	return b.String()
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
		// One line in, one line out. Chroma's Makefile lexer hands its recipe
		// lines to a bash lexer, which returns them with a trailing newline and
		// padding; a diff row carrying one becomes two rows on screen, and the
		// wash behind a changed line floods across the break into the next.
		value, broke := untilLineBreak(tok.Value)
		if value != "" {
			if st, ok := entryStyle(h.style.Get(tok.Type)); ok {
				b.WriteString(st.Render(value))
			} else {
				b.WriteString(value)
			}
		}
		if broke {
			break
		}
	}
	return b.String()
}

// untilLineBreak is the text up to the first line break, and whether it found
// one. A lexer given a single line should return a single line; anything past a
// break it introduced is its own bookkeeping, not code to show.
func untilLineBreak(s string) (string, bool) {
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		return s[:i], true
	}
	return s, false
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
