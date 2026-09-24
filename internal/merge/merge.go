// Package merge combines a stack of layers into one result.
//
// One rule covers every file, from docs/design.md:
//
//   - Instructions concatenate, first layer first, each block under a line
//     that names its layer.
//   - Skills union by name. On a collision the layer nearest the repo wins.
//   - JSON deep-merges. An object merges key by key. A list unions, keeping
//     order and dropping duplicates. Anything else takes the value from the
//     layer nearest the repo.
//   - Environment variables merge by name, nearest layer wins.
//
// One exception: an allowlist of models narrows instead of widening. See
// Narrowing.
package merge

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/jonathanleek/mi6/internal/layer"
)

// Merged is the result of merging a stack.
type Merged struct {
	Instructions string
	// Skills by name, each with the layer it came from.
	Skills   map[string]layer.Skill
	MCP      layer.Object
	Claude   layer.Object
	OpenCode layer.Object
	// Env is the merged env.json, nearest layer winning per name.
	Env map[string]string
	// Warnings collects per-layer warnings and cross-layer collisions.
	Warnings []string

	pending *narrowed
}

// Stack merges layers in order, first applied first. Display names the
// layer in the instructions header and in warnings.
func Stack(layers []*layer.Layer, display func(string) string) *Merged {
	m := &Merged{Skills: map[string]layer.Skill{}, Env: map[string]string{}}
	var text strings.Builder
	for _, l := range layers {
		if l.HasInstructions() {
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			fmt.Fprintf(&text, "<!-- mi6 layer: %s -->\n\n", display(l.Path))
			text.WriteString(l.Instructions)
		}
		for name, s := range l.Skills {
			if prev, ok := m.Skills[name]; ok {
				m.Warnings = append(m.Warnings, fmt.Sprintf("skill %s from %s hides the one from %s", name, display(l.Path), display(prev.Dir)))
			}
			m.Skills[name] = s
		}
		m.MCP = JSON(m.MCP, l.MCP)
		m.Claude = m.narrow(m.Claude, l.Claude, "availableModels", display(l.Path))
		m.Claude = JSON(m.Claude, l.Claude)
		m.Claude = m.apply(m.Claude)
		m.OpenCode = m.narrow(m.OpenCode, l.OpenCode, "enabled_providers", display(l.Path))
		m.OpenCode = JSON(m.OpenCode, l.OpenCode)
		m.OpenCode = m.apply(m.OpenCode)
		for k, v := range l.Env {
			m.Env[k] = v
		}
		m.Warnings = append(m.Warnings, l.Warnings...)
	}
	m.Instructions = text.String()
	sort.Strings(m.Warnings)
	return m
}

// Narrowing is the one per-key exception to the list rule. The keys are
// allowlists of models: availableModels for Claude Code and enabled_providers
// for OpenCode. Under the union rule a nearer layer could only widen them,
// which is backwards for an allowlist. So when both layers set the key, the
// result is the entries of the outer list that the nearer list also names,
// in the outer list's order. Matching is by exact string. If nothing matches,
// the outer list stays and a warning says so, since an empty allowlist would
// block every model and a mismatch such as "sonnet" against
// "claude-sonnet-5" is more likely a typo than an intent. An empty nearer
// list is ignored for the same reason.

// narrow records the narrowed value for key, if both objects set it, so
// that apply can put it back after JSON has unioned the lists.
func (m *Merged) narrow(base, over layer.Object, key, layerName string) layer.Object {
	m.pending = nil
	outer, ok1 := base[key].([]any)
	inner, ok2 := over[key].([]any)
	if !ok1 || !ok2 || len(inner) == 0 {
		return base
	}
	var kept []any
	for _, v := range outer {
		if contains(inner, v) {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		m.Warnings = append(m.Warnings, fmt.Sprintf("%s in %s shares no entry with the layer above; keeping the outer list", key, layerName))
		kept = outer
	}
	m.pending = &narrowed{key: key, value: kept}
	return base
}

// apply puts the narrowed value recorded by narrow into obj.
func (m *Merged) apply(obj layer.Object) layer.Object {
	if m.pending == nil {
		return obj
	}
	obj[m.pending.key] = m.pending.value
	m.pending = nil
	return obj
}

type narrowed struct {
	key   string
	value []any
}

// EnvNames returns the merged variable names, sorted.
func (m *Merged) EnvNames() []string {
	names := make([]string, 0, len(m.Env))
	for n := range m.Env {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// SkillNames returns the merged skill names, sorted.
func (m *Merged) SkillNames() []string {
	names := make([]string, 0, len(m.Skills))
	for n := range m.Skills {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// JSON merges over onto base and returns the result. Neither input is
// modified. A nil over returns base; a nil base returns a copy of over.
func JSON(base, over layer.Object) layer.Object {
	if over == nil {
		return base
	}
	if base == nil {
		return mergeValue(nil, over).(layer.Object)
	}
	return mergeValue(base, over).(layer.Object)
}

func mergeValue(base, over any) any {
	switch o := over.(type) {
	case map[string]any:
		b, _ := base.(map[string]any)
		out := make(map[string]any, len(b)+len(o))
		for k, v := range b {
			out[k] = v
		}
		for k, v := range o {
			out[k] = mergeValue(b[k], v)
		}
		return out
	case []any:
		b, _ := base.([]any)
		out := make([]any, 0, len(b)+len(o))
		out = append(out, b...)
		for _, v := range o {
			if !contains(out, v) {
				out = append(out, v)
			}
		}
		return out
	default:
		return over
	}
}

func contains(list []any, v any) bool {
	for _, x := range list {
		if reflect.DeepEqual(x, v) {
			return true
		}
	}
	return false
}
