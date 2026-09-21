package doctor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/doctor"
	"github.com/mahmood/lens/internal/hooks"
	"github.com/mahmood/lens/internal/store"
)

// find is one named check out of a report.
func find(t *testing.T, r doctor.Report, name string) doctor.Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %q check in the report", name)
	return doctor.Check{}
}

// healthy sets up a project whose capture is working, and returns the options
// to examine it with.
func healthy(t *testing.T) doctor.Options {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	proj := t.TempDir()
	bin := filepath.Join(t.TempDir(), "lens")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := hooks.Install(settings, bin); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open("sess")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMeta(proj); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(capture.Event{Rel: "a.go", Added: 1}); err != nil {
		t.Fatal(err)
	}

	return doctor.Options{Project: proj, Settings: settings, Binary: bin, Version: "0.1.0"}
}

// Nothing wrong is itself worth reporting: the reader needs to know the tool
// looked and found nothing rather than that it gave up.
func TestRun_AHealthyInstall(t *testing.T) {
	o := healthy(t)

	r := doctor.Run(o)

	if find(t, r, "hooks").Level != doctor.OK {
		t.Errorf("hooks = %+v", find(t, r, "hooks"))
	}
	if find(t, r, "binary").Level != doctor.OK {
		t.Errorf("binary = %+v", find(t, r, "binary"))
	}
	if got := find(t, r, "capture"); got.Level != doctor.OK || !strings.Contains(got.Detail, "1 edits") {
		t.Errorf("capture = %+v, want the edit it recorded", got)
	}
}

// The failure that looks most like the tool being broken.
func TestRun_NoHooksInstalled(t *testing.T) {
	o := healthy(t)
	o.Settings = filepath.Join(t.TempDir(), "empty.json")

	got := find(t, doctor.Run(o), "hooks")

	if got.Level != doctor.Fail {
		t.Errorf("level = %v, want fail", got.Level)
	}
	if got.Fix == "" {
		t.Error("a failure the reader can fix should say how")
	}
}

// Half-installed is the interesting case: say which are missing.
func TestRun_SomeHooksMissing(t *testing.T) {
	o := healthy(t)
	path := filepath.Join(t.TempDir(), "partial.json")
	if err := os.WriteFile(path, []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"`+o.Binary+` hook stop"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	o.Settings = path

	got := find(t, doctor.Run(o), "hooks")

	if got.Level != doctor.Warn {
		t.Errorf("level = %v, want warn", got.Level)
	}
	if !strings.Contains(got.Detail, "PostToolUse") {
		t.Errorf("detail = %q, want it to name what is missing", got.Detail)
	}
}

// Hooks left pointing at an older install are why an upgrade can seem to do
// nothing at all.
func TestRun_HooksPointingAtAnotherBinary(t *testing.T) {
	o := healthy(t)
	if err := hooks.Install(o.Settings, "/somewhere/else/lens"); err != nil {
		t.Fatal(err)
	}

	got := find(t, doctor.Run(o), "hooks")

	if got.Level == doctor.OK {
		t.Errorf("level = %v, want a warning", got.Level)
	}
	if !strings.Contains(got.Detail, "/somewhere/else/lens") {
		t.Errorf("detail = %q, want it to name the other binary", got.Detail)
	}
}

// A muted project looks exactly like a broken one from the outside.
func TestRun_AutoOpenMuted(t *testing.T) {
	o := healthy(t)
	if err := store.SetAutoOpen(o.Project, false); err != nil {
		t.Fatal(err)
	}

	got := find(t, doctor.Run(o), "auto-open")

	if got.Level != doctor.Warn {
		t.Errorf("level = %v, want warn", got.Level)
	}
	if !strings.Contains(got.Fix, "lens auto on") {
		t.Errorf("fix = %q, want the command that turns it back on", got.Fix)
	}
}

// A session that recorded nothing is the symptom; saying so is the diagnosis.
func TestRun_SessionWithNoEdits(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	proj := t.TempDir()
	s, _ := store.Open("empty")
	s.EnsureMeta(proj)

	got := find(t, doctor.Run(doctor.Options{Project: proj, Binary: "/x/lens"}), "capture")

	if got.Level != doctor.Warn {
		t.Errorf("level = %v, want warn", got.Level)
	}
	if !strings.Contains(got.Detail, "no edits") {
		t.Errorf("detail = %q", got.Detail)
	}
}

// The bug that hid a whole session from the panel.
func TestRun_SessionWithNoProjectRecorded(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("noproj")
	s.MarkEnded() // writes a meta with no project
	s.Append(capture.Event{Rel: "a.go", Added: 1})

	got := find(t, doctor.Run(doctor.Options{Project: t.TempDir(), Binary: "/x/lens"}), "capture")

	if !strings.Contains(got.Detail, "records no project") {
		t.Errorf("detail = %q, want it to notice the missing project", got.Detail)
	}
}

// Home is watchable in principle and useless in practice.
func TestRun_ProjectIsHome(t *testing.T) {
	o := healthy(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	o.Project = home

	got := find(t, doctor.Run(o), "project")

	if got.Level != doctor.Warn {
		t.Errorf("level = %v, want warn", got.Level)
	}
}

func TestRun_MissingBinary(t *testing.T) {
	o := healthy(t)
	o.Binary = filepath.Join(t.TempDir(), "not-there")

	got := find(t, doctor.Run(o), "binary")

	if got.Level != doctor.Fail {
		t.Errorf("level = %v, want fail", got.Level)
	}
}

// Settings that cannot be parsed are a failure worth naming, not a crash.
func TestRun_UnreadableSettings(t *testing.T) {
	o := healthy(t)
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	o.Settings = path

	got := find(t, doctor.Run(o), "hooks")

	if got.Level != doctor.Fail {
		t.Errorf("level = %v, want fail", got.Level)
	}
}

// Worst is what the exit status is built on.
func TestReport_WorstLevel(t *testing.T) {
	r := doctor.Report{Checks: []doctor.Check{
		{Level: doctor.OK}, {Level: doctor.Warn}, {Level: doctor.OK},
	}}
	if r.Worst() != doctor.Warn {
		t.Errorf("worst = %v, want warn", r.Worst())
	}
	if (doctor.Report{}).Worst() != doctor.OK {
		t.Error("an empty report is not a failure")
	}
}

// Every check reports something: a blank line tells the reader nothing.
func TestRun_EveryCheckSaysSomething(t *testing.T) {
	for _, r := range []doctor.Report{
		doctor.Run(healthy(t)),
		doctor.Run(doctor.Options{}),
	} {
		for _, c := range r.Checks {
			if c.Name == "" || c.Detail == "" {
				t.Errorf("check %+v says nothing", c)
			}
		}
	}
}

// The exit status is for scripts, and a warning is not a failure: the panel
// works with a muted project or an editor elsewhere, it just has something to
// say. Only a check that stops it working should make the command fail.
func TestReport_FailedOnlyOnFailures(t *testing.T) {
	for _, c := range []struct {
		name   string
		checks []doctor.Check
		want   bool
	}{
		{"nothing wrong", []doctor.Check{{Level: doctor.OK}}, false},
		{"a warning", []doctor.Check{{Level: doctor.OK}, {Level: doctor.Warn}}, false},
		{"a failure", []doctor.Check{{Level: doctor.OK}, {Level: doctor.Fail}}, true},
		{"both", []doctor.Check{{Level: doctor.Warn}, {Level: doctor.Fail}}, true},
		{"nothing checked", nil, false},
	} {
		if got := (doctor.Report{Checks: c.checks}).Failed(); got != c.want {
			t.Errorf("%s: Failed() = %v, want %v", c.name, got, c.want)
		}
	}
}
