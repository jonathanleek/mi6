package tool

import (
	"path/filepath"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
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

func (Claude) Plan(m *merge.Merged, dir string) (*set.Plan, error) {
	settings := m.Claude
	if settings == nil {
		settings = layer.Object{}
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
