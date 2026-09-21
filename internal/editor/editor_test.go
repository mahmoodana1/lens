package editor_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mahmood/lens/internal/editor"
)

// startNvim brings up a headless nvim listening on its own socket, in a
// directory of its own. It dies with the test. Never the reader's own editor.
func startNvim(t *testing.T, dir string) string {
	t.Helper()
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not installed")
	}
	sock := filepath.Join(t.TempDir(), "nvim.sock")
	cmd := exec.Command("nvim", "--headless", "--listen", sock, "-u", "NONE")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start nvim: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			if out, err := exec.Command("nvim", "--server", sock, "--remote-expr", "1").Output(); err == nil && strings.TrimSpace(string(out)) == "1" {
				return sock
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Skip("nvim did not come up in time")
	return ""
}

func ask(t *testing.T, sock, expr string) string {
	t.Helper()
	out, err := exec.Command("nvim", "--server", sock, "--remote-expr", expr).Output()
	if err != nil {
		t.Fatalf("asking nvim %q: %v", expr, err)
	}
	return strings.TrimSpace(string(out))
}

// The editor already open on the project is the one to use, and the file lands
// in it as a buffer at the right line.
func TestJump_OpensTheFileInTheRunningEditor(t *testing.T) {
	proj := t.TempDir()
	file := filepath.Join(proj, "main.go")
	if err := os.WriteFile(file, []byte("a\nb\nc\nd\ne\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := startNvim(t, proj)

	if err := editor.JumpTo(editor.Server{Addr: sock}, file, 4); err != nil {
		t.Fatal(err)
	}

	if got := ask(t, sock, `expand("%:p")`); got != file {
		t.Errorf("open buffer = %q, want %q", got, file)
	}
	if got := ask(t, sock, `line(".")`); got != "4" {
		t.Errorf("cursor on line %s, want 4", got)
	}
}

// Asked for the same file twice, the second jump moves within the buffer it
// already has rather than stacking another copy of it.
func TestJump_ReusesABufferItAlreadyHas(t *testing.T) {
	proj := t.TempDir()
	file := filepath.Join(proj, "main.go")
	if err := os.WriteFile(file, []byte("a\nb\nc\nd\ne\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := startNvim(t, proj)

	if err := editor.JumpTo(editor.Server{Addr: sock}, file, 2); err != nil {
		t.Fatal(err)
	}
	before := ask(t, sock, `len(getbufinfo({"buflisted":1}))`)

	if err := editor.JumpTo(editor.Server{Addr: sock}, file, 5); err != nil {
		t.Fatal(err)
	}

	if after := ask(t, sock, `len(getbufinfo({"buflisted":1}))`); after != before {
		t.Errorf("buffers went from %s to %s; the file was already open", before, after)
	}
	if got := ask(t, sock, `line(".")`); got != "5" {
		t.Errorf("cursor on line %s, want 5", got)
	}
}

// A path with a space in it is still a path.
func TestJump_HandlesAwkwardPaths(t *testing.T) {
	proj := t.TempDir()
	dir := filepath.Join(proj, "a dir")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "my file.go")
	if err := os.WriteFile(file, []byte("x\ny\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := startNvim(t, proj)

	if err := editor.JumpTo(editor.Server{Addr: sock}, file, 2); err != nil {
		t.Fatal(err)
	}
	if got := ask(t, sock, `expand("%:p")`); got != file {
		t.Errorf("open buffer = %q, want %q", got, file)
	}
}

// Of several editors running, the one working in this project is the one meant.
func TestServers_PicksTheOneOnThisProject(t *testing.T) {
	mine, other := t.TempDir(), t.TempDir()
	mySock := startNvim(t, mine)
	startNvim(t, other)

	got, err := editor.For(mine, []string{mySock, filepath.Join(other, "nope.sock")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Addr != mySock {
		t.Errorf("chose %q, want the editor on this project", got.Addr)
	}
}

// A subdirectory of the project is still the project.
func TestServers_MatchesFromASubdirectory(t *testing.T) {
	proj := t.TempDir()
	sub := filepath.Join(proj, "internal", "ui")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	sock := startNvim(t, proj)

	got, err := editor.For(sub, []string{sock})
	if err != nil {
		t.Fatal(err)
	}
	if got.Addr != sock {
		t.Errorf("chose %q, want %q", got.Addr, sock)
	}
}

// A socket left behind by a dead editor is skipped, not fatal.
func TestServers_IgnoresDeadSockets(t *testing.T) {
	proj := t.TempDir()
	dead := filepath.Join(t.TempDir(), "stale.sock")
	if err := os.WriteFile(dead, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sock := startNvim(t, proj)

	got, err := editor.For(proj, []string{dead, sock})
	if err != nil {
		t.Fatal(err)
	}
	if got.Addr != sock {
		t.Errorf("chose %q, want the live one", got.Addr)
	}
}

// Nothing running on this project is a plain answer, not an error to swallow.
func TestServers_NoneForThisProject(t *testing.T) {
	other := t.TempDir()
	sock := startNvim(t, other)

	if got, err := editor.For(t.TempDir(), []string{sock}); err == nil {
		t.Errorf("found %q, want none", got.Addr)
	}
}

// Show is the whole gesture: find the editor on this project, put the file in
// it, and bring it to the front. With none running, one is started.
func TestShow_UsesTheRunningEditor(t *testing.T) {
	proj := t.TempDir()
	file := filepath.Join(proj, "main.go")
	if err := os.WriteFile(file, []byte("a\nb\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := startNvim(t, proj)

	started := false
	err := editor.Show(editor.Request{
		Project: proj, File: file, Line: 3,
		Sockets: []string{sock},
		Reveal:  func(string) error { return nil },
		Start:   func(string, ...string) error { started = true; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if started {
		t.Error("started a second editor while one was already open")
	}
	if got := ask(t, sock, `line(".")`); got != "3" {
		t.Errorf("cursor on line %s, want 3", got)
	}
}

// With nothing open on the project, the file is opened in a window of its own —
// rooted a level above the file, not at the session's project, which can be
// several levels further up than the work.
func TestShow_StartsAnEditorWhenNoneIsOpen(t *testing.T) {
	proj := t.TempDir()
	sub := filepath.Join(proj, "tools", "reporter")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "main.go")
	if err := os.WriteFile(file, []byte("a\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var dir string
	var argv []string
	err := editor.Show(editor.Request{
		Project: proj, File: file, Line: 2,
		Sockets: nil,
		Reveal:  func(string) error { return nil },
		Start:   func(d string, a ...string) error { dir, argv = d, a; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(proj, "tools"); dir != want {
		t.Errorf("started in %q, want %q: one level above the file", dir, want)
	}
	want := []string{"nvim", "+2", file}
	if strings.Join(argv, " ") != strings.Join(want, " ") {
		t.Errorf("ran %v, want %v", argv, want)
	}
}

// The session's project can be far wider than the work: a Claude session
// started in the home directory that goes on to create ~/bashkit. The editor
// is then rooted deeper than the project, and asking "does the editor's
// directory contain the project" answers no — so every press started another
// editor instead of reusing the one already open.
func TestServers_FindsAnEditorRootedDeeperThanTheProject(t *testing.T) {
	home := t.TempDir() // stands in for the session's project
	proj := filepath.Join(home, "bashkit")
	if err := os.MkdirAll(filepath.Join(proj, "test"), 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(proj, "test", "suite.bats")
	if err := os.WriteFile(file, []byte("echo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := startNvim(t, proj) // the editor sits at ~/bashkit, below home

	got, err := editor.For(file, []string{sock})
	if err != nil {
		t.Fatalf("no editor found for %s: %v", file, err)
	}
	if got.Addr != sock {
		t.Errorf("chose %q, want the editor the file lives under", got.Addr)
	}
}

// An editor on somebody else's project is not the one to hand the file to,
// even when both sit under the same wide session directory.
func TestServers_IgnoresASiblingProjectsEditor(t *testing.T) {
	home := t.TempDir()
	mine := filepath.Join(home, "bashkit")
	theirs := filepath.Join(home, "something-else")
	for _, d := range []string{mine, theirs} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(mine, "main.sh")
	if err := os.WriteFile(file, []byte("echo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := startNvim(t, theirs)

	if got, err := editor.For(file, []string{sock}); err == nil {
		t.Errorf("handed the file to %q, an editor on another project", got.CWD)
	}
}

// The whole gesture, from the situation that broke it: the session's project is
// the home directory, the editor is rooted at ~/bashkit below it, and the file
// is inside that. Nothing new should be started.
func TestShow_ReusesAnEditorRootedDeeperThanTheProject(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, "bashkit")
	if err := os.MkdirAll(filepath.Join(proj, "test"), 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(proj, "test", "suite.bats")
	if err := os.WriteFile(file, []byte("a\nb\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := startNvim(t, proj)

	started := false
	err := editor.Show(editor.Request{
		Project: home, File: file, Line: 3,
		Sockets: []string{sock},
		Reveal:  func(string) error { return nil },
		Start:   func(string, ...string) error { started = true; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if started {
		t.Error("started another editor while one was already open on the file")
	}
	if got := ask(t, sock, `line(".")`); got != "3" {
		t.Errorf("cursor on line %s, want 3", got)
	}
}
