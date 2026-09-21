package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
	"github.com/mahmood/lens/internal/ui"
)

func sessionLastTouched(ago time.Duration) store.Session {
	return store.Session{
		Meta:    store.Meta{CWD: "/p", Ended: true}, // the hook says so; it is not proof
		Events:  []capture.Event{{Seq: 1, Rel: "a.go", Path: "/p/a.go", Added: 1, Time: time.Now().Add(-ago)}},
		Prompts: map[string]string{},
	}
}

// Claude Code fires SessionEnd when a conversation is cleared or compacted, not
// only when it is over, so the flag reads true for sessions still being used.
// Saying "(session ended)" on a live session is worse than saying nothing.
func TestStatus_DoesNotClaimASessionEndedOnTheHooksWord(t *testing.T) {
	m := browsing(sessionLastTouched(5 * time.Second))
	m.Resize(90, 12)

	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if strings.Contains(head, "ended") {
		t.Errorf("status = %q; the flag is not proof the session is over", head)
	}
}

// What can be said honestly is how long it has been quiet.
func TestStatus_SaysHowLongItHasBeenQuiet(t *testing.T) {
	m := browsing(sessionLastTouched(12 * time.Minute))
	m.Resize(90, 12)

	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if !strings.Contains(head, "idle 12m") {
		t.Errorf("status = %q, want it to report 12m idle", head)
	}
}

// A session working right now is not idle at all, and saying so would be noise.
func TestStatus_SaysNothingWhileItIsBusy(t *testing.T) {
	m := browsing(sessionLastTouched(30 * time.Second))
	m.Resize(90, 12)

	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if strings.Contains(head, "idle") {
		t.Errorf("status = %q, want no idle note so soon", head)
	}
}

func TestStatus_IdleInHours(t *testing.T) {
	m := browsing(sessionLastTouched(3 * time.Hour))
	m.Resize(90, 12)

	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if !strings.Contains(head, "idle 3h") {
		t.Errorf("status = %q, want 3h", head)
	}
}
