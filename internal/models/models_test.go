package models

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jonathanleek/mi6/internal/layer"
)

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

// stack builds layers named /l0, /l1, ... from models.json texts. An empty
// text is a layer with no models.json.
func stack(t *testing.T, texts ...string) []*layer.Layer {
	t.Helper()
	var layers []*layer.Layer
	for i, s := range texts {
		layers = append(layers, &layer.Layer{Path: "/l" + string(rune('0'+i)), Models: obj(t, s)})
	}
	return layers
}

func display(p string) string { return p }

const catalog = `{
	"tags": {"chinese": "Made in China", "network": "On the home network", "cloud": "A third-party API", "anthropic": "Made by Anthropic"},
	"providers": {"anthropic": {"tags": ["cloud"]}, "lmstudio": {"tags": ["network"]}},
	"models": {
		"anthropic/claude-sonnet-5": {"tags": ["anthropic"], "claude": "sonnet"},
		"anthropic/claude-haiku-5": {"tags": ["anthropic"], "claude": "haiku"},
		"lmstudio/qwen3-coder-30b": {"tags": ["chinese"]},
		"lmstudio/gpt-oss-120b": {"tags": []}
	}
}`

func ids(r *Result) string { return strings.Join(r.IDs(), " ") }

func removed(r *Result) string {
	var out []string
	for _, x := range r.Removed {
		out = append(out, x.Key+": "+x.Why)
	}
	return strings.Join(out, "; ")
}

func TestNoPolicy(t *testing.T) {
	p := Merge(stack(t, ""), display)
	if p.Active() || len(p.Errors) > 0 || len(p.Models) > 0 {
		t.Errorf("empty stack: %+v", p)
	}
	// A catalog with no rule restricts nothing, but a model a tool has no
	// name for is still not offered to it.
	p = Merge(stack(t, catalog), display)
	if p.Active() || len(p.Errors) > 0 {
		t.Errorf("catalog without rules: active=%v errors=%v", p.Active(), p.Errors)
	}
	oc := p.Evaluate("opencode", KeyID)
	if got := ids(oc); got != "anthropic/claude-haiku-5 anthropic/claude-sonnet-5 lmstudio/gpt-oss-120b lmstudio/qwen3-coder-30b" || oc.Active {
		t.Errorf("opencode: %q active=%v", got, oc.Active)
	}
	cl := p.Evaluate("claude", NamedID("claude"))
	if got := ids(cl); got != "haiku sonnet" {
		t.Errorf("claude: %q", got)
	}
	if got := removed(cl); got != "lmstudio/gpt-oss-120b: no claude name; lmstudio/qwen3-coder-30b: no claude name" {
		t.Errorf("claude removed: %q", got)
	}
}

func TestProviderTagsReachModels(t *testing.T) {
	p := Merge(stack(t, catalog), display)
	if got := p.Models["lmstudio/qwen3-coder-30b"].Tags; !reflect.DeepEqual(got, []string{"chinese", "network"}) {
		t.Errorf("tags %v", got)
	}
	if got := p.Models["lmstudio/gpt-oss-120b"].Tags; !reflect.DeepEqual(got, []string{"network"}) {
		t.Errorf("tags %v", got)
	}
	if got := p.Models["anthropic/claude-sonnet-5"].IDs; !reflect.DeepEqual(got, map[string]string{"claude": "sonnet"}) {
		t.Errorf("ids %v", got)
	}
}

func TestRules(t *testing.T) {
	cases := []struct {
		name          string
		rules         []string // models.json texts for the layers after the catalog
		wantOpenCode  string
		wantClaude    string
		wantOCRemoved string
	}{
		{"deny by tag",
			[]string{`{"deny": ["chinese"]}`},
			"anthropic/claude-haiku-5 anthropic/claude-sonnet-5 lmstudio/gpt-oss-120b", "haiku sonnet",
			"lmstudio/qwen3-coder-30b: deny chinese (/l1)"},
		{"allow by tag",
			[]string{`{"allow": ["network"]}`},
			"lmstudio/gpt-oss-120b lmstudio/qwen3-coder-30b", "",
			"anthropic/claude-haiku-5: not in allow (/l1); anthropic/claude-sonnet-5: not in allow (/l1)"},
		{"deny above, allow below",
			[]string{`{"deny": ["chinese"]}`, `{"allow": ["network"]}`},
			"lmstudio/gpt-oss-120b", "",
			"anthropic/claude-haiku-5: not in allow (/l2); anthropic/claude-sonnet-5: not in allow (/l2); lmstudio/qwen3-coder-30b: deny chinese (/l1)"},
		{"allow is any of its tags",
			[]string{`{"allow": ["network", "anthropic"]}`},
			"anthropic/claude-haiku-5 anthropic/claude-sonnet-5 lmstudio/gpt-oss-120b lmstudio/qwen3-coder-30b", "haiku sonnet", ""},
		{"a lower allow can only narrow",
			[]string{`{"allow": ["network"]}`, `{"allow": ["network", "cloud", "anthropic"]}`},
			"lmstudio/gpt-oss-120b lmstudio/qwen3-coder-30b", "",
			"anthropic/claude-haiku-5: not in allow (/l1); anthropic/claude-sonnet-5: not in allow (/l1)"},
		{"a lower layer cannot lift a deny",
			[]string{`{"deny": ["chinese"]}`, `{"deny": []}`},
			"anthropic/claude-haiku-5 anthropic/claude-sonnet-5 lmstudio/gpt-oss-120b", "haiku sonnet",
			"lmstudio/qwen3-coder-30b: deny chinese (/l1)"},
		{"a lower layer adds a tag, never removes one",
			[]string{`{"deny": ["chinese"]}`, `{"models": {"lmstudio/qwen3-coder-30b": {"tags": []}, "lmstudio/gpt-oss-120b": {"tags": ["chinese"]}}}`},
			"anthropic/claude-haiku-5 anthropic/claude-sonnet-5", "haiku sonnet",
			"lmstudio/gpt-oss-120b: deny chinese (/l1); lmstudio/qwen3-coder-30b: deny chinese (/l1)"},
		{"a lower layer adds a model and a tool name",
			[]string{`{"deny": ["chinese"]}`, `{"tags": {"local": "This machine"}, "providers": {"ollama": {"tags": ["local"]}}, "models": {"ollama/llama": {"tags": []}, "anthropic/claude-haiku-5": {"claude": "haiku-5"}}}`},
			"anthropic/claude-haiku-5 anthropic/claude-sonnet-5 lmstudio/gpt-oss-120b ollama/llama", "haiku-5 sonnet",
			"lmstudio/qwen3-coder-30b: deny chinese (/l1)"},
		{"deny wins over allow",
			[]string{`{"allow": ["chinese"], "deny": ["chinese"]}`},
			"", "",
			"anthropic/claude-haiku-5: not in allow (/l1); anthropic/claude-sonnet-5: not in allow (/l1); lmstudio/gpt-oss-120b: not in allow (/l1); lmstudio/qwen3-coder-30b: deny chinese (/l1)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Merge(stack(t, append([]string{catalog}, c.rules...)...), display)
			if len(p.Errors) > 0 {
				t.Fatalf("errors %v", p.Errors)
			}
			if !p.Active() {
				t.Fatal("rules should make the policy active")
			}
			oc := p.Evaluate("opencode", KeyID)
			if got := ids(oc); got != c.wantOpenCode {
				t.Errorf("opencode allowed %q, want %q", got, c.wantOpenCode)
			}
			if got := removed(oc); got != c.wantOCRemoved {
				t.Errorf("opencode removed %q, want %q", got, c.wantOCRemoved)
			}
			if got := ids(p.Evaluate("claude", NamedID("claude"))); got != c.wantClaude {
				t.Errorf("claude allowed %q, want %q", got, c.wantClaude)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name  string
		texts []string
		want  string // a substring of one error
	}{
		{"tag used in deny but never defined", []string{catalog, `{"deny": ["chineese"]}`}, `tag "chineese" in deny is not defined`},
		{"tag used in allow but never defined", []string{catalog, `{"allow": ["locl"]}`}, `tag "locl" in allow is not defined`},
		{"tag attached to a model but never defined", []string{catalog, `{"models": {"lmstudio/x": {"tags": ["fast"]}}}`}, `tag "fast" in model lmstudio/x is not defined`},
		{"tag attached to a provider but never defined", []string{catalog, `{"providers": {"lmstudio": {"tags": ["home"]}}}`}, `tag "home" in provider lmstudio is not defined`},
		{"model with no provider entry", []string{catalog, `{"models": {"openrouter/x": {"tags": []}}}`}, `model "openrouter/x" names provider "openrouter", which has no providers entry`},
		{"model key without a provider", []string{`{"models": {"sonnet": {}}}`}, `must be named provider/model`},
		{"unknown top-level key", []string{`{"denies": ["chinese"]}`}, `unknown key "denies"`},
		{"tags not an object", []string{`{"tags": ["chinese"]}`}, `tags must be an object`},
		{"tag without a meaning", []string{`{"tags": {"chinese": 1}}`}, `tag "chinese" needs a meaning`},
		{"deny not a list", []string{`{"deny": "chinese"}`}, `deny must be a list of tags`},
		{"provider with an unknown key", []string{`{"providers": {"x": {"tag": ["a"]}}}`}, `provider "x" has an unknown key "tag"`},
		{"tool name not a string", []string{`{"providers": {"a": {}}, "models": {"a/b": {"claude": ["x"]}}}`}, `"claude" must be the name the claude tool uses`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Merge(stack(t, c.texts...), display)
			for _, e := range p.Errors {
				if strings.Contains(e, c.want) {
					if !strings.HasPrefix(e, "models.json in /l") {
						t.Errorf("error should name the layer: %q", e)
					}
					return
				}
			}
			t.Errorf("errors %v lack %q", p.Errors, c.want)
		})
	}
}

func TestErrorInOneLayerDoesNotHideOthers(t *testing.T) {
	// A file with the wrong shape is reported and skipped, but the rest of
	// the stack still merges, so the message can point at every problem.
	p := Merge(stack(t, catalog, `{"deny": "chinese"}`, `{"deny": ["chinese"]}`), display)
	if len(p.Errors) != 1 || !strings.Contains(p.Errors[0], "/l1") {
		t.Errorf("errors %v", p.Errors)
	}
	if got := removed(p.Evaluate("opencode", KeyID)); got != "lmstudio/qwen3-coder-30b: deny chinese (/l2)" {
		t.Errorf("removed %q", got)
	}
}

func TestMeaningAndFrom(t *testing.T) {
	p := Merge(stack(t, catalog, `{"tags": {"chinese": "Made by a company based in China"}, "deny": ["chinese"]}`, `{"deny": ["chinese", "cloud"]}`), display)
	if tag := p.Tags["chinese"]; tag.From != "/l0" || tag.Meaning != "Made by a company based in China" {
		t.Errorf("tag %+v: the first layer defines it, the nearest meaning wins", tag)
	}
	if !reflect.DeepEqual(p.Deny, []Rule{{"chinese", "/l1"}, {"cloud", "/l2"}}) {
		t.Errorf("deny %v: a rule keeps the first layer that listed it", p.Deny)
	}
	if got := p.TagNames(); !reflect.DeepEqual(got, []string{"anthropic", "chinese", "cloud", "network"}) {
		t.Errorf("tag names %v", got)
	}
}

func TestHash(t *testing.T) {
	a := Merge(stack(t, catalog, `{"deny": ["chinese"]}`), display).Hash()
	b := Merge(stack(t, catalog, `{"deny": ["chinese"]}`), display).Hash()
	c := Merge(stack(t, catalog, `{"deny": ["cloud"]}`), display).Hash()
	if a != b || a == c || !strings.HasPrefix(a, "sha256:") {
		t.Errorf("hashes %s %s %s", a, b, c)
	}
}

func TestResultHelpers(t *testing.T) {
	p := Merge(stack(t, catalog, `{"deny": ["chinese"]}`), display)
	r := p.Evaluate("claude", NamedID("claude"))
	if !r.Allows("sonnet") || r.Allows("opus") || r.Allows("lmstudio/gpt-oss-120b") {
		t.Errorf("allows: %+v", r.Allowed)
	}
}
