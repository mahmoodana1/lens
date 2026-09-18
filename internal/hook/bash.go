package hook

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/difftext"
	"github.com/mahmood/lens/internal/fsindex"
	"github.com/mahmood/lens/internal/pane"
	"github.com/mahmood/lens/internal/store"
)

// SessionStart establishes what the project looked like before Claude touched
// it, so the first shell command of a session produces a real diff rather than
// a file that appears from nowhere.
func SessionStart(stdin io.Reader) error {
	_, env, err := read(stdin)
	if err != nil {
		return err
	}
	s, err := store.Open(env.SessionID)
	if err != nil {
		Debugf("session-start: open store: %v", err)
		return err
	}
	if err := s.EnsureMeta(env.CWD); err != nil {
		Debugf("session-start: meta: %v", err)
	}

	return withIndex(s, func(ix *fsindex.Index) error {
		ix.AddRoot(env.CWD)
		ix.Rescan(time.Now()) // the first scan is the baseline and reports nothing
		return nil
	})
}

// bashChanges finds what a shell command changed on disk and records it.
func bashChanges(env envelope) error {
	s, err := store.Open(env.SessionID)
	if err != nil {
		Debugf("bash: open store: %v", err)
		return err
	}
	if err := s.EnsureMeta(env.CWD); err != nil {
		Debugf("bash: meta: %v", err)
	}

	// Files older than the session belong to the project, not to Claude.
	since := time.Now()
	if meta, err := s.Read(); err == nil && !meta.Meta.Started.IsZero() {
		since = meta.Meta.Started
	}

	var changes []fsindex.Change
	err = withIndex(s, func(ix *fsindex.Index) error {
		ix.AddRoot(env.CWD)
		// A session started outside a project finds it through the paths the
		// command names — that is how a just-created project gets watched.
		ix.SeedFromCommand(env.ToolInput.Command)
		changes = ix.Rescan(since)
		return nil
	})
	if err != nil {
		Debugf("bash: index: %v", err)
		return err
	}
	if len(changes) == 0 {
		return nil
	}

	for _, c := range changes {
		hunks := difftext.Hunks(c.Old, c.New)
		if len(hunks) == 0 {
			continue
		}
		added, removed := difftext.Counts(hunks)
		e := capture.Event{
			Time:     time.Now(),
			Tool:     "Bash",
			Path:     c.Path,
			Rel:      relativeTo(env.CWD, c.Path),
			Kind:     c.Kind,
			PromptID: env.PromptID,
			Hunks:    hunks,
			Added:    added,
			Removed:  removed,
		}
		if err := s.Append(e); err != nil {
			Debugf("bash: append: %v", err)
		}
	}

	notePaneFailure(s, pane.Ensure(env.SessionID, self()))
	return nil
}

// notePaneFailure records why the panel did not open, once per session.
// Without a panel the capture is invisible, so the reason must be findable —
// but it must not repeat on every command either.
func notePaneFailure(s *store.Store, err error) {
	if err == nil {
		return
	}
	marker := filepath.Join(s.Dir(), "pane-unavailable")
	if _, statErr := os.Stat(marker); statErr == nil {
		return
	}
	os.WriteFile(marker, []byte(err.Error()+"\n"), 0o600)
	Debugf("panel not opened: %v (TMUX=%q TMUX_PANE=%q)",
		err, os.Getenv("TMUX"), os.Getenv("TMUX_PANE"))
}

// noteIndexed records a tool-reported change in the index, so the next scan
// does not rediscover it.
func noteIndexed(s *store.Store, path string) error {
	if path == "" {
		return nil
	}
	dir := filepath.Join(s.Dir(), "index")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	return fsindex.Note(dir, path)
}

// withIndex runs fn against the session's file index under an exclusive lock.
// Claude runs shell commands in parallel, and two scans must not interleave.
func withIndex(s *store.Store, fn func(*fsindex.Index) error) error {
	dir := filepath.Join(s.Dir(), "index")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	lock, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	ix, err := fsindex.Load(dir)
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}
	if err := fn(ix); err != nil {
		return err
	}
	return ix.Save()
}

// relativeTo shortens a path against the project root when it sits inside it.
func relativeTo(root, path string) string {
	if root == "" || path == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
