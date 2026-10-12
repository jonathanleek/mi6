package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

// Model enforcement, verified on 2.0.25 with scripts/verify-enforcement.sh.
// OpenCode blocks at two levels: a provider outside enabled_providers is
// unavailable, and so is a model absent from the models map of a provider
// the config defines. whitelist and blacklist only hide models from the
// picker; -m reaches them. So a defined provider keeps only its allowed
// models, a built-in provider is enabled only when the policy removes none
// of its models, since nothing short of disabling it blocks one, and -m is
// checked here as well. The set's lists win over the global and the
// checkout's config because the set is read last of the files; the
// checkout can still set what the set does not, such as a provider's
// address, and OPENCODE_CONFIG_CONTENT is read after the set, so those are
// refused. OpenCode 2.x also shares one background server across every
// launch, spawned with the first launch's environment; service.json in the
// set turns that off so each launch reads its own config.
func (OpenCode) ID() models.ID { return models.KeyID }
func (o OpenCode) Result(m *merge.Merged) *models.Result {
	return restrict(m.Policy.Evaluate(o.Name(), o.ID()), m.OpenCode)
}
func (OpenCode) ListKeys() []string {
	return []string{"enabled_providers", "disabled_providers", "provider.*.whitelist", "provider.*.blacklist"}
}
func (OpenCode) ModelKeys() []string {
	return []string{"model", "small_model", "agent.*.model", "mode.*.model"}
}
func (OpenCode) Vars() []string      { return []string{"OPENCODE_CONFIG_CONTENT"} }
func (OpenCode) Args() []string      { return nil }
func (OpenCode) ModelArgs() []string { return []string{"--model", "-m"} }
func (OpenCode) CheckoutFiles() []string {
	return []string{"opencode.json", "opencode.jsonc", ".opencode/opencode.json", ".opencode/opencode.jsonc"}
}
func (OpenCode) CheckoutKeys() []string { return []string{"provider"} }

// OpenCode keeps reading its global config under OPENCODE_CONFIG_DIR.
func (OpenCode) OutsideFiles(home string) []string {
	dir := filepath.Join(home, ".config")
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		dir = x
	}
	return []string{filepath.Join(dir, "opencode", "opencode.json"), filepath.Join(dir, "opencode", "opencode.jsonc")}
}

// Paths from OpenCode's config/managed.ts at 1.18.30. The console account's
// organization config is read after the set too, but it lives in
// OpenCode's database, so it is not checked.
func (OpenCode) ManagedPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Library/Application Support/opencode",
			"/Library/Managed Preferences/ai.opencode.managed.plist",
			filepath.Join("/Library/Managed Preferences", os.Getenv("USER"), "ai.opencode.managed.plist"),
		}
	case "windows":
		return []string{filepath.Join(os.Getenv("ProgramData"), "opencode")}
	default:
		return []string{"/etc/opencode"}
	}
}

func (o OpenCode) Plan(m *merge.Merged, dir string) (*set.Plan, error) {
	cfg := layer.Object{}
	for k, v := range m.OpenCode {
		cfg[k] = v
	}
	if _, ok := cfg["$schema"]; !ok {
		cfg["$schema"] = "https://opencode.ai/config.json"
	}
	if m.Policy != nil && m.Policy.Active() {
		enforce(cfg, o.Result(m))
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
			{Path: filepath.Join(dir, "service.json"), Content: []byte("{\n  \"disabled\": true\n}\n")},
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

// defined reports whether the config defines the provider, with an npm
// package or a models map. Such a provider serves only the models its map
// names, verified on 2.0.25, so the map is where the policy is enforced. A
// built-in provider's list comes from OpenCode and cannot be narrowed.
func defined(cfg layer.Object, provider string) bool {
	entry := object(object(cfg["provider"])[provider])
	_, npm := entry["npm"]
	_, models := entry["models"].(map[string]any)
	return npm || models
}

// restrict drops what OpenCode cannot keep apart: on a built-in provider
// the policy can only disable the whole provider, so when it removes one
// of that provider's models, the rest go too, with the reason.
func restrict(r *models.Result, cfg layer.Object) *models.Result {
	removed := map[string]string{}
	for _, x := range r.Removed {
		prov, _, _ := strings.Cut(x.Key, "/")
		if _, seen := removed[prov]; !seen && !defined(cfg, prov) {
			removed[prov] = x.Key
		}
	}
	out := &models.Result{Tool: r.Tool, Active: r.Active, Removed: r.Removed}
	for _, e := range r.Allowed {
		prov, _, _ := strings.Cut(e.Key, "/")
		if culprit, ok := removed[prov]; ok {
			out.Removed = append(out.Removed, models.Removal{Key: e.Key, Tags: e.Tags,
				Why: fmt.Sprintf("provider %s is built in and the policy removes %s; OpenCode can only drop the whole provider", prov, culprit)})
			continue
		}
		out.Allowed = append(out.Allowed, e)
	}
	sort.Slice(out.Removed, func(i, j int) bool { return out.Removed[i].Key < out.Removed[j].Key })
	return out
}

// enforce writes the allowed list into cfg: enabled_providers is the
// providers with an allowed model, a defined provider's models map keeps
// only its allowed models, and each provider's whitelist hides the rest
// from the picker. Providers are copied before they are changed, so the
// merged settings stay as they were.
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
		if all, ok := entry["models"].(map[string]any); ok || defined(cfg, prov) {
			kept := layer.Object{}
			for _, model := range byProvider[prov] {
				if v, ok := all[model]; ok {
					kept[model] = v
				} else {
					kept[model] = layer.Object{}
				}
			}
			entry["models"] = kept
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
