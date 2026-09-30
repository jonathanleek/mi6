// Package catalog edits one layer's models.json: defining tags, adding
// models, attaching and removing tags, and importing another catalog. Rules
// are not edited here; a policy change is a hand-edit on purpose.
//
// Every edit is checked against the stack before it is written, so a tag
// that is defined nowhere, or a file that would not load, is refused with
// the reason instead of being saved.
package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/models"
	"github.com/jonathanleek/mi6/internal/resolve"
)

// File is the layer's models.json, loaded for editing.
type File struct {
	// Path is the file's path, and Name how messages show it.
	Path, Name string
	// Obj is its content, an empty object for a file that does not exist.
	Obj layer.Object
	// Notes say what an edit did beyond the ask, such as adding a provider
	// entry for a new model.
	Notes []string
}

// Open loads the models.json of the layer at dir. dir may be the layer
// itself or the folder that holds it.
func Open(dir string) (*File, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if filepath.Base(abs) != resolve.LayerDir {
		abs = filepath.Join(abs, resolve.LayerDir)
	}
	f := &File{Path: filepath.Join(abs, models.File), Obj: layer.Object{}}
	f.Name = f.Path
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	obj, err := layer.ParseObject(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Path, err)
	}
	f.Obj = obj
	return f, nil
}

// Save checks the file against the stack's layers and writes it, with
// sorted keys. The layer with the same path in layers is replaced by this
// file's content for the check; otherwise the file is checked on top of
// them. An error names what is wrong and nothing is written.
func (f *File) Save(layers []*layer.Layer, display func(string) string) error {
	if errs := f.check(layers, display); len(errs) > 0 {
		return fmt.Errorf("not saved:\n  %s", strings.Join(errs, "\n  "))
	}
	b, err := f.Encode()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f.Path, b, 0o644)
}

// Encode renders the file with sorted keys and tab indentation, the way
// mi6 init writes a layer.
func (f *File) Encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "\t")
	if err := enc.Encode(f.Obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (f *File) check(layers []*layer.Layer, display func(string) string) []string {
	dir := filepath.Dir(f.Path)
	stack := make([]*layer.Layer, 0, len(layers)+1)
	replaced := false
	for _, l := range layers {
		if l.Path == dir {
			dup := *l
			dup.Models = f.Obj
			l = &dup
			replaced = true
		}
		stack = append(stack, l)
	}
	if !replaced {
		stack = append(stack, &layer.Layer{Path: dir, Models: f.Obj})
	}
	p := models.Merge(stack, display)
	var errs []string
	for _, e := range p.Errors {
		if strings.Contains(e, display(dir)) {
			errs = append(errs, e)
		}
	}
	return errs
}

func (f *File) section(name string) layer.Object {
	o, ok := f.Obj[name].(map[string]any)
	if !ok {
		o = layer.Object{}
		f.Obj[name] = o
	}
	return o
}

// DefineTag defines a tag, or changes its meaning.
func (f *File) DefineTag(name, meaning string) {
	f.section("tags")[name] = meaning
}

// AddModel adds a model with tags and tool names, as "tool=name". A model
// already in the file is an error. A provider with no entry in the file
// gets an empty one, with a note.
func (f *File) AddModel(key string, tags []string, ids map[string]string) error {
	ms := f.section("models")
	if _, ok := ms[key]; ok {
		return fmt.Errorf("%s is already in %s; use mi6 tag to add tags", key, f.Name)
	}
	prov, rest, found := strings.Cut(key, "/")
	if !found || prov == "" || rest == "" {
		return fmt.Errorf("a model is named provider/model, not %q", key)
	}
	entry := layer.Object{"tags": strList(tags)}
	for tool, id := range ids {
		entry[tool] = id
	}
	ms[key] = entry
	f.EnsureProvider(prov)
	return nil
}

// EnsureProvider adds an empty entry for a provider the file lacks.
func (f *File) EnsureProvider(id string) {
	ps := f.section("providers")
	if _, ok := ps[id]; !ok {
		ps[id] = layer.Object{"tags": []any{}}
		f.Notes = append(f.Notes, fmt.Sprintf("added an entry for provider %s with no tags", id))
	}
}

// Tag attaches tags to a model or a provider in the file. A model not in
// the file gets an entry with just the tags, so a lower layer can tag a
// model a higher one defines.
func (f *File) Tag(key string, tags []string) {
	var section string
	if strings.Contains(key, "/") {
		section = "models"
	} else {
		section = "providers"
	}
	entries := f.section(section)
	entry, ok := entries[key].(map[string]any)
	if !ok {
		entry = layer.Object{"tags": []any{}}
		entries[key] = entry
		if section == "models" {
			f.Notes = append(f.Notes, fmt.Sprintf("%s was not in this file; added an entry with just the tags", key))
		}
	}
	have := strs(entry["tags"])
	for _, t := range tags {
		if !contains(have, t) {
			have = append(have, t)
		}
	}
	entry["tags"] = strList(have)
}

// Untag removes tags from a model or provider in the file. A tag the file
// does not attach is an error: only the layer that attached a tag can
// remove it.
func (f *File) Untag(key string, tags []string) error {
	section := "providers"
	if strings.Contains(key, "/") {
		section = "models"
	}
	entry, ok := f.section(section)[key].(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not in %s", key, f.Name)
	}
	have := strs(entry["tags"])
	var kept []string
	for _, t := range tags {
		if !contains(have, t) {
			return fmt.Errorf("%s does not attach %s to %s; only the layer that attached a tag can remove it", f.Name, t, key)
		}
	}
	for _, t := range have {
		if !contains(tags, t) {
			kept = append(kept, t)
		}
	}
	entry["tags"] = strList(kept)
	return nil
}

// Export is a catalog on its own: the tag definitions, providers, and
// models of a merged policy, with no rules.
func Export(p *models.Policy) layer.Object {
	tags := layer.Object{}
	for _, n := range p.TagNames() {
		tags[n] = p.Tags[n].Meaning
	}
	providers := layer.Object{}
	for id, pr := range p.Providers {
		providers[id] = layer.Object{"tags": strList(pr.Tags)}
	}
	ms := layer.Object{}
	for _, key := range p.ModelKeys() {
		m := p.Models[key]
		entry := layer.Object{"tags": strList(m.Own)}
		for tool, id := range m.IDs {
			entry[tool] = id
		}
		ms[key] = entry
	}
	return layer.Object{"tags": tags, "providers": providers, "models": ms}
}

// Import adds to the file what the stack lacks from another catalog: a tag
// the stack does not define, a provider or model it does not have, a tag
// the stack does not attach, a tool name it does not give. What the stack
// already has is left alone. A tag whose meaning differs or a tool name
// that differs is a conflict, reported and not imported. Rules in the file
// are ignored, with a note. stack is the merged policy the file's layer is
// part of.
func (f *File) Import(other layer.Object, stack *models.Policy) (conflicts []string) {
	if errs := checkShape(other); len(errs) > 0 {
		return errs
	}
	for _, k := range []string{"allow", "deny"} {
		if _, ok := other[k]; ok {
			f.Notes = append(f.Notes, fmt.Sprintf("the file's %s was ignored; rules are not imported", k))
		}
	}
	added := 0
	for n, v := range object(other["tags"]) {
		meaning := v.(string)
		if have, ok := stack.Tags[n]; ok {
			if have.Meaning != meaning {
				conflicts = append(conflicts, fmt.Sprintf("tag %s means %q in %s and %q in the file; kept ours", n, have.Meaning, have.From, meaning))
			}
			continue
		}
		f.section("tags")[n] = meaning
		added++
	}
	for id, v := range object(other["providers"]) {
		if missing := lacking(stack.Providers[id].Tags, strs(object(v)["tags"])); len(missing) > 0 || !known(stack.Providers, id) {
			f.Tag(id, missing)
			added++
		}
	}
	for key, v := range object(other["models"]) {
		entry := object(v)
		have, ok := stack.Models[key]
		if !ok {
			ids := map[string]string{}
			for tool, x := range entry {
				if tool != "tags" {
					ids[tool] = x.(string)
				}
			}
			if err := f.AddModel(key, strs(entry["tags"]), ids); err == nil {
				added++
			}
			continue
		}
		if missing := lacking(have.Own, strs(entry["tags"])); len(missing) > 0 {
			f.Tag(key, missing)
			added++
		}
		for tool, x := range entry {
			if tool == "tags" {
				continue
			}
			id := x.(string)
			if ours, ok := have.IDs[tool]; ok {
				if ours != id {
					conflicts = append(conflicts, fmt.Sprintf("%s is %q for %s in the stack and %q in the file; kept ours", key, ours, tool, id))
				}
				continue
			}
			f.Tag(key, nil)
			f.section("models")[key].(map[string]any)[tool] = id
			added++
		}
	}
	f.Notes = append(f.Notes, fmt.Sprintf("%d additions", added))
	sort.Strings(conflicts)
	return conflicts
}

// lacking returns the entries of want that have lacks.
func lacking(have, want []string) []string {
	var out []string
	for _, w := range want {
		if !contains(have, w) {
			out = append(out, w)
		}
	}
	return out
}

func known(providers map[string]models.Provider, id string) bool {
	_, ok := providers[id]
	return ok
}

// checkShape is the shape check the models package applies, for a file
// being imported.
func checkShape(o layer.Object) []string {
	probe := models.Merge([]*layer.Layer{{Path: "the file", Models: o}}, func(p string) string { return p })
	var errs []string
	for _, e := range probe.Errors {
		if !strings.Contains(e, "is not defined") && !strings.Contains(e, "has no providers entry") {
			errs = append(errs, e)
		}
	}
	return errs
}

func object(v any) map[string]any {
	o, _ := v.(map[string]any)
	return o
}

func strs(v any) []string {
	l, _ := v.([]any)
	var out []string
	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func strList(s []string) []any {
	out := make([]any, 0, len(s))
	for _, x := range s {
		out = append(out, x)
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
