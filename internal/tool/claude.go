package tool

import (
	"path/filepath"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/models"
	"github.com/jonathanleek/mi6/internal/set"
)

// Claude is Claude Code. CLAUDE_CONFIG_DIR moves everything it keeps under
// ~/.claude: settings, skills, session history, the login, and .claude.json,
// which holds its MCP servers next to state it writes for itself.
type Claude struct{}

func init() { register(Claude{}) }

func (Claude) Name() string    { return "claude" }
func (Claude) Command() string { return "claude" }

func (Claude) Env(dir string) []string {
	return []string{"CLAUDE_CONFIG_DIR=" + dir}
}

// Model enforcement, verified on 2.1.285 with scripts/verify-enforcement.sh.
// availableModels in the set's settings.json holds against --model, the
// default, and the background request of a session; enforceAvailableModels
// makes the default obey it. Project settings outrank it, an env can
// redirect the API, and --settings replaces it, so those are refused.
func (Claude) ID() models.ID       { return models.NamedID("claude") }
func (Claude) ListKeys() []string  { return []string{"availableModels", "enforceAvailableModels"} }
func (Claude) ModelKeys() []string { return []string{"model"} }
func (Claude) Vars() []string {
	return []string{
		"ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_SMALL_FAST_MODEL", "CLAUDE_CODE_SUBAGENT_MODEL",
		"ANTHROPIC_BASE_URL",
	}
}
func (Claude) Args() []string { return []string{"--settings", "--setting-sources"} }
func (Claude) CheckoutFiles() []string {
	return []string{".claude/settings.json", ".claude/settings.local.json"}
}
func (Claude) CheckoutKeys() []string { return nil }

func (c Claude) Plan(m *merge.Merged, dir string) (*set.Plan, error) {
	settings := layer.Object{}
	for k, v := range m.Claude {
		settings[k] = v
	}
	if m.Policy != nil && m.Policy.Active() {
		settings["availableModels"] = m.Policy.Evaluate(c.Name(), c.ID()).IDs()
		settings["enforceAvailableModels"] = true
	}
	settingsJSON, err := set.MarshalJSON(settings)
	if err != nil {
		return nil, err
	}

	servers := layer.Object{}
	if s, ok := m.MCP["mcpServers"].(map[string]any); ok {
		servers = s
	}

	skillsDir := filepath.Join(dir, "skills")
	return &set.Plan{
		Files: []set.File{
			{Path: filepath.Join(dir, "CLAUDE.md"), Content: []byte(m.Instructions)},
			{Path: filepath.Join(dir, "settings.json"), Content: settingsJSON},
		},
		Links:    skillLinks(m, skillsDir),
		LinkDirs: []string{skillsDir},
		Patches: []set.Patch{
			{Path: filepath.Join(dir, ".claude.json"), Key: "mcpServers", Value: servers},
		},
	}, nil
}
