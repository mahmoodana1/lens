package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const helpText = "j/k move · l open file · o edit · dd hide · u undo · / find · tab focus · q quit"

const insideHelpText = "j/k move · enter full diff · o edit · h back · dd hide · n/N hunk · q quit"

const panelHelpText = "j/k line · n/N hunk · o edit · J/K file · ctrl-f/b page · esc back · q quit"

// View renders the whole panel. It never exceeds the terminal's bounds: the
// panel shares a window with Claude Code and must not reflow it.
func (m *Model) View() string {
	body := m.bodyHeight()

	var lines []string
	lines = append(lines, truncateStyled(m.statusLine(), m.width))

	if m.showHelp {
		lines = append(lines, m.helpLines(body)...)
	} else if len(m.rows) == 0 {
		lines = append(lines, m.waitingLines(body)...)
	} else {
		lines = append(lines, m.paneLines(body)...)
	}

	lines = append(lines, m.footer())

	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

// footer is the help line, or the filter prompt while one is being typed.
func (m *Model) footer() string {
	if m.filtering {
		return m.promptLine()
	}
	help := helpText
	if m.panel {
		return styHelp.Render(truncateVisible(panelHelpText, m.width))
	}
	if m.openFile != "" {
		help = insideHelpText
	}
	if m.focus == focusDiff {
		help = "diff focused · " + help
	}
	return styHelp.Render(truncateVisible(help, m.width))
}

// promptLine shows what has been typed and how much of the list survived it.
func (m *Model) promptLine() string {
	left := truncateVisible("/ "+m.filter+"█", m.width)
	right := truncateVisible(
		fmt.Sprintf("%d of %d", m.matchCount(), len(m.sessionFiles())),
		maxInt(0, m.width-VisibleWidth(left)-1))
	gap := maxInt(0, m.width-VisibleWidth(left)-VisibleWidth(right))

	return styIntent.Render(left) + strings.Repeat(" ", gap) + styDim.Render(right)
}

// matchCount is how many files the filter keeps, and totalFiles how many there
// are. The prompt counts what the list shows, which is files, not edits.
func (m *Model) matchCount() int {
	n := 0
	for _, rel := range m.sessionFiles() {
		if _, _, ok := Match(m.filter, rel); ok {
			n++
		}
	}
	return n
}

// sessionFiles is every path the session touched, in first-appearance order.
func (m *Model) sessionFiles() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range m.sess.Events {
		if !seen[e.Rel] {
			seen[e.Rel] = true
			out = append(out, e.Rel)
		}
	}
	return out
}

// rowUnread reports whether a row still has something to read. A file stands
// for the change it previews, which is the latest one made to it — so a new
// edit relights a file you had already read.
//
// A hunk is part of the edit listed above it, which carries the mark already;
// repeating it on every hunk would say the same thing several times over.
func (m *Model) rowUnread(r Row) bool {
	return r.Kind != RowHunk && r.Event != nil && !m.seen[r.Event.Seq]
}

// paneLines renders the list and diff side by side, or — in the diff panel —
// the diff alone across the whole popup.
func (m *Model) paneLines(height int) []string {
	lw := m.listWidth()
	dw := m.diffWidth()

	var list []string
	if !m.panel {
		m.scrollListIntoView(height)
		list = m.listLines(height, lw)
	}
	diff := m.renderDiff()
	focused := m.focus == focusDiff

	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		left := ""
		if i < len(list) {
			left = list[i]
		}

		right := ""
		if j := m.diffTop + i; j >= 0 && j < len(diff.Lines) {
			// Padding first: a wash has to reach the edge of the pane, not stop
			// where the code happens to end.
			right = padVisible(truncateStyled(diff.Lines[j], dw), dw)
			if bg := diffBackground(diff.Kinds[j], j == m.diffCursor, focused); bg != "" {
				right = withBackground(stripBackground(right), bg)
			}
		}

		if m.panel {
			out = append(out, right)
			continue
		}
		left = padVisible(left, lw)
		out = append(out, left+" "+styBorder.Render("│")+" "+right)
	}
	return out
}

// scrollListIntoView keeps the cursor on screen.
func (m *Model) scrollListIntoView(height int) {
	if m.cursor < m.listTop {
		m.listTop = m.cursor
	}
	if m.cursor >= m.listTop+height {
		m.listTop = m.cursor - height + 1
	}
	if m.listTop < 0 {
		m.listTop = 0
	}
}

func (m *Model) listLines(height, width int) []string {
	out := make([]string, 0, height)
	for i := m.listTop; i < len(m.rows) && len(out) < height; i++ {
		out = append(out, m.listRow(i, width))
	}
	return out
}

func (m *Model) listRow(i, width int) string {
	r := m.rows[i]

	if r.Kind == RowDir {
		// A heading owns the full width: the files under it carry the numbers.
		return styDim.Render(padVisible(ElidePath(r.Label, width), width))
	}

	selected := i == m.cursor
	unread := m.rowUnread(r)

	// Two marker columns: where the cursor is, and whether this is still unread.
	cursor, dot := " ", " "
	if selected {
		cursor = "❯"
	}
	if unread {
		dot = "●"
	}
	marker := cursor + dot + indent(r)

	stat := shortCounts(r.Added, r.Removed)
	room := width - VisibleWidth(marker) - VisibleWidth(stripANSI(stat)) - 1
	if room < 4 {
		room = 4
	}
	// A name is identified by its end, so a file gives way at the front and
	// keeps its extension; a hunk is identified by its line number and the
	// start of what changed, so it gives way at the end instead.
	label, cut := r.Label, 0
	if r.Kind == RowHunk {
		label = truncateVisible(label, room)
	} else {
		label, cut = elideLeft(label, room)
	}
	match := shiftMatches(r.Match, cut, len([]rune(label)))

	// Rows already read recede; unread ones keep the terminal's own foreground.
	base := styRow
	switch {
	case selected:
		base = stySelected
	case r.Kind == RowEdit:
		base = styHeading
	case !unread:
		base = styDim
	}

	pad := maxInt(0, width-VisibleWidth(marker)-VisibleWidth(label)-VisibleWidth(stripANSI(stat)))
	return base.Render(marker) + highlight(label, match, base) + strings.Repeat(" ", pad) + stat
}

// indent sets a row under what it belongs to: files under their directory,
// hunks under the edit that made them.
func indent(r Row) string {
	if r.Kind == RowHunk {
		return "    "
	}
	return "  "
}

// highlight underlines the characters the filter matched, on top of whatever
// style the row already wears, so the marks survive selection and dimming alike.
func highlight(label string, match []int, base lipgloss.Style) string {
	if len(match) == 0 {
		return base.Render(label)
	}
	hit := make(map[int]bool, len(match))
	for _, p := range match {
		hit[p] = true
	}

	marked := base.Underline(true).Bold(true)
	var b strings.Builder
	for i, r := range []rune(label) {
		if hit[i] {
			b.WriteString(marked.Render(string(r)))
			continue
		}
		b.WriteString(base.Render(string(r)))
	}
	return b.String()
}

// trimMatches drops positions past the end of a truncated label.
func trimMatches(match []int, n int) []int {
	out := match[:0:0]
	for _, p := range match {
		if p < n {
			out = append(out, p)
		}
	}
	return out
}

// elideLeft cuts a label down to width from its front, marking the cut with an
// ellipsis, and reports how many runes went.
func elideLeft(label string, width int) (string, int) {
	runes := []rune(label)
	if len(runes) <= width || width < 2 {
		return truncateVisible(label, width), 0
	}
	cut := len(runes) - (width - 1)
	return "…" + string(runes[cut:]), cut
}

// shiftMatches moves the filter's highlight positions along with an elided
// label, dropping the ones the cut swallowed.
func shiftMatches(match []int, cut, n int) []int {
	if cut == 0 {
		return trimMatches(match, n)
	}
	out := match[:0:0]
	for _, p := range match {
		if p -= cut - 1; p > 0 && p < n { // 0 is the ellipsis
			out = append(out, p)
		}
	}
	return out
}

func shortCounts(added, removed int) string {
	parts := []string{}
	if added > 0 {
		parts = append(parts, styAdd.Render(fmt.Sprintf("+%d", added)))
	}
	if removed > 0 {
		parts = append(parts, styDel.Render(fmt.Sprintf("-%d", removed)))
	}
	return strings.Join(parts, " ")
}

func (m *Model) waitingLines(height int) []string {
	out := make([]string, 0, height)
	out = append(out, "")
	out = append(out, styDim.Render(truncateVisible("  waiting for Claude to change something…", m.width)))
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

func (m *Model) helpLines(height int) []string {
	rows := [][2]string{
		{"j / k", "move down / up"},
		{"/", "filter by file name (⏎ keep, esc clear)"},
		{"ctrl-n / ctrl-p", "move while the filter prompt is open"},
		{"ctrl-d / ctrl-u", "half page down / up"},
		{"ctrl-e / ctrl-y", "scroll the diff a line, either pane focused"},
		{"ctrl-f / ctrl-b", "scroll the diff a page, either pane focused"},
		{"gg / G", "first / last"},
		{"n / N", "next / previous hunk"},
		{"J / K", "next / previous file, in the panel too"},
		{"l / space", "open the file: its edits and the hunks they made"},
		{"h / esc", "back out a level"},
		{"enter", "open the file, then its full-width diff panel"},
		{"f", "the diff panel from anywhere"},
		{"o", "open where you are looking, in nvim (starts one if none is open)"},
		{"dd", "clear the file, edit or hunk out of the view"},
		{"u / ctrl-r", "undo / redo one dd at a time"},
		{"+ / -", "more / less surrounding context"},
		{"tab", "move focus between list and diff (the diff gets a cursor)"},
		{"●", "an edit you have not looked at yet"},
		{"?", "close this help"},
		{"q", "quit and clear this session"},
	}

	out := make([]string, 0, height)
	out = append(out, "")
	for _, r := range rows {
		out = append(out, truncateVisible(fmt.Sprintf("  %-18s %s", r[0], r[1]), m.width))
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out[:height]
}

// padVisible right-pads a possibly styled string to an exact cell width.
func padVisible(s string, width int) string {
	w := VisibleWidth(s)
	if w > width {
		return truncateStyled(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

// truncateStyled cuts a styled string to width, dropping styling if it must cut.
func truncateStyled(s string, width int) string {
	if VisibleWidth(s) <= width {
		return s
	}
	return truncateVisible(stripANSI(s), width)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
