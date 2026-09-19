package main

import (
	"path/filepath"
	"testing"
)

// `lens auto on --project X` is the natural order to type, and the one a tmux
// binding ends up with. Go's flag package stops at the first non-flag argument,
// so the flag has to be found whichever side of the verb it lands on.
func TestParseAutoArgs_FlagAfterTheVerb(t *testing.T) {
	action, dir, err := parseAutoArgs([]string{"off", "--project", "/p/one"})
	if err != nil {
		t.Fatal(err)
	}
	if action != "off" {
		t.Errorf("action = %q, want off", action)
	}
	if dir != "/p/one" {
		t.Errorf("project = %q, want /p/one", dir)
	}
}

func TestParseAutoArgs_FlagBeforeTheVerb(t *testing.T) {
	action, dir, err := parseAutoArgs([]string{"--project", "/p/one", "off"})
	if err != nil || action != "off" || dir != "/p/one" {
		t.Errorf("parsed %q %q err=%v, want off /p/one", action, dir, err)
	}
}

// Bare `lens auto` flips the working directory, which is what the binding uses.
func TestParseAutoArgs_DefaultsToToggleHere(t *testing.T) {
	action, dir, err := parseAutoArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if action != "toggle" {
		t.Errorf("action = %q, want toggle", action)
	}
	if dir == "" {
		t.Error("project is empty; it should default to the working directory")
	}
}

func TestParseAutoArgs_RejectsAnUnknownVerb(t *testing.T) {
	if _, _, err := parseAutoArgs([]string{"enable"}); err == nil {
		t.Error("an unknown verb should be an error, not a silent toggle")
	}
}

// Two verbs is a typo, and quietly acting on the first would be a surprise.
func TestParseAutoArgs_RejectsASecondVerb(t *testing.T) {
	if _, _, err := parseAutoArgs([]string{"on", "off"}); err == nil {
		t.Error("two verbs should be an error")
	}
}

// The message has to name the project the setting was actually filed under, or
// `lens auto off --project ..` reports success against a path that means
// nothing to the reader.
func TestParseAutoArgs_ResolvesTheProjectItReports(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, got, err := parseAutoArgs([]string{"off", "--project", "."})
	if err != nil {
		t.Fatal(err)
	}
	if got == "." {
		t.Errorf("project = %q, want the directory it resolves to", got)
	}
	if want, _ := filepath.EvalSymlinks(dir); got != want {
		t.Errorf("project = %q, want %q", got, want)
	}
}
