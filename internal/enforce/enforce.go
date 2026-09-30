// Package enforce decides whether a tool may start under the model policy,
// and with what environment. It applies the facts each tool states through
// tool.Enforcer to the layers, the merged settings, the checkout's own
// settings, and the arguments. Every refusal names what is at fault.
package enforce

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/models"
	"github.com/jonathanleek/mi6/internal/tool"
)

// Input is what Check looks at.
type Input struct {
	Tool   tool.Tool
	Layers []*layer.Layer
	Merged *merge.Merged
	// Dir is the working directory and Checkout the checkout's root, or
	// empty outside a repository. The checkout's settings are looked for in
	// every directory from Dir up to Checkout.
	Dir, Checkout string
	// Args are the arguments passed through to the tool.
	Args []string
	// Display names a path in messages.
	Display func(string) string
}

// Check returns the refusals for starting the tool. None means it may
// start. Outside a policy only a broken models.json refuses.
func Check(in Input) []string {
	p := in.Merged.Policy
	if p == nil {
		return nil
	}
	refusals := append([]string(nil), p.Errors...)
	if !p.Active() {
		return refusals
	}
	name := in.Tool.Name()
	e, ok := in.Tool.(tool.Enforcer)
	if !ok {
		return append(refusals, fmt.Sprintf("%s has no way to enforce a model list, so it cannot start under a policy", name))
	}
	file := name + ".json"
	r := p.Evaluate(name, e.ID())

	for _, l := range in.Layers {
		where := in.Display(l.Path)
		settings := l.Settings(name)
		for _, key := range e.ListKeys() {
			for _, m := range find(settings, key) {
				refusals = append(refusals, fmt.Sprintf("%s in %s sets %s; model lists belong to models.json", file, where, m.path))
			}
		}
		for _, v := range e.Vars() {
			if _, set := l.Env[v]; set {
				refusals = append(refusals, fmt.Sprintf("env.json in %s sets %s, which redirects %s's model", where, v, name))
			}
			if len(find(settings, "env."+v)) > 0 {
				refusals = append(refusals, fmt.Sprintf("%s in %s sets env.%s, which redirects %s's model", file, where, v, name))
			}
		}
	}

	for _, key := range e.ModelKeys() {
		for _, m := range find(in.Merged.Settings(name), key) {
			id, _ := m.value.(string)
			if r.Allows(id) {
				continue
			}
			from := ""
			for _, l := range in.Layers {
				if len(find(l.Settings(name), m.path)) > 0 {
					from = in.Display(l.Path)
				}
			}
			refusals = append(refusals, fmt.Sprintf("%s in %s names %s %q, which is not allowed here", file, from, m.path, id))
		}
	}

	if len(r.Allowed) == 0 {
		refusals = append(refusals, "no model is allowed here\n"+Explain(p, r))
	}

	for _, arg := range in.Args {
		for _, a := range e.Args() {
			if arg == a || strings.HasPrefix(arg, a+"=") {
				refusals = append(refusals, fmt.Sprintf("the argument %s replaces %s's settings, so under a policy it is refused", a, name))
			}
		}
	}

	return append(refusals, checkout(in, e, name)...)
}

// checkout refuses a settings file in the checkout that touches models.
// Both tools read such files, and they outrank the set's.
func checkout(in Input, e tool.Enforcer, name string) []string {
	var refusals []string
	keys := append(append(append([]string{}, e.ListKeys()...), e.ModelKeys()...), e.CheckoutKeys()...)
	for _, v := range e.Vars() {
		keys = append(keys, "env."+v)
	}
	for _, dir := range dirs(in.Dir, in.Checkout) {
		for _, rel := range e.CheckoutFiles() {
			path := filepath.Join(dir, rel)
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			obj, err := layer.ParseObject(b)
			if err != nil {
				refusals = append(refusals, fmt.Sprintf("%s cannot be read, and it outranks the set's settings: %v", in.Display(path), err))
				continue
			}
			for _, key := range keys {
				for _, m := range find(obj, key) {
					refusals = append(refusals, fmt.Sprintf("%s sets %s; the checkout's settings outrank the set's, so under a policy that is refused", in.Display(path), m.path))
				}
			}
		}
	}
	return refusals
}

// dirs lists dir and its ancestors up to checkout. Outside a checkout, or
// when dir is not below it, just dir and checkout.
func dirs(dir, checkout string) []string {
	if checkout == "" {
		return []string{dir}
	}
	out := []string{dir}
	for d := dir; d != checkout; {
		parent := filepath.Dir(d)
		if parent == d {
			return []string{dir, checkout}
		}
		d = parent
		out = append(out, d)
	}
	return out
}

// Env returns env without the tool's redirecting variables.
func Env(env []string, t tool.Tool) []string {
	e, ok := t.(tool.Enforcer)
	if !ok {
		return env
	}
	var out []string
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, v := range e.Vars() {
			if key == v {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

// Explain renders the rules in force and each catalog model with its
// standing, for a refusal or for resolve.
func Explain(p *models.Policy, r *models.Result) string {
	var b strings.Builder
	for _, d := range p.Deny {
		fmt.Fprintf(&b, "  deny   %-10s from %s\n", d.Tag, d.From)
	}
	for _, a := range p.Allows {
		fmt.Fprintf(&b, "  allow  %-10s from %s\n", strings.Join(a.Tags, ","), a.From)
	}
	fmt.Fprintf(&b, "  %s models in the catalog:\n", r.Tool)
	if len(r.Allowed)+len(r.Removed) == 0 {
		b.WriteString("    none\n")
	}
	for _, e := range r.Allowed {
		fmt.Fprintf(&b, "    %-32s tags: %-28s allowed as %s\n", e.Key, strings.Join(e.Tags, ", "), e.ID)
	}
	for _, x := range r.Removed {
		fmt.Fprintf(&b, "    %-32s tags: %-28s removed: %s\n", x.Key, strings.Join(x.Tags, ", "), x.Why)
	}
	return strings.TrimRight(b.String(), "\n")
}

// match is one settings value found by find.
type match struct {
	path  string
	value any
}

// find returns every value in obj at key, where key is a path with "."
// between levels and "*" for any name at a level. Paths are in sorted
// order.
func find(obj layer.Object, key string) []match {
	if obj == nil {
		return nil
	}
	return walk(obj, strings.Split(key, "."), "")
}

func walk(v any, parts []string, path string) []match {
	if len(parts) == 0 {
		return []match{{path, v}}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	var names []string
	if parts[0] == "*" {
		for n := range m {
			names = append(names, n)
		}
		sort.Strings(names)
	} else if _, ok := m[parts[0]]; ok {
		names = []string{parts[0]}
	}
	var out []match
	for _, n := range names {
		p := n
		if path != "" {
			p = path + "." + n
		}
		out = append(out, walk(m[n], parts[1:], p)...)
	}
	return out
}
