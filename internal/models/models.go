// Package models is the model policy: the merged models.json of a stack,
// and which models each tool may use under it.
//
// A models.json holds tags, providers and models that carry them, and
// allow and deny rules that name them. From docs/design.md:
//
//   - tags, providers, models, and deny merge by the ordinary rule, so a
//     tag list unions and nothing below can remove a tag or lift a deny.
//   - allow is kept per layer, and a model must pass every layer's allow.
//   - The policy applies when any layer has an allow or a deny. A model
//     outside the catalog is never allowed.
//   - A tag used but never defined, or a model whose provider has no
//     entry, is an error, since a deny on a misspelled tag denies nothing.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
)

// File is the name of the policy file in a layer.
const File = "models.json"

// Tag is a defined tag.
type Tag struct {
	Name    string
	Meaning string
	// From is the display name of the first layer that defines it.
	From string
}

// Provider is a provider entry.
type Provider struct {
	ID   string
	Tags []string
}

// Model is a catalog entry. Its key is provider/model.
type Model struct {
	Key      string
	Provider string
	// Own are the tags attached to the model itself. Tags adds the
	// provider's.
	Own  []string
	Tags []string
	// IDs are the tool names in the entry, each with the name that tool
	// uses for the model. OpenCode's name is the key, so it has no entry.
	IDs map[string]string
}

// Rule is one tag in a deny list, with the layer that listed it.
type Rule struct {
	Tag  string
	From string
}

// Allow is one layer's allow list.
type Allow struct {
	Tags []string
	From string
}

// Policy is the merged models.json of a stack.
type Policy struct {
	Tags      map[string]Tag
	Providers map[string]Provider
	Models    map[string]Model
	Deny      []Rule
	Allows    []Allow
	// Errors are the refusals that apply to every tool: a tag used but
	// never defined, a model whose provider has no entry, a file with the
	// wrong shape. Each names the layer.
	Errors []string
}

// Active reports whether any layer has a rule.
func (p *Policy) Active() bool { return len(p.Deny) > 0 || len(p.Allows) > 0 }

// ModelKeys returns the catalog keys, sorted.
func (p *Policy) ModelKeys() []string {
	keys := make([]string, 0, len(p.Models))
	for k := range p.Models {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TagNames returns the defined tags, sorted.
func (p *Policy) TagNames() []string {
	names := make([]string, 0, len(p.Tags))
	for n := range p.Tags {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Hash identifies the policy, so an audit line can say which rules were in
// force without copying the catalog into it.
func (p *Policy) Hash() string {
	type entry struct {
		Tags, Own []string
		IDs       map[string]string
	}
	canon := struct {
		Tags      map[string]string
		Providers map[string][]string
		Models    map[string]entry
		Deny      []string
		Allows    [][]string
	}{map[string]string{}, map[string][]string{}, map[string]entry{}, nil, nil}
	for n, t := range p.Tags {
		canon.Tags[n] = t.Meaning
	}
	for id, pr := range p.Providers {
		canon.Providers[id] = pr.Tags
	}
	for k, m := range p.Models {
		canon.Models[k] = entry{m.Tags, m.Own, m.IDs}
	}
	for _, r := range p.Deny {
		canon.Deny = append(canon.Deny, r.Tag)
	}
	for _, a := range p.Allows {
		canon.Allows = append(canon.Allows, a.Tags)
	}
	b, _ := json.Marshal(canon)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Merge combines the layers' models.json files, first layer first. Display
// names a layer in errors and rules.
func Merge(layers []*layer.Layer, display func(string) string) *Policy {
	p := &Policy{Tags: map[string]Tag{}, Providers: map[string]Provider{}, Models: map[string]Model{}}
	var combined layer.Object
	for _, l := range layers {
		if l.Models == nil {
			continue
		}
		name := display(l.Path)
		if errs := checkShape(l.Models); len(errs) > 0 {
			for _, e := range errs {
				p.Errors = append(p.Errors, fmt.Sprintf("%s in %s: %s", File, name, e))
			}
			continue
		}
		// Record where each definition and rule first appears.
		for n, v := range object(l.Models["tags"]) {
			if _, ok := p.Tags[n]; !ok {
				p.Tags[n] = Tag{Name: n, Meaning: v.(string), From: name}
			}
		}
		for _, v := range list(l.Models["deny"]) {
			tag := v.(string)
			if !hasRule(p.Deny, tag) {
				p.Deny = append(p.Deny, Rule{Tag: tag, From: name})
			}
		}
		if allow := list(l.Models["allow"]); len(allow) > 0 {
			a := Allow{From: name}
			for _, v := range allow {
				a.Tags = append(a.Tags, v.(string))
			}
			p.Allows = append(p.Allows, a)
		}
		// Everything but allow follows the ordinary merge rule.
		without := layer.Object{}
		for k, v := range l.Models {
			if k != "allow" {
				without[k] = v
			}
		}
		combined = merge.JSON(combined, without)
	}
	if combined == nil {
		return p
	}
	for n, v := range object(combined["tags"]) {
		if t, ok := p.Tags[n]; ok {
			t.Meaning = v.(string)
			p.Tags[n] = t
		}
	}
	for id, v := range object(combined["providers"]) {
		p.Providers[id] = Provider{ID: id, Tags: strs(object(v)["tags"])}
	}
	for key, v := range object(combined["models"]) {
		entry := object(v)
		m := Model{Key: key, IDs: map[string]string{}}
		if i := strings.Index(key, "/"); i > 0 {
			m.Provider = key[:i]
		}
		for k, x := range entry {
			if k == "tags" {
				m.Own = strs(x)
				continue
			}
			m.IDs[k] = x.(string)
		}
		m.Tags = append([]string{}, m.Own...)
		if pr, ok := p.Providers[m.Provider]; ok {
			m.Tags = union(m.Tags, pr.Tags)
		}
		p.Models[key] = m
	}
	p.check(layers, display)
	return p
}

// check adds the cross-layer errors: undefined tags, and models whose
// provider has no entry.
func (p *Policy) check(layers []*layer.Layer, display func(string) string) {
	var errs []string
	for _, l := range layers {
		if l.Models == nil || len(checkShape(l.Models)) > 0 {
			continue
		}
		name := display(l.Path)
		use := func(tag, where string) {
			if _, ok := p.Tags[tag]; !ok {
				errs = append(errs, fmt.Sprintf("%s in %s: tag %q in %s is not defined in any layer", File, name, tag, where))
			}
		}
		for id, v := range object(l.Models["providers"]) {
			for _, t := range strs(object(v)["tags"]) {
				use(t, "provider "+id)
			}
		}
		for key, v := range object(l.Models["models"]) {
			for _, t := range strs(object(v)["tags"]) {
				use(t, "model "+key)
			}
			prov, _, _ := strings.Cut(key, "/")
			if _, ok := p.Providers[prov]; !ok {
				errs = append(errs, fmt.Sprintf("%s in %s: model %q names provider %q, which has no providers entry in any layer", File, name, key, prov))
			}
		}
		for _, v := range list(l.Models["allow"]) {
			use(v.(string), "allow")
		}
		for _, v := range list(l.Models["deny"]) {
			use(v.(string), "deny")
		}
	}
	sort.Strings(errs)
	p.Errors = append(p.Errors, errs...)
}

// checkShape reports what is wrong with one file's shape. Every key is
// optional, but a key that is present must have the right shape, and an
// unknown top-level key is a mistake, since a misspelled "deny" would deny
// nothing.
func checkShape(o layer.Object) []string {
	var errs []string
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	for k, v := range o {
		switch k {
		case "tags":
			for n, m := range object(v) {
				if _, ok := m.(string); !ok {
					bad("tag %q needs a meaning, a string", n)
				}
			}
			if _, ok := v.(map[string]any); !ok {
				bad("tags must be an object of tag names to meanings")
			}
		case "providers":
			if _, ok := v.(map[string]any); !ok {
				bad("providers must be an object")
			}
			for id, pv := range object(v) {
				entry, ok := pv.(map[string]any)
				if !ok {
					bad("provider %q must be an object", id)
					continue
				}
				for pk, x := range entry {
					if pk != "tags" {
						bad("provider %q has an unknown key %q", id, pk)
					} else if !isStrList(x) {
						bad("provider %q: tags must be a list of strings", id)
					}
				}
			}
		case "models":
			if _, ok := v.(map[string]any); !ok {
				bad("models must be an object")
			}
			for key, mv := range object(v) {
				entry, ok := mv.(map[string]any)
				if !ok {
					bad("model %q must be an object", key)
					continue
				}
				if prov, rest, found := strings.Cut(key, "/"); !found || prov == "" || rest == "" {
					bad("model %q must be named provider/model", key)
				}
				for mk, x := range entry {
					if mk == "tags" {
						if !isStrList(x) {
							bad("model %q: tags must be a list of strings", key)
						}
					} else if _, ok := x.(string); !ok {
						bad("model %q: %q must be the name the %s tool uses, a string", key, mk, mk)
					}
				}
			}
		case "allow", "deny":
			if !isStrList(v) {
				bad("%s must be a list of tags", k)
			}
		default:
			bad("unknown key %q; the keys are tags, providers, models, allow, and deny", k)
		}
	}
	sort.Strings(errs)
	return errs
}

// Entry is a model in a tool's allowed list.
type Entry struct {
	Key string
	// ID is the name the tool uses for it.
	ID   string
	Tags []string
}

// Removal is a catalog model a tool may not use, with the reason.
type Removal struct {
	Key  string
	Tags []string
	Why  string
}

// Result is what one tool may use under the policy.
type Result struct {
	Tool    string
	Active  bool
	Allowed []Entry
	Removed []Removal
}

// ID says how a tool names a catalog model, or that it cannot use it.
type ID func(m Model) (string, bool)

// KeyID names a model by its catalog key, as OpenCode does.
func KeyID(m Model) (string, bool) { return m.Key, true }

// NamedID names a model by the tool's own key in the entry, as Claude Code
// does with "claude": "sonnet". A model without one is not offered.
func NamedID(tool string) ID {
	return func(m Model) (string, bool) {
		id, ok := m.IDs[tool]
		return id, ok
	}
}

// Evaluate computes the allowed list for one tool. Catalog order, by key.
func (p *Policy) Evaluate(tool string, id ID) *Result {
	r := &Result{Tool: tool, Active: p.Active()}
	for _, key := range p.ModelKeys() {
		m := p.Models[key]
		name, usable := id(m)
		why := ""
		switch {
		case !usable:
			why = fmt.Sprintf("no %s name", tool)
		case !r.Active:
		default:
			why = p.reason(m)
		}
		if why == "" {
			r.Allowed = append(r.Allowed, Entry{Key: key, ID: name, Tags: m.Tags})
		} else {
			r.Removed = append(r.Removed, Removal{Key: key, Tags: m.Tags, Why: why})
		}
	}
	return r
}

// reason says why the rules remove m, or "" if they allow it.
func (p *Policy) reason(m Model) string {
	for _, d := range p.Deny {
		if contains(m.Tags, d.Tag) {
			return fmt.Sprintf("deny %s (%s)", d.Tag, d.From)
		}
	}
	for _, a := range p.Allows {
		if !anyOf(m.Tags, a.Tags) {
			return fmt.Sprintf("not in allow (%s)", a.From)
		}
	}
	return ""
}

// IDs returns the allowed models' tool names, in catalog order.
func (r *Result) IDs() []string {
	out := make([]string, 0, len(r.Allowed))
	for _, e := range r.Allowed {
		out = append(out, e.ID)
	}
	return out
}

// Allows reports whether the tool may use the model with that tool name.
func (r *Result) Allows(id string) bool {
	for _, e := range r.Allowed {
		if e.ID == id {
			return true
		}
	}
	return false
}

func object(v any) map[string]any {
	o, _ := v.(map[string]any)
	return o
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func strs(v any) []string {
	var out []string
	for _, x := range list(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func isStrList(v any) bool {
	l, ok := v.([]any)
	if !ok {
		return false
	}
	for _, x := range l {
		if _, ok := x.(string); !ok {
			return false
		}
	}
	return true
}

func hasRule(rules []Rule, tag string) bool {
	for _, r := range rules {
		if r.Tag == tag {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func anyOf(have, want []string) bool {
	for _, w := range want {
		if contains(have, w) {
			return true
		}
	}
	return false
}

func union(a, b []string) []string {
	out := append([]string{}, a...)
	for _, s := range b {
		if !contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
