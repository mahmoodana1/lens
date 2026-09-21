package pane_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mahmood/lens/internal/pane"
	"github.com/mahmood/lens/internal/store"
)

// startTestTmux brings up a detached tmux server on a private socket and
// returns the socket path. The server dies with the test.
func startTestTmux(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sock := filepath.Join(t.TempDir(), "sock")
	cmd := exec.Command("tmux", "-S", sock, "new-session", "-d", "-x", "200", "-y", "50", "sh", "-c", "sleep 60")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot start tmux: %v: %s", err, out)
	}
	t.Cleanup(func() { exec.Command("tmux", "-S", sock, "kill-server").Run() })
	return sock
}

// attachClient gives the test server a client. tmux refuses to show a popup
// with none attached ("no current client"), and a client needs a terminal, so
// one is borrowed from script(1).
func attachClient(t *testing.T, sock string) {
	t.Helper()
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script(1) not installed; cannot attach a tmux client")
	}
	cmd := exec.Command("script", "-qfc", "tmux -S "+sock+" attach", "/dev/null")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot attach a client: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := exec.Command("tmux", "-S", sock, "list-clients", "-F", "#{client_name}").Output()
		if strings.TrimSpace(string(out)) != "" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Skip("no tmux client attached within 5s; this environment has no usable terminal")
}

func firstPaneID(t *testing.T, sock string) string {
	t.Helper()
	out, err := exec.Command("tmux", "-S", sock, "list-panes", "-a", "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")[0]
}

// writeScript creates an executable stand-in for the lens binary. A popup
// leaves no pane behind to inspect, so what it ran is observed through the
// file the script touches.
func writeScript(t *testing.T, body string) (path, marker string) {
	t.Helper()
	dir := t.TempDir()
	marker = filepath.Join(dir, "opened")
	path = filepath.Join(dir, "fake-lens")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + marker + "\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, marker
}

// waitForMarker polls for the script's marker file and returns its contents.
func waitForMarker(t *testing.T, marker string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(marker); err == nil {
			return strings.TrimSpace(string(b))
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("popup never ran: %s was not created", marker)
	return ""
}

// The panel runs inside a tmux popup, which takes the keyboard by construction.
func TestOpen_RunsThePanelInAPopup(t *testing.T) {
	sock := startTestTmux(t)
	attachClient(t, sock)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", sock+",0,0")
	t.Setenv("TMUX_PANE", firstPaneID(t, sock))
	store.Open("s1")

	script, marker := writeScript(t, "")

	if err := pane.Open("s1", script); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := waitForMarker(t, marker); got != "--session s1" {
		t.Errorf("panel args = %q, want %q", got, "--session s1")
	}
}

// display-popup blocks its caller for as long as the popup lives. A hook that
// waited on it would freeze the session until the popup was dismissed, so the
// spawn must be detached.
func TestOpen_ReturnsWhileThePopupIsStillOpen(t *testing.T) {
	sock := startTestTmux(t)
	attachClient(t, sock)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", sock+",0,0")
	t.Setenv("TMUX_PANE", firstPaneID(t, sock))
	store.Open("s2")

	script, marker := writeScript(t, "sleep 30")

	start := time.Now()
	if err := pane.Open("s2", script); err != nil {
		t.Fatalf("Open: %v", err)
	}
	elapsed := time.Since(start)

	waitForMarker(t, marker) // the popup really is open and still running
	if elapsed > 2*time.Second {
		t.Errorf("Open blocked for %v; it must not wait on the popup", elapsed)
	}
}

// With no tmux to host a popup, capture carries on and nothing is spawned.
func TestOpen_ReportsWhenThereIsNoTmux(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("PATH", t.TempDir()) // no tmux binary to find
	store.Open("s3")

	err := pane.Open("s3", "/bin/true")
	if !errors.Is(err, pane.ErrNoTmux) {
		t.Errorf("err = %v, want ErrNoTmux", err)
	}
}

// tmux runs a key binding's command from the server: it inherits $TMUX but no
// $TMUX_PANE, and has no controlling terminal. The binding that reopens the
// panel arrives this way, so the pane has to be found without either.
func TestOpen_FindsThePaneWithoutTmuxPaneOrATTY(t *testing.T) {
	sock := startTestTmux(t)
	attachClient(t, sock)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", sock+",0,0")
	t.Setenv("TMUX_PANE", "")
	store.Open("s4")

	script, marker := writeScript(t, "")

	if err := pane.Open("s4", script); err != nil {
		t.Fatalf("Open: %v", err)
	}
	waitForMarker(t, marker)
}

// A key binding that reopens the panel says nothing about which project it was
// pressed in — tmux runs it from the server, whose directory is its own. The
// pane the popup will cover is the one the reader is looking at, and tmux knows
// what it is working in, so that is where the project comes from.
func TestCurrentPath_IsThePanesWorkingDirectory(t *testing.T) {
	proj := t.TempDir()
	sock := startTestTmuxIn(t, proj)
	attachClient(t, sock)
	t.Setenv("TMUX", sock+",0,0")
	t.Setenv("TMUX_PANE", firstPaneID(t, sock))

	got := pane.CurrentPath()

	want, _ := filepath.EvalSymlinks(proj)
	if got != want {
		t.Errorf("CurrentPath = %q, want %q", got, want)
	}
}

// With no tmux there is no pane to ask, and no project either. Saying so is
// how the caller knows to fall back.
func TestCurrentPath_EmptyWithoutTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("PATH", t.TempDir())

	if got := pane.CurrentPath(); got != "" {
		t.Errorf("CurrentPath = %q, want empty", got)
	}
}

// startTestTmuxIn is startTestTmux with the session started in a directory.
func startTestTmuxIn(t *testing.T, dir string) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sock := filepath.Join(t.TempDir(), "sock")
	cmd := exec.Command("tmux", "-S", sock, "new-session", "-d", "-c", dir, "-x", "200", "-y", "50", "sh", "-c", "sleep 60")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot start tmux: %v: %s", err, out)
	}
	t.Cleanup(func() { exec.Command("tmux", "-S", sock, "kill-server").Run() })
	return sock
}

// The editor is usually in another window — nvim in one, Claude in another —
// so handing it a file is only half the job: the reader has to be taken there.
func TestEditorPane_FindsTheWindowRunningAnEditor(t *testing.T) {
	proj := t.TempDir()
	sock := startTestTmux(t)
	t.Setenv("TMUX", sock+",0,0")
	t.Setenv("TMUX_PANE", firstPaneID(t, sock))
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not installed")
	}

	// A second window, running an editor in the project.
	if out, err := exec.Command("tmux", "-S", sock, "new-window", "-c", proj, "nvim", "-u", "NONE").CombinedOutput(); err != nil {
		t.Skipf("cannot open a window: %v: %s", err, out)
	}
	waitForCommand(t, sock, "nvim")

	id, ok := pane.EditorPane(proj)
	if !ok {
		t.Fatal("no editor pane found")
	}
	if id == "" || id == firstPaneID(t, sock) {
		t.Errorf("pane = %q, want the one running the editor", id)
	}
}

// An editor open on somebody else's project is not this project's editor.
func TestEditorPane_IgnoresAnEditorElsewhere(t *testing.T) {
	proj, other := t.TempDir(), t.TempDir()
	sock := startTestTmux(t)
	t.Setenv("TMUX", sock+",0,0")
	t.Setenv("TMUX_PANE", firstPaneID(t, sock))
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not installed")
	}

	if out, err := exec.Command("tmux", "-S", sock, "new-window", "-c", other, "nvim", "-u", "NONE").CombinedOutput(); err != nil {
		t.Skipf("cannot open a window: %v: %s", err, out)
	}
	waitForCommand(t, sock, "nvim")

	if id, ok := pane.EditorPane(proj); ok {
		t.Errorf("found %q for a project no editor is on", id)
	}
}

// With no editor running, one is started in a window of its own.
func TestNewWindow_StartsAProgramInTheProject(t *testing.T) {
	proj := t.TempDir()
	sock := startTestTmux(t)
	t.Setenv("TMUX", sock+",0,0")
	t.Setenv("TMUX_PANE", firstPaneID(t, sock))

	marker := filepath.Join(proj, "ran")
	if err := pane.NewWindow(proj, "sh", "-c", "pwd > "+marker+"; sleep 5"); err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	got := strings.TrimSpace(waitForMarker(t, marker))
	want, _ := filepath.EvalSymlinks(proj)
	if real, _ := filepath.EvalSymlinks(got); real != want {
		t.Errorf("ran in %q, want the project directory %q", got, want)
	}
}

// waitForCommand blocks until some pane reports running cmd.
func waitForCommand(t *testing.T, sock, cmd string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := exec.Command("tmux", "-S", sock, "list-panes", "-a", "-F", "#{pane_current_command}").Output()
		if strings.Contains(string(out), cmd) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Skipf("%s never started in a pane", cmd)
}
