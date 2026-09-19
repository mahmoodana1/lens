package capture_test

import (
	"testing"

	"github.com/mahmood/lens/internal/capture"
)

func TestHiddenPath(t *testing.T) {
	const root = "/home/me/proj"
	for _, c := range []struct {
		name string
		path string
		want bool
	}{
		{"an ordinary file", root + "/main.go", false},
		{"an ordinary file in a subdirectory", root + "/src/app/main.go", false},
		{"a dot-file at the top", root + "/.gitignore", true},
		{"a dot-file deeper in", root + "/src/.env", true},
		{"inside a dot-directory", root + "/.git/config", true},
		{"inside an unlisted dot-directory", root + "/.claude/settings.json", true},
		{"deep inside a dot-directory", root + "/.venv/lib/python3/site.py", true},
		{"a dot-directory in the middle", root + "/src/.cache/blob", true},
		{"a file whose name merely contains a dot", root + "/main.test.go", false},
		{"a file outside the project, in the open", "/tmp/scratch/notes.txt", false},
		{"a file outside the project, hidden", "/home/me/.claude/settings.json", true},
	} {
		if got := capture.HiddenPath(root, c.path); got != c.want {
			t.Errorf("%s: HiddenPath(%q) = %v, want %v", c.name, c.path, got, c.want)
		}
	}
}

// A project that itself lives under a hidden directory is not hidden from
// itself: the rule is about what is hidden inside the project, not where the
// project happens to sit.
func TestHiddenPath_JudgedFromTheProjectRoot(t *testing.T) {
	const root = "/home/me/.config/nvim"

	if capture.HiddenPath(root, root+"/init.lua") {
		t.Error("init.lua is an ordinary file of this project")
	}
	if capture.HiddenPath(root, root+"/lua/plugins/ui.lua") {
		t.Error("a plain subdirectory of this project is not hidden")
	}
	if !capture.HiddenPath(root, root+"/.git/config") {
		t.Error(".git is hidden wherever the project lives")
	}
}

// With no root recorded there is nothing to measure against, so the path is
// read as it stands.
func TestHiddenPath_WithoutARoot(t *testing.T) {
	if !capture.HiddenPath("", "/home/me/proj/.git/config") {
		t.Error("a dot-directory is still hidden with no root to compare against")
	}
	if capture.HiddenPath("", "/home/me/proj/main.go") {
		t.Error("an ordinary path is not hidden")
	}
}

func TestInside(t *testing.T) {
	const root = "/home/me/proj"
	for _, c := range []struct {
		name string
		path string
		want bool
	}{
		{"a file in the project", root + "/main.go", true},
		{"a file deep in the project", root + "/a/b/c.go", true},
		{"the project directory itself", root, true},
		{"a sibling with a shared prefix", "/home/me/proj-other/main.go", false},
		{"somewhere else entirely", "/tmp/scratch/x.txt", false},
		{"a parent of the project", "/home/me/notes.txt", false},
	} {
		if got := capture.Inside(root, c.path); got != c.want {
			t.Errorf("%s: Inside(%q, %q) = %v, want %v", c.name, root, c.path, got, c.want)
		}
	}
}

// With no root recorded there is nothing to be outside of, so nothing is.
func TestInside_WithoutARoot(t *testing.T) {
	if !capture.Inside("", "/anywhere/at/all") {
		t.Error("with no project root, a path cannot be judged outside it")
	}
}
