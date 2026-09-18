package pane_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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

func countPanes(t *testing.T, sock string) int {
	t.Helper()
	out, err := exec.Command("tmux", "-S", sock, "list-panes", "-a", "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

func firstPaneID(t *testing.T, sock string) string {
	t.Helper()
	out, err := exec.Command("tmux", "-S", sock, "list-panes", "-a", "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")[0]
}

func TestEnsure_NoopOutsideTmux(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	store.Open("s1")

	if err := pane.Ensure("s1", "/bin/true"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir("s1"), store.PaneFile)); !os.IsNotExist(err) {
		t.Error("pane file created outside tmux")
	}
}

// Claude edits files in parallel; only one panel may be spawned.
func TestEnsure_ConcurrentCallsSpawnOnce(t *testing.T) {
	sock := startTestTmux(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", sock+",0,0")
	t.Setenv("TMUX_PANE", firstPaneID(t, sock))
	store.Open("s2")

	before := countPanes(t, sock)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pane.Ensure("s2", "/bin/cat")
		}()
	}
	wg.Wait()

	if got := countPanes(t, sock); got != before+1 {
		t.Errorf("panes = %d, want %d (exactly one spawned)", got, before+1)
	}
	b, err := os.ReadFile(filepath.Join(store.Dir("s2"), store.PaneFile))
	if err != nil {
		t.Fatalf("pane file not written: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(b)), "%") {
		t.Errorf("pane file = %q, want a tmux pane id", b)
	}
}

// A second edit must reuse the existing panel rather than stacking panes.
func TestEnsure_SecondCallReusesPane(t *testing.T) {
	sock := startTestTmux(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", sock+",0,0")
	t.Setenv("TMUX_PANE", firstPaneID(t, sock))
	store.Open("s3")

	before := countPanes(t, sock)
	if err := pane.Ensure("s3", "/bin/cat"); err != nil {
		t.Fatal(err)
	}
	if err := pane.Ensure("s3", "/bin/cat"); err != nil {
		t.Fatal(err)
	}

	if got := countPanes(t, sock); got != before+1 {
		t.Errorf("panes = %d, want %d", got, before+1)
	}
}
