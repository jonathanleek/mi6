package set

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jonathanleek/mi6/internal/layer"
)

func whats(changes []Change) []string {
	var out []string
	for _, c := range changes {
		out = append(out, c.What+" "+filepath.Base(c.Path))
	}
	return out
}

func TestApplyThenRefreshIsNoop(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "layer", "skill")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	plan := &Plan{
		Files:    []File{{Path: filepath.Join(dir, "set", "CLAUDE.md"), Content: []byte("hi\n")}},
		Links:    []Link{{Path: filepath.Join(dir, "set", "skills", "skill"), Target: target}},
		LinkDirs: []string{filepath.Join(dir, "set", "skills")},
		Patches:  []Patch{{Path: filepath.Join(dir, "set", ".claude.json"), Key: "mcpServers", Value: layer.Object{"a": layer.Object{"command": "x"}}}},
	}

	changes, err := Apply(plan)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"patched .claude.json", "wrote CLAUDE.md", "linked skill"}
	if got := whats(changes); !reflect.DeepEqual(got, want) {
		t.Errorf("first apply %v, want %v", got, want)
	}

	changes, err = Apply(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("second apply changed %v", whats(changes))
	}
}

func TestPatchKeepsOtherKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(p, []byte(`{"userID":"u1","mcpServers":{"old":{"command":"o"}},"n":1.25}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(&Plan{Patches: []Patch{{Path: p, Key: "mcpServers", Value: layer.Object{"new": layer.Object{"command": "n"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	got, err := layer.ParseObject(b)
	if err != nil {
		t.Fatal(err)
	}
	if got["userID"] != "u1" || got["n"].(interface{ String() string }).String() != "1.25" {
		t.Errorf("other keys changed: %s", b)
	}
	servers := got["mcpServers"].(map[string]any)
	if _, ok := servers["old"]; ok {
		t.Error("old server should be replaced, not merged")
	}
	if _, ok := servers["new"]; !ok {
		t.Error("new server missing")
	}
}

func TestPatchBadFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(p, []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(&Plan{Patches: []Patch{{Path: p, Key: "k", Value: 1}}}); err == nil {
		t.Error("a corrupt tool file must be an error, not overwritten")
	}
}

func TestPruneManagedLinks(t *testing.T) {
	dir := t.TempDir()
	skills := filepath.Join(dir, "skills")
	target := filepath.Join(dir, "t")
	for _, d := range []string{skills, target, filepath.Join(skills, "real-dir")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(target, filepath.Join(skills, "stale")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skills, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	changes, err := Apply(&Plan{
		Links:    []Link{{Path: filepath.Join(skills, "keep"), Target: target}},
		LinkDirs: []string{skills},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := whats(changes); !reflect.DeepEqual(got, []string{"linked keep", "removed stale"}) {
		t.Errorf("changes %v", got)
	}
	for _, name := range []string{"real-dir", "file", "keep"} {
		if _, err := os.Lstat(filepath.Join(skills, name)); err != nil {
			t.Errorf("%s should survive: %v", name, err)
		}
	}
}

func TestLinkRetargets(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "l")
	if err := os.Symlink(filepath.Join(dir, "old"), link); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(&Plan{Links: []Link{{Path: link, Target: filepath.Join(dir, "new")}}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(link); got != filepath.Join(dir, "new") {
		t.Errorf("link -> %s", got)
	}
}

func TestLinkOverRealDirIsAnError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "skill")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(&Plan{Links: []Link{{Path: p, Target: dir}}}); err == nil {
		t.Error("a real directory in the way must not be deleted")
	}
}
