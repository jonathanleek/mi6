package resolve

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixture is a fake home directory with a tree of repos under it, built in a
// temp dir. Every path is symlink-resolved so it compares equal to what git
// reports.
type fixture struct {
	t    *testing.T
	home string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, home: home}
}

// path joins under home.
func (f *fixture) path(parts ...string) string {
	return filepath.Join(append([]string{f.home}, parts...)...)
}

// layer creates a .mi6 directory under the given folder and returns its path.
func (f *fixture) layer(parts ...string) string {
	f.t.Helper()
	p := filepath.Join(f.path(parts...), LayerDir)
	if err := os.MkdirAll(p, 0o755); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// repo creates a git repo with one commit and returns its path.
func (f *fixture) repo(parts ...string) string {
	f.t.Helper()
	p := f.path(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		f.t.Fatal(err)
	}
	f.git(p, "init", "-q")
	f.git(p, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
	return p
}

// worktree adds a linked worktree of repo at the given path and returns it.
func (f *fixture) worktree(repo string, parts ...string) string {
	f.t.Helper()
	p := f.path(parts...)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	f.git(repo, "worktree", "add", "-q", p, "-b", filepath.Base(p))
	return p
}

func (f *fixture) git(dir string, args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func (f *fixture) resolve(dir string) *Stack {
	f.t.Helper()
	st, err := Resolve(Options{Dir: dir, Home: f.home})
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

func layerPaths(st *Stack) []string {
	var out []string
	for _, l := range st.Layers {
		out = append(out, l.Path)
	}
	return out
}

func assertLayers(t *testing.T, st *Stack, want ...string) {
	t.Helper()
	got := layerPaths(st)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("layers\n got: %q\nwant: %q", got, want)
	}
}

func TestRepoInTree(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	tree := f.layer("git")
	work := f.layer("git", "work")
	f.layer("git", "personal") // a sibling, must not apply
	repo := f.repo("git", "work", "pipeline")

	st := f.resolve(repo)
	assertLayers(t, st, home, tree, work)
	if st.Checkout != repo {
		t.Errorf("checkout %q, want %q", st.Checkout, repo)
	}
	if st.Worktree {
		t.Error("main checkout reported as worktree")
	}
	if st.Layers[0].Origin != OriginHome || st.Layers[1].Origin != OriginTree {
		t.Errorf("origins %v", st.Layers)
	}
}

func TestSubdirectoryOfRepo(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	work := f.layer("git", "work")
	repo := f.repo("git", "work", "pipeline")
	sub := filepath.Join(repo, "dags", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	st := f.resolve(sub)
	assertLayers(t, st, home, work)
}

func TestWorktreeResolvesLikeMainCheckout(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	work := f.layer("git", "work")
	f.layer("tasks") // a layer above the worktree, must not apply
	repo := f.repo("git", "work", "pipeline")
	wt := f.worktree(repo, "tasks", "pipeline", "task-1")

	st := f.resolve(wt)
	assertLayers(t, st, home, work)
	if st.Checkout != repo {
		t.Errorf("checkout %q, want %q", st.Checkout, repo)
	}
	if !st.Worktree {
		t.Error("worktree not detected")
	}
}

func TestParentOverride(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	tree := f.layer("git")
	work := f.layer("git", "work")
	f.layer("elsewhere") // above the repo on disk, must not apply once overridden
	repo := f.repo("elsewhere", "pipeline")
	f.git(repo, "config", "mi6.parent", "~/git/work")

	st := f.resolve(repo)
	assertLayers(t, st, home, tree, work)
	if st.Start != f.path("git", "work") {
		t.Errorf("start %q", st.Start)
	}
}

func TestParentOverrideMissingFolder(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	elsewhere := f.layer("elsewhere")
	repo := f.repo("elsewhere", "pipeline")
	f.git(repo, "config", "mi6.parent", "~/does/not/exist")

	st := f.resolve(repo)
	assertLayers(t, st, home, elsewhere)
	if len(st.Notes) != 1 || !strings.Contains(st.Notes[0], "does not exist") {
		t.Errorf("notes %v", st.Notes)
	}
}

func TestRepoOutsideTree(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	f.layer("git")
	repo := f.repo("scratch", "thing")

	st := f.resolve(repo)
	assertLayers(t, st, home)
}

func TestPlainDirectoryIncludesItself(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	notes := f.layer("notes")
	inner := f.layer("notes", "today")

	st := f.resolve(f.path("notes", "today"))
	assertLayers(t, st, home, notes, inner)
	if st.Checkout != "" {
		t.Errorf("checkout %q for a plain directory", st.Checkout)
	}
	if st.Start != f.path("notes", "today") {
		t.Errorf("start %q", st.Start)
	}
}

func TestCheckoutLayerNeedsTrust(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	repo := f.repo("git", "mine")
	own := f.layer("git", "mine")

	st := f.resolve(repo)
	assertLayers(t, st, home)
	if len(st.Skipped) != 1 || st.Skipped[0].Path != own {
		t.Fatalf("skipped %v", st.Skipped)
	}
	if !strings.Contains(st.Skipped[0].Reason, "mi6.trust") {
		t.Errorf("reason %q", st.Skipped[0].Reason)
	}

	f.git(repo, "config", "mi6.trust", "true")
	st = f.resolve(repo)
	assertLayers(t, st, home, own)
	if st.Layers[1].Origin != OriginCheckout {
		t.Errorf("origin %q", st.Layers[1].Origin)
	}
	if len(st.Skipped) != 0 {
		t.Errorf("skipped %v", st.Skipped)
	}
}

func TestCheckoutLayerAppliesInWorktree(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	repo := f.repo("git", "mine")
	own := f.layer("git", "mine")
	f.git(repo, "config", "mi6.trust", "true")
	wt := f.worktree(repo, "tasks", "mine", "t1")

	st := f.resolve(wt)
	assertLayers(t, st, home, own)
}

func TestNoHomeLayer(t *testing.T) {
	f := newFixture(t)
	work := f.layer("git", "work")
	repo := f.repo("git", "work", "pipeline")

	st := f.resolve(repo)
	assertLayers(t, st, work)
}

func TestStaleWorktree(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	f.layer("git", "work")
	repo := f.repo("git", "work", "pipeline")
	wt := f.worktree(repo, "tasks", "pipeline", "t1")
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}

	st := f.resolve(wt)
	assertLayers(t, st, home)
	if len(st.Notes) != 1 || !strings.Contains(st.Notes[0], "main checkout missing") {
		t.Errorf("notes %v", st.Notes)
	}
}

func TestSymlinkedLayer(t *testing.T) {
	f := newFixture(t)
	home := f.layer()
	target := f.path("config-repo", "work")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(f.path("git", "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(f.path("git", "work"), LayerDir)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	repo := f.repo("git", "work", "pipeline")

	st := f.resolve(repo)
	assertLayers(t, st, home, link)
}

func TestMissingDir(t *testing.T) {
	f := newFixture(t)
	if _, err := Resolve(Options{Dir: f.path("nope"), Home: f.home}); err == nil {
		t.Error("expected an error for a missing directory")
	}
}

func TestDisplayPath(t *testing.T) {
	cases := map[string]string{
		"/home/u":          "~",
		"/home/u/.mi6":     "~/.mi6",
		"/home/user/.mi6":  "/home/user/.mi6",
		"/opt/thing/.mi6":  "/opt/thing/.mi6",
		"/home/u/git/work": "~/git/work",
	}
	for in, want := range cases {
		if got := DisplayPath(in, "/home/u"); got != want {
			t.Errorf("DisplayPath(%q) = %q, want %q", in, got, want)
		}
	}
}
