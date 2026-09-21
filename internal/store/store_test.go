package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
)

func TestAppendAndRead(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s, err := store.Open("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMeta("/home/u/proj"); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(capture.Event{Tool: "Edit", Rel: "main.go", Added: 3}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(capture.Event{Tool: "Write", Rel: "b.go", Added: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordPrompt("p1", "add a parser"); err != nil {
		t.Fatal(err)
	}

	got, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(got.Events))
	}
	if got.Events[0].Seq != 1 || got.Events[1].Seq != 2 {
		t.Errorf("seqs = %d,%d, want 1,2", got.Events[0].Seq, got.Events[1].Seq)
	}
	if got.Meta.CWD != "/home/u/proj" {
		t.Errorf("cwd = %q", got.Meta.CWD)
	}
	if got.Prompts["p1"] != "add a parser" {
		t.Errorf("prompt = %q, want %q", got.Prompts["p1"], "add a parser")
	}
	if got.Meta.Ended {
		t.Error("session marked ended before SessionEnd")
	}
}

// EnsureMeta runs on every captured edit; it must not clobber the start time.
func TestEnsureMetaIsIdempotent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("sess-meta")

	if err := s.EnsureMeta("/first"); err != nil {
		t.Fatal(err)
	}
	first, _ := s.Read()
	if err := s.EnsureMeta("/second"); err != nil {
		t.Fatal(err)
	}
	second, _ := s.Read()

	if second.Meta.CWD != "/first" {
		t.Errorf("cwd = %q, want the original /first", second.Meta.CWD)
	}
	if !second.Meta.Started.Equal(first.Meta.Started) {
		t.Error("start time was overwritten")
	}
}

// A hook killed mid-append leaves a partial final line; earlier events must survive.
func TestReadToleratesTruncatedTail(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("sess-2")
	if err := s.Append(capture.Event{Tool: "Edit", Rel: "a.go"}); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(filepath.Join(store.Dir("sess-2"), "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"tool":"Edit","rel":`)
	f.Close()

	got, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 {
		t.Errorf("events = %d, want 1", len(got.Events))
	}
}

// Claude runs tools in parallel, so concurrent appends are routine.
func TestConcurrentAppendsAllSurvive(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("sess-race")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Append(capture.Event{Tool: "Edit", Rel: "a.go"})
		}()
	}
	wg.Wait()

	got, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 20 {
		t.Errorf("events = %d, want 20", len(got.Events))
	}
}

func TestMarkEnded(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("sess-3")
	s.EnsureMeta("/p")

	if err := s.MarkEnded(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Read()
	if !got.Meta.Ended {
		t.Error("session not marked ended")
	}
}

func TestDestroyRemovesEverything(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("sess-4")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go"})

	if err := s.Destroy(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Dir("sess-4")); !os.IsNotExist(err) {
		t.Error("session directory survived Destroy")
	}
}

func TestSweepRemovesOldSessionsOnly(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	old, _ := store.Open("old")
	old.EnsureMeta("/p")
	fresh, _ := store.Open("fresh")
	fresh.EnsureMeta("/p")

	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(store.Dir("old"), past, past); err != nil {
		t.Fatal(err)
	}

	if err := store.Sweep(24 * time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Dir("old")); !os.IsNotExist(err) {
		t.Error("old session survived the sweep")
	}
	if _, err := os.Stat(store.Dir("fresh")); err != nil {
		t.Error("fresh session was swept")
	}
}

func TestReadMissingSessionIsEmptyNotAnError(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("sess-5")

	got, err := s.Read()
	if err != nil {
		t.Fatalf("reading a session with no events yet: %v", err)
	}
	if len(got.Events) != 0 {
		t.Errorf("events = %d, want 0", len(got.Events))
	}
}

// Read state has to outlive the panel. Closing the popup and opening it again
// must not turn everything unread, or the marks are worthless.
func TestSeenSurvivesReopening(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s, err := store.Open("sess-seen")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(capture.Event{Tool: "Edit", Rel: "a.go"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(capture.Event{Tool: "Edit", Rel: "b.go"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSeen(1); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open("sess-seen")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := reopened.Read()
	if err != nil {
		t.Fatal(err)
	}

	if !sess.Seen[1] {
		t.Error("edit 1 was read, but came back unread")
	}
	if sess.Seen[2] {
		t.Error("edit 2 was never selected, but came back read")
	}
}

// The same edit being selected over and over must not grow the file forever.
func TestMarkSeenIsIdempotent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s, err := store.Open("sess-twice")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(capture.Event{Tool: "Edit", Rel: "a.go"}); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := s.MarkSeen(1); err != nil {
			t.Fatal(err)
		}
	}

	b, err := os.ReadFile(filepath.Join(store.Dir("sess-twice"), "seen"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(strings.TrimSpace(string(b)), "\n") + 1; got != 1 {
		t.Errorf("seen file holds %d lines, want 1", got)
	}
}

// The panel opens for edits, not for turns. Remembering how far it has already
// been opened is what keeps a turn that changed nothing from reopening it.
func TestAnnouncedSurvivesReopening(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s, err := store.Open("sess-ann")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Announced(); err != nil || got != 0 {
		t.Fatalf("a fresh session has announced nothing: got %d, %v", got, err)
	}
	if err := s.Announce(4); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open("sess-ann")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.Announced(); err != nil || got != 4 {
		t.Errorf("announced = %d, %v; want 4", got, err)
	}
}

// Auto-open is a per-project preference: silencing a noisy repo must not
// silence every other one.
func TestAutoOpenIsRememberedPerProject(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if !store.AutoOpen("/p/one") || !store.AutoOpen("/p/two") {
		t.Fatal("auto-open should be on by default everywhere")
	}

	on, err := store.ToggleAutoOpen("/p/one")
	if err != nil {
		t.Fatal(err)
	}
	if on || store.AutoOpen("/p/one") {
		t.Error("the first toggle should turn /p/one off")
	}
	if !store.AutoOpen("/p/two") {
		t.Error("turning /p/one off also silenced /p/two")
	}

	if on, err = store.ToggleAutoOpen("/p/one"); err != nil || !on || !store.AutoOpen("/p/one") {
		t.Errorf("toggling again should turn /p/one back on: on=%v err=%v", on, err)
	}
}

// A path and the same path with a trailing slash are one project.
func TestAutoOpenIgnoresPathShape(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := store.SetAutoOpen("/p/one/", false); err != nil {
		t.Fatal(err)
	}
	if store.AutoOpen("/p/one") {
		t.Error("/p/one and /p/one/ should be the same project")
	}
}

// The hook knows a project by its absolute path; the CLI is run from inside it.
// A relative path has to name the same project, or muting silently does nothing.
func TestAutoOpenResolvesRelativePaths(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	dir := t.TempDir()
	t.Chdir(dir)

	if err := store.SetAutoOpen(".", false); err != nil {
		t.Fatal(err)
	}
	if store.AutoOpen(dir) {
		t.Errorf("muting %q did not mute %q", ".", dir)
	}
}

// A project reached through a symlink is the same project.
func TestAutoOpenResolvesSymlinks(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := store.SetAutoOpen(link, false); err != nil {
		t.Fatal(err)
	}
	if store.AutoOpen(real) {
		t.Errorf("muting the symlink %q did not mute %q", link, real)
	}
}

// What a reader clears out of the panel belongs to that session, and has to
// still be cleared when the popup is reopened over it.
func TestDismissalsAreRememberedPerSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	one, err := store.Open("one")
	if err != nil {
		t.Fatal(err)
	}
	two, err := store.Open("two")
	if err != nil {
		t.Fatal(err)
	}

	trail := []store.Dismissal{{Rel: "a.go", Hunk: -1}, {Seq: 4, Hunk: 2}}
	if err := one.SetDismissals(trail); err != nil {
		t.Fatal(err)
	}

	sess, err := one.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Dismissed) != 2 || sess.Dismissed[0].Rel != "a.go" || sess.Dismissed[1].Seq != 4 {
		t.Errorf("read back %v, want the trail in the order it was made", sess.Dismissed)
	}

	other, err := two.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Dismissed) != 0 {
		t.Errorf("session two has %v, want nothing dismissed of its own", other.Dismissed)
	}
}

// Undoing everything has to clear the file, not leave a stale trail behind.
func TestDismissalsCanBeEmptied(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s, err := store.Open("s")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetDismissals([]store.Dismissal{{Rel: "a.go", Hunk: -1}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDismissals(nil); err != nil {
		t.Fatal(err)
	}

	sess, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Dismissed) != 0 {
		t.Errorf("dismissed = %v, want none", sess.Dismissed)
	}
}

// A session nobody has dismissed anything in reads as empty, not as an error.
func TestDismissalsDefaultToNone(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s, _ := store.Open("fresh")
	sess, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Dismissed) != 0 {
		t.Errorf("dismissed = %v, want none", sess.Dismissed)
	}
}

// A session whose meta was created before its project was known — SessionEnd
// firing before anything else wrote one — must have the gap filled the moment
// a hook does know. Left empty, the session is invisible to every question
// asked by project, and its panel is never the one that opens.
func TestEnsureMeta_FillsInAProjectItDidNotHave(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s, err := store.Open("late")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkEnded(); err != nil { // writes a meta with no project
		t.Fatal(err)
	}
	if got, _ := s.Read(); got.Meta.CWD != "" {
		t.Fatalf("setup: cwd = %q, want it missing", got.Meta.CWD)
	}

	if err := s.EnsureMeta("/home/me/minigit"); err != nil {
		t.Fatal(err)
	}

	got, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.CWD != "/home/me/minigit" {
		t.Errorf("cwd = %q, want it filled in", got.Meta.CWD)
	}
	if !got.Meta.Ended {
		t.Error("filling in the project forgot that the session had ended")
	}
}

// The project is the one the session started in, so the first answer stands:
// an agent that changes directory must not move the session with it.
func TestEnsureMeta_KeepsTheProjectItAlreadyHas(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s, _ := store.Open("first")
	if err := s.EnsureMeta("/home/me/proj"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMeta("/home/me/proj/subdir"); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Read()
	if got.Meta.CWD != "/home/me/proj" {
		t.Errorf("cwd = %q, want the directory it started in", got.Meta.CWD)
	}
}

// Ending a session says nothing about when it began, so it must not invent a
// beginning: the walk uses that time to decide which files predate the session.
func TestMarkEnded_DoesNotInventAStartTime(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s, _ := store.Open("noStart")
	if err := s.MarkEnded(); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Read()
	if !got.Meta.Started.IsZero() {
		t.Errorf("started = %v, want it left unknown", got.Meta.Started)
	}
}

// A session is found by its project only once its meta names one. Until then
// another session wins, however stale — which is how a panel came to open on an
// empty session while the one holding the work sat unmatched beside it.
func TestLatest_FindsASessionOnceItsProjectIsKnown(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	other, _ := store.Open("other")
	if err := other.EnsureMeta("/home/me/proj"); err != nil {
		t.Fatal(err)
	}
	other.Append(capture.Event{Rel: "b.go", Added: 1})

	s, _ := store.Open("late")
	s.MarkEnded() // a meta with no project
	s.Append(capture.Event{Rel: "a.go", Added: 1})

	if got, _ := store.Latest("/home/me/proj"); got != "other" {
		t.Fatalf("setup: Latest = %q, want other: late names no project yet", got)
	}

	if err := s.EnsureMeta("/home/me/proj"); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Latest("/home/me/proj"); got != "late" {
		t.Errorf("Latest = %q, want late now that its project is known", got)
	}
}
