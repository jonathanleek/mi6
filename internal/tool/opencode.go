package tool

import (
	"path/filepath"
	"sort"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/set"
)

// OpenCode reads opencode.json, AGENTS.md, and skills/ from
// OPENCODE_CONFIG_DIR. Two things differ from Claude Code, verified on
// 1.18.30: that variable adds a directory and OpenCode keeps reading
// ~/.config/opencode, and OpenCode scans ~/.claude/skills on its own unless
// told not to.
type OpenCode struct{}

func init() { register(OpenCode{}) }

func (OpenCode) Name() string    { return "opencode" }
func (OpenCode) Command() string { return "opencode" }

func (OpenCode) Env(dir string) []string {
	return []string{
		"OPENCODE_CONFIG_DIR=" + dir,
		"OPENCODE_CONFIG=" + filepath.Join(dir, "opencode.json"),
		"OPENCODE_DISABLE_EXTERNAL_SKILLS=1",
	}
}

func (OpenCode) Plan(m *merge.Merged, dir string) (*set.Plan, error) {
	cfg := layer.Object{}
	for k, v := range m.OpenCode {
		cfg[k] = v
	}
	if _, ok := cfg["$schema"]; !ok {
		cfg["$schema"] = "https://opencode.ai/config.json"
	}
	if servers, ok := m.MCP["mcpServers"].(map[string]any); ok && len(servers) > 0 {
		existing, _ := cfg["mcp"].(map[string]any)
		cfg["mcp"] = merge.JSON(existing, translateMCP(servers))
	}
	cfgJSON, err := set.MarshalJSON(cfg)
	if err != nil {
		return nil, err
	}

	skillsDir := filepath.Join(dir, "skills")
	return &set.Plan{
		Files: []set.File{
			{Path: filepath.Join(dir, "AGENTS.md"), Content: []byte(m.Instructions)},
			{Path: filepath.Join(dir, "opencode.json"), Content: cfgJSON},
		},
		Links:    skillLinks(m, skillsDir),
		LinkDirs: []string{skillsDir},
	}, nil
}

// translateMCP turns Claude Code's mcpServers shape into OpenCode's mcp
// shape. A command entry becomes type local with the command and arguments
// as one list; a url entry becomes type remote.
func translateMCP(servers map[string]any) layer.Object {
	out := layer.Object{}
	names := make([]string, 0, len(servers))
	for n := range servers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		src, ok := servers[name].(map[string]any)
		if !ok {
			continue
		}
		dst := layer.Object{"enabled": true}
		switch {
		case src["url"] != nil:
			dst["type"] = "remote"
			dst["url"] = src["url"]
			if h, ok := src["headers"]; ok {
				dst["headers"] = h
			}
		case src["command"] != nil:
			dst["type"] = "local"
			cmd := []any{src["command"]}
			if args, ok := src["args"].([]any); ok {
				cmd = append(cmd, args...)
			}
			dst["command"] = cmd
			if env, ok := src["env"]; ok {
				dst["environment"] = env
			}
		default:
			continue
		}
		out[name] = dst
	}
	return out
}
