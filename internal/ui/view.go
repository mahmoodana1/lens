package ui

import (
	"fmt"
	"strings"
)

const helpText = "j/k move · n/N hunk · J/K file · t toggle view · +/- context · tab focus · ⏎ open · q quit"

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

	help := helpText
	if m.focus == focusDiff {
		help = "diff focused · " + help
	}
	lines = append(lines, styHelp.Render(truncateVisible(help, m.width)))

	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

// paneLines renders the list and diff side by side.
func (m *Model) paneLines(height int) []string {
	lw := m.listWidth()
	dw := m.diffWidth()

	m.scrollListIntoView(height)
	list := m.listLines(height, lw)
	diff := m.renderDiff().Lines

	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		left := ""
		if i < len(list) {
			left = list[i]
		}
		right := ""
		if j := m.diffTop + i; j >= 0 && j < len(diff) {
			right = diff[j]
		}
		left = padVisible(left, lw)
		out = append(out, left+" "+styBorder.Render("│")+" "+truncateStyled(right, dw))
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
	selected := i == m.cursor

	marker := "  "
	if selected {
		marker = "▸ "
	}

	var label string
	switch r.Kind {
	case RowFileHeader:
		label = fmt.Sprintf("%s (%d)", r.Rel, r.Edits)
	default:
		if m.view == ByFile {
			label = "  " + r.Event.Time.Format("15:04:05")
		} else {
			label = r.Rel
		}
	}

	stat := shortCounts(r.Added, r.Removed)
	room := width - VisibleWidth(marker) - VisibleWidth(stripANSI(stat)) - 1
	if room < 4 {
		room = 4
	}
	label = truncateVisible(label, room)

	line := marker + label
	line += strings.Repeat(" ", maxInt(0, width-VisibleWidth(line)-VisibleWidth(stripANSI(stat))))
	line += stat

	if selected {
		return stySelected.Render(stripANSI(line))
	}
	if r.Kind == RowFileHeader {
		return styHeading.Render(stripANSI(marker + label)) + line[len(marker+label):]
	}
	return line
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
		{"ctrl-d / ctrl-u", "half page down / up"},
		{"gg / G", "first / last"},
		{"n / N", "next / previous hunk"},
		{"J / K", "next / previous file"},
		{"t", "toggle view (timeline ⇄ by file)"},
		{"+ / -", "more / less surrounding context"},
		{"tab", "move focus between list and diff"},
		{"enter", "open the file in your editor"},
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
