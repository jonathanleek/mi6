// Package build turns a resolved stack into a set on disk, one directory per
// tool under the state directory.
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/resolve"
	"github.com/jonathanleek/mi6/internal/set"
	"github.com/jonathanleek/mi6/internal/tool"
)

// Result says what Build did.
type Result struct {
	// Dir is the set's directory. Each tool has a subdirectory named after it.
	Dir     string
	Merged  *merge.Merged
	Changes map[string][]set.Change
}

// ToolDir is the directory for one tool inside the set.
func (r *Result) ToolDir(t tool.Tool) string { return filepath.Join(r.Dir, t.Name()) }

// Options controls Build.
type Options struct {
	// StateDir is where sets live. Empty means StateDir().
	StateDir string
	// Home is used to shorten paths in generated text.
	Home string
	// Tools to build. Empty means all.
	Tools []tool.Tool
}

// StateDir is $XDG_STATE_HOME/mi6, or ~/.local/state/mi6.
func StateDir(home string) string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "mi6")
	}
	return filepath.Join(home, ".local", "state", "mi6")
}

// SetDir names the set for a stack: a hash of its layer paths.
func SetDir(stateDir string, st *resolve.Stack) string {
	h := sha256.New()
	for _, l := range st.Layers {
		h.Write([]byte(l.Path))
		h.Write([]byte{0})
	}
	return filepath.Join(stateDir, "sets", hex.EncodeToString(h.Sum(nil))[:12])
}

// Build loads the stack's layers, merges them, and writes or refreshes the
// set for each tool.
func Build(st *resolve.Stack, opts Options) (*Result, error) {
	home := opts.Home
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return nil, err
		}
	}
	stateDir := opts.StateDir
	if stateDir == "" {
		stateDir = StateDir(home)
	}
	tools := opts.Tools
	if len(tools) == 0 {
		tools = tool.All()
	}

	layers := make([]*layer.Layer, 0, len(st.Layers))
	for _, l := range st.Layers {
		loaded, err := layer.Load(l.Path)
		if err != nil {
			return nil, err
		}
		layers = append(layers, loaded)
	}
	display := func(p string) string { return resolve.DisplayPath(p, home) }
	m := merge.Stack(layers, display)

	r := &Result{Dir: SetDir(stateDir, st), Merged: m, Changes: map[string][]set.Change{}}

	// A file that lists the layers, so the hashed directory name can be read.
	var listing strings.Builder
	for _, l := range st.Layers {
		fmt.Fprintf(&listing, "%s\n", l.Path)
	}
	changes, err := set.Apply(&set.Plan{Files: []set.File{{Path: filepath.Join(r.Dir, "layers"), Content: []byte(listing.String())}}})
	if err != nil {
		return nil, err
	}
	if len(changes) > 0 {
		r.Changes["set"] = changes
	}

	for _, t := range tools {
		plan, err := t.Plan(m, r.ToolDir(t))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.Name(), err)
		}
		changes, err := set.Apply(plan)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.Name(), err)
		}
		r.Changes[t.Name()] = changes
	}
	return r, nil
}
