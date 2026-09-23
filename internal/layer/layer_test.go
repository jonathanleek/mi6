package layer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func real(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l.HasInstructions() || len(l.Skills) != 0 || l.MCP != nil || l.Claude != nil || l.OpenCode != nil {
		t.Errorf("empty layer loaded as %+v", l)
	}
}

func TestLoadFull(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "AGENTS.md"), "rules\n\n\n")
	write(t, filepath.Join(dir, "mcp.json"), `{"mcpServers":{"gh":{"command":"gh","args":["mcp","serve"]}}}`)
	write(t, filepath.Join(dir, "claude.json"), `{"permissions":{"allow":["Bash(ls)"]},"n":1.50}`)
	write(t, filepath.Join(dir, "opencode.json"), `{"model":"a/b"}`)
	write(t, filepath.Join(dir, "skills", "one", "SKILL.md"), "---\nname: one\n---\n")
	write(t, filepath.Join(dir, "skills", "README.md"), "not a skill")
	write(t, filepath.Join(dir, "skills", "nofile", "notes.txt"), "no SKILL.md here")

	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l.Instructions != "rules\n" {
		t.Errorf("instructions %q: trailing newlines should collapse to one", l.Instructions)
	}
	if l.MCP["mcpServers"] == nil || l.OpenCode["model"] != "a/b" {
		t.Errorf("json %v %v", l.MCP, l.OpenCode)
	}
	if n := l.Claude["n"]; n.(interface{ String() string }).String() != "1.50" {
		t.Errorf("number not preserved: %v", n)
	}
	if len(l.Skills) != 1 || l.Skills["one"].Dir != real(t, filepath.Join(dir, "skills", "one")) {
		t.Errorf("skills %v", l.Skills)
	}
}

func TestSkillsOneLevelDeeperThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "some-checkout", "skills")
	write(t, filepath.Join(repo, "alpha", "SKILL.md"), "")
	write(t, filepath.Join(repo, "beta", "SKILL.md"), "")
	write(t, filepath.Join(repo, "tooDeep", "nested", "gamma", "SKILL.md"), "")
	layerDir := filepath.Join(dir, ".mi6")
	if err := os.MkdirAll(filepath.Join(layerDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repo, filepath.Join(layerDir, "skills", "from-repo")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(layerDir, "skills", "local", "SKILL.md"), "")

	l, err := Load(layerDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range l.Skills {
		names = append(names, n)
	}
	if len(names) != 3 {
		t.Fatalf("skills %v, want alpha beta local", names)
	}
	if l.Skills["alpha"].Dir != real(t, filepath.Join(repo, "alpha")) {
		t.Errorf("alpha dir %q should be the resolved target", l.Skills["alpha"].Dir)
	}
	if _, ok := l.Skills["gamma"]; ok {
		t.Error("gamma is two levels deep and must not be found")
	}
}

func TestSkillCollisionInOneLayer(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "skills", "a-group", "dup", "SKILL.md"), "")
	write(t, filepath.Join(dir, "skills", "b-group", "dup", "SKILL.md"), "")

	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l.Skills["dup"].Dir != real(t, filepath.Join(dir, "skills", "a-group", "dup")) {
		t.Errorf("first in sorted order should win: %v", l.Skills["dup"])
	}
	if len(l.Warnings) != 1 || !strings.Contains(l.Warnings[0], "hidden") {
		t.Errorf("warnings %v", l.Warnings)
	}
}

func TestBadJSON(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "claude.json"), `{"a":`)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "claude.json") {
		t.Errorf("err %v, want one naming the file", err)
	}

	dir = t.TempDir()
	write(t, filepath.Join(dir, "mcp.json"), `[1,2]`)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "not a JSON object") {
		t.Errorf("err %v", err)
	}
}
