package hook_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/fsindex"
	"github.com/mahmood/lens/internal/hook"
	"github.com/mahmood/lens/internal/store"
)

func fixtureFor(t *testing.T, event, tool string) []byte {
	t.Helper()
	b, err := os.ReadFile("../capture/testdata/probe-payloads.jsonl")
	if err != nil {
		t.Fatalf("reading fixtures: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var rec struct {
			Event string          `json:"__event"`
			Raw   json.RawMessage `json:"__raw"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil || rec.Event != event {
			continue
		}
		var raw map[string]any
		if json.Unmarshal(rec.Raw, &raw) != nil {
			continue
		}
		if tool == "" || raw["tool_name"] == tool {
			return rec.Raw
		}
	}
	t.Fatalf("no fixture for %s/%s", event, tool)
	return nil
}

func sessionIDOf(t *testing.T, raw []byte) string {
	t.Helper()
	var p struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	return p.SessionID
}

func TestPostToolUse_AppendsEvent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	raw := fixtureFor(t, "PostToolUse", "Edit")

	if err := hook.PostToolUse(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open(sessionIDOf(t, raw))
	got, _ := s.Read()
	if len(got.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(got.Events))
	}
	if got.Events[0].Rel == "" {
		t.Error("rel path not set")
	}
	if got.Meta.CWD == "" {
		t.Error("meta not written")
	}
}

func TestPrompt_RecordsText(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	raw := fixtureFor(t, "UserPromptSubmit", "")

	if err := hook.Prompt(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open(sessionIDOf(t, raw))
	got, _ := s.Read()
	if len(got.Prompts) != 1 {
		t.Fatalf("prompts = %d, want 1", len(got.Prompts))
	}
	for _, text := range got.Prompts {
		if !strings.Contains(text, "three things") {
			t.Errorf("prompt text = %q", text)
		}
	}
}

// An edit and the prompt that caused it are joined by prompt_id.
func TestPromptAndEditShareAPromptID(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	promptRaw := fixtureFor(t, "UserPromptSubmit", "")
	editRaw := fixtureFor(t, "PostToolUse", "Edit")

	if err := hook.Prompt(bytes.NewReader(promptRaw)); err != nil {
		t.Fatal(err)
	}
	if err := hook.PostToolUse(bytes.NewReader(editRaw)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open(sessionIDOf(t, editRaw))
	got, _ := s.Read()
	if len(got.Events) != 1 {
		t.Fatalf("events = %d", len(got.Events))
	}
	id := got.Events[0].PromptID
	if id == "" {
		t.Fatal("event has no prompt id")
	}
	if _, ok := got.Prompts[id]; !ok {
		t.Errorf("prompt %q not found; the intent line would be blank", id)
	}
}

func TestSessionEnd_MarksEnded(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	raw := fixtureFor(t, "SessionEnd", "")

	if err := hook.SessionEnd(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open(sessionIDOf(t, raw))
	got, _ := s.Read()
	if !got.Meta.Ended {
		t.Error("session not marked ended")
	}
}

func TestPostToolUse_GarbageIsLoggedNotFatal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	err := hook.PostToolUse(strings.NewReader("not json"))
	if err == nil {
		t.Error("want an error returned so the caller can log it")
	}

	b, readErr := os.ReadFile(filepath.Join(dir, "lens", "debug.log"))
	if readErr != nil {
		t.Fatalf("debug log not written: %v", readErr)
	}
	if !strings.Contains(string(b), "not json") {
		t.Error("payload not recorded to debug.log")
	}
}

// Tool calls with nothing to diff are skipped quietly, not reported as failures.
func TestPostToolUse_NoPatchIsNotAnError(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	raw := []byte(`{"session_id":"s","cwd":"/p","tool_name":"Edit","tool_response":{"type":"update","structuredPatch":[]}}`)

	if err := hook.PostToolUse(bytes.NewReader(raw)); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	s, _ := store.Open("s")
	got, _ := s.Read()
	if len(got.Events) != 0 {
		t.Errorf("events = %d, want 0", len(got.Events))
	}
}

// Claude often writes files with shell heredocs rather than the Write tool.
// Those changes must show up in the panel too.
func TestBash_CapturesHeredocWrites(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("TMUX", "")
	proj := t.TempDir()

	if err := os.WriteFile(filepath.Join(proj, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Session start establishes the baseline, before anything runs.
	start := payload(map[string]any{
		"hook_event_name": "SessionStart", "session_id": "bash1", "cwd": proj,
	})
	if err := hook.SessionStart(bytes.NewReader(start)); err != nil {
		t.Fatal(err)
	}

	// The command actually runs, then the hook fires.
	if err := os.WriteFile(filepath.Join(proj, "main.go"),
		[]byte("package main\n\nfunc main() { println(\"hi\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "helper.go"),
		[]byte("package main\n\nfunc help() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bash := payload(map[string]any{
		"hook_event_name": "PostToolUse", "session_id": "bash1", "cwd": proj,
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "cat > main.go <<'EOF'\n...\nEOF"},
	})
	if err := hook.PostToolUse(bytes.NewReader(bash)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open("bash1")
	got, _ := s.Read()
	if len(got.Events) != 2 {
		t.Fatalf("events = %d, want 2 (one modified, one created): %+v", len(got.Events), got.Events)
	}

	byRel := map[string]capture.Event{}
	for _, e := range got.Events {
		byRel[e.Rel] = e
	}
	if e, ok := byRel["main.go"]; !ok {
		t.Error("the modified file was not captured")
	} else if e.Kind != "update" || e.Added != 1 || e.Removed != 1 {
		t.Errorf("main.go = %s +%d -%d, want update +1 -1", e.Kind, e.Added, e.Removed)
	}
	if e, ok := byRel["helper.go"]; !ok {
		t.Error("the created file was not captured")
	} else if e.Kind != "create" {
		t.Errorf("helper.go kind = %q, want create", e.Kind)
	}
}

// A command that reads but writes nothing must not produce entries.
func TestBash_NoChangesNoEvents(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	proj := t.TempDir()
	os.WriteFile(filepath.Join(proj, "a.txt"), []byte("hello\n"), 0o644)

	start := payload(map[string]any{"hook_event_name": "SessionStart", "session_id": "bash2", "cwd": proj})
	hook.SessionStart(bytes.NewReader(start))

	bash := payload(map[string]any{
		"hook_event_name": "PostToolUse", "session_id": "bash2", "cwd": proj,
		"tool_name": "Bash", "tool_input": map[string]any{"command": "ls -la"},
	})
	if err := hook.PostToolUse(bytes.NewReader(bash)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open("bash2")
	got, _ := s.Read()
	if len(got.Events) != 0 {
		t.Errorf("events = %d, want 0 for a read-only command", len(got.Events))
	}
}

// Running from home, the project named in the command is what gets watched.
// A session started in a bare home directory has nothing to walk — home itself
// is too broad — so the project it goes on to create is found through the paths
// the command names. It still has to be under where the session started: that
// is the boundary, and the reason a command naming /tmp acquires nothing.
func TestBash_SeedsRootFromCommandWhenStartedFromHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	home := t.TempDir()
	t.Setenv("HOME", home) // home is refused as a root: too broad to walk
	proj := filepath.Join(home, "newproj")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}

	start := payload(map[string]any{"hook_event_name": "SessionStart", "session_id": "bash3", "cwd": home})
	hook.SessionStart(bytes.NewReader(start))

	os.WriteFile(filepath.Join(proj, "CMakeLists.txt"), []byte("project(x)\n"), 0o644)

	bash := payload(map[string]any{
		"hook_event_name": "PostToolUse", "session_id": "bash3", "cwd": home,
		"tool_name": "Bash",
		"tool_input": map[string]any{
			"command": "mkdir -p " + proj + "/src && cat > " + proj + "/CMakeLists.txt <<'EOF'\nproject(x)\nEOF",
		},
	})
	if err := hook.PostToolUse(bytes.NewReader(bash)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open("bash3")
	got, _ := s.Read()
	if len(got.Events) != 1 {
		t.Fatalf("events = %d, want 1; the project named in the command should be watched", len(got.Events))
	}
	if got.Events[0].Kind != "create" {
		t.Errorf("kind = %q, want create", got.Events[0].Kind)
	}
}

func payload(m map[string]any) []byte {
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

// An Edit tool call and the Bash scan must not both report the same change.
func TestEditThenBash_ReportsChangeOnce(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	proj := t.TempDir()
	path := filepath.Join(proj, "main.cpp")
	os.WriteFile(path, []byte("int main() { return 0; }\n"), 0o644)

	start := payload(map[string]any{"hook_event_name": "SessionStart", "session_id": "dup", "cwd": proj})
	if err := hook.SessionStart(bytes.NewReader(start)); err != nil {
		t.Fatal(err)
	}

	// Claude edits the file with the Edit tool; the hook gets a real patch.
	os.WriteFile(path, []byte("int main() { return 1; }\n"), 0o644)
	edit := payload(map[string]any{
		"hook_event_name": "PostToolUse", "session_id": "dup", "cwd": proj,
		"tool_name":  "Edit",
		"tool_input": map[string]any{"file_path": path},
		"tool_response": map[string]any{
			"filePath": path, "type": "update",
			"originalFile": "int main() { return 0; }\n",
			"structuredPatch": []map[string]any{{
				"oldStart": 1, "oldLines": 1, "newStart": 1, "newLines": 1,
				"lines": []string{"-int main() { return 0; }", "+int main() { return 1; }"},
			}},
		},
	})
	if err := hook.PostToolUse(bytes.NewReader(edit)); err != nil {
		t.Fatal(err)
	}

	// A later shell command scans the project and must not re-report that edit.
	bash := payload(map[string]any{
		"hook_event_name": "PostToolUse", "session_id": "dup", "cwd": proj,
		"tool_name": "Bash", "tool_input": map[string]any{"command": "ls"},
	})
	if err := hook.PostToolUse(bytes.NewReader(bash)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open("dup")
	got, _ := s.Read()
	if len(got.Events) != 1 {
		t.Errorf("events = %d, want 1; the same change was recorded twice: %+v",
			len(got.Events), summarize(got.Events))
	}
}

func summarize(events []capture.Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.Tool+" "+e.Rel)
	}
	return out
}

// noPanelMarker is where a failed attempt to open the panel is recorded. With
// no tmux reachable, its presence is how these tests see that lens tried.
func noPanelMarker(id string) string {
	return filepath.Join(store.Dir(id), "pane-unavailable")
}

// isolateFromTmux puts the tests out of reach of any real tmux server, so a
// stray popup can never land in the developer's own session.
func isolateFromTmux(t *testing.T) {
	t.Helper()
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("PATH", t.TempDir())
}

// Recording is silent. The panel opens when Claude stops, not part-way through
// an edit, so nothing interrupts a turn in progress.
func TestPostToolUse_DoesNotOpenThePanel(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	isolateFromTmux(t)
	raw := fixtureFor(t, "PostToolUse", "Edit")

	if err := hook.PostToolUse(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(noPanelMarker(sessionIDOf(t, raw))); err == nil {
		t.Error("PostToolUse tried to open the panel; it must only record")
	}
}

// Claude answered a question without touching a file: there is nothing to show.
func TestStop_WithNoEditsOpensNothing(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	isolateFromTmux(t)
	store.Open("s-quiet")

	if err := hook.Stop(bytes.NewReader(payload(map[string]any{"session_id": "s-quiet"}))); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(noPanelMarker("s-quiet")); err == nil {
		t.Error("panel opened for a session with no edits")
	}
}

// Claude finished and did change something, so the panel opens over the session.
func TestStop_WithEditsOpensThePanel(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	isolateFromTmux(t)
	raw := fixtureFor(t, "PostToolUse", "Edit")
	if err := hook.PostToolUse(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	id := sessionIDOf(t, raw)

	if err := hook.Stop(bytes.NewReader(payload(map[string]any{"session_id": id}))); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(noPanelMarker(id)); err != nil {
		t.Error("Stop did not try to open the panel for a session with edits")
	}
}

// Muting a project is the whole point of `lens auto off`: the turn still
// records everything, it just must not take the keyboard.
func TestStop_MutedProjectOpensNothing(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	isolateFromTmux(t)

	project := t.TempDir()
	s, err := store.Open("s-muted")
	if err != nil {
		t.Fatal(err)
	}
	s.EnsureMeta(project)
	s.Append(capture.Event{Rel: "a.go", Added: 1})

	if err := store.SetAutoOpen(project, false); err != nil {
		t.Fatal(err)
	}
	if err := hook.Stop(bytes.NewReader(payload(map[string]any{"session_id": "s-muted"}))); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(noPanelMarker("s-muted")); err == nil {
		t.Error("the panel opened for a project whose auto-open is off")
	}
}

// Turning it back on catches up: the edits made while it was off were never
// announced, so the next turn shows them rather than skipping them.
func TestStop_UnmutingShowsWhatWasMissed(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	isolateFromTmux(t)

	project := t.TempDir()
	s, err := store.Open("s-catchup")
	if err != nil {
		t.Fatal(err)
	}
	s.EnsureMeta(project)
	s.Append(capture.Event{Rel: "a.go", Added: 1})

	if err := store.SetAutoOpen(project, false); err != nil {
		t.Fatal(err)
	}
	stop := func() {
		if err := hook.Stop(bytes.NewReader(payload(map[string]any{"session_id": "s-catchup"}))); err != nil {
			t.Fatal(err)
		}
	}
	stop()

	if n, err := s.Announced(); err != nil || n != 0 {
		t.Fatalf("announced = %d (err %v) while muted, want 0 so the edit is not written off", n, err)
	}

	if err := store.SetAutoOpen(project, true); err != nil {
		t.Fatal(err)
	}
	stop()

	if _, err := os.Stat(noPanelMarker("s-catchup")); err != nil {
		t.Error("turning auto-open back on did not show the edit made while it was off")
	}
}

// The setting is keyed on the project, and the session knows which one it is
// even when the hook payload does not carry a cwd.
func TestStop_MutingFollowsTheSessionsProject(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	isolateFromTmux(t)

	muted, other := t.TempDir(), t.TempDir()
	if err := store.SetAutoOpen(muted, false); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open("s-other")
	if err != nil {
		t.Fatal(err)
	}
	s.EnsureMeta(other)
	s.Append(capture.Event{Rel: "a.go", Added: 1})

	if err := hook.Stop(bytes.NewReader(payload(map[string]any{"session_id": "s-other"}))); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(noPanelMarker("s-other")); err != nil {
		t.Errorf("muting %q also silenced %q", muted, other)
	}
}

// An agent that changes directory mid-session must not make one file look like
// two. The payload's cwd moves with it; the session's root does not, so that is
// what a path is shortened against.
func TestPostToolUse_PathsAreRelativeToTheSessionRoot(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	root := t.TempDir()
	sub := filepath.Join(root, "hangman")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "main.py")

	s, err := store.Open("cd")
	if err != nil {
		t.Fatal(err)
	}
	s.EnsureMeta(root) // the session started at the project root

	// The same file, edited after the agent moved into the subdirectory.
	raw := editPayload("cd", sub, file)
	if err := hook.PostToolUse(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	sess, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(sess.Events))
	}
	if got := sess.Events[0].Rel; got != "hangman/main.py" {
		t.Errorf("rel = %q, want hangman/main.py — relative to the session root", got)
	}
}

// editPayload builds a PostToolUse payload for one Edit, the shape Claude Code
// sends: a structured patch plus the file's prior contents.
func editPayload(sessionID, cwd, file string) []byte {
	return payload(map[string]any{
		"session_id":      sessionID,
		"cwd":             cwd,
		"hook_event_name": "PostToolUse",
		"tool_name":       "Edit",
		"tool_input":      map[string]any{"file_path": file},
		"tool_response": map[string]any{
			"filePath":        file,
			"originalFile":    "one\ntwo\n",
			"structuredPatch": []any{map[string]any{"oldStart": 1, "oldLines": 1, "newStart": 1, "newLines": 1, "lines": []string{"-one", "+ONE"}}},
		},
	})
}

// Shell-captured changes take the other code path into the log, and must name
// files the same way: against the session's root, not the shell's directory.
func TestBash_PathsAreRelativeToTheSessionRoot(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	isolateFromTmux(t)

	root := t.TempDir()
	sub := filepath.Join(root, "hangman")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	// The session begins at the project root, and the index takes its baseline.
	start := payload(map[string]any{"session_id": "cdbash", "cwd": root, "hook_event_name": "SessionStart"})
	if err := hook.SessionStart(bytes.NewReader(start)); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(sub, "main.py")
	if err := os.WriteFile(file, []byte("print()\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The command ran after the agent moved into the subdirectory.
	done := payload(map[string]any{
		"session_id": "cdbash", "cwd": sub, "hook_event_name": "PostToolUse",
		"tool_name": "Bash", "tool_input": map[string]any{"command": "python main.py"},
	})
	if err := hook.PostToolUse(bytes.NewReader(done)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open("cdbash")
	sess, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Events) == 0 {
		t.Fatal("the shell change was not captured at all")
	}
	if got := sess.Events[0].Rel; got != "hangman/main.py" {
		t.Errorf("rel = %q, want hangman/main.py — relative to the session root", got)
	}
}

// Edits made with the Edit and Write tools bypass the file walk entirely, so
// they need the same rule: nothing hidden is recorded.
func TestPostToolUse_RecordsNothingHidden(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()

	s, err := store.Open("dots")
	if err != nil {
		t.Fatal(err)
	}
	s.EnsureMeta(root)

	for _, rel := range []string{".gitignore", ".env", ".claude/settings.json", ".git/config", "src/.cache/blob"} {
		if err := hook.PostToolUse(bytes.NewReader(editPayload("dots", root, filepath.Join(root, rel)))); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
	}

	sess, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Events) != 0 {
		t.Errorf("recorded %d hidden edits, want none: %v", len(sess.Events), relsOf(sess))
	}

	// And an ordinary file still gets through.
	if err := hook.PostToolUse(bytes.NewReader(editPayload("dots", root, filepath.Join(root, "main.go")))); err != nil {
		t.Fatal(err)
	}
	if sess, _ := s.Read(); len(sess.Events) != 1 {
		t.Errorf("ordinary edits = %d, want 1", len(sess.Events))
	}
}

// A project that lives under a hidden directory is not hidden from itself.
func TestPostToolUse_RecordsAProjectThatLivesSomewhereHidden(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), ".config", "nvim")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open("nvim")
	s.EnsureMeta(root)

	if err := hook.PostToolUse(bytes.NewReader(editPayload("nvim", root, filepath.Join(root, "init.lua")))); err != nil {
		t.Fatal(err)
	}

	sess, _ := s.Read()
	if len(sess.Events) != 1 || sess.Events[0].Rel != "init.lua" {
		t.Errorf("recorded %v, want init.lua", relsOf(sess))
	}
}

func relsOf(sess store.Session) []string {
	out := []string{}
	for _, e := range sess.Events {
		out = append(out, e.Rel)
	}
	return out
}

// A shell command that names a path elsewhere must not make the walk follow it
// there. Claude Code's own temp directories churn constantly; the panel is for
// your project.
func TestBash_DoesNotFollowCommandsOutOfTheProject(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	isolateFromTmux(t)

	root := t.TempDir()
	elsewhere := t.TempDir()

	start := payload(map[string]any{"session_id": "stray", "cwd": root, "hook_event_name": "SessionStart"})
	if err := hook.SessionStart(bytes.NewReader(start)); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(elsewhere, "noise.txt"), []byte("churn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The command names the outside directory, which is how the walk used to
	// acquire it as a root.
	done := payload(map[string]any{
		"session_id": "stray", "cwd": root, "hook_event_name": "PostToolUse",
		"tool_name": "Bash", "tool_input": map[string]any{"command": "cat " + filepath.Join(elsewhere, "noise.txt")},
	})
	if err := hook.PostToolUse(bytes.NewReader(done)); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open("stray")
	sess, _ := s.Read()
	for _, e := range sess.Events {
		if !strings.HasPrefix(e.Path, root) {
			t.Errorf("recorded %q, outside the project", e.Path)
		}
	}
	if len(sess.Events) != 1 || sess.Events[0].Rel != "main.go" {
		t.Errorf("recorded %v, want just main.go", relsOf(sess))
	}
}

// An index that strayed before the walk was confined is brought back, so an
// existing session stops reporting the junk it had acquired.
func TestBash_PrunesRootsAnOlderSessionStrayedTo(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	isolateFromTmux(t)

	root := t.TempDir()
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, "noise.txt"), []byte("churn\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	start := payload(map[string]any{"session_id": "stale", "cwd": root, "hook_event_name": "SessionStart"})
	if err := hook.SessionStart(bytes.NewReader(start)); err != nil {
		t.Fatal(err)
	}

	// Plant the stray root the way an older lens would have left it.
	s, _ := store.Open("stale")
	ix, err := fsindex.Load(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	ix.AddRoot(elsewhere)
	ix.Rescan(time.Time{})
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(elsewhere, "noise.txt"), []byte("more churn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := payload(map[string]any{
		"session_id": "stale", "cwd": root, "hook_event_name": "PostToolUse",
		"tool_name": "Bash", "tool_input": map[string]any{"command": "true"},
	})
	if err := hook.PostToolUse(bytes.NewReader(done)); err != nil {
		t.Fatal(err)
	}

	sess, _ := s.Read()
	if len(sess.Events) != 0 {
		t.Errorf("recorded %v from a root outside the project", relsOf(sess))
	}
}
