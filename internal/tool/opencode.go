package tool

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/models"
	"github.com/jonathanleek/mi6/internal/set"
)

// OpenCode reads opencode.json, AGENTS.md, and skills/ from
// OPENCODE_CONFIG_DIR. Two things differ from Claude Code, verified on
// 1.18.30: that variable adds a directory and OpenCode keeps reading
// ~/.config/opencode, and OpenCode scans ~/.claude/skills on its own unless
// told not to. Unreachable MCP servers in the generated config do not delay
// its start.
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

// Model enforcement, verified on 1.18.30 with scripts/verify-enforcement.sh.
// enabled_providers and provider.<id>.whitelist in the set hold against -m,
// the /model picker, agents, and the default, and win over the global and
// the checkout's config because the set is read last of the files. The
// checkout can still set what the set does not, such as a provider's
// address, and OPENCODE_CONFIG_CONTENT is read after the set, so those are
// refused.
func (OpenCode) ID() models.ID { return models.KeyID }
func (OpenCode) ListKeys() []string {
	return []string{"enabled_providers", "disabled_providers", "provider.*.whitelist", "provider.*.blacklist"}
}
func (OpenCode) ModelKeys() []string {
	return []string{"model", "small_model", "agent.*.model", "mode.*.model"}
}
func (OpenCode) Vars() []string { return []string{"OPENCODE_CONFIG_CONTENT"} }
func (OpenCode) Args() []string { return nil }
func (OpenCode) CheckoutFiles() []string {
	return []string{"opencode.json", "opencode.jsonc", ".opencode/opencode.json", ".opencode/opencode.jsonc"}
}
func (OpenCode) CheckoutKeys() []string { return []string{"provider"} }

func (o OpenCode) Plan(m *merge.Merged, dir string) (*set.Plan, error) {
	cfg := layer.Object{}
	for k, v := range m.OpenCode {
		cfg[k] = v
	}
	if _, ok := cfg["$schema"]; !ok {
		cfg["$schema"] = "https://opencode.ai/config.json"
	}
	if m.Policy != nil && m.Policy.Active() {
		enforce(cfg, m.Policy.Evaluate(o.Name(), o.ID()))
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

// enforce writes the allowed list into cfg: enabled_providers is the
// providers with an allowed model, and each one's whitelist is its allowed
// models. Providers are copied before they are changed, so the merged
// settings stay as they were.
func enforce(cfg layer.Object, r *models.Result) {
	providers := layer.Object{}
	for k, v := range object(cfg["provider"]) {
		providers[k] = v
	}
	var enabled []string
	byProvider := map[string][]string{}
	for _, e := range r.Allowed {
		prov, model, _ := strings.Cut(e.Key, "/")
		if _, ok := byProvider[prov]; !ok {
			enabled = append(enabled, prov)
		}
		byProvider[prov] = append(byProvider[prov], model)
	}
	sort.Strings(enabled)
	for _, prov := range enabled {
		entry := layer.Object{}
		for k, v := range object(providers[prov]) {
			entry[k] = v
		}
		entry["whitelist"] = byProvider[prov]
		providers[prov] = entry
	}
	if enabled == nil {
		enabled = []string{}
	}
	cfg["enabled_providers"] = enabled
	cfg["provider"] = providers
}

func object(v any) map[string]any {
	o, _ := v.(map[string]any)
	return o
}
