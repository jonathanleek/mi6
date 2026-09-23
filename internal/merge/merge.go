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
	// Warnings collects per-layer warnings and cross-layer collisions.
	Warnings []string
}

// Stack merges layers in order, first applied first. Display names the
// layer in the instructions header and in warnings.
func Stack(layers []*layer.Layer, display func(string) string) *Merged {
	m := &Merged{Skills: map[string]layer.Skill{}}
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
		m.Claude = JSON(m.Claude, l.Claude)
		m.OpenCode = JSON(m.OpenCode, l.OpenCode)
		m.Warnings = append(m.Warnings, l.Warnings...)
	}
	m.Instructions = text.String()
	sort.Strings(m.Warnings)
	return m
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
