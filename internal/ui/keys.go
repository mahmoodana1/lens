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

	// gg is the only chord, so a pending g is tracked directly.
	if m.pendingG {
		m.pendingG = false
		if key == "g" {
			m.toTop()
			return m, nil
		}
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
	case "g":
		m.pendingG = true
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
	case "n":
		m.jumpHunk(1)
	case "N":
		m.jumpHunk(-1)
	case "J":
		m.jumpFile(1)
	case "K":
		m.jumpFile(-1)
	case "t":
		m.toggleView()
	case "+", "=":
		m.setCtx(m.ctx + ctxStep)
	case "-", "_":
		m.setCtx(m.ctx - ctxStep)
	case "tab":
		if m.focus == focusList {
			m.focus = focusDiff
		} else {
			m.focus = focusList
		}
	case "/":
		m.filtering = true
	case "esc":
		m.showHelp = false
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
	m.rows = BuildRows(m.sess, m.view, m.filter)
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
	m.onSelectionChange()
	m.markSelectedRead()
}

func (m *Model) toBottom() {
	if m.focus == focusDiff {
		m.setDiffCursor(m.diffLen() - 1)
		return
	}
	m.cursor = len(m.rows) - 1
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

// moveCursor moves the list selection and reads whatever it lands on.
func (m *Model) moveCursor(n int) {
	m.cursor += n
	m.clampCursor()
	m.onSelectionChange()
	m.markSelectedRead()
}

// onSelectionChange resets the diff so a new edit starts from its first line.
func (m *Model) onSelectionChange() {
	m.diffTop, m.diffCursor = 0, 0
	m.dirty = true
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

// jumpFile moves the selection to the next or previous file in the list.
func (m *Model) jumpFile(dir int) {
	cur := m.SelectedRel()
	for i := m.cursor + dir; i >= 0 && i < len(m.rows); i += dir {
		if m.rows[i].Rel != cur {
			m.cursor = i
			m.onSelectionChange()
			m.markSelectedRead()
			return
		}
	}
}

func (m *Model) toggleView() {
	if m.view == Timeline {
		m.view = ByFile
	} else {
		m.view = Timeline
	}
	m.rebuild()
	m.onSelectionChange()
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
