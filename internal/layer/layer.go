// Package layer loads one .mi6 directory into a struct.
//
// A layer holds any of these, all optional:
//
//	AGENTS.md                 instructions
//	skills/<name>/SKILL.md    a skill; an entry may be a symlink to a folder
//	                          of skills, and we look one level deeper
//	mcp.json                  MCP servers, in Claude Code's mcpServers shape
//	claude.json               Claude Code settings
//	opencode.json             OpenCode settings
//	env.json                  variables to export to the tool, a flat object
//	                          of names to strings
package layer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Object is a decoded JSON object. Numbers are json.Number so they round-trip.
type Object = map[string]any

// Skill is one skill found under a layer's skills directory.
type Skill struct {
	Name string
	// Dir is the skill's directory with symlinks resolved, the target a set
	// links to.
	Dir string
}

// Layer is a loaded .mi6 directory.
type Layer struct {
	Path         string
	Instructions string
	// Skills by name. Within one layer the first entry in sorted order wins a
	// name collision; Warnings says so.
	Skills   map[string]Skill
	MCP      Object
	Claude   Object
	OpenCode Object
	// Env is env.json, or nil.
	Env      map[string]string
	Warnings []string
}

// HasInstructions reports whether the layer has an AGENTS.md.
func (l *Layer) HasInstructions() bool { return l.Instructions != "" }

// Load reads the layer at path. A missing file is not an error. A file that
// exists but cannot be parsed is.
func Load(path string) (*Layer, error) {
	l := &Layer{Path: path, Skills: map[string]Skill{}}

	b, err := os.ReadFile(filepath.Join(path, "AGENTS.md"))
	switch {
	case err == nil:
		l.Instructions = string(bytes.TrimRight(b, "\n")) + "\n"
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}

	for name, dst := range map[string]*Object{
		"mcp.json":      &l.MCP,
		"claude.json":   &l.Claude,
		"opencode.json": &l.OpenCode,
	} {
		obj, err := loadObject(filepath.Join(path, name))
		if err != nil {
			return nil, err
		}
		*dst = obj
	}

	if err := l.loadSkills(filepath.Join(path, "skills")); err != nil {
		return nil, err
	}

	envObj, err := loadObject(filepath.Join(path, "env.json"))
	if err != nil {
		return nil, err
	}
	if envObj != nil {
		l.Env = map[string]string{}
		for k, v := range envObj {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("%s: %s is not a string", filepath.Join(path, "env.json"), k)
			}
			l.Env[k] = s
		}
	}
	return l, nil
}

func loadObject(p string) (Object, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	obj, err := ParseObject(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return obj, nil
}

// ParseObject decodes a JSON object, keeping numbers as json.Number.
func ParseObject(b []byte) (Object, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("top level is not a JSON object")
	}
	return obj, nil
}

// loadSkills fills l.Skills from dir. Each entry is a skill if it holds a
// SKILL.md. Otherwise, if it is a directory, each of its subdirectories with
// a SKILL.md is a skill. Symlinks are followed at both levels.
func (l *Layer) loadSkills(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		p := filepath.Join(dir, name)
		if !isDir(p) {
			continue
		}
		if isFile(filepath.Join(p, "SKILL.md")) {
			l.addSkill(name, p)
			continue
		}
		subs, err := os.ReadDir(p)
		if err != nil {
			return err
		}
		subNames := make([]string, 0, len(subs))
		for _, s := range subs {
			subNames = append(subNames, s.Name())
		}
		sort.Strings(subNames)
		for _, sub := range subNames {
			sp := filepath.Join(p, sub)
			if isDir(sp) && isFile(filepath.Join(sp, "SKILL.md")) {
				l.addSkill(sub, sp)
			}
		}
	}
	return nil
}

func (l *Layer) addSkill(name, dir string) {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		real = dir
	}
	if prev, ok := l.Skills[name]; ok {
		l.Warnings = append(l.Warnings, fmt.Sprintf("skill %s at %s is hidden by %s in the same layer", name, real, prev.Dir))
		return
	}
	l.Skills[name] = Skill{Name: name, Dir: real}
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
