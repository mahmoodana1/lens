package fsindex_test

import (
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
	ix.SeedFromCommand(cmd)

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
