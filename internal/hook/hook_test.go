package hook_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
