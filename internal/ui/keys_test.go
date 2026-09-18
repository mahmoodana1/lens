package ui_test

import (
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
	"github.com/mahmood/lens/internal/ui"
)

func TestKeys_JKMoveSelectionAndClampAtEnds(t *testing.T) {
	m := browsing(threeEvents())

	if got := m.Send("j").Cursor(); got != 1 {
		t.Errorf("after j, cursor = %d, want 1", got)
	}
	if got := m.Send("k").Send("k").Cursor(); got != 0 {
		t.Errorf("k past the top should clamp at 0, got %d", got)
	}
	if got := m.Send("G").Cursor(); got != 2 {
		t.Errorf("G should land on the last row, got %d", got)
	}
	if got := m.Send("j").Cursor(); got != 2 {
		t.Errorf("j past the bottom should clamp, got %d", got)
	}
	if got := m.Send("g").Send("g").Cursor(); got != 0 {
		t.Errorf("gg should land on the first row, got %d", got)
	}
}

// A lone g is a pending chord, not a jump.
func TestKeys_SingleGDoesNotJump(t *testing.T) {
	m := browsing(threeEvents()).Send("G")
	if got := m.Send("g").Cursor(); got != 2 {
		t.Errorf("single g moved the cursor to %d; it should wait for the second g", got)
	}
}

func TestKeys_ToggleSwitchesView(t *testing.T) {
	m := browsing(threeEvents())

	before := m.RowCount()
	m.Send("t")
	if m.RowCount() == before {
		t.Error("t did not change the row layout")
	}
	m.Send("t")
	if m.RowCount() != before {
		t.Error("toggling twice did not return to the original view")
	}
}

// Toggling views must keep you on the same edit, not reset to the top.
func TestKeys_ToggleKeepsSelectedEvent(t *testing.T) {
	m := browsing(threeEvents())
	m.Send("G") // the third event, a.go seq 3

	want := m.SelectedSeq()
	m.Send("t")
	if got := m.SelectedSeq(); got != want {
		t.Errorf("after toggling, selected seq = %d, want %d", got, want)
	}
}

func TestKeys_ContextWidthNeverNegative(t *testing.T) {
	m := browsing(threeEvents())

	if got := m.Send("-").Send("-").Send("-").Ctx(); got != 0 {
		t.Errorf("ctx = %d, want 0", got)
	}
	if got := m.Send("+").Ctx(); got <= 0 {
		t.Errorf("ctx = %d, want a positive width after +", got)
	}
}

func TestKeys_ContextClampsAtCapturedMaximum(t *testing.T) {
	m := browsing(threeEvents())
	for i := 0; i < 40; i++ {
		m.Send("+")
	}
	if got := m.Ctx(); got > capture.MaxContext {
		t.Errorf("ctx = %d, want no more than the %d lines captured", got, capture.MaxContext)
	}
}

// J and K move between files, skipping the other edits in between.
func TestKeys_ShiftJKMovesBetweenFiles(t *testing.T) {
	m := browsing(threeEvents()) // a.go, b.go, a.go

	m.Send("J")
	if got := m.SelectedRel(); got != "b.go" {
		t.Errorf("after J, on %q, want b.go", got)
	}
	m.Send("J")
	if got := m.SelectedRel(); got != "a.go" {
		t.Errorf("after a second J, on %q, want a.go", got)
	}
	m.Send("K")
	if got := m.SelectedRel(); got != "b.go" {
		t.Errorf("after K, on %q, want b.go", got)
	}
}

func TestKeys_TabSwitchesFocus(t *testing.T) {
	m := browsing(threeEvents())
	if !m.ListFocused() {
		t.Fatal("the list should have focus initially")
	}
	if m.Send("tab").ListFocused() {
		t.Error("tab did not move focus to the diff pane")
	}
	if !m.Send("tab").ListFocused() {
		t.Error("tab did not move focus back to the list")
	}
}

// With the diff focused, j/k scroll the diff instead of changing the selection.
func TestKeys_JMovesTheDiffCursorWhenFocused(t *testing.T) {
	m := browsing(manyHunks())
	m.Resize(80, 12) // short enough that the diff overflows and can scroll
	m.Send("tab")

	before := m.Cursor()
	m.Send("j")
	if m.Cursor() != before {
		t.Error("j changed the list selection while the diff was focused")
	}
	if got := m.DiffCursor(); got != 1 {
		t.Errorf("diff cursor = %d, want 1", got)
	}

	// Pushing past the bottom of the window scrolls the view after it.
	for range 20 {
		m.Send("j")
	}
	if m.DiffTop() == 0 {
		t.Error("the view never followed the cursor down")
	}
}

func TestKeys_NextHunkJumps(t *testing.T) {
	m := browsing(manyHunks())

	m.Send("n")
	first := m.DiffCursor()
	if first == 0 {
		t.Fatal("n did not move to the next hunk")
	}
	m.Send("n")
	if m.DiffCursor() <= first {
		t.Error("a second n did not advance further")
	}
	m.Send("N")
	if got := m.DiffCursor(); got != first {
		t.Errorf("N should return to the previous hunk at %d, got %d", first, got)
	}
}

func TestKeys_HelpToggles(t *testing.T) {
	m := browsing(threeEvents())
	// A phrase unique to the overlay: the footer also mentions "toggle view".
	const overlayOnly = "quit and clear this session"

	if strings.Contains(m.View(), overlayOnly) {
		t.Error("help shown before it was asked for")
	}
	if !strings.Contains(m.Send("?").View(), overlayOnly) {
		t.Error("? did not show help")
	}
	if strings.Contains(m.Send("?").View(), overlayOnly) {
		t.Error("? did not hide help again")
	}
}

func TestView_EmptySessionExplainsItself(t *testing.T) {
	m := browsing(store.Session{})
	out := m.View()
	if !strings.Contains(strings.ToLower(out), "waiting") {
		t.Errorf("an empty panel should say it is waiting for edits, got:\n%s", out)
	}
}

func TestView_RendersWithinTerminalBounds(t *testing.T) {
	m := browsing(threeEvents())
	m.Resize(80, 24)

	lines := strings.Split(m.View(), "\n")
	if len(lines) > 24 {
		t.Errorf("view is %d lines tall, want at most 24", len(lines))
	}
	for i, ln := range lines {
		if got := ui.VisibleWidth(ln); got > 80 {
			t.Errorf("line %d is %d cells wide, want at most 80", i, got)
		}
	}
}

func TestTail_PicksUpAppendedEvents(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("live")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go", Added: 1})

	m, err := ui.New("live")
	if err != nil {
		t.Fatal(err)
	}
	if got := m.RowCount(); got != 1 {
		t.Fatalf("rows = %d, want 1", got)
	}

	s.Append(capture.Event{Rel: "b.go", Added: 2})
	m.Reload()

	if got := m.RowCount(); got != 2 {
		t.Errorf("rows after reload = %d, want 2", got)
	}
}

// New events must not yank the selection out from under the reader.
func TestTail_KeepsSelectionWhileStreaming(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("live2")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go"})
	s.Append(capture.Event{Rel: "b.go"})

	m, _ := ui.New("live2")
	m.Send("j") // sitting on b.go, seq 2
	want := m.SelectedSeq()

	s.Append(capture.Event{Rel: "c.go"})
	m.Reload()

	if got := m.SelectedSeq(); got != want {
		t.Errorf("selection jumped to seq %d, want %d", got, want)
	}
}

func manyHunks() store.Session {
	hunk := func(start int) capture.Hunk {
		return capture.Hunk{
			OldStart: start, OldLines: 2, NewStart: start, NewLines: 2,
			Lines: []string{" keep", "-old", "+new"},
			Above: []string{"x", "y"}, Below: []string{"z"},
		}
	}
	return store.Session{
		Events: []capture.Event{{
			Seq: 1, Rel: "main.go", Added: 3, Removed: 3,
			Hunks: []capture.Hunk{hunk(10), hunk(40), hunk(80)},
		}},
		Prompts: map[string]string{},
	}
}

// Sitting on a file header, a reload must not slide the cursor onto the edit
// below it: a header and its first edit share a sequence number.
func TestReload_KeepsCursorOnFileHeader(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("hdr")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go", Added: 1})
	s.Append(capture.Event{Rel: "b.go", Added: 1})

	m, _ := ui.New("hdr")
	m.Send("esc") // dismiss the opening search prompt
	m.Send("t")   // by file
	m.Send("g").Send("g")
	if !m.OnFileHeader() {
		t.Fatalf("expected to start on a file header, cursor=%d", m.Cursor())
	}

	want := m.Cursor()
	for i := 0; i < 3; i++ {
		m.Reload()
	}
	if got := m.Cursor(); got != want {
		t.Errorf("cursor drifted from %d to %d across reloads", want, got)
	}
	if !m.OnFileHeader() {
		t.Error("cursor slid off the file header")
	}
}

// J in by-file view lands on the next file's header, not its first edit.
func TestKeys_JumpFileLandsOnHeaderInByFileView(t *testing.T) {
	m := browsing(threeEvents())
	m.Send("t").Send("g").Send("g")

	m.Send("J")
	if !m.OnFileHeader() {
		t.Errorf("J landed on row %d, which is not a file header", m.Cursor())
	}
	if got := m.SelectedRel(); got != "b.go" {
		t.Errorf("J landed on %q, want b.go", got)
	}
}

// Holding j down does not produce one message per press: the terminal delivers
// the repeats together, and bubbletea hands them over as a single key message
// carrying every rune. Each one still has to move.
func TestKeys_HeldKeyRepeatMovesOncePerRune(t *testing.T) {
	m := browsing(threeEvents())

	if got := m.Send("jj").Cursor(); got != 2 {
		t.Errorf("after a held j arriving as one message, cursor = %d, want 2", got)
	}
	if got := m.Send("kk").Cursor(); got != 0 {
		t.Errorf("after a held k, cursor = %d, want 0", got)
	}
}

// The chord still works when both g's arrive in the same message.
func TestKeys_GGInOneMessageJumpsToTop(t *testing.T) {
	m := browsing(threeEvents()).Send("G")

	if got := m.Send("gg").Cursor(); got != 0 {
		t.Errorf("gg in one message left the cursor at %d, want 0", got)
	}
}

func filterFixture() store.Session {
	return store.Session{
		Events: []capture.Event{
			{Seq: 1, Rel: "internal/ui/keys.go", Added: 3, Tool: "Edit"},
			{Seq: 2, Rel: "internal/store/store.go", Added: 1, Tool: "Edit"},
			{Seq: 3, Rel: "internal/ui/view.go", Added: 2, Tool: "Edit"},
		},
		Prompts: map[string]string{},
	}
}

func TestFilter_SlashNarrowsTheListAsYouType(t *testing.T) {
	m := browsing(filterFixture()).Send("/").Send("store")

	if got := m.Filter(); got != "store" {
		t.Errorf("filter = %q, want %q", got, "store")
	}
	if got := m.RowCount(); got != 1 {
		t.Errorf("rows = %d, want 1", got)
	}
}

// While the prompt is open every key is a character, or nothing could be typed.
func TestFilter_TypingAMotionKeyIsJustACharacter(t *testing.T) {
	m := browsing(filterFixture()).Send("/").Send("j")

	if got := m.Cursor(); got != 0 {
		t.Errorf("j while typing moved the cursor to %d", got)
	}
	if got := m.Filter(); got != "j" {
		t.Errorf("filter = %q, want %q", got, "j")
	}
}

func TestFilter_BackspaceWidensItAgain(t *testing.T) {
	m := browsing(filterFixture()).Send("/").Send("store").Send("backspace")

	if got := m.Filter(); got != "stor" {
		t.Errorf("filter = %q, want %q", got, "stor")
	}
}

func TestFilter_EscapeClearsIt(t *testing.T) {
	m := browsing(filterFixture()).Send("/").Send("store").Send("esc")

	if got := m.Filter(); got != "" {
		t.Errorf("filter = %q, want it cleared", got)
	}
	if got := m.RowCount(); got != 3 {
		t.Errorf("rows = %d, want the whole list back", got)
	}
}

// Enter keeps what you typed and hands the keyboard back to the motions.
func TestFilter_EnterKeepsTheFilterAndRestoresMotions(t *testing.T) {
	m := browsing(filterFixture()).Send("/").Send("ui").Send("enter")

	if m.Filtering() {
		t.Error("enter should close the prompt")
	}
	if got := m.Filter(); got != "ui" {
		t.Errorf("filter = %q, want it kept", got)
	}
	if got := m.Send("j").Cursor(); got != 1 {
		t.Errorf("after enter, j should move again; cursor = %d", got)
	}
}

// fzf-style: pick a file without leaving the prompt.
func TestFilter_CtrlNAndCtrlPMoveWhileTyping(t *testing.T) {
	m := browsing(filterFixture()).Send("/").Send("go")

	if got := m.Send("ctrl+n").Cursor(); got != 1 {
		t.Errorf("ctrl+n cursor = %d, want 1", got)
	}
	if got := m.Send("ctrl+p").Cursor(); got != 0 {
		t.Errorf("ctrl+p cursor = %d, want 0", got)
	}
	if !m.Filtering() {
		t.Error("moving should not close the prompt")
	}
}

// Typing re-ranks the list, so the cursor follows it rather than sitting on
// whatever index it happened to hold.
func TestFilter_CursorFollowsTheBestMatch(t *testing.T) {
	m := browsing(filterFixture()).Send("j").Send("j")
	if got := m.Cursor(); got != 2 {
		t.Fatalf("setup: cursor = %d, want 2", got)
	}

	m.Send("/").Send("go")

	if got := m.Cursor(); got != 0 {
		t.Errorf("cursor = %d, want it back on the best match", got)
	}
}

// The panel is for reading, so looking at an edit is reading it.
func TestRead_SelectingAnEditMarksItRead(t *testing.T) {
	m := browsing(threeEvents())

	if !m.IsRead(1) {
		t.Error("the edit the panel opens on is being read")
	}
	if m.IsRead(2) {
		t.Error("edit 2 has not been selected yet")
	}

	m.Send("j")

	if !m.IsRead(2) {
		t.Error("landing on edit 2 should mark it read")
	}
}

func TestRead_UnreadCountFallsAsYouGo(t *testing.T) {
	m := browsing(threeEvents())

	before := m.Unread()
	m.Send("j")

	if got := m.Unread(); got != before-1 {
		t.Errorf("unread = %d, want %d", got, before-1)
	}
}

// Read state comes from the session, so a reopened popup does not start over.
func TestRead_HonoursWhatWasAlreadyRead(t *testing.T) {
	sess := threeEvents()
	sess.Seen = map[int]bool{2: true}

	m := browsing(sess)

	if !m.IsRead(2) {
		t.Error("edit 2 was read before the popup was reopened")
	}
	if got := m.Unread(); got != 1 {
		t.Errorf("unread = %d, want 1 (edit 3; edit 1 is selected now)", got)
	}
}

// Unread edits have to stand out, or the panel cannot tell you what is new.
func TestView_MarksUnreadEdits(t *testing.T) {
	m := browsing(threeEvents()) // opens on edit 1, which reads it
	m.Resize(90, 24)

	out := ui.StripANSIForTest(m.View())

	if got := strings.Count(out, "●"); got != 2 {
		t.Errorf("unread dots = %d, want 2 (edits 2 and 3)", got)
	}
}

func TestView_DotsDisappearAsEditsAreRead(t *testing.T) {
	m := browsing(threeEvents())
	m.Resize(90, 24)

	m.Send("j").Send("j")
	out := ui.StripANSIForTest(m.View())

	if got := strings.Count(out, "●"); got != 0 {
		t.Errorf("unread dots = %d after reading everything, want 0", got)
	}
}

func TestView_StatusLineCountsUnread(t *testing.T) {
	m := browsing(threeEvents())
	m.Resize(90, 24)

	if out := ui.StripANSIForTest(m.View()); !strings.Contains(out, "2 unread") {
		t.Errorf("status line does not report the unread count:\n%s", strings.SplitN(out, "\n", 2)[0])
	}
}

// The prompt replaces the help line while you are typing, and says how much of
// the list survived.
func TestView_ShowsTheFilterPrompt(t *testing.T) {
	m := browsing(filterFixture())
	m.Resize(90, 24)
	m.Send("/").Send("ui")

	lines := strings.Split(ui.StripANSIForTest(m.View()), "\n")
	prompt := lines[len(lines)-1]

	if !strings.Contains(prompt, "ui") {
		t.Errorf("prompt = %q, want it to show the query", prompt)
	}
	if !strings.Contains(prompt, "2 of 3") {
		t.Errorf("prompt = %q, want it to show 2 of 3", prompt)
	}
}

// Highlighting matched characters must not make rows wider than the pane.
func TestView_FilteredRowsStayWithinBounds(t *testing.T) {
	m := browsing(filterFixture())
	m.Resize(72, 20)
	m.Send("/").Send("ui").Send("enter")

	for i, ln := range strings.Split(m.View(), "\n") {
		if got := ui.VisibleWidth(ln); got > 72 {
			t.Errorf("line %d is %d cells wide, want at most 72", i, got)
		}
	}
}

// browsing is a panel with the opening search prompt dismissed, which is where
// the motions are exercised from.
func browsing(sess store.Session) *ui.Model {
	return ui.NewForTest(sess).Send("esc")
}

func longDiff() store.Session {
	lines := make([]string, 0, 80)
	for range 80 {
		lines = append(lines, "+\tcode")
	}
	return store.Session{
		Events: []capture.Event{{
			Seq: 1, Rel: "a.go", Added: 80, Tool: "Edit",
			Hunks: []capture.Hunk{{OldStart: 1, OldLines: 0, NewStart: 1, NewLines: 80, Lines: lines}},
		}},
		Prompts: map[string]string{},
	}
}

// The panel is for finding a file among many, so it opens ready to search.
func TestPanel_OpensInSearchMode(t *testing.T) {
	m := ui.NewForTest(threeEvents())

	if !m.Filtering() {
		t.Error("the panel should open with the search prompt live")
	}
	if got := m.RowCount(); got != 3 {
		t.Errorf("rows = %d, want the whole list until something is typed", got)
	}
}

func TestFilter_CtrlJAndCtrlKMoveThroughResults(t *testing.T) {
	m := ui.NewForTest(filterFixture()).Send("go")

	if got := m.Send("ctrl+j").Cursor(); got != 1 {
		t.Errorf("ctrl+j cursor = %d, want 1", got)
	}
	if got := m.Send("ctrl+k").Cursor(); got != 0 {
		t.Errorf("ctrl+k cursor = %d, want 0", got)
	}
	if !m.Filtering() {
		t.Error("moving through results should not close the prompt")
	}
}

// The diff has its own cursor, so you can see where you are in a long hunk.
func TestDiff_JKMoveTheCursorWhenFocused(t *testing.T) {
	m := browsing(longDiff()).Send("tab")

	if got := m.Send("j").DiffCursor(); got != 1 {
		t.Errorf("after j, diff cursor = %d, want 1", got)
	}
	if got := m.Send("j").Send("k").DiffCursor(); got != 1 {
		t.Errorf("after j k, diff cursor = %d, want 1", got)
	}
}

// The view follows the cursor rather than the cursor running off the screen.
func TestDiff_ViewFollowsTheCursor(t *testing.T) {
	m := browsing(longDiff()).Send("tab")

	for range 40 {
		m.Send("j")
	}

	cursor, top := m.DiffCursor(), m.DiffTop()
	if cursor < top || cursor >= top+m.BodyHeight() {
		t.Errorf("cursor %d is outside the visible window [%d,%d)", cursor, top, top+m.BodyHeight())
	}
}

// A long diff must be readable without Tabbing into it first.
func TestDiff_CtrlEAndCtrlYScrollWhileTheListIsFocused(t *testing.T) {
	m := browsing(longDiff())
	if !m.ListFocused() {
		t.Fatal("setup: the list should have focus")
	}

	if got := m.Send("ctrl+e").DiffTop(); got != 1 {
		t.Errorf("ctrl+e diff top = %d, want 1", got)
	}
	if got := m.Send("ctrl+y").DiffTop(); got != 0 {
		t.Errorf("ctrl+y diff top = %d, want 0", got)
	}
	if got := m.Cursor(); got != 0 {
		t.Errorf("scrolling the diff moved the list selection to %d", got)
	}
}

func TestDiff_CtrlFPagesWhileTheListIsFocused(t *testing.T) {
	m := browsing(longDiff())

	top := m.Send("ctrl+f").DiffTop()

	if top < m.BodyHeight()-2 {
		t.Errorf("ctrl+f scrolled %d lines, want about a page (%d)", top, m.BodyHeight())
	}
	if got := m.Send("ctrl+b").DiffTop(); got != 0 {
		t.Errorf("ctrl+b diff top = %d, want 0", got)
	}
}

func TestDiff_GAndGGMoveTheCursorWhenFocused(t *testing.T) {
	m := browsing(longDiff()).Send("tab")

	m.Send("G")
	last := m.DiffCursor()
	if last == 0 {
		t.Fatal("G did not move the diff cursor")
	}

	m.Send("g").Send("g")
	if got := m.DiffCursor(); got != 0 {
		t.Errorf("gg diff cursor = %d, want 0", got)
	}
}
