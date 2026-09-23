package build

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonathanleek/mi6/internal/resolve"
)

var update = flag.Bool("update", false, "rewrite golden files")

// examplesDir is the repo's examples folder, used as the fixture: the golden
// file is what the shipped example builds to.
func examplesDir(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func globexStack(t *testing.T) *resolve.Stack {
	ex := examplesDir(t)
	layers := []string{
		"home/.mi6",
		"tree/.mi6",
		"tree/work/.mi6",
		"tree/work/clients/.mi6",
		"tree/work/clients/globex/.mi6",
	}
	st := &resolve.Stack{}
	for i, l := range layers {
		origin := resolve.OriginTree
		if i == 0 {
			origin = resolve.OriginHome
		}
		st.Layers = append(st.Layers, resolve.Layer{Path: filepath.Join(ex, l), Origin: origin})
	}
	return st
}

// manifest renders a set directory as text: every file's content and every
// symlink's target, with machine-specific prefixes replaced.
func manifest(t *testing.T, dir string, replace map[string]string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		switch {
		case d.IsDir():
			return nil
		case d.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, "== %s -> %s\n", rel, target)
		default:
			content, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, "== %s\n%s", rel, content)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for from, to := range replace {
		out = strings.ReplaceAll(out, from, to)
	}
	return out
}

func TestBuildExample(t *testing.T) {
	st := globexStack(t)
	state := t.TempDir()
	home := "/fake/home"

	r, err := Build(st, Options{StateDir: state, Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Merged.Warnings) != 0 {
		t.Errorf("warnings %v", r.Merged.Warnings)
	}
	for _, name := range []string{"set", "claude", "opencode"} {
		if len(r.Changes[name]) == 0 {
			t.Errorf("first build changed nothing for %s", name)
		}
	}

	got := manifest(t, r.Dir, map[string]string{examplesDir(t): "$EXAMPLES"})
	golden := filepath.Join("testdata", "example.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("set differs from %s. Run go test ./internal/build -update and review the diff.\n--- got ---\n%s", golden, got)
	}

	r2, err := Build(st, Options{StateDir: state, Home: home})
	if err != nil {
		t.Fatal(err)
	}
	for name, changes := range r2.Changes {
		if len(changes) != 0 {
			t.Errorf("second build changed %s: %v", name, changes)
		}
	}
	if r2.Dir != r.Dir {
		t.Errorf("set dir changed between builds: %s vs %s", r.Dir, r2.Dir)
	}
}

func TestBuildRemovesDroppedSkillAndServer(t *testing.T) {
	st := globexStack(t)
	state := t.TempDir()
	r, err := Build(st, Options{StateDir: state, Home: "/fake/home"})
	if err != nil {
		t.Fatal(err)
	}
	claudeDir := filepath.Join(r.Dir, "claude")
	if _, err := os.Lstat(filepath.Join(claudeDir, "skills", "repo-conventions")); err != nil {
		t.Fatalf("skill link missing: %v", err)
	}

	// The same set directory, rebuilt from a stack without the layers that
	// carry the skill and the MCP servers: hash the same by keeping the
	// paths, but load nothing from them by pointing at empty dirs.
	empty := t.TempDir()
	var stripped resolve.Stack
	for range st.Layers {
		stripped.Layers = append(stripped.Layers, resolve.Layer{Path: empty})
	}
	// Force the same set directory.
	r2Dir := SetDir(state, &stripped)
	if err := os.Rename(r.Dir, r2Dir); err != nil {
		t.Fatal(err)
	}
	r2, err := Build(&stripped, Options{StateDir: state, Home: "/fake/home"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(r2.Dir, "claude", "skills", "repo-conventions")); err == nil {
		t.Error("dropped skill link still present")
	}
	b, _ := os.ReadFile(filepath.Join(r2.Dir, "claude", ".claude.json"))
	if !strings.Contains(string(b), `"mcpServers": {}`) {
		t.Errorf("servers not cleared: %s", b)
	}
}

func TestSetDirDependsOnLayers(t *testing.T) {
	a := &resolve.Stack{Layers: []resolve.Layer{{Path: "/h/.mi6"}}}
	b := &resolve.Stack{Layers: []resolve.Layer{{Path: "/h/.mi6"}, {Path: "/h/git/.mi6"}}}
	if SetDir("/s", a) == SetDir("/s", b) {
		t.Error("different stacks must not share a set")
	}
	if SetDir("/s", a) != SetDir("/s", a) {
		t.Error("set dir must be stable")
	}
	if !strings.HasPrefix(SetDir("/s", a), "/s/sets/") {
		t.Errorf("set dir %s", SetDir("/s", a))
	}
}

func TestStateDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	if got := StateDir("/h"); got != "/h/.local/state/mi6" {
		t.Errorf("default %s", got)
	}
	t.Setenv("XDG_STATE_HOME", "/x")
	if got := StateDir("/h"); got != "/x/mi6" {
		t.Errorf("xdg %s", got)
	}
}
