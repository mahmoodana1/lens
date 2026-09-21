package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mahmood/lens/internal/capture"
)

// handleKey implements the motions. The list has focus by default; Tab hands it
// to the diff, where j and k scroll instead of changing the selection.
//
// A held key does not arrive one press at a time: the terminal coalesces the
// repeats and bubbletea delivers them as a single message carrying every rune.
// Those are dispatched individually, so holding j scrolls instead of matching
// nothing and sitting still.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
		var cmd tea.Cmd
		for _, r := range msg.Runes {
			one := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt}
			if _, c := m.handleKey(one); c != nil {
				cmd = c
			}
		}
		return m, cmd
	}

	if m.filtering {
		return m.handleFilterKey(msg)
	}

	key := msg.String()

	// gg and dd are the only chords, so the waiting key is tracked directly.
	if pending := m.pending; pending != "" {
		m.pending = ""
		if key == pending {
			switch key {
			case "g":
				m.toTop()
			case "d":
				m.dismissSelected()
			}
			return m, nil
		}
	}

	switch key {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "o":
		// Reading a change and then going to find it by hand is the gap this
		// closes. The popup owns the keyboard, so it stands aside as it goes.
		if file, line := m.OpenAt(); file != "" && m.onOpen != nil {
			m.onOpen(file, line)
			m.quitting = true
			return m, tea.Quit
		}
	case "?":
		m.showHelp = !m.showHelp
	case "g", "d":
		m.pending = key
	case "G":
		m.toBottom()
	case "j", "down":
		m.moveDown(1)
	case "k", "up":
		m.moveUp(1)
	case "ctrl+d":
		m.moveDown(m.bodyHeight() / 2)
	case "ctrl+u":
		m.moveUp(m.bodyHeight() / 2)
	// The diff scrolls under these whichever pane has focus, so a long hunk can
	// be read without tabbing away from the list first.
	case "ctrl+e":
		m.scrollDiff(1)
	case "ctrl+y":
		m.scrollDiff(-1)
	case "ctrl+f":
		m.scrollDiff(m.bodyHeight())
	case "ctrl+b":
		m.scrollDiff(-m.bodyHeight())
	case "l", "right", " ":
		m.descend()
	case "h", "left":
		m.ascend()
	case "enter":
		// One key, one direction: into the file, then into its diff.
		if m.openFile == "" && m.OnFileRow() {
			m.descend()
			break
		}
		m.togglePanel()
	case "f":
		m.togglePanel()
	case "u":
		m.undoDismiss()
	case "ctrl+r":
		m.redoDismiss()
	case "n":
		m.jumpHunk(1)
	case "N":
		m.jumpHunk(-1)
	case "J":
		m.jumpFile(1)
	case "K":
		m.jumpFile(-1)
	case "+", "=":
		m.setCtx(m.ctx + ctxStep)
	case "-", "_":
		m.setCtx(m.ctx - ctxStep)
	case "tab":
		if m.panel {
			break // there is no list to hand the keyboard to
		}
		if m.focus == focusList {
			m.focus = focusDiff
		} else {
			m.focus = focusList
		}
	case "/":
		// Searching is over files, so it belongs at the top level with a list
		// to show the results in.
		m.closePanel()
		m.leaveFile()
		m.filtering = true
	case "esc":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.panel || m.openFile != "" {
			m.ascend()
			return m, nil
		}
		m.setFilter("")
	}
	return m, nil
}

// handleFilterKey drives the file-name prompt. Every printable key is a
// character while it is open — otherwise there would be no way to type a file
// called "j" — so the motions come back only on enter or esc.
func (m *Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.filtering = false
		m.setFilter("")
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyBackspace:
		if q := []rune(m.filter); len(q) > 0 {
			m.setFilter(string(q[:len(q)-1]))
		}
	// Picking a file without leaving the prompt, as fzf does.
	case tea.KeyCtrlN, tea.KeyCtrlJ, tea.KeyDown:
		m.moveCursor(1)
	case tea.KeyCtrlP, tea.KeyCtrlK, tea.KeyUp:
		m.moveCursor(-1)
	case tea.KeyRunes:
		m.setFilter(m.filter + string(msg.Runes))
	}
	return m, nil
}

// setFilter narrows the list and puts the cursor on the best match, so the
// selection follows what you are typing instead of holding a stale index.
func (m *Model) setFilter(q string) {
	if q == m.filter {
		return
	}
	m.filter = q
	m.rows = BuildRows(m.sess, m.openFile, m.filter, m.hidden)
	m.cursor = BestMatch(m.rows, m.filter)
	m.clampCursor()
	m.onSelectionChange()
	m.markSelectedRead()
}

func (m *Model) toTop() {
	if m.focus == focusDiff {
		m.setDiffCursor(0)
		return
	}
	m.cursor = 0
	m.settle(1)
	m.onSelectionChange()
	m.markSelectedRead()
}

func (m *Model) toBottom() {
	if m.focus == focusDiff {
		m.setDiffCursor(m.diffLen() - 1)
		return
	}
	m.cursor = len(m.rows) - 1
	m.settle(-1)
	m.clampCursor()
	m.onSelectionChange()
	m.markSelectedRead()
}

func (m *Model) moveDown(n int) {
	if m.focus == focusDiff {
		m.setDiffCursor(m.diffCursor + n)
		return
	}
	m.moveCursor(n)
}

func (m *Model) moveUp(n int) {
	if m.focus == focusDiff {
		m.setDiffCursor(m.diffCursor - n)
		return
	}
	m.moveCursor(-n)
}

// moveCursor moves the list selection and reads whatever it lands on. Directory
// headings are stepped over rather than landed on, so j and k move file to file.
func (m *Model) moveCursor(n int) {
	dir := 1
	if n < 0 {
		dir = -1
	}
	for i := 0; i < abs(n); i++ {
		next := m.nextSelectable(m.cursor, dir)
		if next == m.cursor {
			break
		}
		m.cursor = next
	}
	m.clampCursor()
	m.onSelectionChange()
	m.markSelectedRead()
}

// nextSelectable is the closest row the cursor may land on in a direction, or
// where it already is when there is none.
func (m *Model) nextSelectable(from, dir int) int {
	for i := from + dir; i >= 0 && i < len(m.rows); i += dir {
		if m.rows[i].Selectable() {
			return i
		}
	}
	return from
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// onSelectionChange resets the diff so a new edit starts from its first line —
// or, when the row picked out one hunk, from that hunk.
func (m *Model) onSelectionChange() {
	m.diffTop, m.diffCursor = 0, 0
	m.dirty = true

	r := m.selected()
	if r == nil || r.Kind != RowHunk {
		return
	}
	if start, ok := m.renderDiff().StartOf(r.Hunk); ok {
		m.setDiffCursor(start)
	}
}

// diffLen is how many lines the diff under the cursor has.
func (m *Model) diffLen() int { return len(m.renderDiff().Lines) }

// setDiffCursor puts the diff cursor on a line and brings it into view.
func (m *Model) setDiffCursor(line int) {
	last := m.diffLen() - 1
	if line > last {
		line = last
	}
	if line < 0 {
		line = 0
	}
	m.diffCursor = line
	m.scrollDiffIntoView()
}

// scrollDiffIntoView keeps the diff cursor on screen.
func (m *Model) scrollDiffIntoView() {
	h := m.bodyHeight()
	if m.diffCursor < m.diffTop {
		m.diffTop = m.diffCursor
	}
	if m.diffCursor >= m.diffTop+h {
		m.diffTop = m.diffCursor - h + 1
	}
	m.clampDiffTop()
}

// scrollDiff scrolls the diff without taking focus, dragging the cursor along
// only when it would otherwise be left off the screen.
func (m *Model) scrollDiff(n int) {
	m.diffTop += n
	m.clampDiffTop()

	if m.diffCursor < m.diffTop {
		m.diffCursor = m.diffTop
	}
	if h := m.bodyHeight(); m.diffCursor >= m.diffTop+h {
		m.diffCursor = m.diffTop + h - 1
	}
}

func (m *Model) clampDiffTop() {
	if max := m.maxDiffTop(); m.diffTop > max {
		m.diffTop = max
	}
	if m.diffTop < 0 {
		m.diffTop = 0
	}
}

func (m *Model) maxDiffTop() int {
	n := len(m.renderDiff().Lines) - m.bodyHeight()
	if n < 0 {
		return 0
	}
	return n
}

// jumpHunk moves the diff cursor to the next or previous hunk.
func (m *Model) jumpHunk(dir int) {
	starts := m.renderDiff().HunkStarts
	if len(starts) == 0 {
		return
	}
	if dir > 0 {
		for _, s := range starts {
			if s > m.diffCursor {
				m.setDiffCursor(s)
				return
			}
		}
		return
	}
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] < m.diffCursor {
			m.setDiffCursor(starts[i])
			return
		}
	}
	m.setDiffCursor(0)
}

// jumpFile moves to the next or previous file. In the file list that is the
// next file row; inside an opened file, or in the diff panel, it opens the next
// file in its place — so file after file can be read without backing out first.
func (m *Model) jumpFile(dir int) {
	if m.openFile == "" {
		cur := m.SelectedRel()
		for i := m.cursor + dir; i >= 0 && i < len(m.rows); i += dir {
			if m.rows[i].Kind == RowFile && m.rows[i].Rel != cur {
				m.cursor = i
				m.onSelectionChange()
				m.markSelectedRead()
				return
			}
		}
		return
	}

	files := m.files()
	for i, rel := range files {
		if rel != m.openFile {
			continue
		}
		if j := i + dir; j >= 0 && j < len(files) {
			m.enterFile(files[j])
		}
		return
	}
}

// descend goes one level in: from the file list into the file under the cursor,
// and from inside a file into the full-width diff.
func (m *Model) descend() {
	if m.panel {
		return
	}
	if m.openFile == "" {
		if r := m.selected(); r != nil && r.Kind == RowFile {
			m.enterFile(r.Rel)
		}
		return
	}
	m.togglePanel()
}

// ascend goes one level back out: the diff panel to the list, the opened file
// to the file list.
func (m *Model) ascend() {
	if m.panel {
		m.closePanel()
		return
	}
	m.leaveFile()
}

// enterFile replaces the list with one file's own history.
func (m *Model) enterFile(rel string) {
	if rel == "" {
		return
	}
	m.openFile = rel
	m.cursor, m.listTop = 0, 0
	m.rebuild()
	m.settle(1)
	m.onSelectionChange()
	m.markSelectedRead()
}

// leaveFile returns to the file list, on the file that was open.
func (m *Model) leaveFile() {
	if m.openFile == "" {
		return
	}
	was := m.openFile
	m.openFile = ""
	m.cursor, m.listTop = 0, 0
	m.rows = BuildRows(m.sess, "", m.filter, m.hidden)
	for i, r := range m.rows {
		if r.Kind == RowFile && r.Rel == was {
			m.cursor = i
			break
		}
	}
	m.settle(1)
	m.dirty = true
	m.onSelectionChange()
	m.markSelectedRead()
}

// files is every path the list holds, in the order it shows them.
func (m *Model) files() []string {
	rows := m.rows
	if m.openFile != "" {
		rows = BuildRows(m.sess, "", m.filter, m.hidden)
	}
	out := []string{}
	for _, r := range rows {
		if r.Kind == RowFile {
			out = append(out, r.Rel)
		}
	}
	return out
}

// togglePanel swaps between the split and the full-width diff. The panel has no
// list, so it always drives the diff.
func (m *Model) togglePanel() {
	if m.panel {
		m.closePanel()
		return
	}
	m.panel = true
	m.focus = focusDiff
	m.dirty = true
	m.scrollDiffIntoView()
}

func (m *Model) closePanel() {
	m.panel = false
	m.focus = focusList
	m.dirty = true
}

func (m *Model) setCtx(n int) {
	if n < 0 {
		n = 0
	}
	if n > capture.MaxContext {
		n = capture.MaxContext
	}
	m.ctx = n
	m.dirty = true
}
