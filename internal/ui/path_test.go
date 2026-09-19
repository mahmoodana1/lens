package ui_test

import (
	"testing"

	"github.com/mahmood/lens/internal/ui"
)

func TestSplitDir(t *testing.T) {
	for _, c := range []struct{ path, dir, name string }{
		{"internal/ui/keys.go", "internal/ui/", "keys.go"},
		{"main.go", "./", "main.go"},
		{"/etc/hosts", "/etc/", "hosts"},
	} {
		dir, name := ui.SplitDir(c.path)
		if dir != c.dir || name != c.name {
			t.Errorf("SplitDir(%q) = %q %q, want %q %q", c.path, dir, name, c.dir, c.name)
		}
	}
}

// A path under the home directory reads as ~, as every other tool writes it.
func TestSplitDir_AbbreviatesHome(t *testing.T) {
	t.Setenv("HOME", "/home/someone")

	if dir, _ := ui.SplitDir("/home/someone/.claude/settings.json"); dir != "~/.claude/" {
		t.Errorf("dir = %q, want ~/.claude/", dir)
	}
}

// A directory too wide for the list keeps its ends: the root says which tree it
// is in and the last component says where in it.
func TestElidePath(t *testing.T) {
	for _, c := range []struct {
		name  string
		path  string
		width int
		want  string
	}{
		{"it already fits", "internal/ui/", 20, "internal/ui/"},
		{"the middle goes", "internal/verylongdirectory/anotherone/", 22, "internal/…/anotherone/"},
		{"more of the middle goes", "a/b/c/d/e/last/", 10, "a/…/last/"},
		{"only the tail fits", "internal/verylongdirectory/anotherone/", 13, "…/anotherone/"},
		{"the tail is cut from its front", "internal/verylongdirectory/anotherone/", 12, "…anotherone/"},
		{"not even a component fits", "internal/verylongdirectory/", 8, "…ectory/"},
	} {
		if got := ui.ElidePath(c.path, c.width); got != c.want {
			t.Errorf("%s: ElidePath(%q, %d) = %q, want %q", c.name, c.path, c.width, got, c.want)
		}
		if got := ui.VisibleWidth(ui.ElidePath(c.path, c.width)); got > c.width {
			t.Errorf("%s: result is %d cells wide, want at most %d", c.name, got, c.width)
		}
	}
}
