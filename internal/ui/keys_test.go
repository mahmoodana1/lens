package ui_test

import (
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
	"github.com/mahmood/lens/internal/ui"
)

// a.go and b.go share a directory, so the list is one heading and two files.
func TestKeys_JKMoveSelectionAndClampAtEnds(t *testing.T) {
	m := browsing(threeEvents())

	if got := m.SelectedRel(); got != "a.go" {
		t.Fatalf("opened on %q, want a.go", got)
	}
	if got := m.Send("j").SelectedRel(); got != "b.go" {
		t.Errorf("after j, on %q, want b.go", got)
	}
	if got := m.Send("k").Send("k").SelectedRel(); got != "a.go" {
		t.Errorf("k past the top should clamp on a.go, got %q", got)
	}
	if got := m.Send("G").SelectedRel(); got != "b.go" {
		t.Errorf("G should land on the last file, got %q", got)
	}
	if got := m.Send("j").SelectedRel(); got != "b.go" {
		t.Errorf("j past the bottom should clamp, got %q", got)
	}
	if got := m.Send("g").Send("g").SelectedRel(); got != "a.go" {
		t.Errorf("gg should land on the first file, got %q", got)
	}
}

// A lone g is a pending chord, not a jump.
func TestKeys_SingleGDoesNotJump(t *testing.T) {
	m := browsing(threeEvents()).Send("G")
	if got := m.Send("g").SelectedRel(); got != "b.go" {
		t.Errorf("single g moved the cursor to %q; it should wait for the second g", got)
	}
}

// Opening a file replaces the list with that file's own history.
func TestKeys_LOpensTheFile(t *testing.T) {
	m := browsing(threeEvents()) // a.go twice, b.go once

	m.Send("l")
	if got := m.OpenFile(); got != "a.go" {
		t.Fatalf("open file = %q, want a.go", got)
	}
	for _, r := range rowsOf(m) {
		if r.Rel != "a.go" {
			t.Errorf("%q is still listed inside a.go", r.Rel)
		}
	}
}

// Space is the same way in, for one-handed browsing.
func TestKeys_SpaceOpensTheFile(t *testing.T) {
	if got := browsing(threeEvents()).Send(" ").OpenFile(); got != "a.go" {
		t.Errorf("open file = %q, want a.go", got)
	}
}

func TestKeys_HGoesBackToTheFileList(t *testing.T) {
	m := browsing(threeEvents()).Send("l")

	m.Send("h")
	if got := m.OpenFile(); got != "" {
		t.Errorf("still inside %q", got)
	}
	if !m.OnFileRow() || m.SelectedRel() != "a.go" {
		t.Errorf("backing out landed on %q (file row=%v), want a.go", m.SelectedRel(), m.OnFileRow())
	}
}

// esc is the same way out, since that is the key a reader reaches for.
func TestKeys_EscGoesBackToTheFileList(t *testing.T) {
	if got := browsing(threeEvents()).Send("l").Send("esc").OpenFile(); got != "" {
		t.Errorf("esc left the panel inside %q", got)
	}
}

// Inside a file, the rows are its edits and the hunks each one made.
func TestKeys_InsideAFileTheRowsAreEditsAndHunks(t *testing.T) {
	m := browsing(twoEditsWithHunks()).Send("l")

	kinds := []ui.RowKind{}
	for _, r := range rowsOf(m) {
		kinds = append(kinds, r.Kind)
	}
	want := []ui.RowKind{ui.RowEdit, ui.RowHunk, ui.RowHunk, ui.RowEdit, ui.RowHunk}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("row %d = %v, want %v", i, kinds[i], want[i])
		}
	}
}

// Picking a hunk is how you choose what to read: the diff jumps to it.
func TestKeys_PickingAHunkMovesTheDiffToIt(t *testing.T) {
	m := browsing(twoEditsWithHunks())
	m.Resize(90, 14)
	m.Send("l") // inside keys.go, on the first edit

	m.Send("j") // its first hunk
	first := m.DiffCursor()
	m.Send("j") // its second hunk
	if second := m.DiffCursor(); second <= first {
		t.Errorf("the second hunk put the diff cursor at %d, want past the first at %d", second, first)
	}
}

// Going back into the same file must not leave the diff where it was.
func TestKeys_ReopeningAFileStartsAtItsFirstRow(t *testing.T) {
	m := browsing(twoEditsWithHunks())
	m.Resize(90, 14)
	m.Send("l").Send("j").Send("j")

	m.Send("h").Send("l")
	if got := m.Cursor(); got != 0 {
		t.Errorf("cursor = %d, want the first row of the file", got)
	}
	if got := m.DiffCursor(); got != 0 {
		t.Errorf("diff cursor = %d, want 0", got)
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
	m := browsing(threeEvents()) // a.go (twice), b.go

	m.Send("J")
	if got := m.SelectedRel(); got != "b.go" || !m.OnFileRow() {
		t.Errorf("after J, on %q (file row=%v), want b.go", got, m.OnFileRow())
	}
	m.Send("K")
	if got := m.SelectedRel(); got != "a.go" || !m.OnFileRow() {
		t.Errorf("after K, on %q (file row=%v), want a.go", got, m.OnFileRow())
	}
}

// Inside a file, J opens the next one in its place: reading file after file
// should not need a trip back out.
func TestKeys_ShiftJInsideAFileOpensTheNextOne(t *testing.T) {
	m := browsing(threeEvents()).Send("l")

	m.Send("J")
	if got := m.OpenFile(); got != "b.go" {
		t.Errorf("open file = %q, want b.go", got)
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
	if got := fileCount(m); got != 1 {
		t.Fatalf("files = %d, want 1", got)
	}

	s.Append(capture.Event{Rel: "b.go", Added: 2})
	m.Reload()

	if got := fileCount(m); got != 2 {
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
	m.Send("g").Send("g")
	if !m.OnFileRow() {
		t.Fatalf("expected to start on a file row, cursor=%d", m.Cursor())
	}

	want := m.Cursor()
	for i := 0; i < 3; i++ {
		m.Reload()
	}
	if got := m.Cursor(); got != want {
		t.Errorf("cursor drifted from %d to %d across reloads", want, got)
	}
	if !m.OnFileRow() {
		t.Error("cursor slid off the file row")
	}
}

// The cursor steps over directory headings rather than landing on one.
func TestKeys_MotionsSkipDirectoryHeadings(t *testing.T) {
	m := browsing(store.Session{Events: []capture.Event{
		{Seq: 1, Rel: "one/a.go", Added: 1},
		{Seq: 2, Rel: "two/b.go", Added: 1},
	}})

	if got := m.SelectedRel(); got != "one/a.go" {
		t.Fatalf("opened on %q, want the first file", got)
	}
	m.Send("j")
	if got := m.SelectedRel(); got != "two/b.go" {
		t.Errorf("j landed on %q, want two/b.go — the heading is not a stop", got)
	}
	m.Send("G")
	if m.SelectedKind() == ui.RowDir {
		t.Error("G landed on a directory heading")
	}
	m.Send("g").Send("g")
	if m.SelectedKind() == ui.RowDir {
		t.Error("gg landed on a directory heading")
	}
}

// Holding j down does not produce one message per press: the terminal delivers
// the repeats together, and bubbletea hands them over as a single key message
// carrying every rune. Each one still has to move.
func TestKeys_HeldKeyRepeatMovesOncePerRune(t *testing.T) {
	m := browsing(threeEvents())

	if got := m.Send("jj").SelectedRel(); got != "b.go" {
		t.Errorf("after a held j arriving as one message, on %q, want b.go", got)
	}
	if got := m.Send("kk").SelectedRel(); got != "a.go" {
		t.Errorf("after a held k, on %q, want a.go", got)
	}
}

// The chord still works when both g's arrive in the same message.
func TestKeys_GGInOneMessageJumpsToTop(t *testing.T) {
	m := browsing(threeEvents()).Send("G")

	if got := m.Send("gg").Cursor(); got != 1 {
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
	if got := fileCount(m); got != 1 {
		t.Errorf("files = %d, want 1", got)
	}
}

// While the prompt is open every key is a character, or nothing could be typed.
// k is both a motion and a letter of keys.go, so it shows which one won.
func TestFilter_TypingAMotionKeyIsJustACharacter(t *testing.T) {
	m := browsing(filterFixture()).Send("/").Send("k")

	if got := m.Filter(); got != "k" {
		t.Errorf("filter = %q, want %q", got, "k")
	}
	if got := m.SelectedRel(); got != "internal/ui/keys.go" {
		t.Errorf("k acted as a motion: the cursor is on %q", got)
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
	if got := fileCount(m); got != 3 {
		t.Errorf("files = %d, want the whole list back", got)
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
	if got := m.Send("j").SelectedRel(); got != "internal/ui/view.go" {
		t.Errorf("after enter, j should move again; on %q", got)
	}
}

// fzf-style: pick a file without leaving the prompt.
func TestFilter_CtrlNAndCtrlPMoveWhileTyping(t *testing.T) {
	m := browsing(filterFixture()).Send("/").Send("go")

	if got := m.Send("ctrl+n").SelectedRel(); got != "internal/ui/view.go" {
		t.Errorf("ctrl+n landed on %q, want the next file", got)
	}
	if got := m.Send("ctrl+p").SelectedRel(); got != "internal/ui/keys.go" {
		t.Errorf("ctrl+p landed on %q, want the previous file", got)
	}
	if !m.Filtering() {
		t.Error("moving should not close the prompt")
	}
}

// Typing re-ranks the list, so the cursor follows it rather than sitting on
// whatever index it happened to hold.
func TestFilter_CursorFollowsTheBestMatch(t *testing.T) {
	m := browsing(filterFixture()).Send("j").Send("j")
	if got := m.SelectedRel(); got != "internal/store/store.go" {
		t.Fatalf("setup: on %q, want the third file", got)
	}

	m.Send("/").Send("go")

	if got := m.Cursor(); got != 1 {
		t.Errorf("cursor = %d, want it back on the best match", got)
	}
}

// The panel is for reading, so looking at an edit is reading it.
// Landing on a file reads the change it previews, which is the newest one.
func TestRead_SelectingAFileReadsItsLatestEdit(t *testing.T) {
	m := browsing(threeEvents())

	if !m.IsRead(3) {
		t.Error("a.go previews edit 3, so opening on it reads edit 3")
	}
	if m.IsRead(1) {
		t.Error("edit 1 is older than the preview and has not been looked at")
	}
	if m.IsRead(2) {
		t.Error("b.go has not been selected yet")
	}

	m.Send("j")

	if !m.IsRead(2) {
		t.Error("landing on b.go should read its edit")
	}
}

// The older edits become readable once the file is open.
func TestRead_ExpandingLetsYouReachOlderEdits(t *testing.T) {
	m := browsing(threeEvents())
	m.Send("l").Send("j")

	if !m.IsRead(1) {
		t.Error("landing on the first edit to a.go should read it")
	}
}

// The count is of files with an unseen change, which is what the list shows.
func TestRead_UnreadCountFallsAsYouGo(t *testing.T) {
	m := browsing(threeEvents())

	if got := m.Unread(); got != 1 {
		t.Fatalf("unread = %d, want 1 (b.go; a.go is selected)", got)
	}
	m.Send("j")
	if got := m.Unread(); got != 0 {
		t.Errorf("unread = %d, want 0", got)
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
	if got := m.Unread(); got != 0 {
		t.Errorf("unread = %d, want 0 (b.go was read before; a.go is selected now)", got)
	}
}

// Unread edits have to stand out, or the panel cannot tell you what is new.
func TestView_MarksUnreadEdits(t *testing.T) {
	m := browsing(threeEvents()) // opens on edit 1, which reads it
	m.Resize(90, 24)

	out := ui.StripANSIForTest(m.View())

	if got := strings.Count(out, "●"); got != 1 {
		t.Errorf("unread dots = %d, want 1 (b.go; a.go is selected)", got)
	}
}

func TestView_DotsDisappearAsEditsAreRead(t *testing.T) {
	m := browsing(threeEvents())
	m.Resize(90, 24)

	m.Send("j")
	out := ui.StripANSIForTest(m.View())

	if got := strings.Count(out, "●"); got != 0 {
		t.Errorf("unread dots = %d after reading everything, want 0", got)
	}
}

func TestView_StatusLineCountsUnread(t *testing.T) {
	m := browsing(threeEvents())
	m.Resize(90, 24)

	if out := ui.StripANSIForTest(m.View()); !strings.Contains(out, "1 unread") {
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
		t.Errorf("rows = %d, want a heading and two files until something is typed", got)
	}
}

func TestFilter_CtrlJAndCtrlKMoveThroughResults(t *testing.T) {
	m := ui.NewForTest(filterFixture()).Send("go")

	if got := m.Send("ctrl+j").SelectedRel(); got != "internal/ui/view.go" {
		t.Errorf("ctrl+j landed on %q, want the next result", got)
	}
	if got := m.Send("ctrl+k").SelectedRel(); got != "internal/ui/keys.go" {
		t.Errorf("ctrl+k landed on %q, want the previous result", got)
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
	if got := m.SelectedRel(); got != "a.go" {
		t.Errorf("scrolling the diff moved the list selection to %q", got)
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

// The diff panel gives the diff the whole popup, so a wide hunk can be read
// without the list eating a third of the width.
func TestPanel_EnterHidesTheListAndWidensTheDiff(t *testing.T) {
	m := browsing(manyHunks())
	m.Resize(100, 24)

	narrow := diffPaneWidth(m)
	m.Send("f")
	if !m.Panel() {
		t.Fatal("enter did not open the diff panel")
	}
	if wide := diffPaneWidth(m); wide <= narrow {
		t.Errorf("diff is %d cells wide in the panel, want more than the %d it had", wide, narrow)
	}
	if strings.Contains(m.View(), "│") {
		t.Error("the panel still draws the list divider")
	}
}

func TestPanel_EscReturnsToTheSplit(t *testing.T) {
	m := browsing(manyHunks()).Send("f")

	if m.Send("esc").Panel() {
		t.Error("esc did not leave the diff panel")
	}
	if !m.ListFocused() {
		t.Error("leaving the panel should give the list its focus back")
	}
}

// f is the same switch, so it can be held down from either side.
func TestPanel_FTogglesIt(t *testing.T) {
	m := browsing(manyHunks())

	if !m.Send("f").Panel() {
		t.Error("f did not open the diff panel")
	}
	if m.Send("f").Panel() {
		t.Error("f did not close the diff panel")
	}
}

// enter is one key going one way: into the file, then into its diff.
func TestPanel_EnterGoesInOneLevelAtATime(t *testing.T) {
	m := browsing(threeEvents())

	m.Send("enter")
	if m.Panel() {
		t.Fatal("enter jumped straight to the diff panel, skipping the file")
	}
	if got := m.OpenFile(); got != "a.go" {
		t.Fatalf("enter did not open a.go, open file = %q", got)
	}
	if !m.Send("enter").Panel() {
		t.Error("enter from inside the file did not open the diff panel")
	}
	if m.Send("esc").Panel() {
		t.Error("esc did not leave the diff panel")
	}
	if got := m.OpenFile(); got != "a.go" {
		t.Errorf("esc from the panel left the file too; open file = %q", got)
	}
}

// The panel is for reading the diff, so motions act on it without a tab first.
func TestPanel_MotionsDriveTheDiff(t *testing.T) {
	m := browsing(manyHunks())
	m.Resize(100, 12)
	m.Send("f")

	if m.ListFocused() {
		t.Fatal("the panel should focus the diff")
	}
	if got := m.Send("j").DiffCursor(); got != 1 {
		t.Errorf("j moved the diff cursor to %d, want 1", got)
	}
}

// Reading file after file full-screen must not need a trip back to the list.
func TestPanel_ShiftJWalksFilesWithoutLeaving(t *testing.T) {
	m := browsing(threeEvents()).Send("f")

	m.Send("J")
	if !m.Panel() {
		t.Error("J closed the panel")
	}
	if got := m.SelectedRel(); got != "b.go" {
		t.Errorf("after J, showing %q, want b.go", got)
	}
	if got := m.DiffCursor(); got != 0 {
		t.Errorf("the new file's diff starts at line %d, want 0", got)
	}
}

// With the list gone, the status line is the only thing saying where you are.
func TestPanel_StatusLineNamesTheFileAndItsPlace(t *testing.T) {
	m := browsing(threeEvents()).Send("f")
	m.Resize(90, 24)

	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if !strings.Contains(head, "a.go") || !strings.Contains(head, "(1/2)") {
		t.Errorf("panel status line = %q, want it to name a.go as 1 of 2", head)
	}
}

// diffPaneWidth is how many cells the diff itself gets, measured from the view:
// the body lines past the divider, or the whole line where there is none.
func diffPaneWidth(m *ui.Model) int {
	lines := strings.Split(ui.StripANSIForTest(m.View()), "\n")
	if len(lines) < 3 {
		return 0
	}
	widest := 0
	for _, ln := range lines[1 : len(lines)-1] { // skip the status and help lines
		if i := strings.Index(ln, "│"); i >= 0 {
			ln = ln[i+len("│"):]
		}
		if w := ui.VisibleWidth(ln); w > widest {
			widest = w
		}
	}
	return widest
}

// Searching needs the list back, so / drops out of the full-width diff.
func TestPanel_SearchingReturnsToTheList(t *testing.T) {
	m := browsing(threeEvents()).Send("l").Send("f")

	m.Send("/")
	if m.Panel() {
		t.Error("/ left the diff panel up, with nothing to show the results in")
	}
	if got := m.OpenFile(); got != "" {
		t.Errorf("/ searches files, so it should leave %q", got)
	}
	if !m.Filtering() {
		t.Error("/ did not open the search prompt")
	}
}

// The prompt counts what the list shows, which is files.
func TestFilter_PromptCountsFiles(t *testing.T) {
	m := browsing(threeEvents()) // a.go twice, b.go once
	m.Resize(90, 24)

	lines := strings.Split(ui.StripANSIForTest(m.Send("/").View()), "\n")
	if got := lines[len(lines)-1]; !strings.Contains(got, "2 of 2") { //nolint
		t.Errorf("prompt = %q, want it to count 2 files, not 3 edits", strings.TrimSpace(got))
	}
}

// Files in one directory say it once, at the top, and then only their names.
func TestView_FilesAreListedByNameUnderTheirDirectory(t *testing.T) {
	m := browsing(store.Session{Events: []capture.Event{
		{Seq: 1, Rel: "internal/ui/keys.go", Added: 1},
		{Seq: 2, Rel: "internal/ui/view.go", Added: 1},
	}})
	m.Resize(90, 12)

	out := listPane(m)
	if !strings.Contains(out, "internal/ui/") {
		t.Errorf("no directory heading:\n%s", out)
	}
	if strings.Count(out, "internal/ui/") != 1 {
		t.Errorf("the directory is repeated on the file rows:\n%s", out)
	}
	if !strings.Contains(out, "keys.go") || !strings.Contains(out, "view.go") {
		t.Errorf("the file names are missing:\n%s", out)
	}
}

// A directory too wide for the column keeps both its ends.
func TestView_LongDirectoriesAreElidedInTheMiddle(t *testing.T) {
	m := browsing(store.Session{Events: []capture.Event{
		{Seq: 1, Rel: "internal/verylongdirectory/anotherone/keys.go", Added: 1},
	}})
	m.Resize(70, 10)

	out := listPane(m)
	if !strings.Contains(out, "internal/") || !strings.Contains(out, "anotherone/") {
		t.Errorf("the ends of the directory did not survive:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("nothing says the directory was cut:\n%s", out)
	}
	if !strings.Contains(out, "keys.go") {
		t.Errorf("the file name was truncated away:\n%s", out)
	}
}

func rowsOf(m *ui.Model) []ui.Row {
	out := make([]ui.Row, 0, m.RowCount())
	for i := 0; i < m.RowCount(); i++ {
		out = append(out, m.RowAt(i))
	}
	return out
}

func twoEditsWithHunks() store.Session {
	h := func(line int, first string) capture.Hunk {
		return capture.Hunk{
			OldStart: line, OldLines: 1, NewStart: line, NewLines: 2,
			Lines: []string{" context", first, " more"},
		}
	}
	return store.Session{
		Events: []capture.Event{
			{Seq: 1, Rel: "keys.go", Tool: "Edit", Added: 2, Hunks: []capture.Hunk{h(42, "+added"), h(88, "-gone")}},
			{Seq: 2, Rel: "keys.go", Tool: "Edit", Added: 1, Hunks: []capture.Hunk{h(12, "+later")}},
		},
		Prompts: map[string]string{},
	}
}

// fileCount is how many files the list is showing.
func fileCount(m *ui.Model) int {
	n := 0
	for _, r := range rowsOf(m) {
		if r.Kind == ui.RowFile {
			n++
		}
	}
	return n
}

// listPane is the left-hand side of the view, without the diff beside it.
func listPane(m *ui.Model) string {
	var b strings.Builder
	for _, ln := range strings.Split(ui.StripANSIForTest(m.View()), "\n") {
		if i := strings.Index(ln, "│"); i >= 0 {
			ln = ln[:i]
		}
		b.WriteString(ln)
		b.WriteString("\n")
	}
	return b.String()
}

// A hunk is identified by its line number and the start of what changed, so it
// is the end of the label that gives way — the reverse of a file name.
func TestView_HunkRowsKeepTheirLineNumber(t *testing.T) {
	m := browsing(store.Session{Events: []capture.Event{{
		Seq: 1, Rel: "a.go", Tool: "Edit", Added: 1,
		Hunks: []capture.Hunk{{OldStart: 27, NewStart: 27, NewLines: 2, Lines: []string{
			" ctx", "+// PostToolUse records what a tool changed and makes sure the panel is showing.",
		}}},
	}}})
	m.Resize(70, 10)
	m.Send("l")

	line := ""
	for _, ln := range strings.Split(listPane(m), "\n") {
		if strings.Contains(ln, "PostToolUse") || strings.Contains(ln, "27") {
			line = ln
			break
		}
	}
	if !strings.Contains(line, "27") {
		t.Errorf("the hunk row lost its line number: %q", line)
	}
	if !strings.Contains(line, "+ // Post") {
		t.Errorf("the hunk row lost the start of what changed: %q", line)
	}
}

// A hunk belongs to the edit above it, which already carries the unread mark;
// repeating it on every hunk would say the same thing four times.
func TestView_HunksDoNotRepeatTheUnreadMark(t *testing.T) {
	sess := twoEditsWithHunks()
	sess.Events = append(sess.Events, capture.Event{
		Seq: 3, Rel: "keys.go", Tool: "Edit", Added: 1,
		Hunks: []capture.Hunk{{OldStart: 5, NewStart: 5, NewLines: 2, Lines: []string{" ctx", "+later"}}},
	})
	m := browsing(sess)
	m.Resize(90, 14)
	m.Send("l")

	pane := listPane(m)
	if !strings.Contains(pane, "●") {
		t.Fatalf("setup: nothing is unread, so there is no mark to misplace:\n%s", pane)
	}
	for _, ln := range strings.Split(pane, "\n") {
		if strings.Contains(ln, "12  + later") && strings.Contains(ln, "●") {
			t.Errorf("a hunk row carries its own unread mark:\n%s", pane)
		}
	}
}
