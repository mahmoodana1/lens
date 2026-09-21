package fsindex_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mahmood/lens/internal/fsindex"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// open builds an index rooted at a project, having already seen its files.
func open(t *testing.T, root string) *fsindex.Index {
	t.Helper()
	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ix.AddRoot(root)
	ix.Rescan(time.Time{}) // baseline: everything already present is not a change
	return ix
}

func TestRescan_BaselineReportsNothing(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")

	ix := open(t, root)
	if got := ix.Rescan(time.Time{}); len(got) != 0 {
		t.Errorf("changes = %d, want 0 with nothing modified", len(got))
	}
}

func TestRescan_DetectsModification(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	write(t, path, "package main\n\nfunc main() {}\n")

	ix := open(t, root)
	write(t, path, "package main\n\nfunc main() { println(\"hi\") }\n")

	changes := ix.Rescan(time.Time{})
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}
	c := changes[0]
	if c.Kind != "update" {
		t.Errorf("kind = %q, want update", c.Kind)
	}
	if !strings.Contains(c.Old, "func main() {}") {
		t.Errorf("old content not recovered: %q", c.Old)
	}
	if !strings.Contains(c.New, "println") {
		t.Errorf("new content wrong: %q", c.New)
	}
}

func TestRescan_DetectsCreation(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")

	ix := open(t, root)
	write(t, filepath.Join(root, "helper.go"), "package main\n\nfunc help() {}\n")

	changes := ix.Rescan(time.Time{})
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}
	if changes[0].Kind != "create" {
		t.Errorf("kind = %q, want create", changes[0].Kind)
	}
	if changes[0].Old != "" {
		t.Errorf("a created file should have no previous content, got %q", changes[0].Old)
	}
}

func TestRescan_ChangesAreReportedOnlyOnce(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	write(t, path, "one\n")

	ix := open(t, root)
	write(t, path, "two\n")

	if got := ix.Rescan(time.Time{}); len(got) != 1 {
		t.Fatalf("first rescan = %d changes, want 1", len(got))
	}
	if got := ix.Rescan(time.Time{}); len(got) != 0 {
		t.Errorf("second rescan = %d changes, want 0", len(got))
	}
}

// The index outlives the process: each hook invocation is a new process.
func TestIndexSurvivesAcrossProcesses(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()
	path := filepath.Join(root, "a.txt")
	write(t, path, "one\n")

	first, err := fsindex.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	first.AddRoot(root)
	first.Rescan(time.Time{})
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}

	write(t, path, "two\n")

	second, err := fsindex.Load(dir) // a fresh "process"
	if err != nil {
		t.Fatal(err)
	}
	changes := second.Rescan(time.Time{})
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1 after reloading the index", len(changes))
	}
	if !strings.Contains(changes[0].Old, "one") {
		t.Errorf("previous content lost across processes: %q", changes[0].Old)
	}
}

func TestRescan_SkipsNoiseDirectories(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")

	ix := open(t, root)
	write(t, filepath.Join(root, ".git", "COMMIT_EDITMSG"), "wip\n")
	write(t, filepath.Join(root, "node_modules", "dep", "index.js"), "module.exports = 1\n")
	write(t, filepath.Join(root, "build", "out.o"), "object\n")

	if got := ix.Rescan(time.Time{}); len(got) != 0 {
		t.Errorf("changes = %d, want 0; build and dependency noise should be skipped: %+v", len(got), got)
	}
}

func TestRescan_SkipsBinaryAndHugeFiles(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")
	ix := open(t, root)

	if err := os.WriteFile(filepath.Join(root, "blob.bin"), []byte{0x00, 0x01, 0x02, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "huge.txt"), strings.Repeat("x", 2*1024*1024))

	if got := ix.Rescan(time.Time{}); len(got) != 0 {
		t.Errorf("changes = %d, want 0; binaries and huge files are not readable diffs: %+v", len(got), got)
	}
}

func TestRescan_IgnoresItsOwnStorage(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")

	// The index living inside the watched tree must not report itself.
	dir := filepath.Join(root, ".lens")
	ix, err := fsindex.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ix.AddRoot(root)
	ix.Rescan(time.Time{})

	write(t, filepath.Join(root, "main.go"), "package main // touched\n")
	ix.Rescan(time.Time{})
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}

	for _, c := range ix.Rescan(time.Time{}) {
		if strings.Contains(c.Path, ".lens") {
			t.Errorf("index reported its own storage as a change: %s", c.Path)
		}
	}
}

func TestAddRoot_RefusesHomeAndRoot(t *testing.T) {
	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()

	ix.AddRoot(home)
	ix.AddRoot("/")
	if got := len(ix.Roots()); got != 0 {
		t.Errorf("roots = %v, want none; walking home or / is never acceptable", ix.Roots())
	}
}

func TestSeedRootsFromCommand(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "testproj")
	write(t, filepath.Join(proj, "CMakeLists.txt"), "project(x)\n")

	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cmd := "mkdir -p " + proj + "/src && cat > " + proj + "/CMakeLists.txt <<'EOF'\nproject(x)\nEOF"
	ix.SeedFromCommand(root, cmd)

	found := false
	for _, r := range ix.Roots() {
		if r == proj {
			found = true
		}
	}
	if !found {
		t.Errorf("roots = %v, want the project directory named in the command", ix.Roots())
	}
}

// Commands often name both a directory and a path inside it. Watching both
// must not report the same file twice.
func TestRescan_OverlappingRootsReportOnce(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	write(t, filepath.Join(proj, "src", "main.cpp"), "int main() { return 0; }\n")

	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Seeded in the order a command would mention them: the nested path first.
	ix.AddRoot(filepath.Join(proj, "src"))
	ix.AddRoot(proj)
	ix.Rescan(time.Time{})

	write(t, filepath.Join(proj, "src", "main.cpp"), "int main() { return 1; }\n")

	changes := ix.Rescan(time.Time{})
	if len(changes) != 1 {
		t.Errorf("changes = %d, want 1; overlapping roots duplicated the file: %+v", len(changes), changes)
	}
}

func TestAddRoot_ParentReplacesNestedRoots(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	write(t, filepath.Join(proj, "src", "x.txt"), "x\n")

	ix, _ := fsindex.Load(t.TempDir())
	ix.AddRoot(filepath.Join(proj, "src"))
	ix.AddRoot(proj)

	if got := ix.Roots(); len(got) != 1 || got[0] != proj {
		t.Errorf("roots = %v, want just %q", got, proj)
	}
}

// Anything hidden is machinery, not work to read: a dot-directory is never
// walked and a dot-file is never recorded, whatever it is called.
func TestRescan_SkipsEverythingHidden(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")

	ix := open(t, root)

	write(t, filepath.Join(root, ".gitignore"), "build/\n")
	write(t, filepath.Join(root, ".env"), "TOKEN=1\n")
	write(t, filepath.Join(root, ".claude", "settings.json"), "{}\n")
	write(t, filepath.Join(root, ".terraform", "plugins", "x.bin"), "data\n")
	write(t, filepath.Join(root, "src", ".cache", "blob"), "data\n")
	write(t, filepath.Join(root, "src", "app.go"), "package app\n")

	var got []string
	for _, c := range ix.Rescan(time.Time{}) {
		rel, _ := filepath.Rel(root, c.Path)
		got = append(got, filepath.ToSlash(rel))
	}

	if len(got) != 1 || got[0] != "src/app.go" {
		t.Errorf("reported %v, want only src/app.go", got)
	}
}

// A hidden tree is not descended into at all, so its size cannot cost anything.
func TestRescan_DoesNotWalkHiddenDirectories(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")
	for i := range 50 {
		write(t, filepath.Join(root, ".venv", "lib", "mod"+string(rune('a'+i%26))+".py", "f.py"), "x\n")
	}

	ix := open(t, root)
	write(t, filepath.Join(root, ".venv", "lib", "new.py"), "x\n")

	if got := ix.Rescan(time.Time{}); len(got) != 0 {
		t.Errorf("reported %v from inside a hidden directory", got)
	}
}

// The walk sweeps whole directories, so it must never leave the project: a
// command naming a path in /tmp would otherwise turn the panel into a log of
// other programs' scratch files.
func TestConfine_RefusesRootsOutsideTheProject(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")
	write(t, filepath.Join(elsewhere, "noise.txt"), "churn\n")

	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ix.Confine(root)
	ix.AddRoot(root)
	ix.AddRoot(elsewhere)
	ix.Rescan(time.Time{})

	write(t, filepath.Join(elsewhere, "noise.txt"), "more churn\n")
	write(t, filepath.Join(root, "main.go"), "package main // touched\n")

	var got []string
	for _, c := range ix.Rescan(time.Time{}) {
		got = append(got, filepath.Base(c.Path))
	}
	if len(got) != 1 || got[0] != "main.go" {
		t.Errorf("reported %v, want only main.go", got)
	}
}

// A subdirectory of the project is still fair game: that is how a project
// created during the session gets watched.
func TestConfine_AllowsRootsInsideTheProject(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "newproj")
	write(t, filepath.Join(sub, "main.go"), "package main\n")

	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ix.Confine(root)
	ix.AddRoot(sub)
	ix.Rescan(time.Time{})

	write(t, filepath.Join(sub, "main.go"), "package main // touched\n")
	if got := ix.Rescan(time.Time{}); len(got) != 1 {
		t.Errorf("changes = %d, want the subdirectory to be watched", len(got))
	}
}

// An index saved before the walk was confined still holds the roots it strayed
// to, so confining has to prune what is already there.
func TestConfine_PrunesRootsAlreadySaved(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main\n")
	write(t, filepath.Join(elsewhere, "noise.txt"), "churn\n")

	dir := t.TempDir()
	stale, err := fsindex.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	stale.AddRoot(root)
	stale.AddRoot(elsewhere) // as an older lens would have
	stale.Rescan(time.Time{})
	if err := stale.Save(); err != nil {
		t.Fatal(err)
	}

	ix, err := fsindex.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ix.Confine(root)

	for _, r := range ix.Roots() {
		if r == elsewhere {
			t.Errorf("roots still include %q, outside the project", r)
		}
	}
	write(t, filepath.Join(elsewhere, "noise.txt"), "more churn\n")
	if got := ix.Rescan(time.Time{}); len(got) != 0 {
		t.Errorf("reported %v from outside the project", got)
	}
}

// A file Claude removes is exactly the change you would most want to catch, so
// it is reported like any other — with the contents it had, from the blob the
// index already kept.
func TestRescan_ReportsDeletions(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "gone.go"), "package main\n\nfunc main() {}\n")
	write(t, filepath.Join(root, "stays.go"), "package main\n")

	ix := open(t, root)
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}

	got := ix.Rescan(time.Time{})

	if len(got) != 1 {
		t.Fatalf("changes = %+v, want the one deletion", got)
	}
	if got[0].Kind != "delete" {
		t.Errorf("kind = %q, want delete", got[0].Kind)
	}
	if filepath.Base(got[0].Path) != "gone.go" {
		t.Errorf("path = %q, want gone.go", got[0].Path)
	}
	if got[0].Old != "package main\n\nfunc main() {}\n" {
		t.Errorf("old = %q, want what the file held", got[0].Old)
	}
	if got[0].New != "" {
		t.Errorf("new = %q, want empty: the file is gone", got[0].New)
	}
}

// Reported once, not on every scan thereafter.
func TestRescan_ReportsADeletionOnlyOnce(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "gone.go"), "package main\n")

	ix := open(t, root)
	os.Remove(filepath.Join(root, "gone.go"))

	if got := ix.Rescan(time.Time{}); len(got) != 1 {
		t.Fatalf("changes = %+v, want one", got)
	}
	if got := ix.Rescan(time.Time{}); len(got) != 0 {
		t.Errorf("changes = %+v, want none the second time", got)
	}
}

// A file the walk simply did not reach this time — the budget ran out, a root
// went away — is still on disk, and must not be mourned as deleted.
func TestRescan_DoesNotMournAFileThatIsStillThere(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	write(t, filepath.Join(sub, "kept.go"), "package main\n")

	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ix.AddRoot(sub)
	ix.Rescan(time.Time{}) // baseline: kept.go is known

	// The root goes away, so the file is never visited again — but it exists.
	ix.Confine(filepath.Join(root, "elsewhere"))

	if got := ix.Rescan(time.Time{}); len(got) != 0 {
		t.Errorf("changes = %+v, want none: the file is still on disk", got)
	}
}

// A deletion the reader never saw the contents of is still worth reporting.
func TestRescan_ReportsADeletionWithNoBlob(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "gone.go"), "package main\n")

	ix := open(t, root)
	os.RemoveAll(filepath.Join(root, "gone.go"))

	got := ix.Rescan(time.Time{})
	if len(got) != 1 || got[0].Kind != "delete" {
		t.Fatalf("changes = %+v, want the deletion", got)
	}
}

// A hidden directory is machinery whether it is walked into or handed over as
// a root. A command naming ~/.config once cost a session 13,000 indexed files
// and every change it was supposed to be watching.
func TestConfine_RefusesHiddenRoots(t *testing.T) {
	proj := t.TempDir()
	write(t, filepath.Join(proj, "main.go"), "package main\n")
	write(t, filepath.Join(proj, ".config", "nvim", "init.lua"), "-- x\n")

	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ix.Confine(proj)
	ix.AddRoot(proj)
	ix.AddRoot(filepath.Join(proj, ".config")) // as a command naming it would
	ix.Rescan(time.Time{})

	for _, r := range ix.Roots() {
		if filepath.Base(r) == ".config" {
			t.Errorf("roots include %q, a hidden directory", r)
		}
	}

	write(t, filepath.Join(proj, ".config", "nvim", "init.lua"), "-- changed\n")
	write(t, filepath.Join(proj, "main.go"), "package main // touched\n")

	var got []string
	for _, c := range ix.Rescan(time.Time{}) {
		got = append(got, filepath.Base(c.Path))
	}
	if len(got) != 1 || got[0] != "main.go" {
		t.Errorf("reported %v, want only main.go", got)
	}
}

// A project that itself lives under a hidden directory is still its own root.
func TestConfine_AProjectInAHiddenDirectoryIsStillWatched(t *testing.T) {
	base := t.TempDir()
	proj := filepath.Join(base, ".config", "nvim")
	write(t, filepath.Join(proj, "init.lua"), "-- x\n")

	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ix.Confine(proj)
	ix.AddRoot(proj)
	ix.Rescan(time.Time{})

	write(t, filepath.Join(proj, "init.lua"), "-- changed\n")
	if got := ix.Rescan(time.Time{}); len(got) != 1 {
		t.Errorf("changes = %+v, want the edit: the project is not hidden from itself", got)
	}
}

// A command's paths are relative to where the command ran, not to wherever
// lens happens to be. An agent working in a subdirectory writes "lib/x.sh",
// and that is a file in its directory, not in the session's.
func TestSeedFromCommand_ResolvesAgainstTheCommandsDirectory(t *testing.T) {
	proj := t.TempDir()
	sub := filepath.Join(proj, "bashkit")
	write(t, filepath.Join(sub, "lib", "fileutil.sh"), "echo x\n")

	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ix.Confine(proj)
	ix.SeedFromCommand(sub, "cat > lib/fileutil.sh <<'EOF'")
	ix.Rescan(time.Time{})

	write(t, filepath.Join(sub, "lib", "fileutil.sh"), "echo changed\n")

	var got []string
	for _, c := range ix.Rescan(time.Time{}) {
		got = append(got, filepath.Base(c.Path))
	}
	if len(got) != 1 || got[0] != "fileutil.sh" {
		t.Errorf("reported %v, want fileutil.sh: the command ran in %s", got, sub)
	}
}

// An absolute path in a command still means what it says.
func TestSeedFromCommand_StillTakesAbsolutePaths(t *testing.T) {
	proj := t.TempDir()
	write(t, filepath.Join(proj, "sub", "x.go"), "package x\n")

	ix, err := fsindex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ix.Confine(proj)
	ix.SeedFromCommand("/elsewhere", "touch "+filepath.Join(proj, "sub", "x.go"))
	ix.Rescan(time.Time{})

	write(t, filepath.Join(proj, "sub", "x.go"), "package x // touched\n")
	if got := ix.Rescan(time.Time{}); len(got) != 1 {
		t.Errorf("changes = %+v, want the one file", got)
	}
}

// An index that adopted a hidden tree before the rule existed still holds it,
// along with every file it walked there. Confining has to let go of both, or a
// session stays poisoned for as long as it lives.
func TestConfine_PrunesHiddenRootsAlreadySaved(t *testing.T) {
	proj := t.TempDir()
	write(t, filepath.Join(proj, "main.go"), "package main\n")
	write(t, filepath.Join(proj, ".config", "nvim", "init.lua"), "-- x\n")

	// Planted as an older lens left it on disk: the project itself was too
	// broad to watch, so the only root it ever took was the hidden tree a
	// command named. The current one will not adopt such a root, so the state
	// has to be written rather than built.
	dir := t.TempDir()
	hidden := filepath.Join(proj, ".config")
	stale := fmt.Sprintf(`{"roots":[%q],"files":{%q:{"mtime":"2020-01-01T00:00:00Z","size":5,"hash":"x"}},"baselined":true}`,
		hidden, filepath.Join(hidden, "nvim", "init.lua"))
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}

	ix, err := fsindex.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Roots()) != 1 {
		t.Fatalf("setup: the planted index has roots %v", ix.Roots())
	}
	ix.Confine(proj)

	for _, r := range ix.Roots() {
		if filepath.Base(r) == ".config" {
			t.Errorf("roots still include %q", r)
		}
	}
	write(t, filepath.Join(proj, ".config", "nvim", "init.lua"), "-- changed\n")
	if got := ix.Rescan(time.Time{}); len(got) != 0 {
		t.Errorf("reported %+v from a hidden tree", got)
	}
}
