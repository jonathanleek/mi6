package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The launch replaces the process, so it is tested by running the built
// binary with fake tools on the PATH. Each fake prints its arguments and
// the variables a launcher sets.

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mi6-test")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "mi6")
	out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
	if err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	stdout, stderr string
	code           int
}

// mi6 runs the binary in dir with a fake home and state directory.
func mi6(t *testing.T, home, state, dir string, extraEnv []string, args ...string) result {
	t.Helper()
	fakeBin, _ := filepath.Abs(filepath.Join("testdata", "bin"))
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append([]string{
		"PATH=" + fakeBin + ":" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_STATE_HOME=" + state,
	}, extraEnv...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if e, ok := err.(*exec.ExitError); ok {
		code = e.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return result{out.String(), errb.String(), code}
}

func fixture(t *testing.T, withLayer bool) (home, state, project string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home = filepath.Join(root, "home")
	state = filepath.Join(root, "state")
	project = filepath.Join(home, "git", "proj")
	for _, d := range []string{state, project} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if withLayer {
		layer := filepath.Join(home, "git", ".mi6")
		if err := os.MkdirAll(layer, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(layer, "AGENTS.md"), []byte("tree rules\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		env := `{"MI6_TEST_VAR": "from-layer", "MI6_TEST_HOME": "~/data", "CLAUDE_CONFIG_DIR": "/hijack"}`
		if err := os.WriteFile(filepath.Join(layer, "env.json"), []byte(env), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home, state, project
}

func line(t *testing.T, out, key string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, key+"=") {
			return strings.TrimPrefix(l, key+"=")
		}
	}
	t.Errorf("no %s line in output:\n%s", key, out)
	return ""
}

func TestLaunchClaude(t *testing.T) {
	home, state, project := fixture(t, true)
	r := mi6(t, home, state, project, nil, "claude", "-p", "hello world")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	if got := line(t, r.stdout, "ARGS"); got != "-p hello world" {
		t.Errorf("args %q", got)
	}
	cfg := line(t, r.stdout, "CLAUDE_CONFIG_DIR")
	if !strings.HasPrefix(cfg, filepath.Join(state, "mi6", "sets")) || filepath.Base(cfg) != "claude" {
		t.Errorf("CLAUDE_CONFIG_DIR %q", cfg)
	}
	if line(t, r.stdout, "MI6_TOOL") != "claude" || line(t, r.stdout, "MI6_SET") != filepath.Dir(cfg) {
		t.Errorf("mi6 vars:\n%s", r.stdout)
	}
	if strings.Contains(r.stdout, "OPENCODE") {
		t.Errorf("opencode variables leaked into a claude launch:\n%s", r.stdout)
	}
	if line(t, r.stdout, "MI6_TEST_VAR") != "from-layer" {
		t.Errorf("layer env not exported:\n%s", r.stdout)
	}
	if line(t, r.stdout, "MI6_TEST_HOME") != filepath.Join(home, "data") {
		t.Errorf("~ in a layer env value not expanded:\n%s", r.stdout)
	}
	if strings.Contains(cfg, "hijack") {
		t.Error("a layer's env.json overrode the tool's config directory")
	}
	b, err := os.ReadFile(filepath.Join(cfg, "CLAUDE.md"))
	if err != nil || !strings.Contains(string(b), "tree rules") {
		t.Errorf("set not built: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cfg), "opencode")); err == nil {
		t.Error("a claude launch should build only the claude directory")
	}
	if r.stderr != "" {
		t.Errorf("unexpected stderr: %s", r.stderr)
	}
}

func TestLaunchOpenCode(t *testing.T) {
	home, state, project := fixture(t, true)
	r := mi6(t, home, state, project, nil, "opencode")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	dir := line(t, r.stdout, "OPENCODE_CONFIG_DIR")
	if filepath.Base(dir) != "opencode" {
		t.Errorf("OPENCODE_CONFIG_DIR %q", dir)
	}
	if line(t, r.stdout, "OPENCODE_CONFIG") != filepath.Join(dir, "opencode.json") {
		t.Errorf("OPENCODE_CONFIG:\n%s", r.stdout)
	}
	if line(t, r.stdout, "OPENCODE_DISABLE_EXTERNAL_SKILLS") != "1" {
		t.Errorf("external skills not disabled:\n%s", r.stdout)
	}
}

func TestLaunchBare(t *testing.T) {
	home, state, project := fixture(t, true)
	r := mi6(t, home, state, project, nil, "--bare", "claude", "x")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	if strings.Contains(r.stdout, "CLAUDE_CONFIG_DIR") || strings.Contains(r.stdout, "MI6_") {
		t.Errorf("bare launch set variables:\n%s", r.stdout)
	}
	if line(t, r.stdout, "ARGS") != "x" {
		t.Errorf("args:\n%s", r.stdout)
	}
	if entries, _ := os.ReadDir(filepath.Join(state, "mi6")); len(entries) != 0 {
		t.Error("bare launch built a set")
	}
}

func TestLaunchNoLayers(t *testing.T) {
	home, state, project := fixture(t, false)
	r := mi6(t, home, state, project, nil, "claude")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	if strings.Contains(r.stdout, "CLAUDE_CONFIG_DIR") {
		t.Errorf("no layers, yet a config dir was set:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "no .mi6 folder applies") {
		t.Errorf("stderr %q", r.stderr)
	}
}

func TestLaunchNested(t *testing.T) {
	home, state, project := fixture(t, true)
	r := mi6(t, home, state, project, []string{"MI6_TOOL=claude", "CLAUDE_CONFIG_DIR=/already/set"}, "claude")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	if line(t, r.stdout, "CLAUDE_CONFIG_DIR") != "/already/set" {
		t.Errorf("nested launch rebuilt the environment:\n%s", r.stdout)
	}
}

func TestLaunchErrors(t *testing.T) {
	home, state, project := fixture(t, true)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"aider"}, "unknown tool"},
		{[]string{"--bare"}, "usage"},
		{[]string{"--wat", "claude"}, "unknown option"},
		{[]string{}, "usage"},
	}
	for _, c := range cases {
		r := mi6(t, home, state, project, nil, c.args...)
		if r.code != 2 || !strings.Contains(r.stderr, c.want) {
			t.Errorf("%v: exit %d, stderr %q, want %q", c.args, r.code, r.stderr, c.want)
		}
	}
}

func TestToolNotOnPath(t *testing.T) {
	home, state, project := fixture(t, true)
	cmd := exec.Command(binary, "claude")
	cmd.Dir = project
	cmd.Env = []string{"PATH=/nonexistent", "HOME=" + home, "XDG_STATE_HOME=" + state}
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "not on your PATH") {
		t.Errorf("err %v, out %s", err, out)
	}
}

func TestResolveAndVersion(t *testing.T) {
	home, state, project := fixture(t, true)
	r := mi6(t, home, state, project, nil, "resolve")
	if r.code != 0 || !strings.Contains(r.stdout, "~/git/.mi6") || !strings.Contains(r.stdout, "set        ") {
		t.Errorf("resolve: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	r = mi6(t, home, state, project, nil, "version")
	if r.code != 0 || !strings.HasPrefix(r.stdout, "mi6 ") {
		t.Errorf("version: %q", r.stdout)
	}
}
