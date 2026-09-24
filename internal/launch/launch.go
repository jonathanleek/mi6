// Package launch starts a tool under the config stack for a directory.
package launch

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/jonathanleek/mi6/internal/build"
	"github.com/jonathanleek/mi6/internal/resolve"
	"github.com/jonathanleek/mi6/internal/tool"
)

// ToolVar is exported to the tool with the tool's name. A nested mi6 for the
// same tool, such as an alias inside a shell the tool spawned, sees it and
// starts the tool plainly instead of building again.
const ToolVar = "MI6_TOOL"

// SetVar is exported to the tool with the set's directory, for hooks and
// status lines that want to show which stack is active.
const SetVar = "MI6_SET"

// Options controls Run.
type Options struct {
	// Bare starts the tool with its plain user config and skips the stack.
	Bare bool
	// Dir is the directory to resolve from. Empty means the current one.
	Dir string
	// Stderr receives notes and warnings. Nil means os.Stderr.
	Stderr io.Writer
	// Exec replaces the process. Nil means syscall.Exec. Tests set it.
	Exec func(path string, argv []string, env []string) error
}

// Run starts the named tool with args. On success it does not return,
// because the process is replaced. It returns an error when the tool is
// unknown, not on the path, or the stack cannot be built.
func Run(name string, args []string, opts Options) error {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	execFn := opts.Exec
	if execFn == nil {
		execFn = syscall.Exec
	}

	t, err := tool.Lookup(name)
	if err != nil {
		return err
	}
	path, err := exec.LookPath(t.Command())
	if err != nil {
		return fmt.Errorf("%s is not on your PATH", t.Command())
	}
	// argv[0] is the command as typed, the way a shell passes it, not the
	// resolved path. A program may look at its own name.
	argv := append([]string{t.Command()}, args...)

	if opts.Bare {
		return execFn(path, argv, os.Environ())
	}
	if os.Getenv(ToolVar) == name {
		// Already inside a session of this tool that mi6 started. The
		// environment is set; build nothing and start the tool as is.
		return execFn(path, argv, os.Environ())
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	st, err := resolve.Resolve(resolve.Options{Dir: opts.Dir, Home: home})
	if err != nil {
		return err
	}
	for _, n := range st.Notes {
		fmt.Fprintln(stderr, "mi6:", n)
	}
	for _, s := range st.Skipped {
		fmt.Fprintf(stderr, "mi6: skipped %s: %s\n", resolve.DisplayPath(s.Path, home), s.Reason)
	}
	if len(st.Layers) == 0 {
		fmt.Fprintf(stderr, "mi6: no %s folder applies here. Starting %s with its plain config.\n", resolve.LayerDir, name)
		return execFn(path, argv, os.Environ())
	}

	r, err := build.Build(st, build.Options{Home: home, Tools: []tool.Tool{t}})
	if err != nil {
		return err
	}
	for _, w := range r.Merged.Warnings {
		fmt.Fprintln(stderr, "mi6: warning:", w)
	}

	dir := r.ToolDir(t)
	env := withVars(os.Environ(), append(t.Env(dir), ToolVar+"="+name, SetVar+"="+r.Dir))
	return execFn(path, argv, env)
}

// withVars returns env with each KEY=VALUE in vars set, replacing any
// existing entry for the same key.
func withVars(env, vars []string) []string {
	out := make([]string, 0, len(env)+len(vars))
	for _, e := range env {
		key := e[:strings.IndexByte(e+"=", '=')]
		replaced := false
		for _, v := range vars {
			if strings.HasPrefix(v, key+"=") {
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, e)
		}
	}
	return append(out, vars...)
}
