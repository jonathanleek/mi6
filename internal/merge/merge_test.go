package merge

import (
	"encoding/json"
	"fmt"
	"reflect"
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

func TestJSON(t *testing.T) {
	cases := []struct{ name, base, over, want string }{
		{"nil over keeps base", `{"a":1}`, ``, `{"a":1}`},
		{"nil base copies over", ``, `{"a":1}`, `{"a":1}`},
		{"scalar takes over", `{"a":1}`, `{"a":2}`, `{"a":2}`},
		{"new key added", `{"a":1}`, `{"b":2}`, `{"a":1,"b":2}`},
		{"objects merge key by key", `{"p":{"x":1,"y":1}}`, `{"p":{"y":2,"z":2}}`, `{"p":{"x":1,"y":2,"z":2}}`},
		{"lists union in order", `{"l":["a","b"]}`, `{"l":["b","c"]}`, `{"l":["a","b","c"]}`},
		{"list of objects dedupes deeply", `{"l":[{"k":1}]}`, `{"l":[{"k":1},{"k":2}]}`, `{"l":[{"k":1},{"k":2}]}`},
		{"over cannot remove a list item", `{"l":["a"]}`, `{"l":[]}`, `{"l":["a"]}`},
		{"type change takes over", `{"a":[1]}`, `{"a":"s"}`, `{"a":"s"}`},
		{"null takes over", `{"a":1}`, `{"a":null}`, `{"a":null}`},
		{"permissions example",
			`{"permissions":{"allow":["Bash(git status)"],"deny":["Read(~/.ssh/**)"]}}`,
			`{"permissions":{"allow":["Bash(make *)"],"deny":["Bash(git push *)"]}}`,
			`{"permissions":{"allow":["Bash(git status)","Bash(make *)"],"deny":["Read(~/.ssh/**)","Bash(git push *)"]}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base, over := obj(t, c.base), obj(t, c.over)
			baseCopy, _ := json.Marshal(base)
			got := JSON(base, over)
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(obj(t, c.want))
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("got %s, want %s", gotJSON, wantJSON)
			}
			if after, _ := json.Marshal(base); string(after) != string(baseCopy) {
				t.Errorf("base was modified: %s", after)
			}
		})
	}
}

func TestStack(t *testing.T) {
	home := &layer.Layer{
		Path:         "/h/.mi6",
		Instructions: "home rules\n",
		Skills:       map[string]layer.Skill{"shared": {Name: "shared", Dir: "/h/.mi6/skills/shared"}},
		Claude:       obj(t, `{"permissions":{"deny":["a"]}}`),
	}
	tree := &layer.Layer{
		Path:   "/h/git/.mi6",
		Skills: map[string]layer.Skill{"shared": {Name: "shared", Dir: "/h/git/.mi6/skills/shared"}, "other": {Name: "other", Dir: "/x"}},
		MCP:    obj(t, `{"mcpServers":{"gh":{"command":"gh"}}}`),
	}
	work := &layer.Layer{
		Path:         "/h/git/work/.mi6",
		Instructions: "work rules\n",
		Claude:       obj(t, `{"permissions":{"deny":["b"]},"model":"x"}`),
		Warnings:     []string{"layer warning"},
	}
	display := func(p string) string { return "~" + p[2:] }

	m := Stack([]*layer.Layer{home, tree, work}, display)

	wantText := "<!-- mi6 layer: ~/.mi6 -->\n\nhome rules\n\n<!-- mi6 layer: ~/git/work/.mi6 -->\n\nwork rules\n"
	if m.Instructions != wantText {
		t.Errorf("instructions:\n%q\nwant\n%q", m.Instructions, wantText)
	}
	if !reflect.DeepEqual(m.SkillNames(), []string{"other", "shared"}) {
		t.Errorf("skills %v", m.SkillNames())
	}
	if m.Skills["shared"].Dir != "/h/git/.mi6/skills/shared" {
		t.Errorf("nearest layer should win: %v", m.Skills["shared"])
	}
	got, _ := json.Marshal(m.Claude)
	if string(got) != `{"model":"x","permissions":{"deny":["a","b"]}}` {
		t.Errorf("claude %s", got)
	}
	if m.MCP["mcpServers"] == nil || m.OpenCode != nil {
		t.Errorf("mcp %v opencode %v", m.MCP, m.OpenCode)
	}
	wantWarn := []string{"layer warning", "skill shared from ~/git/.mi6 hides the one from ~/.mi6/skills/shared"}
	if !reflect.DeepEqual(m.Warnings, wantWarn) {
		t.Errorf("warnings %v", m.Warnings)
	}
}

func TestStackEmpty(t *testing.T) {
	m := Stack(nil, func(s string) string { return s })
	if m.Instructions != "" || len(m.Skills) != 0 || m.Claude != nil {
		t.Errorf("empty stack: %+v", m)
	}
}

func TestNarrowing(t *testing.T) {
	display := func(p string) string { return p }
	cases := []struct {
		name         string
		outer, inner string
		want         string
		warn         bool
	}{
		{"nearer narrows in outer order", `["opus","sonnet","haiku"]`, `["haiku","sonnet"]`, `["sonnet","haiku"]`, false},
		{"nearer cannot widen", `["sonnet"]`, `["sonnet","opus"]`, `["sonnet"]`, false},
		{"only outer sets it", `["sonnet"]`, ``, `["sonnet"]`, false},
		{"only nearer sets it", ``, `["sonnet"]`, `["sonnet"]`, false},
		{"no match keeps outer and warns", `["claude-sonnet-5"]`, `["sonnet"]`, `["claude-sonnet-5"]`, true},
		{"empty nearer is ignored", `["sonnet"]`, `[]`, `["sonnet"]`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var layers []*layer.Layer
			for i, list := range []string{c.outer, c.inner} {
				l := &layer.Layer{Path: fmt.Sprintf("/l%d", i)}
				if list != "" {
					l.Claude = obj(t, `{"availableModels":`+list+`,"model":"x"}`)
					l.OpenCode = obj(t, `{"enabled_providers":`+list+`}`)
				}
				layers = append(layers, l)
			}
			m := Stack(layers, display)
			for name, got := range map[string]any{"claude": m.Claude["availableModels"], "opencode": m.OpenCode["enabled_providers"]} {
				gotJSON, _ := json.Marshal(got)
				if string(gotJSON) != c.want {
					t.Errorf("%s: got %s, want %s", name, gotJSON, c.want)
				}
			}
			if (len(m.Warnings) > 0) != c.warn {
				t.Errorf("warnings %v, want warning=%v", m.Warnings, c.warn)
			}
			if c.outer != "" && c.inner != "" && m.Claude["model"] != "x" {
				t.Errorf("other keys must still merge: %v", m.Claude)
			}
		})
	}
}

func TestNarrowingThreeLayers(t *testing.T) {
	display := func(p string) string { return p }
	layers := []*layer.Layer{
		{Path: "/a", Claude: obj(t, `{"availableModels":["opus","sonnet","haiku"]}`)},
		{Path: "/b", Claude: obj(t, `{"availableModels":["sonnet","haiku"]}`)},
		{Path: "/c", Claude: obj(t, `{"availableModels":["haiku","opus"]}`)},
	}
	got, _ := json.Marshal(Stack(layers, display).Claude["availableModels"])
	if string(got) != `["haiku"]` {
		t.Errorf("got %s, want [\"haiku\"]", got)
	}
}
