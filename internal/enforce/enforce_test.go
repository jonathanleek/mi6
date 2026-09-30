package enforce

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/set"
	"github.com/jonathanleek/mi6/internal/tool"
)

const catalog = `{
	"tags": {"chinese": "Made in China", "network": "On the home network", "cloud": "A third-party API"},
	"providers": {"anthropic": {"tags": ["cloud"]}, "lmstudio": {"tags": ["network"]}},
	"models": {
		"anthropic/claude-sonnet-5": {"tags": [], "claude": "sonnet"},
		"lmstudio/qwen3-coder-30b": {"tags": ["chinese"]},
		"lmstudio/gpt-oss-120b": {"tags": []}
	},
	"deny": ["chinese"]
}`

func obj(t *testing.T, s string) layer.Object {
	t.Helper()
	if s == "" {
		return nil
	}
	o, err := layer.ParseObject([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func display(p string) string { return p }

// input builds a stack of a home layer with the catalog and one more layer.
func input(t *testing.T, name string, home, near *layer.Layer, args ...string) Input {
	t.Helper()
	home.Path, near.Path = "/home/.mi6", "/near/.mi6"
	if home.Models == nil {
		home.Models = obj(t, catalog)
	}
	layers := []*layer.Layer{home, near}
	tl, err := tool.Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	return Input{Tool: tl, Layers: layers, Merged: merge.Stack(layers, display), Dir: t.TempDir(), Args: args, Display: display}
}

func has(refusals []string, want string) bool {
	for _, r := range refusals {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}

func TestNoPolicy(t *testing.T) {
	l := &layer.Layer{Claude: obj(t, `{"availableModels": ["opus"], "model": "opus", "env": {"ANTHROPIC_BASE_URL": "x"}}`)}
	in := input(t, "claude", &layer.Layer{Models: obj(t, `{"tags": {}}`)}, l, "--settings", "{}")
	if got := Check(in); got != nil {
		t.Errorf("without a rule nothing is refused: %v", got)
	}
}

func TestCatalogErrorsRefuseEvenWithoutRules(t *testing.T) {
	in := input(t, "claude", &layer.Layer{Models: obj(t, `{"deny": "chinese"}`)}, &layer.Layer{})
	if got := Check(in); len(got) != 1 || !has(got, "deny must be a list") {
		t.Errorf("got %v", got)
	}
}

type mute struct{ tool.Tool }

func (mute) Name() string { return "mute" }

func TestToolThatCannotEnforce(t *testing.T) {
	in := input(t, "claude", &layer.Layer{}, &layer.Layer{})
	in.Tool = mute{in.Tool}
	if got := Check(in); len(got) != 1 || !has(got, "mute has no way to enforce") {
		t.Errorf("got %v", got)
	}
}

func TestClean(t *testing.T) {
	near := &layer.Layer{Claude: obj(t, `{"model": "sonnet", "permissions": {"deny": ["x"]}}`), OpenCode: obj(t, `{"model": "lmstudio/gpt-oss-120b", "small_model": "anthropic/claude-sonnet-5", "agent": {"build": {"model": "anthropic/claude-sonnet-5"}}}`)}
	for _, name := range []string{"claude", "opencode"} {
		if got := Check(input(t, name, &layer.Layer{}, near, "-p", "hi", "--model", "opus")); got != nil {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestLayerRefusals(t *testing.T) {
	cases := []struct {
		name, tool string
		near       *layer.Layer
		want       string
	}{
		{"hand-written availableModels", "claude", &layer.Layer{Claude: obj(t, `{"availableModels": ["sonnet"]}`)}, "claude.json in /near/.mi6 sets availableModels; model lists belong to models.json"},
		{"enforceAvailableModels off", "claude", &layer.Layer{Claude: obj(t, `{"enforceAvailableModels": false}`)}, "sets enforceAvailableModels"},
		{"enabled_providers", "opencode", &layer.Layer{OpenCode: obj(t, `{"enabled_providers": ["lmstudio"]}`)}, "opencode.json in /near/.mi6 sets enabled_providers"},
		{"a provider whitelist", "opencode", &layer.Layer{OpenCode: obj(t, `{"provider": {"lmstudio": {"whitelist": ["x"]}}}`)}, "sets provider.lmstudio.whitelist"},
		{"a redirecting variable in env.json", "claude", &layer.Layer{Env: map[string]string{"ANTHROPIC_BASE_URL": "http://x"}}, "env.json in /near/.mi6 sets ANTHROPIC_BASE_URL, which redirects claude's model"},
		{"a redirecting variable in the settings env", "claude", &layer.Layer{Claude: obj(t, `{"env": {"ANTHROPIC_DEFAULT_SONNET_MODEL": "x"}}`)}, "claude.json in /near/.mi6 sets env.ANTHROPIC_DEFAULT_SONNET_MODEL"},
		{"OPENCODE_CONFIG_CONTENT in env.json", "opencode", &layer.Layer{Env: map[string]string{"OPENCODE_CONFIG_CONTENT": "{}"}}, "env.json in /near/.mi6 sets OPENCODE_CONFIG_CONTENT"},
		{"a default model that is denied", "opencode", &layer.Layer{OpenCode: obj(t, `{"model": "lmstudio/qwen3-coder-30b"}`)}, `opencode.json in /near/.mi6 names model "lmstudio/qwen3-coder-30b", which is not allowed here`},
		{"a small_model outside the catalog", "opencode", &layer.Layer{OpenCode: obj(t, `{"small_model": "openrouter/x"}`)}, `names small_model "openrouter/x"`},
		{"an agent model that is denied", "opencode", &layer.Layer{OpenCode: obj(t, `{"agent": {"fast": {"model": "lmstudio/qwen3-coder-30b"}}}`)}, `names agent.fast.model`},
		{"a claude default outside the list", "claude", &layer.Layer{Claude: obj(t, `{"model": "opus"}`)}, `claude.json in /near/.mi6 names model "opus"`},
		{"a passed-through --settings", "claude", &layer.Layer{}, "the argument --settings replaces claude's settings"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var args []string
			if strings.Contains(c.name, "--settings") {
				args = []string{"--settings={}", "-p"}
			}
			got := Check(input(t, c.tool, &layer.Layer{}, c.near, args...))
			if !has(got, c.want) {
				t.Errorf("got %v\nwant one containing %q", got, c.want)
			}
		})
	}
}

func TestModelKeyNamesTheLayerThatSetIt(t *testing.T) {
	home := &layer.Layer{Claude: obj(t, `{"model": "sonnet"}`)}
	near := &layer.Layer{Claude: obj(t, `{"model": "opus"}`)}
	got := Check(input(t, "claude", home, near))
	if len(got) != 1 || !strings.HasPrefix(got[0], "claude.json in /near/.mi6 names model") {
		t.Errorf("got %v", got)
	}
}

func TestEmptyListExplains(t *testing.T) {
	near := &layer.Layer{Models: obj(t, `{"allow": ["network"]}`)}
	got := Check(input(t, "claude", &layer.Layer{}, near))
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	want := "no model is allowed here\n" +
		"  deny   chinese    from /home/.mi6\n" +
		"  allow  network    from /near/.mi6\n" +
		"  claude models in the catalog:\n" +
		"    anthropic/claude-sonnet-5        tags: cloud                        removed: not in allow (/near/.mi6)\n" +
		"    lmstudio/gpt-oss-120b            tags: network                      removed: no claude name\n" +
		"    lmstudio/qwen3-coder-30b         tags: chinese, network             removed: no claude name"
	if got[0] != want {
		t.Errorf("got:\n%s\nwant:\n%s", got[0], want)
	}
	// The same layer leaves OpenCode one model, so it may start.
	if got := Check(input(t, "opencode", &layer.Layer{}, near)); got != nil {
		t.Errorf("opencode: %v", got)
	}
}

func TestCheckout(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(filepath.Join(sub, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	in := input(t, "claude", &layer.Layer{}, &layer.Layer{})
	in.Dir, in.Checkout = sub, root
	if got := Check(in); got != nil {
		t.Fatalf("clean checkout: %v", got)
	}

	write(".claude/settings.json", `{"permissions": {"allow": ["Bash(ls)"]}}`)
	if got := Check(in); got != nil {
		t.Errorf("a checkout file that leaves models alone is fine: %v", got)
	}
	write(".claude/settings.json", `{"availableModels": ["opus"]}`)
	if got := Check(in); len(got) != 1 || !has(got, filepath.Join(root, ".claude/settings.json")+" sets availableModels; the checkout's settings outrank") {
		t.Errorf("root file: %v", got)
	}
	write(".claude/settings.json", `{}`)
	write("a/b/.claude/settings.local.json", `{"env": {"ANTHROPIC_BASE_URL": "http://x"}}`)
	if got := Check(in); len(got) != 1 || !has(got, "a/b/.claude/settings.local.json sets env.ANTHROPIC_BASE_URL") {
		t.Errorf("working-directory file: %v", got)
	}
	write("a/b/.claude/settings.local.json", `{"model": "sonnet"}`)
	if got := Check(in); len(got) != 1 || !has(got, "sets model;") {
		t.Errorf("a model key, even an allowed one, touches models: %v", got)
	}
	write("a/b/.claude/settings.local.json", `not json`)
	if got := Check(in); len(got) != 1 || !has(got, "cannot be read") {
		t.Errorf("unreadable: %v", got)
	}

	oc := input(t, "opencode", &layer.Layer{}, &layer.Layer{})
	oc.Dir, oc.Checkout = sub, root
	write("a/b/.claude/settings.local.json", `{}`)
	write("a/.opencode/opencode.json", `{"provider": {"anthropic": {"options": {"baseURL": "http://x"}}}}`)
	if got := Check(oc); len(got) != 1 || !has(got, "a/.opencode/opencode.json sets provider;") {
		t.Errorf("opencode provider: %v", got)
	}
	write("a/.opencode/opencode.json", `{"theme": "dark"}`)
	if got := Check(oc); got != nil {
		t.Errorf("harmless opencode file: %v", got)
	}
}

func TestDirs(t *testing.T) {
	if got := dirs("/r/a/b", "/r"); !reflect.DeepEqual(got, []string{"/r/a/b", "/r/a", "/r"}) {
		t.Errorf("%v", got)
	}
	if got := dirs("/r", "/r"); !reflect.DeepEqual(got, []string{"/r"}) {
		t.Errorf("%v", got)
	}
	if got := dirs("/x", "/r"); !reflect.DeepEqual(got, []string{"/x", "/r"}) {
		t.Errorf("not below: %v", got)
	}
	if got := dirs("/x", ""); !reflect.DeepEqual(got, []string{"/x"}) {
		t.Errorf("no checkout: %v", got)
	}
}

func TestEnv(t *testing.T) {
	c, _ := tool.Lookup("claude")
	env := []string{"HOME=/h", "ANTHROPIC_BASE_URL=http://x", "ANTHROPIC_MODEL=m", "PATH=/bin"}
	if got := Env(env, c); !reflect.DeepEqual(got, []string{"HOME=/h", "PATH=/bin"}) {
		t.Errorf("%v", got)
	}
	if got := Env(env, mute{c}); !reflect.DeepEqual(got, env) {
		t.Errorf("a tool without variables keeps the environment: %v", got)
	}
}

func TestFind(t *testing.T) {
	o := obj(t, `{"model": "m", "agent": {"a": {"model": "x"}, "b": {"model": "y"}, "c": {}}, "provider": {"p": {"whitelist": ["w"]}}}`)
	got := find(o, "agent.*.model")
	if len(got) != 2 || got[0].path != "agent.a.model" || got[0].value != "x" || got[1].path != "agent.b.model" {
		t.Errorf("%v", got)
	}
	if got := find(o, "provider.*.whitelist"); len(got) != 1 || got[0].path != "provider.p.whitelist" {
		t.Errorf("%v", got)
	}
	if got := find(o, "model"); len(got) != 1 || got[0].value != "m" {
		t.Errorf("%v", got)
	}
	if got := find(o, "small_model"); got != nil {
		t.Errorf("%v", got)
	}
	if got := find(nil, "model"); got != nil {
		t.Errorf("%v", got)
	}
}

// The plans write the lists the tools enforce, and leave the merged
// settings alone.
func TestPlansWriteTheLists(t *testing.T) {
	near := &layer.Layer{OpenCode: obj(t, `{"provider": {"lmstudio": {"npm": "x"}}}`), Models: obj(t, `{"allow": ["network", "cloud"]}`)}
	in := input(t, "opencode", &layer.Layer{}, near)
	before := len(in.Merged.OpenCode["provider"].(map[string]any)["lmstudio"].(map[string]any))
	plan, err := in.Tool.Plan(in.Merged, "/set/opencode")
	if err != nil {
		t.Fatal(err)
	}
	cfg := parsePlan(t, plan, "opencode.json")
	if got, _ := cfg["enabled_providers"].([]any); !reflect.DeepEqual(got, []any{"anthropic", "lmstudio"}) {
		t.Errorf("enabled_providers %v", cfg["enabled_providers"])
	}
	lm := cfg["provider"].(map[string]any)["lmstudio"].(map[string]any)
	if lm["npm"] != "x" || !reflect.DeepEqual(lm["whitelist"], []any{"gpt-oss-120b"}) {
		t.Errorf("lmstudio %v", lm)
	}
	an := cfg["provider"].(map[string]any)["anthropic"].(map[string]any)
	if !reflect.DeepEqual(an["whitelist"], []any{"claude-sonnet-5"}) {
		t.Errorf("anthropic %v", an)
	}
	if after := len(in.Merged.OpenCode["provider"].(map[string]any)["lmstudio"].(map[string]any)); after != before {
		t.Error("the merged settings were changed")
	}

	cin := input(t, "claude", &layer.Layer{}, &layer.Layer{})
	plan, err = cin.Tool.Plan(cin.Merged, "/set/claude")
	if err != nil {
		t.Fatal(err)
	}
	settings := parsePlan(t, plan, "settings.json")
	if !reflect.DeepEqual(settings["availableModels"], []any{"sonnet"}) || settings["enforceAvailableModels"] != true {
		t.Errorf("settings %v", settings)
	}
}

func parsePlan(t *testing.T, plan *set.Plan, name string) layer.Object {
	t.Helper()
	for _, f := range plan.Files {
		if filepath.Base(f.Path) == name {
			return obj(t, string(f.Content))
		}
	}
	t.Fatalf("no %s in plan", name)
	return nil
}
