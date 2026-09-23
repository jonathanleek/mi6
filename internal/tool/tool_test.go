package tool

import (
	"encoding/json"
	"testing"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
)

func TestRegistry(t *testing.T) {
	if got := Names(); len(got) != 2 || got[0] != "claude" || got[1] != "opencode" {
		t.Errorf("names %v", got)
	}
	if _, err := Lookup("aider"); err == nil {
		t.Error("unknown tool should be an error")
	}
}

func TestTranslateMCP(t *testing.T) {
	in, err := layer.ParseObject([]byte(`{
		"gh": {"command": "gh", "args": ["mcp", "serve"], "env": {"A": "1"}},
		"wh": {"url": "https://x/sse", "headers": {"H": "v"}},
		"bare": {"command": "solo"},
		"junk": {"neither": true}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(translateMCP(in))
	want := `{"bare":{"command":["solo"],"enabled":true,"type":"local"},` +
		`"gh":{"command":["gh","mcp","serve"],"enabled":true,"environment":{"A":"1"},"type":"local"},` +
		`"wh":{"enabled":true,"headers":{"H":"v"},"type":"remote","url":"https://x/sse"}}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestOpenCodePlanAddsSchemaAndMCP(t *testing.T) {
	m := &merge.Merged{
		OpenCode: layer.Object{"model": "a/b", "mcp": layer.Object{"mine": layer.Object{"type": "local", "command": []any{"x"}}}},
		MCP:      layer.Object{"mcpServers": map[string]any{"gh": map[string]any{"command": "gh"}}},
	}
	plan, err := OpenCode{}.Plan(m, "/set/opencode")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(plan.Files[1].Content, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["$schema"] == nil || cfg["model"] != "a/b" {
		t.Errorf("cfg %v", cfg)
	}
	mcp := cfg["mcp"].(map[string]any)
	if mcp["mine"] == nil || mcp["gh"] == nil {
		t.Errorf("mcp should hold both the layer's own entry and the translated one: %v", mcp)
	}
}

func TestClaudePlanEmptyMerged(t *testing.T) {
	plan, err := Claude{}.Plan(&merge.Merged{Skills: map[string]layer.Skill{}}, "/set/claude")
	if err != nil {
		t.Fatal(err)
	}
	if string(plan.Files[1].Content) != "{}\n" {
		t.Errorf("settings %q", plan.Files[1].Content)
	}
	if len(plan.Patches) != 1 || plan.Patches[0].Key != "mcpServers" {
		t.Errorf("patches %v", plan.Patches)
	}
}
