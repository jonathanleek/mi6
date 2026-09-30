// Package tool knows, for each supported agent, the environment variable that
// moves its config, the files it reads from that directory, where MCP
// servers go, how to start it, and how it enforces a model list. Adding a
// tool is one file here.
package tool

import (
	"fmt"
	"sort"

	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/models"
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

// Enforcer is a tool that can enforce a model list. Under a policy a tool
// that is not one is refused. Each method states a fact about the tool;
// the enforce package applies them. A key is a path into the tool's
// settings, with "." between levels and "*" for any name at a level.
type Enforcer interface {
	Tool
	// ID says how the tool names a catalog model.
	ID() models.ID
	// ListKeys are the settings keys that hold a model list or switch its
	// enforcement. The policy owns them: Plan writes them, and a layer or
	// the checkout that sets one is refused.
	ListKeys() []string
	// ModelKeys are the settings keys that name one model. Each must be
	// allowed.
	ModelKeys() []string
	// Vars are the variables that redirect the tool's model or replace its
	// config. A layer that sets one is refused, and they are removed from
	// the environment at launch.
	Vars() []string
	// Args are the passed-through arguments that replace or reselect the
	// tool's settings. They are refused.
	Args() []string
	// CheckoutFiles are the checkout's own settings files, which outrank
	// the set's. They are looked for in every directory from the working
	// directory up to the checkout root. One that sets a ListKey, a
	// ModelKey, env.<Var>, or a CheckoutKey is refused.
	CheckoutFiles() []string
	// CheckoutKeys are further keys refused in a checkout file, for what
	// the set cannot pin, such as a provider's address.
	CheckoutKeys() []string
	// OutsideFiles are settings files outside the set that the tool still
	// reads under it. The set's lists win over them, but doctor warns when
	// one touches models. home is the user's home directory.
	OutsideFiles(home string) []string
	// ManagedPaths are where an administrator's config would be, which is
	// read after the set and is out of mi6's reach. doctor reports one
	// that exists.
	ManagedPaths() []string
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
