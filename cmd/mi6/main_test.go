package main

import (
	"encoding/json"
	"fmt"
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
	if _, err := os.Stat(filepath.Join(state, "mi6", "sets")); err == nil {
		t.Error("bare launch built a set")
	}
	if e := auditLast(t, state); e["outcome"] != "bare" || e["tool"] != "claude" {
		t.Errorf("audit entry %v", e)
	}
}

// auditLast returns the last line of the audit log, decoded.
func auditLast(t *testing.T, state string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(state, "mi6", "audit.jsonl"))
	if err != nil {
		t.Fatalf("no audit log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	var e map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &e); err != nil {
		t.Fatalf("audit line %q: %v", lines[len(lines)-1], err)
	}
	return e
}

func writeLayer(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const testCatalog = `{
	"tags": {"chinese": "Made in China", "network": "On the home network"},
	"providers": {"lmstudio": {"tags": ["network"]}, "anthropic": {}},
	"models": {"anthropic/claude-sonnet-5": {"claude": "sonnet"}, "lmstudio/qwen": {"tags": ["chinese"]}, "lmstudio/oss": {}},
	"deny": ["chinese"]
}`

func TestAuditLogsAPolicyLaunch(t *testing.T) {
	home, state, project := fixture(t, true)
	writeLayer(t, filepath.Join(home, ".mi6"), "models.json", testCatalog)
	r := mi6(t, home, state, project, nil, "opencode", "run", "hi")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	e := auditLast(t, state)
	if e["outcome"] != "started" || e["tool"] != "opencode" || e["dir"] != project || e["mi6"] == "" {
		t.Errorf("entry %v", e)
	}
	if !strings.HasPrefix(e["policy"].(string), "sha256:") {
		t.Errorf("policy %v", e["policy"])
	}
	if got := fmt.Sprint(e["allowed"]); got != "[anthropic/claude-sonnet-5 lmstudio/oss]" {
		t.Errorf("allowed %v", got)
	}
	if got := fmt.Sprint(e["removed"]); !strings.Contains(got, "lmstudio/qwen") || !strings.Contains(got, "deny chinese") {
		t.Errorf("removed %v", got)
	}
	if got := fmt.Sprint(e["deny"]); !strings.Contains(got, "chinese") || !strings.Contains(got, "~/.mi6") {
		t.Errorf("deny %v", got)
	}
	if got := fmt.Sprint(e["layers"]); !strings.Contains(got, "~/.mi6") || !strings.Contains(got, "~/git/.mi6") {
		t.Errorf("layers %v", got)
	}
	if got := fmt.Sprint(e["set"]); !strings.HasPrefix(got, "~/") && !strings.Contains(got, "sets") {
		t.Errorf("set %v", got)
	}
}

func TestPolicyRefusalIsLoggedAndBlocks(t *testing.T) {
	home, state, project := fixture(t, true)
	writeLayer(t, filepath.Join(home, ".mi6"), "models.json", testCatalog)
	// The tree layer allows only network models, which leaves Claude Code nothing.
	writeLayer(t, filepath.Join(home, "git", ".mi6"), "models.json", `{"allow": ["network"]}`)
	r := mi6(t, home, state, project, nil, "claude", "-p", "hi")
	if r.code == 0 || strings.Contains(r.stdout, "TOOL=claude") {
		t.Fatalf("the tool started:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "not starting claude:") || !strings.Contains(r.stderr, "no model is allowed here") || !strings.Contains(r.stderr, "not in allow (~/git/.mi6)") {
		t.Errorf("stderr:\n%s", r.stderr)
	}
	e := auditLast(t, state)
	if e["outcome"] != "refused" || fmt.Sprint(e["allowed"]) != "[]" || !strings.Contains(fmt.Sprint(e["reasons"]), "no model is allowed") {
		t.Errorf("entry %v", e)
	}
	// OpenCode still starts there, and its variables were stripped.
	r = mi6(t, home, state, project, []string{"OPENCODE_CONFIG_CONTENT={}", "MI6_TEST_KEEP=1"}, "opencode", "run")
	if r.code != 0 {
		t.Fatalf("opencode: exit %d\n%s", r.code, r.stderr)
	}
	if strings.Contains(r.stdout, "OPENCODE_CONFIG_CONTENT") || line(t, r.stdout, "MI6_TEST_KEEP") != "1" {
		t.Errorf("environment:\n%s", r.stdout)
	}
}

func TestUnwritableAuditLogRefuses(t *testing.T) {
	home, state, project := fixture(t, true)
	if err := os.MkdirAll(filepath.Join(state, "mi6", "audit.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"claude"}, {"--bare", "claude"}} {
		r := mi6(t, home, state, project, nil, args...)
		if r.code == 0 || strings.Contains(r.stdout, "TOOL=") || !strings.Contains(r.stderr, "audit log") {
			t.Errorf("%v: exit %d\nstdout %s\nstderr %s", args, r.code, r.stdout, r.stderr)
		}
	}
}

func TestResolveAndDoctorShowThePolicy(t *testing.T) {
	home, state, project := fixture(t, true)
	writeLayer(t, filepath.Join(home, ".mi6"), "models.json", testCatalog)
	r := mi6(t, home, state, project, nil, "resolve")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	for _, want := range []string{"models     policy sha256:", "deny   chinese    ~/.mi6", "claude    allowed  sonnet", "opencode  allowed  anthropic/claude-sonnet-5, lmstudio/oss", "removed  lmstudio/qwen"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("resolve lacks %q:\n%s", want, r.stdout)
		}
	}
	r = mi6(t, home, state, project, []string{"ANTHROPIC_BASE_URL=http://x"}, "doctor")
	if r.code != 0 {
		t.Fatalf("doctor exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{"ok    models   policy in force: claude 1 allowed, opencode 2 allowed", "warn  env      ANTHROPIC_BASE_URL is set in this shell", "ok    audit    "} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("doctor lacks %q:\n%s", want, r.stdout)
		}
	}

	// A refusal fails resolve and doctor, and names the tool.
	writeLayer(t, filepath.Join(home, "git", ".mi6"), "claude.json", `{"availableModels": ["opus"]}`)
	r = mi6(t, home, state, project, nil, "resolve")
	if r.code != 1 || !strings.Contains(r.stdout, "refused    claude\n  claude.json in ~/git/.mi6 sets availableModels") {
		t.Errorf("resolve exit %d:\n%s", r.code, r.stdout)
	}
	r = mi6(t, home, state, project, nil, "doctor")
	if r.code != 1 || !strings.Contains(r.stdout, "fail  models   claude: claude.json in ~/git/.mi6 sets availableModels") {
		t.Errorf("doctor exit %d:\n%s", r.code, r.stdout)
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

func TestDoctor(t *testing.T) {
	home, state, project := fixture(t, true)
	r := mi6(t, home, state, project, nil, "doctor")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{"ok    git", "ok    claude", "ok    opencode", "ok    state", "ok    layers   1 apply here", "ok    skills", "ok    session"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "fail") || strings.Contains(r.stdout, "warn") {
		t.Errorf("unexpected fail or warn:\n%s", r.stdout)
	}
}

func TestDoctorNoLayers(t *testing.T) {
	home, state, project := fixture(t, false)
	r := mi6(t, home, state, project, nil, "doctor")
	if r.code != 0 || !strings.Contains(r.stdout, "warn  layers   none apply") {
		t.Errorf("exit %d\n%s", r.code, r.stdout)
	}
}

func TestDoctorBrokenLayerFails(t *testing.T) {
	home, state, project := fixture(t, true)
	if err := os.WriteFile(filepath.Join(home, "git", ".mi6", "claude.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := mi6(t, home, state, project, nil, "doctor")
	if r.code != 1 || !strings.Contains(r.stdout, "fail  layer") || !strings.Contains(r.stdout, "claude.json") {
		t.Errorf("exit %d\n%s", r.code, r.stdout)
	}
}

func TestDoctorMissingToolFails(t *testing.T) {
	home, state, project := fixture(t, true)
	cmd := exec.Command(binary, "doctor")
	cmd.Dir = project
	// git only, no fake tools.
	cmd.Env = []string{"PATH=" + filepath.Dir(gitPath(t)), "HOME=" + home, "XDG_STATE_HOME=" + state}
	out, err := cmd.Output()
	if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 {
		t.Fatalf("err %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "fail  claude   claude is not on your PATH") {
		t.Errorf("out:\n%s", out)
	}
}

func TestDoctorInsideSessionWarns(t *testing.T) {
	home, state, project := fixture(t, true)
	r := mi6(t, home, state, project, []string{"MI6_TOOL=claude"}, "doctor")
	if r.code != 0 || !strings.Contains(r.stdout, "warn  session  MI6_TOOL=claude") {
		t.Errorf("exit %d\n%s", r.code, r.stdout)
	}
}

func gitPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInit(t *testing.T) {
	home, state, project := fixture(t, false)
	r := mi6(t, home, state, project, nil, "init")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{"layer      ~/git/proj/.mi6", "created  AGENTS.md", "created  claude.json", "created  skills/README.md"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".mi6", "env.json")); err != nil {
		t.Error("env.json not created")
	}

	// The new layer resolves. It is inside a plain directory, not a repo, so
	// no trust is needed and no hint is printed.
	if strings.Contains(r.stdout, "note") {
		t.Errorf("hint outside a repo:\n%s", r.stdout)
	}
	r = mi6(t, home, state, project, nil, "resolve")
	if !strings.Contains(r.stdout, "~/git/proj/.mi6") {
		t.Errorf("new layer not in the stack:\n%s", r.stdout)
	}

	r = mi6(t, home, state, project, nil, "init")
	if r.code != 0 || strings.Contains(r.stdout, "created") || !strings.Contains(r.stdout, "kept     AGENTS.md") {
		t.Errorf("second init:\n%s", r.stdout)
	}
}
