// Package tool knows, for each supported agent, the environment variable that
// moves its config, the files it reads from that directory, where MCP
// servers go, and how to start it. Adding a tool is one file here.
package tool

import (
	"fmt"
	"sort"

	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/set"
)

// Tool is one supported agent.
type Tool interface {
	// Name is the word after mi6 on the command line.
	Name() string
	// Command is the executable to start.
	Command() string
	// Plan says what the tool's directory in a set should hold.
	Plan(m *merge.Merged, dir string) (*set.Plan, error)
	// Env returns KEY=VALUE pairs that point the tool at dir.
	Env(dir string) []string
}

var registry = map[string]Tool{}

func register(t Tool) { registry[t.Name()] = t }

// Lookup returns the tool with that name.
func Lookup(name string) (Tool, error) {
	t, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q; known: %v", name, Names())
	}
	return t, nil
}

// All returns every tool, sorted by name.
func All() []Tool {
	out := make([]Tool, 0, len(registry))
	for _, n := range Names() {
		out = append(out, registry[n])
	}
	return out
}

// Names returns the registered tool names, sorted.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// skillLinks is the part of a plan every tool shares: skills/<name> links
// into the layers.
func skillLinks(m *merge.Merged, skillsDir string) []set.Link {
	var links []set.Link
	for _, name := range m.SkillNames() {
		links = append(links, set.Link{
			Path:   skillsDir + "/" + name,
			Target: m.Skills[name].Dir,
		})
	}
	return links
}
