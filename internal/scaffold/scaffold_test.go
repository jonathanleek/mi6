package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jonathanleek/mi6/internal/layer"
)

var wantFiles = []string{"README.md", "AGENTS.md", "mcp.json", "claude.json", "opencode.json", "env.json", "skills/README.md"}

func TestCreateThenKeep(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "git", "work")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	r, err := Create(dir, home)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Created, wantFiles) || len(r.Kept) != 0 {
		t.Errorf("created %v kept %v", r.Created, r.Kept)
	}
	if r.Dir != filepath.Join(dir, ".mi6") {
		t.Errorf("dir %s", r.Dir)
	}
	if r.Hint != "" {
		t.Errorf("hint outside a repo: %q", r.Hint)
	}

	// The scaffold loads as a layer with nothing in it but valid shapes.
	l, err := layer.Load(r.Dir)
	if err != nil {
		t.Fatalf("scaffold does not load: %v", err)
	}
	if !strings.HasPrefix(l.Instructions, "# Rules for ~/git/work\n") {
		t.Errorf("instructions %q", l.Instructions)
	}
	if len(l.Skills) != 0 || l.MCP["mcpServers"] == nil || l.Claude["permissions"] == nil || l.OpenCode["$schema"] == nil || l.Env == nil {
		t.Errorf("layer %+v", l)
	}

	// A second run overwrites nothing.
	if err := os.WriteFile(filepath.Join(r.Dir, "AGENTS.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r2, err := Create(dir, home)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Created) != 0 || !reflect.DeepEqual(r2.Kept, wantFiles) {
		t.Errorf("second run created %v kept %v", r2.Created, r2.Kept)
	}
	b, _ := os.ReadFile(filepath.Join(r.Dir, "AGENTS.md"))
	if string(b) != "mine\n" {
		t.Errorf("existing file overwritten: %q", b)
	}
}

func TestCreateInRepoHints(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "proj")
	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}

	r, err := Create(repo, home)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Hint, "mi6.trust") {
		t.Errorf("no trust hint at a repo root: %q", r.Hint)
	}

	r, err = Create(filepath.Join(repo, "sub"), home)
	if err != nil {
		t.Fatal(err)
	}
	if r.Hint != "" {
		t.Errorf("hint below the repo root: %q", r.Hint)
	}
}

func TestCreateMissingDir(t *testing.T) {
	if _, err := Create(filepath.Join(t.TempDir(), "nope"), "/h"); err == nil {
		t.Error("expected an error for a missing directory")
	}
}
