package capture_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
)

// fixtureFor returns the raw payload of the first recorded hook event matching
// the given event name and tool. The fixtures are real payloads captured from a
// live Claude Code session, not hand-written approximations.
func fixtureFor(t *testing.T, event, tool string) []byte {
	t.Helper()
	return fixtureMatching(t, func(ev string, raw map[string]any) bool {
		if ev != event {
			return false
		}
		if tool == "" {
			return true
		}
		return raw["tool_name"] == tool
	})
}

func fixtureMatching(t *testing.T, want func(event string, raw map[string]any) bool) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/probe-payloads.jsonl")
	if err != nil {
		t.Fatalf("reading fixtures: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var rec struct {
			Event string          `json:"__event"`
			Raw   json.RawMessage `json:"__raw"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(rec.Raw, &raw); err != nil {
			continue
		}
		if want(rec.Event, raw) {
			return rec.Raw
		}
	}
	t.Fatal("no matching fixture")
	return nil
}

func TestParsePostToolUse_Edit(t *testing.T) {
	raw := fixtureFor(t, "PostToolUse", "Edit")

	rec, err := capture.ParsePostToolUse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Event.Tool != "Edit" {
		t.Errorf("tool = %q, want Edit", rec.Event.Tool)
	}
	if rec.Event.Kind != "update" {
		t.Errorf("kind = %q, want update", rec.Event.Kind)
	}
	if rec.Event.Added != 1 || rec.Event.Removed != 1 {
		t.Errorf("+%d -%d, want +1 -1", rec.Event.Added, rec.Event.Removed)
	}
	if len(rec.Event.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(rec.Event.Hunks))
	}
	if got := rec.Event.Hunks[0].Lines[1]; got != "-line two" {
		t.Errorf("lines[1] = %q, want %q", got, "-line two")
	}
	if rec.SessionID == "" {
		t.Error("session id not populated")
	}
	if rec.CWD == "" {
		t.Error("cwd not populated")
	}
	if rec.Event.PromptID == "" {
		t.Error("prompt id not populated")
	}
}

// The overwriting Write in the fixtures produces a patch whose final line is
// the "\ No newline at end of file" marker. It is not a changed line.
func TestParsePostToolUse_NoNewlineMarkerNotCounted(t *testing.T) {
	raw := fixtureMatching(t, func(ev string, raw map[string]any) bool {
		if ev != "PostToolUse" || raw["tool_name"] != "Write" {
			return false
		}
		resp, _ := raw["tool_response"].(map[string]any)
		return resp["type"] == "update"
	})

	rec, err := capture.ParsePostToolUse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Event.Added != 2 || rec.Event.Removed != 3 {
		t.Errorf("+%d -%d, want +2 -3", rec.Event.Added, rec.Event.Removed)
	}
}

func TestParsePostToolUse_CreateHasNoOriginal(t *testing.T) {
	raw := fixtureMatching(t, func(ev string, raw map[string]any) bool {
		if ev != "PostToolUse" {
			return false
		}
		resp, _ := raw["tool_response"].(map[string]any)
		return resp["type"] == "create"
	})

	rec, err := capture.ParsePostToolUse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Event.Kind != "create" {
		t.Errorf("kind = %q, want create", rec.Event.Kind)
	}
	if rec.Event.Removed != 0 {
		t.Errorf("removed = %d, want 0 for a new file", rec.Event.Removed)
	}
	// Creation ships an empty patch; the whole file must still be readable.
	if rec.Event.Added != 1 {
		t.Errorf("added = %d, want 1 (the created file's single line)", rec.Event.Added)
	}
	if len(rec.Event.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1 synthesized hunk", len(rec.Event.Hunks))
	}
	if got := rec.Event.Hunks[0].Lines[0]; got != "+hello" {
		t.Errorf("lines[0] = %q, want %q", got, "+hello")
	}
}

func TestParsePostToolUse_EmptyPatchNonCreateIsSkipped(t *testing.T) {
	raw := []byte(`{"tool_name":"Edit","tool_response":{"type":"update","structuredPatch":[]}}`)
	if _, err := capture.ParsePostToolUse(raw); !errors.Is(err, capture.ErrNoPatch) {
		t.Errorf("err = %v, want ErrNoPatch", err)
	}
}

func TestParsePostToolUse_RelativeToProjectRoot(t *testing.T) {
	raw := fixtureFor(t, "PostToolUse", "Edit")
	rec, err := capture.ParsePostToolUse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(rec.Event.Rel, "/") {
		t.Errorf("rel = %q, want a path relative to the project root", rec.Event.Rel)
	}
	if rec.Event.Rel != "existing.txt" {
		t.Errorf("rel = %q, want existing.txt", rec.Event.Rel)
	}
}

func TestParsePostToolUse_MissingPatchIsSkipped(t *testing.T) {
	_, err := capture.ParsePostToolUse([]byte(`{"tool_name":"Edit","tool_response":{}}`))
	if !errors.Is(err, capture.ErrNoPatch) {
		t.Errorf("err = %v, want ErrNoPatch", err)
	}
}

func TestParsePostToolUse_Garbage(t *testing.T) {
	if _, err := capture.ParsePostToolUse([]byte("not json")); err == nil {
		t.Error("want an error for unparseable input")
	}
}
