package capture_test

import (
	"testing"

	"github.com/mahmood/lens/internal/capture"
)

// The panel and the thing that decides to open it have to agree about what
// counts as worth showing.
//
// They did not. The popup was opened when the highest recorded edit was newer
// than the last one announced, counting every event on disk — while the panel
// then hid some of them. A turn whose only writes were a shell command touching
// files outside the project opened a popup with nothing new in it, and a
// session whose every event was like that opened an empty one. In one real
// session, 375 of 661 events were of exactly that kind.
func TestShowable(t *testing.T) {
	root := "/home/me/project"
	for _, c := range []struct {
		name string
		e    capture.Event
		want bool
	}{
		{"an ordinary edit in the project",
			capture.Event{Tool: "Edit", Path: root + "/main.go"}, true},
		{"a shell write in the project",
			capture.Event{Tool: "Bash", Path: root + "/main.go"}, true},

		// The walk sweeps directories, so a command naming a path elsewhere
		// used to drag another program's scratch files in with it.
		{"a shell write outside the project",
			capture.Event{Tool: "Bash", Path: "/tmp/scratch/x.go"}, false},

		// Someone meant this one, wherever the file lives.
		{"a deliberate edit outside the project",
			capture.Event{Tool: "Edit", Path: "/tmp/scratch/x.go"}, true},
		{"a deliberate write outside the project",
			capture.Event{Tool: "Write", Path: "/etc/hosts"}, true},

		{"anything hidden", capture.Event{Tool: "Edit", Path: root + "/.env"}, false},
		{"anything under a hidden directory",
			capture.Event{Tool: "Write", Path: root + "/.git/config"}, false},
		{"a hidden file reached by a shell command",
			capture.Event{Tool: "Bash", Path: root + "/.secret/token"}, false},

		// A project that lives somewhere hidden is not hidden from itself.
		{"an ordinary file in a project under a dot directory",
			capture.Event{Tool: "Edit", Path: "/home/me/.config/nvim/init.lua"}, true},

		// Nothing to judge: an event with no path is shown rather than dropped.
		{"an event with no path at all", capture.Event{Tool: "Edit"}, true},
	} {
		r := root
		if c.name == "an ordinary file in a project under a dot directory" {
			r = "/home/me/.config/nvim"
		}
		if got := capture.Showable(r, c.e); got != c.want {
			t.Errorf("%s: Showable = %v, want %v", c.name, got, c.want)
		}
	}
}

// With no root recorded there is nothing to measure against, and refusing
// everything would empty the panel.
func TestShowable_WithNoRoot(t *testing.T) {
	for _, e := range []capture.Event{
		{Tool: "Bash", Path: "/tmp/x.go"},
		{Tool: "Edit", Path: "/anywhere/y.go"},
	} {
		if !capture.Showable("", e) {
			t.Errorf("Showable(\"\", %+v) = false; with no root, keep it", e)
		}
	}
}

// Counting is what the trigger needs: how many of these would the panel draw?
func TestCountShowable(t *testing.T) {
	root := "/home/me/project"
	events := []capture.Event{
		{Seq: 1, Tool: "Edit", Path: root + "/a.go"},
		{Seq: 2, Tool: "Bash", Path: "/tmp/scratch/b.go"},
		{Seq: 3, Tool: "Bash", Path: root + "/.git/index"},
		{Seq: 4, Tool: "Write", Path: root + "/c.go"},
	}
	if got := capture.CountShowable(root, events); got != 2 {
		t.Errorf("CountShowable = %d, want 2", got)
	}
}

// The highest edit the panel would actually draw. This is what the popup should
// be opened for, so a turn that only produced hidden or strayed writes leaves
// the marker where it was and opens nothing.
func TestLastShowable(t *testing.T) {
	root := "/home/me/project"
	events := []capture.Event{
		{Seq: 1, Tool: "Edit", Path: root + "/a.go"},
		{Seq: 7, Tool: "Write", Path: root + "/c.go"},
		{Seq: 9, Tool: "Bash", Path: "/tmp/scratch/b.go"},
	}
	if got := capture.LastShowable(root, events); got != 7 {
		t.Errorf("LastShowable = %d, want 7 (9 is a strayed shell write)", got)
	}
	if got := capture.LastShowable(root, nil); got != 0 {
		t.Errorf("LastShowable(nil) = %d, want 0", got)
	}
	none := []capture.Event{{Seq: 3, Tool: "Bash", Path: "/tmp/x"}}
	if got := capture.LastShowable(root, none); got != 0 {
		t.Errorf("LastShowable with nothing showable = %d, want 0", got)
	}
}
