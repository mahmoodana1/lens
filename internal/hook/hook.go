// Package hook implements the three Claude Code hooks that feed the panel.
//
// Hooks run on every edit in every session, so they do bounded work — parse,
// append, return — and never block on the panel. Callers must exit 0 whatever
// these functions return: a capture failure must not disrupt a Claude session.
package hook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/pane"
	"github.com/mahmood/lens/internal/store"
)

// envelope is the part of every hook payload that identifies the session.
type envelope struct {
	SessionID string `json:"session_id"`
	PromptID  string `json:"prompt_id"`
	CWD       string `json:"cwd"`
	Prompt    string `json:"prompt"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// PostToolUse records what a tool changed and makes sure the panel is showing.
func PostToolUse(stdin io.Reader) error {
	raw, env, err := read(stdin)
	if err != nil {
		return err
	}

	// Shell commands describe nothing about the files they touch, so their
	// changes are found by looking at the project rather than the payload.
	if env.ToolName == "Bash" {
		return bashChanges(env)
	}

	rec, err := capture.ParsePostToolUse(raw)
	if err != nil {
		if errors.Is(err, capture.ErrNoPatch) {
			// Nothing to show for this call; not a failure.
			return nil
		}
		Debugf("post-tool-use: %v: %s", err, truncate(raw))
		return err
	}

	id := rec.SessionID
	if id == "" {
		id = env.SessionID
	}
	s, err := store.Open(id)
	if err != nil {
		Debugf("post-tool-use: open store: %v", err)
		return err
	}
	if err := s.EnsureMeta(rec.CWD); err != nil {
		Debugf("post-tool-use: meta: %v", err)
	}
	// The payload's cwd is wherever the agent is now; the session's root is
	// where it began. Shortening against the root keeps one file one name.
	root := s.ProjectRoot()
	if root == "" {
		root = rec.CWD
	}
	if capture.HiddenPath(root, rec.Event.Path) {
		return nil // machinery, not work to read
	}
	rec.Event.Rel = capture.RelativeTo(root, rec.Event.Path)
	if err := s.Append(rec.Event); err != nil {
		Debugf("post-tool-use: append: %v", err)
		return err
	}

	// The scan that follows the next shell command must not report this again.
	if err := noteIndexed(s, rec.Event.Path); err != nil {
		Debugf("post-tool-use: note: %v", err)
	}

	return nil
}

// Prompt records what was asked, so each edit can show the request behind it.
func Prompt(stdin io.Reader) error {
	_, env, err := read(stdin)
	if err != nil {
		return err
	}
	s, err := store.Open(env.SessionID)
	if err != nil {
		Debugf("prompt: open store: %v", err)
		return err
	}
	if env.CWD != "" {
		s.EnsureMeta(env.CWD)
	}
	if err := s.RecordPrompt(env.PromptID, env.Prompt); err != nil {
		Debugf("prompt: record: %v", err)
		return err
	}
	return nil
}

// SessionEnd marks the session finished. The panel keeps rendering so the
// session can still be read afterwards.
func SessionEnd(stdin io.Reader) error {
	_, env, err := read(stdin)
	if err != nil {
		return err
	}
	s, err := store.Open(env.SessionID)
	if err != nil {
		Debugf("session-end: open store: %v", err)
		return err
	}
	if err := s.MarkEnded(); err != nil {
		Debugf("session-end: mark: %v", err)
		return err
	}
	return nil
}

func read(stdin io.Reader) ([]byte, envelope, error) {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		Debugf("read stdin: %v", err)
		return nil, envelope{}, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		Debugf("unmarshal payload: %v: %s", err, truncate(raw))
		return raw, envelope{}, fmt.Errorf("hook: unmarshal payload: %w", err)
	}
	return raw, env, nil
}

// Debugf appends to a shared log. Anything the hooks cannot make sense of ends
// up here rather than failing a session.
func Debugf(format string, args ...any) {
	root := store.Root()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(root, "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// self is the path to this binary, used when spawning the panel.
func self() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "lens"
}

func truncate(b []byte) string {
	const max = 400
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}

// Stop opens the panel when Claude finishes a turn.
//
// Edits are recorded silently while Claude works and shown once, at the end: a
// popup holds the keyboard, so opening one mid-turn would take the session away
// from whoever is still talking to it.
func Stop(stdin io.Reader) error {
	_, env, err := read(stdin)
	if err != nil {
		return err
	}
	s, err := store.Open(env.SessionID)
	if err != nil {
		Debugf("stop: open store: %v", err)
		return err
	}
	sess, err := s.Read()
	if err != nil {
		Debugf("stop: read session: %v", err)
		return err
	}
	announced, err := s.Announced()
	if err != nil {
		Debugf("stop: read announced: %v", err)
	}

	project := sess.Meta.CWD
	if project == "" {
		project = env.CWD
	}

	seq, ok := shouldOpen(project, sess.Events, announced, store.AutoOpen(project))
	if !ok {
		return nil
	}

	// A popup is drawn on its client's screen rather than inside the pane it
	// names, so opening one for a pane nobody is watching puts it over whatever
	// they are watching instead — mid-turn, showing another project. Nothing is
	// announced in that case: the edits are shown the next time a turn ends
	// with the reader here, rather than passed over for good.
	if !pane.WatchingHere() {
		Debugf("stop: nobody is watching this pane; leaving %d unannounced", seq)
		return nil
	}

	// Recorded before opening, not after: a turn is announced once whether or
	// not a popup could actually be shown, so a missing tmux cannot turn every
	// later turn into a fresh attempt.
	if err := s.Announce(seq); err != nil {
		Debugf("stop: announce: %v", err)
	}

	// Showing the panel is best effort; a missing panel must not fail a turn.
	notePaneFailure(s, pane.Open(env.SessionID, self()))
	return nil
}

// shouldOpen reports whether a finished turn has anything new to show, and the
// edit to record as announced if so.
//
// The test is "is there an edit the panel has not been opened for", not "does
// this session have any edits at all" — otherwise the first edit of a session
// would reopen the panel at the end of every later turn, including the ones
// that only answered a question.
//
// It asks only about the edits the panel would draw. Counting everything
// recorded is what opened empty popups: the file walk records a shell
// command's writes outside the project, the panel hides them, and a turn whose
// only writes were those would take the keyboard to show nothing. In one real
// session 375 of 661 events were of that kind.
func shouldOpen(root string, events []capture.Event, announced int, autoOpen bool) (int, bool) {
	if !autoOpen || len(events) == 0 {
		return 0, false
	}
	latest := capture.LastShowable(root, events)
	if latest == 0 || latest <= announced {
		return 0, false
	}
	return latest, true
}
