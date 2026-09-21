package hooks_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mahmood/lens/internal/hooks"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("settings are not valid JSON after the edit: %v\n%s", err, b)
	}
	return out
}

// commandsFor is every hook command registered for an event.
func commandsFor(t *testing.T, path, event string) []string {
	t.Helper()
	settings := read(t, path)
	hooksByEvent, _ := settings["hooks"].(map[string]any)
	entries, _ := hooksByEvent[event].([]any)

	var out []string
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		list, _ := entry["hooks"].([]any)
		for _, h := range list {
			hook, _ := h.(map[string]any)
			if cmd, ok := hook["command"].(string); ok {
				out = append(out, cmd)
			}
		}
	}
	return out
}

func TestInstall_AddsEveryHookItNeeds(t *testing.T) {
	path := write(t, `{}`)

	if err := hooks.Install(path, "/opt/lens"); err != nil {
		t.Fatal(err)
	}

	for _, want := range []struct{ event, command string }{
		{"PostToolUse", "/opt/lens hook post-tool-use"},
		{"UserPromptSubmit", "/opt/lens hook prompt"},
		{"SessionStart", "/opt/lens hook session-start"},
		{"SessionEnd", "/opt/lens hook session-end"},
		{"Stop", "/opt/lens hook stop"},
	} {
		got := commandsFor(t, path, want.event)
		if len(got) != 1 || got[0] != want.command {
			t.Errorf("%s = %v, want [%s]", want.event, got, want.command)
		}
	}
}

// Shell commands write files too — heredocs, sed — so those tools are watched
// as well, and the matcher has to say so.
func TestInstall_MatchesTheToolsThatChangeFiles(t *testing.T) {
	path := write(t, `{}`)
	if err := hooks.Install(path, "/opt/lens"); err != nil {
		t.Fatal(err)
	}

	settings := read(t, path)
	byEvent, _ := settings["hooks"].(map[string]any)
	entries, _ := byEvent["PostToolUse"].([]any)
	entry, _ := entries[0].(map[string]any)

	if got := entry["matcher"]; got != "Edit|Write|MultiEdit|NotebookEdit|Bash" {
		t.Errorf("matcher = %v", got)
	}
}

// The file belongs to its owner: everything lens did not put there survives.
func TestInstall_LeavesTheRestOfTheSettingsAlone(t *testing.T) {
	path := write(t, `{
	  "model": "opus",
	  "permissions": {"allow": ["Bash(git:*)"]},
	  "hooks": {
	    "SessionStart": [{"hooks": [{"type": "command", "command": "/other/tool start"}]}],
	    "PreToolUse":   [{"matcher": "Bash", "hooks": [{"type": "command", "command": "/other/guard"}]}]
	  }
	}`)

	if err := hooks.Install(path, "/opt/lens"); err != nil {
		t.Fatal(err)
	}

	settings := read(t, path)
	if settings["model"] != "opus" {
		t.Errorf("model = %v, want opus", settings["model"])
	}
	if settings["permissions"] == nil {
		t.Error("permissions went missing")
	}
	if got := commandsFor(t, path, "PreToolUse"); len(got) != 1 || got[0] != "/other/guard" {
		t.Errorf("PreToolUse = %v, want another tool's hook untouched", got)
	}
	got := commandsFor(t, path, "SessionStart")
	if len(got) != 2 {
		t.Fatalf("SessionStart = %v, want the other tool's hook and lens's", got)
	}
	if got[0] != "/other/tool start" {
		t.Errorf("SessionStart = %v, want the other tool's hook kept first", got)
	}
}

// Installing again is how you upgrade, so it must not stack up entries.
func TestInstall_IsIdempotent(t *testing.T) {
	path := write(t, `{}`)

	for range 3 {
		if err := hooks.Install(path, "/opt/lens"); err != nil {
			t.Fatal(err)
		}
	}

	if got := commandsFor(t, path, "Stop"); len(got) != 1 {
		t.Errorf("Stop = %v, want one entry however many times it is installed", got)
	}
}

// Moving the binary is an upgrade too: the old path must not be left behind.
func TestInstall_ReplacesAnEarlierInstallElsewhere(t *testing.T) {
	path := write(t, `{}`)
	if err := hooks.Install(path, "/old/place/lens"); err != nil {
		t.Fatal(err)
	}

	if err := hooks.Install(path, "/new/place/lens"); err != nil {
		t.Fatal(err)
	}

	got := commandsFor(t, path, "Stop")
	if len(got) != 1 || got[0] != "/new/place/lens hook stop" {
		t.Errorf("Stop = %v, want only the new path", got)
	}
}

func TestRemove_TakesOutLensAndNothingElse(t *testing.T) {
	path := write(t, `{
	  "hooks": {
	    "SessionStart": [{"hooks": [{"type": "command", "command": "/other/tool start"}]}],
	    "PreToolUse":   [{"matcher": "Bash", "hooks": [{"type": "command", "command": "/other/guard"}]}]
	  }
	}`)
	if err := hooks.Install(path, "/opt/lens"); err != nil {
		t.Fatal(err)
	}

	removed, err := hooks.Remove(path)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 5 {
		t.Errorf("removed %d hooks, want 5", removed)
	}

	for _, event := range []string{"PostToolUse", "UserPromptSubmit", "SessionEnd", "Stop"} {
		if got := commandsFor(t, path, event); len(got) != 0 {
			t.Errorf("%s = %v, want nothing left of lens", event, got)
		}
	}
	if got := commandsFor(t, path, "SessionStart"); len(got) != 1 || got[0] != "/other/tool start" {
		t.Errorf("SessionStart = %v, want the other tool's hook kept", got)
	}
	if got := commandsFor(t, path, "PreToolUse"); len(got) != 1 {
		t.Errorf("PreToolUse = %v, want it untouched", got)
	}
}

func TestRemove_WhenNothingIsInstalled(t *testing.T) {
	path := write(t, `{"model": "opus"}`)

	removed, err := hooks.Remove(path)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if read(t, path)["model"] != "opus" {
		t.Error("the settings were disturbed for nothing")
	}
}

// Settings that cannot be parsed must be left exactly as they are: rewriting
// someone's broken config from a guess is worse than refusing.
func TestInstall_RefusesToTouchUnreadableSettings(t *testing.T) {
	const broken = `{"hooks": [ this is not json`
	path := write(t, broken)

	if err := hooks.Install(path, "/opt/lens"); err == nil {
		t.Error("expected an error on malformed settings")
	}

	b, _ := os.ReadFile(path)
	if string(b) != broken {
		t.Errorf("the file was modified:\n%s", b)
	}
}

// A settings file that does not exist yet is a fresh install, not a failure.
func TestInstall_CreatesSettingsThatAreNotThereYet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")

	if err := hooks.Install(path, "/opt/lens"); err != nil {
		t.Fatal(err)
	}
	if got := commandsFor(t, path, "Stop"); len(got) != 1 {
		t.Errorf("Stop = %v, want lens installed", got)
	}
}

// Status is what the doctor reports: which of them are wired, and to what.
func TestStatus_ReportsWhatIsWired(t *testing.T) {
	path := write(t, `{}`)
	if err := hooks.Install(path, "/opt/lens"); err != nil {
		t.Fatal(err)
	}

	got, err := hooks.Status(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("status covers %d events, want 5", len(got))
	}
	for _, s := range got {
		if !s.Installed {
			t.Errorf("%s reported as not installed", s.Event)
		}
		if s.Binary != "/opt/lens" {
			t.Errorf("%s binary = %q, want /opt/lens", s.Event, s.Binary)
		}
	}
}

// A half-installed set is the interesting case: the doctor has to name which.
func TestStatus_NamesTheOnesThatAreMissing(t *testing.T) {
	path := write(t, `{
	  "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "/opt/lens hook stop"}]}]}
	}`)

	got, err := hooks.Status(path)
	if err != nil {
		t.Fatal(err)
	}
	installed := map[string]bool{}
	for _, s := range got {
		installed[s.Event] = s.Installed
	}
	if !installed["Stop"] {
		t.Error("Stop is installed and was not reported so")
	}
	if installed["PostToolUse"] {
		t.Error("PostToolUse is not installed and was reported as if it were")
	}
}

// Nothing installed at all, and no settings file: still an answer, not an error.
func TestStatus_WithNoSettingsFile(t *testing.T) {
	got, err := hooks.Status(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range got {
		if s.Installed {
			t.Errorf("%s reported installed with no settings file", s.Event)
		}
	}
}
