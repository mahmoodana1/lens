package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
)

// plant writes a session with a given project, end state and last-touched time.
func plant(t *testing.T, id, cwd string, ended bool, age time.Duration) {
	t.Helper()
	s, err := store.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	if cwd != "" {
		if err := s.EnsureMeta(cwd); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Append(capture.Event{Rel: "a.go", Added: 1}); err != nil {
		t.Fatal(err)
	}
	if ended {
		if err := s.MarkEnded(); err != nil {
			t.Fatal(err)
		}
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(store.Dir(id), when, when); err != nil {
		t.Fatal(err)
	}
}

// The panel must belong to the project in front of you. Another project's
// session being touched more recently is no reason to show it instead.
func TestLatest_PrefersTheProjectYouAreIn(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	plant(t, "other", "/home/me/projects/lens", true, 0)               // the newest
	plant(t, "mine", "/home/me/minigit_modular", true, 10*time.Minute) // older, but here

	got, err := store.Latest("/home/me/minigit_modular")
	if err != nil {
		t.Fatal(err)
	}
	if got != "mine" {
		t.Errorf("Latest = %q, want mine: the session for this project", got)
	}
}

// Inside a subdirectory of the project, its session is still the right one.
func TestLatest_MatchesFromASubdirectory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	plant(t, "other", "/home/me/elsewhere", true, 0)
	plant(t, "mine", "/home/me/proj", true, time.Minute)

	got, _ := store.Latest("/home/me/proj/internal/ui")
	if got != "mine" {
		t.Errorf("Latest = %q, want mine", got)
	}
}

// Two sessions could both contain the directory; the closer one is the project.
func TestLatest_PrefersTheClosestProject(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	plant(t, "broad", "/home/me", true, 0)               // newest, but a whole home
	plant(t, "narrow", "/home/me/proj", true, time.Hour) // oldest, but the project

	got, _ := store.Latest("/home/me/proj/src")
	if got != "narrow" {
		t.Errorf("Latest = %q, want narrow: the more specific project wins", got)
	}
}

// "Ended" is not something the flag can be trusted for: Claude Code fires the
// SessionEnd hook on a clear or a compaction as well as on a real exit, so a
// session still in use reads as finished. Recency decides instead.
func TestLatest_IgnoresWhetherASessionSaysItEnded(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	plant(t, "saysEnded", "/home/me/proj", true, time.Minute)
	plant(t, "saysLive", "/home/me/proj", false, time.Hour)

	got, _ := store.Latest("/home/me/proj")
	if got != "saysEnded" {
		t.Errorf("Latest = %q, want saysEnded: it was touched more recently", got)
	}
}

// Among equals, the one touched most recently.
func TestLatest_FallsBackToTheMostRecentOfTheProject(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	plant(t, "older", "/home/me/proj", true, time.Hour)
	plant(t, "newer", "/home/me/proj", true, time.Minute)

	got, _ := store.Latest("/home/me/proj")
	if got != "newer" {
		t.Errorf("Latest = %q, want newer", got)
	}
}

// From a directory no session knows, the most recent is all there is to go on —
// which is what `lens` in a bare terminal relies on.
func TestLatest_WithoutAMatchTakesTheMostRecent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	plant(t, "old", "/home/me/a", true, time.Hour)
	plant(t, "recent", "/home/me/b", true, 0)

	got, err := store.Latest("/somewhere/unrelated")
	if err != nil {
		t.Fatal(err)
	}
	if got != "recent" {
		t.Errorf("Latest = %q, want recent", got)
	}
}

func TestLatest_NoSessionsAtAll(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if got, err := store.Latest("/home/me/proj"); err == nil {
		t.Errorf("Latest = %q, want an error when there is nothing to show", got)
	}
}

// A directory with no meta yet is not a candidate for a project, but is still
// available as a last resort.
func TestLatest_IgnoresSessionsWithNoProject(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	plant(t, "nometa", "", true, 0)
	plant(t, "mine", "/home/me/proj", true, time.Hour)

	if got, _ := store.Latest("/home/me/proj"); got != "mine" {
		t.Errorf("Latest = %q, want mine", got)
	}
	if got, _ := store.Latest("/unrelated"); got != "nometa" {
		t.Errorf("Latest = %q, want the most recent as a last resort", got)
	}
}

// Stray files in the state directory are not sessions.
func TestSessions_SkipsWhatIsNotASession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	plant(t, "real", "/home/me/proj", true, 0)

	if err := os.WriteFile(filepath.Join(store.Root(), "auto-open-off"), []byte("/x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := store.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "real" {
		t.Errorf("sessions = %+v, want just the one", got)
	}
	if got[0].CWD != "/home/me/proj" {
		t.Errorf("cwd = %q, want the project it ran in", got[0].CWD)
	}
}
