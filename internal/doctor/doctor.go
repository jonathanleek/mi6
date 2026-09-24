// Package doctor checks that a launch from the current directory would work,
// and says what is wrong when it would not.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonathanleek/mi6/internal/build"
	"github.com/jonathanleek/mi6/internal/launch"
	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/resolve"
	"github.com/jonathanleek/mi6/internal/tool"
)

// Status is the outcome of one check.
type Status string

const (
	OK   Status = "ok"
	Warn Status = "warn"
	Fail Status = "fail"
)

// Check is one line of the report.
type Check struct {
	Status Status
	Name   string
	Detail string
}

// Report is every check, and whether any failed.
type Report struct {
	Checks []Check
}

// Failed reports whether any check failed. Warnings do not count.
func (r *Report) Failed() bool {
	for _, c := range r.Checks {
		if c.Status == Fail {
			return true
		}
	}
	return false
}

// Options controls Run. Zero values mean the current directory and the
// current user's home.
type Options struct {
	Dir  string
	Home string
}

// Run performs every check and returns the report.
func Run(opts Options) (*Report, error) {
	home := opts.Home
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return nil, err
		}
	}
	show := func(p string) string { return resolve.DisplayPath(p, home) }
	r := &Report{}
	add := func(s Status, name, detail string) { r.Checks = append(r.Checks, Check{s, name, detail}) }

	// git, which resolve needs.
	if path, ver, err := version("git"); err != nil {
		add(Fail, "git", "not on your PATH; resolving a repo needs it")
	} else {
		add(OK, "git", show(path)+" "+ver)
	}

	// Each tool.
	for _, t := range tool.All() {
		path, ver, err := version(t.Command())
		if err != nil {
			add(Fail, t.Name(), t.Command()+" is not on your PATH")
			continue
		}
		add(OK, t.Name(), show(path)+" "+ver)
	}

	// State directory.
	stateDir := build.StateDir(home)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		add(Fail, "state", fmt.Sprintf("cannot create %s: %v", show(stateDir), err))
	} else if probe, err := os.CreateTemp(stateDir, ".doctor-*"); err != nil {
		add(Fail, "state", fmt.Sprintf("%s is not writable: %v", show(stateDir), err))
	} else {
		probe.Close()
		os.Remove(probe.Name())
		add(OK, "state", show(stateDir)+" is writable")
	}

	// The stack for this directory, and whether every layer parses.
	st, err := resolve.Resolve(resolve.Options{Dir: opts.Dir, Home: home})
	if err != nil {
		add(Fail, "layers", err.Error())
		return r, nil
	}
	var layers []*layer.Layer
	bad := 0
	for _, l := range st.Layers {
		loaded, err := layer.Load(l.Path)
		if err != nil {
			bad++
			add(Fail, "layer", err.Error())
			continue
		}
		layers = append(layers, loaded)
	}
	switch {
	case bad > 0:
		// Reported above, one line per broken layer.
	case len(st.Layers) == 0:
		add(Warn, "layers", fmt.Sprintf("none apply here; a tool would start with its plain config. Create %s to begin", show(filepath.Join(home, resolve.LayerDir))))
	default:
		add(OK, "layers", fmt.Sprintf("%d apply here, all parse", len(st.Layers)))
	}
	for _, s := range st.Skipped {
		add(Warn, "layer", fmt.Sprintf("%s skipped: %s", show(s.Path), s.Reason))
	}
	for _, n := range st.Notes {
		add(Warn, "layers", n)
	}

	// Collisions and other merge warnings.
	m := merge.Stack(layers, show)
	if len(m.Warnings) == 0 {
		add(OK, "skills", fmt.Sprintf("%d, no collisions", len(m.Skills)))
	}
	for _, w := range m.Warnings {
		add(Warn, "skills", w)
	}

	// Already inside a session that mi6 started.
	if t := os.Getenv(launch.ToolVar); t != "" {
		add(Warn, "session", fmt.Sprintf("%s=%s is set: this shell is inside a tool that mi6 started, and mi6 %s here would start it as is", launch.ToolVar, t, t))
	} else {
		add(OK, "session", "not inside a tool that mi6 started")
	}

	return r, nil
}

// version finds cmd on the PATH and returns its path and the first line of
// its --version output. A tool that takes longer than five seconds to say
// its version is reported as found, with no version.
func version(cmd string) (path, ver string, err error) {
	path, err = exec.LookPath(cmd)
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, runErr := exec.CommandContext(ctx, path, "--version").Output()
	if runErr != nil {
		return path, "(version unknown)", nil
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return path, line, nil
}
