// Package ui is the panel: a list of what Claude changed on the left, the diff
// on the right, driven by vim motions.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mahmood/lens/internal/store"
)

// focus is which pane the motions act on.
type focus int

const (
	focusList focus = iota
	focusDiff
)

const (
	defaultCtx  = 0
	ctxStep     = 3
	listMinCols = 26
	listMaxCols = 44
)

// Model is the panel's state.
type Model struct {
	sessionID string
	store     *store.Store

	sess store.Session
	rows []Row
	view View

	cursor  int // index into rows
	listTop int // first visible list row
	diffTop int // first visible diff line
	ctx     int // how much surrounding context to show
	focus   focus

	width, height int
	pendingG      bool
	showHelp      bool
	diff          Rendered
	dirty         bool // the diff needs re-rendering
}

// New opens the panel on a live session.
func New(sessionID string) (*Model, error) {
	s, err := store.Open(sessionID)
	if err != nil {
		return nil, err
	}
	m := &Model{
		sessionID: sessionID,
		store:     s,
		ctx:       defaultCtx,
		width:     100,
		height:    30,
	}
	m.Reload()
	return m, nil
}

// NewForTest builds a panel over an in-memory session, touching no disk.
func NewForTest(sess store.Session) *Model {
	m := &Model{ctx: defaultCtx, width: 100, height: 30}
	m.setSession(sess)
	return m
}

func (m *Model) setSession(sess store.Session) {
	if sess.Prompts == nil {
		sess.Prompts = map[string]string{}
	}
	m.sess = sess
	m.rebuild()
}

// rebuild recomputes rows, holding the selection where it was.
//
// A file header and its first edit share a sequence number, so matching on the
// number alone would slide the cursor off a header and onto the edit below it
// on every reload. The row kind is part of the identity.
func (m *Model) rebuild() {
	wantKind, wantSeq, wantRel := RowEvent, m.SelectedSeq(), m.SelectedRel()
	if r := m.selected(); r != nil {
		wantKind = r.Kind
	}

	m.rows = BuildRows(m.sess, m.view)
	m.dirty = true

	if wantSeq == 0 && wantRel == "" {
		m.clampCursor()
		return
	}
	for i, r := range m.rows {
		if r.Kind != wantKind {
			continue
		}
		if wantKind == RowFileHeader {
			if r.Rel == wantRel {
				m.cursor = i
				m.clampCursor()
				return
			}
			continue
		}
		if r.Event != nil && r.Event.Seq == wantSeq {
			m.cursor = i
			m.clampCursor()
			return
		}
	}
	m.clampCursor()
}

// Reload re-reads the log. New events never move the selection.
func (m *Model) Reload() {
	if m.store == nil {
		return
	}
	sess, err := m.store.Read()
	if err != nil {
		return
	}
	m.sess = sess
	if m.sess.Prompts == nil {
		m.sess.Prompts = map[string]string{}
	}
	m.rebuild()
}

// Init satisfies tea.Model.
func (m *Model) Init() tea.Cmd { return tick() }

type reloadMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return reloadMsg(t) })
}

// Update satisfies tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case reloadMsg:
		m.Reload()
		return m, tick()
	case tea.WindowSizeMsg:
		m.Resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// Resize lays the panel out for a new terminal size.
func (m *Model) Resize(w, h int) {
	m.width, m.height = w, h
	m.dirty = true
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.dirty = true
}

// selected returns the row under the cursor, if any.
func (m *Model) selected() *Row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

// SelectedSeq is the sequence number of the highlighted edit, or 0.
func (m *Model) SelectedSeq() int {
	r := m.selected()
	if r == nil || r.Event == nil {
		return 0
	}
	return r.Event.Seq
}

// SelectedRel is the file path of the highlighted row.
func (m *Model) SelectedRel() string {
	r := m.selected()
	if r == nil {
		return ""
	}
	return r.Rel
}

// Cursor is the index of the highlighted row.
func (m *Model) Cursor() int { return m.cursor }

// Ctx is how many lines of surrounding context the diff shows.
func (m *Model) Ctx() int { return m.ctx }

// RowCount is how many rows the list holds.
func (m *Model) RowCount() int { return len(m.rows) }

// DiffTop is the first visible line of the diff pane.
func (m *Model) DiffTop() int { return m.diffTop }

// ListFocused reports whether motions act on the list.
func (m *Model) ListFocused() bool { return m.focus == focusList }

// Send feeds a key to the panel, for tests.
func (m *Model) Send(key string) *Model {
	m.handleKey(keyMsg(key))
	return m
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// listWidth is how wide the list pane is at the current terminal size.
func (m *Model) listWidth() int {
	w := m.width / 3
	if w < listMinCols {
		w = listMinCols
	}
	if w > listMaxCols {
		w = listMaxCols
	}
	if w > m.width-24 {
		w = m.width - 24
	}
	if w < 0 {
		w = 0
	}
	return w
}

func (m *Model) diffWidth() int {
	w := m.width - m.listWidth() - 3
	if w < 20 {
		w = 20
	}
	return w
}

// bodyHeight is the number of rows available to the panes.
func (m *Model) bodyHeight() int {
	h := m.height - 2 // title and help line
	if h < 1 {
		h = 1
	}
	return h
}

// render lays out the diff for the selected row, caching the result.
func (m *Model) renderDiff() Rendered {
	if !m.dirty {
		return m.diff
	}
	m.dirty = false

	r := m.selected()
	if r == nil || r.Event == nil {
		m.diff = Rendered{}
		return m.diff
	}
	prompt := m.sess.Prompts[r.Event.PromptID]
	m.diff = RenderDiffFull(*r.Event, prompt, m.ctx, m.diffWidth())
	return m.diff
}

func (m *Model) statusLine() string {
	name := "lens"
	if m.sess.Meta.CWD != "" {
		name = shortPath(m.sess.Meta.CWD)
	}
	state := ""
	if m.sess.Meta.Ended {
		state = styDim.Render("  (session ended)")
	}
	return styHeading.Render(name) + styDim.Render(fmt.Sprintf("  %d edits · %s", len(m.sess.Events), m.view)) + state
}

func shortPath(p string) string {
	parts := strings.Split(strings.TrimRight(p, "/"), "/")
	if len(parts) <= 2 {
		return p
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}

// OnFileHeader reports whether the highlighted row is a file heading.
func (m *Model) OnFileHeader() bool {
	r := m.selected()
	return r != nil && r.Kind == RowFileHeader
}
