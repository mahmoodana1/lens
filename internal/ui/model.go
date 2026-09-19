// Package ui is the panel: a list of what Claude changed on the left, the diff
// on the right, driven by vim motions.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mahmood/lens/internal/capture"
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

	sess   store.Session
	rows   []Row
	filter string

	openFile string // the file the list has drilled into, empty at the top level
	panel    bool   // the diff has the whole popup, list hidden

	hidden      Hidden      // rows cleared out of the view with dd
	undone      []dismissal // the trail u walks back along
	redone      []dismissal // and the one ctrl-r walks forward again
	loadedTrail bool        // the session's trail has been taken on

	seen map[int]bool // sequence numbers of edits already read

	cursor     int // index into rows
	listTop    int // first visible list row
	diffTop    int // first visible diff line
	diffCursor int // the line the diff cursor sits on
	ctx        int // how much surrounding context to show
	focus      focus

	width, height int
	pending       string // the first key of a chord, waiting for its second
	filtering     bool   // the filter prompt has the keyboard
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
		filtering: true, // the panel opens ready to search
	}
	m.Reload()
	return m, nil
}

// NewForTest builds a panel over an in-memory session, touching no disk.
func NewForTest(sess store.Session) *Model {
	m := &Model{ctx: defaultCtx, width: 100, height: 30, filtering: true}
	m.setSession(sess)
	return m
}

func (m *Model) setSession(sess store.Session) {
	if sess.Prompts == nil {
		sess.Prompts = map[string]string{}
	}
	normalise(&sess)
	m.sess = sess
	m.adoptSeen(sess.Seen)
	m.adoptDismissals(sess.Dismissed)
	m.rebuild()
}

// normalise settles what a session's events are called and drops the ones the
// panel has no business showing, before anything is laid out over them.
func normalise(sess *store.Session) {
	canonicalRels(sess)
	dropHidden(sess)
	dropStrayed(sess)
}

// dropStrayed removes what the file walk swept up outside the project.
//
// The walk is confined to the project now, but a log written before that still
// holds another program's scratch files — Claude Code's own temp repositories,
// a plugin's preload files. An edit made deliberately with the Edit or Write
// tool is a different thing: outside the project or not, someone meant it, so
// it stays.
func dropStrayed(sess *store.Session) {
	root := sess.Meta.CWD
	if root == "" {
		return
	}
	kept := sess.Events[:0]
	for _, e := range sess.Events {
		if e.Tool == "Bash" && e.Path != "" && !capture.Inside(root, e.Path) {
			continue
		}
		kept = append(kept, e)
	}
	sess.Events = kept
}

// dropHidden removes edits to hidden files. They are turned away when captured
// now, but a log written before that still holds them, and it is the panel that
// has to stop showing them.
func dropHidden(sess *store.Session) {
	kept := sess.Events[:0]
	for _, e := range sess.Events {
		if e.Path != "" && capture.HiddenPath(sess.Meta.CWD, e.Path) {
			continue
		}
		kept = append(kept, e)
	}
	sess.Events = kept
}

// canonicalRels gives every edit the one name its file has in this session.
//
// An event's recorded name was measured against the agent's working directory
// at the time, and that directory moves: an agent that changes into a
// subdirectory starts calling the same file something shorter, and the panel
// would list it twice. The absolute path and the session's root do not move, so
// where both are known the name is taken from them instead.
//
// Logs written before this was fixed are repaired the same way, since it is the
// stored path that is trustworthy, not the stored name.
func canonicalRels(sess *store.Session) {
	root := sess.Meta.CWD
	if root == "" {
		return
	}
	for i := range sess.Events {
		if e := &sess.Events[i]; e.Path != "" {
			e.Rel = capture.RelativeTo(root, e.Path)
		}
	}
}

// adoptSeen folds what the session already knows was read into this panel's
// view of it. A reopened popup must not present everything as new again.
func (m *Model) adoptSeen(seen map[int]bool) {
	if m.seen == nil {
		m.seen = map[int]bool{}
	}
	for seq := range seen {
		m.seen[seq] = true
	}
}

// markSelectedRead records the edit under the cursor as read. The panel is for
// reading, so landing on a row is what reads what it shows — for a file, that
// is the latest edit to it, which is what the diff pane previews.
func (m *Model) markSelectedRead() {
	r := m.selected()
	if r == nil || r.Event == nil {
		return
	}
	seq := r.Event.Seq
	if seq <= 0 || m.seen[seq] {
		return
	}
	if m.seen == nil {
		m.seen = map[int]bool{}
	}
	m.seen[seq] = true
	if m.store != nil {
		if err := m.store.MarkSeen(seq); err != nil {
			return // read state is a convenience; losing a mark is not fatal
		}
	}
}

// IsRead reports whether an edit has already been read.
func (m *Model) IsRead(seq int) bool { return m.seen[seq] }

// Unread is how many rows still have something to read. It counts what the list
// shows — a folded file is one row however many edits it holds.
func (m *Model) Unread() int {
	n := 0
	for _, r := range m.rows {
		if r.Event != nil && !m.seen[r.Event.Seq] {
			n++
		}
	}
	return n
}

// Filter is the current file-name query, empty when the whole list is shown.
func (m *Model) Filter() string { return m.filter }

// Filtering reports whether the filter prompt holds the keyboard.
func (m *Model) Filtering() bool { return m.filtering }

// rebuild recomputes rows, holding the selection where it was.
//
// A file row, an edit under it and that edit's hunks all share a sequence
// number, so matching on the number alone would slide the cursor between them
// on every reload. The kind and the hunk are part of the identity.
func (m *Model) rebuild() {
	want := m.selectedID()

	m.rows = BuildRows(m.sess, m.openFile, m.filter, m.hidden)
	m.dirty = true

	if want.kind == RowDir { // nothing was selected
		m.clampCursor()
		m.markSelectedRead()
		return
	}
	for i, r := range m.rows {
		if m.rowID(r) == want {
			m.cursor = i
			m.clampCursor()
			m.markSelectedRead()
			return
		}
	}
	m.clampCursor()
	m.markSelectedRead()
}

// rowID is what identifies a row across a rebuild.
type rowID struct {
	kind RowKind
	rel  string
	seq  int
	hunk int
}

func (m *Model) rowID(r Row) rowID {
	id := rowID{kind: r.Kind, rel: r.Rel, hunk: r.Hunk}
	if r.Event != nil {
		id.seq = r.Event.Seq
	}
	return id
}

func (m *Model) selectedID() rowID {
	r := m.selected()
	if r == nil {
		return rowID{kind: RowDir}
	}
	return m.rowID(*r)
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
	normalise(&sess)
	m.sess = sess
	if m.sess.Prompts == nil {
		m.sess.Prompts = map[string]string{}
	}
	m.adoptSeen(sess.Seen)
	m.adoptDismissals(sess.Dismissed)
	m.rebuild()
}

// OpenFile is the file the list has drilled into, empty at the top level.
func (m *Model) OpenFile() string { return m.openFile }

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

// clampCursor keeps the cursor inside the list and off the directory headings,
// which are signposts rather than things to select.
func (m *Model) clampCursor() {
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.settle(1)
	m.dirty = true
}

// settle slides the cursor off an unselectable row, preferring dir and falling
// back the other way at the end of the list.
func (m *Model) settle(dir int) {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	for _, d := range []int{dir, -dir} {
		for i := m.cursor; i >= 0 && i < len(m.rows); i += d {
			if m.rows[i].Selectable() {
				m.cursor = i
				return
			}
		}
	}
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

// DiffCursor is the diff line the cursor sits on.
func (m *Model) DiffCursor() int { return m.diffCursor }

// BodyHeight is how many rows the panes have to work with.
func (m *Model) BodyHeight() int { return m.bodyHeight() }

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
	case "ctrl+r":
		return tea.KeyMsg{Type: tea.KeyCtrlR}
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+j":
		return tea.KeyMsg{Type: tea.KeyCtrlJ}
	case "ctrl+k":
		return tea.KeyMsg{Type: tea.KeyCtrlK}
	case "ctrl+e":
		return tea.KeyMsg{Type: tea.KeyCtrlE}
	case "ctrl+y":
		return tea.KeyMsg{Type: tea.KeyCtrlY}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	case "ctrl+b":
		return tea.KeyMsg{Type: tea.KeyCtrlB}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// listWidth is how wide the list pane is at the current terminal size. The
// diff panel hides the list, so there it is nothing at all.
func (m *Model) listWidth() int {
	if m.panel {
		return 0
	}
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
	if m.panel {
		return maxInt(20, m.width)
	}
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
	m.diff = RenderDiffFull(*r.Event, prompt, m.ctx, m.diffWidth(), m.hidden)
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
	if m.panel {
		return m.panelStatusLine() + state
	}

	if m.openFile != "" {
		return m.fileStatusLine() + state
	}

	// Both counts are of what the list is showing: a header that keeps counting
	// dismissed rows disagrees with the list under it.
	counts := fmt.Sprintf("  %s · %s", plural(m.visibleEdits(), "edit"), plural(m.fileCount(), "file"))
	if n := m.Unread(); n > 0 {
		counts += fmt.Sprintf(" · %d unread", n)
	}
	// Rows that vanish without a word are unnerving, and u is how they come back.
	if n := m.hidden.Len(); n > 0 {
		counts += fmt.Sprintf(" · %d hidden", n)
	}
	return styHeading.Render(name) + styDim.Render(counts) + state
}

func shortPath(p string) string {
	parts := strings.Split(strings.TrimRight(p, "/"), "/")
	if len(parts) <= 2 {
		return p
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}

// fileStatusLine heads the opened file's own list: which file it is, how much
// happened to it, and a mark saying there is a level to go back to.
func (m *Model) fileStatusLine() string {
	added, removed, edits := m.fileTotals(m.openFile)
	head := styHeading.Render("◀ " + m.openFile)
	return head + styDim.Render(fmt.Sprintf("  %s  ", plural(edits, "edit"))) + counts(added, removed)
}

// panelStatusLine names the file on show and where it sits among the others,
// which is the only orientation left once the list is hidden.
func (m *Model) panelStatusLine() string {
	r := m.selected()
	if r == nil || r.Event == nil {
		return styHeading.Render("lens")
	}
	// The file's totals, where the diff's own header below gives this edit's:
	// two different numbers are worth two lines, the same number is not.
	added, removed, _ := m.fileTotals(r.Rel)
	head := styHeading.Render(r.Rel)
	if m.openFile == "" {
		at, total := m.filePosition()
		head += styDim.Render(fmt.Sprintf("  (%d/%d)", at, total))
	}
	return head + styDim.Render("  ") + counts(added, removed)
}

// fileTotals adds up what the session did to one file, counting only what is
// still in the view: a heading that keeps counting dismissed edits is a heading
// that disagrees with the list under it.
func (m *Model) fileTotals(rel string) (added, removed, edits int) {
	for i := range m.sess.Events {
		e := &m.sess.Events[i]
		if e.Rel != rel || m.hidden.Empty(e) {
			continue
		}
		added += e.Added
		removed += e.Removed
		edits++
	}
	return added, removed, edits
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// filePosition is which file the cursor is in, counting from one.
func (m *Model) filePosition() (at, total int) {
	for i, r := range m.rows {
		if r.Kind != RowFile {
			continue
		}
		total++
		if i <= m.cursor {
			at = total
		}
	}
	return at, total
}

// visibleEdits is how many edits are still in the view.
func (m *Model) visibleEdits() int {
	n := 0
	for i := range m.sess.Events {
		if !m.hidden.Empty(&m.sess.Events[i]) {
			n++
		}
	}
	return n
}

// fileCount is how many files the list shows.
func (m *Model) fileCount() int {
	n := 0
	for _, r := range m.rows {
		if r.Kind == RowFile {
			n++
		}
	}
	return n
}

// OnFileRow reports whether the highlighted row is a file in the file list.
func (m *Model) OnFileRow() bool {
	r := m.selected()
	return r != nil && r.Kind == RowFile
}

// SelectedKind is what the highlighted row stands for.
func (m *Model) SelectedKind() RowKind {
	r := m.selected()
	if r == nil {
		return RowDir
	}
	return r.Kind
}

// Panel reports whether the diff has the whole popup to itself.
func (m *Model) Panel() bool { return m.panel }

// HiddenCount is how many things have been cleared out of the view.
func (m *Model) HiddenCount() int { return m.hidden.Len() }

// RowAt is the row at an index, for tests.
func (m *Model) RowAt(i int) Row {
	if i < 0 || i >= len(m.rows) {
		return Row{}
	}
	return m.rows[i]
}
