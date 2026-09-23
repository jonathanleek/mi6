// Package set writes a built config directory and refreshes it in place.
//
// A tool describes what its directory should hold as a Plan: generated
// files, symlinks, and JSON files where one key is owned by mi6 and the rest
// belongs to the tool. Apply compares the plan with the disk and touches
// nothing that already matches.
package set

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/jonathanleek/mi6/internal/layer"
)

// File is a generated file.
type File struct {
	Path    string
	Content []byte
}

// Link is a symlink.
type Link struct {
	Path   string
	Target string
}

// Patch is a JSON file the tool owns, in which mi6 owns one top-level key.
// Apply keeps every other key as it finds it.
type Patch struct {
	Path  string
	Key   string
	Value any
}

// Plan is everything a tool's directory should hold.
type Plan struct {
	Files   []File
	Links   []Link
	Patches []Patch
	// LinkDirs are directories where every symlink is managed by mi6.
	// Symlinks there that the plan does not name are removed. Anything that
	// is not a symlink is left alone.
	LinkDirs []string
}

// Change is one thing Apply did.
type Change struct {
	Path string
	What string // "wrote", "linked", "patched", "removed"
}

// Apply makes the disk match plan and returns what it changed.
func Apply(plan *Plan) ([]Change, error) {
	var changes []Change

	for _, f := range plan.Files {
		cur, err := os.ReadFile(f.Path)
		if err == nil && bytes.Equal(cur, f.Content) {
			continue
		}
		if err := writeFile(f.Path, f.Content); err != nil {
			return changes, err
		}
		changes = append(changes, Change{f.Path, "wrote"})
	}

	wanted := map[string]bool{}
	for _, l := range plan.Links {
		wanted[l.Path] = true
		if cur, err := os.Readlink(l.Path); err == nil && cur == l.Target {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
			return changes, err
		}
		if fi, err := os.Lstat(l.Path); err == nil {
			if fi.Mode()&os.ModeSymlink == 0 {
				return changes, fmt.Errorf("%s exists and is not a symlink; move it aside", l.Path)
			}
			if err := os.Remove(l.Path); err != nil {
				return changes, err
			}
		}
		if err := os.Symlink(l.Target, l.Path); err != nil {
			return changes, err
		}
		changes = append(changes, Change{l.Path, "linked"})
	}

	for _, dir := range plan.LinkDirs {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return changes, err
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.Type()&os.ModeSymlink == 0 || wanted[p] {
				continue
			}
			if err := os.Remove(p); err != nil {
				return changes, err
			}
			changes = append(changes, Change{p, "removed"})
		}
	}

	for _, p := range plan.Patches {
		changed, err := applyPatch(p)
		if err != nil {
			return changes, err
		}
		if changed {
			changes = append(changes, Change{p.Path, "patched"})
		}
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func applyPatch(p Patch) (bool, error) {
	obj := layer.Object{}
	b, err := os.ReadFile(p.Path)
	switch {
	case err == nil:
		obj, err = layer.ParseObject(b)
		if err != nil {
			return false, fmt.Errorf("%s: %w", p.Path, err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return false, err
	}
	if cur, ok := obj[p.Key]; ok && reflect.DeepEqual(normalize(cur), normalize(p.Value)) {
		return false, nil
	}
	obj[p.Key] = p.Value
	out, err := MarshalJSON(obj)
	if err != nil {
		return false, err
	}
	return true, writeFile(p.Path, out)
}

// normalize round-trips a value through JSON so that json.Number and
// float64, or []any and a typed slice, compare equal when they encode the
// same text.
func normalize(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}

// MarshalJSON encodes v indented, without escaping HTML, ending in a newline.
func MarshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeFile writes atomically: to a temp file in the same directory, then a
// rename over the target.
func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}
