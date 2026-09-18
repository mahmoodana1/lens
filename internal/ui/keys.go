package ui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mahmood/lens/internal/capture"
)

// handleKey implements the motions. The list has focus by default; Tab hands it
// to the diff, where j and k scroll instead of changing the selection.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	case "enter":
		m.openInEditor()
	case "esc":
		m.showHelp = false
	}
	return m, nil
}

func (m *Model) toTop() {
	if m.focus == focusDiff {
		m.diffTop = 0
		return
	}
	m.cursor = 0
	m.onSelectionChange()
}

func (m *Model) toBottom() {
	if m.focus == focusDiff {
		m.diffTop = m.maxDiffTop()
		return
	}
	m.cursor = len(m.rows) - 1
	m.clampCursor()
	m.onSelectionChange()
}

func (m *Model) moveDown(n int) {
	if m.focus == focusDiff {
		m.diffTop += n
		if max := m.maxDiffTop(); m.diffTop > max {
			m.diffTop = max
		}
		return
	}
	m.cursor += n
	m.clampCursor()
	m.onSelectionChange()
}

func (m *Model) moveUp(n int) {
	if m.focus == focusDiff {
		m.diffTop -= n
		if m.diffTop < 0 {
			m.diffTop = 0
		}
		return
	}
	m.cursor -= n
	m.clampCursor()
	m.onSelectionChange()
}

// onSelectionChange resets the diff scroll so a new edit starts from the top.
func (m *Model) onSelectionChange() {
	m.diffTop = 0
	m.dirty = true
}

func (m *Model) maxDiffTop() int {
	n := len(m.renderDiff().Lines) - m.bodyHeight()
	if n < 0 {
		return 0
	}
	return n
}

// jumpHunk scrolls the diff to the next or previous hunk.
func (m *Model) jumpHunk(dir int) {
	starts := m.renderDiff().HunkStarts
	if len(starts) == 0 {
		return
	}
	if dir > 0 {
		for _, s := range starts {
			if s > m.diffTop {
				m.diffTop = s
				return
			}
		}
		return
	}
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] < m.diffTop {
			m.diffTop = starts[i]
			return
		}
	}
	m.diffTop = 0
}

// jumpFile moves the selection to the next or previous file in the list.
func (m *Model) jumpFile(dir int) {
	cur := m.SelectedRel()
	for i := m.cursor + dir; i >= 0 && i < len(m.rows); i += dir {
		if m.rows[i].Rel != cur {
			m.cursor = i
			m.onSelectionChange()
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

// openInEditor opens the selected file at the hunk under the cursor.
func (m *Model) openInEditor() {
	r := m.selected()
	if r == nil || r.Event == nil || r.Event.Path == "" {
		return
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nvim"
	}
	line := 1
	if len(r.Event.Hunks) > 0 {
		line = r.Event.Hunks[0].NewStart
		if line < 1 {
			line = 1
		}
	}

	// Open in a new tmux window so the panel keeps its pane.
	if os.Getenv("TMUX") != "" {
		exec.Command("tmux", "new-window", "--",
			editor, "+"+itoa(line), r.Event.Path).Run()
		return
	}
	exec.Command(editor, "+"+itoa(line), r.Event.Path).Run()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b strings.Builder
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		b.WriteByte('-')
	}
	b.Write(digits)
	return b.String()
}
