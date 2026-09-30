// Package launch starts a tool under the config stack for a directory.
package launch

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/jonathanleek/mi6/internal/audit"
	"github.com/jonathanleek/mi6/internal/build"
	"github.com/jonathanleek/mi6/internal/enforce"
	"github.com/jonathanleek/mi6/internal/models"
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
	// Version is mi6's version, for the audit log.
	Version string
}

// Run starts the named tool with args. On success it does not return,
// because the process is replaced. It returns an error when the tool is
// unknown, not on the path, the stack cannot be built, the model policy
// refuses the launch, or the audit log cannot be written. Every launch
// but a nested one is logged, bare and refused ones included.
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

	if os.Getenv(ToolVar) == name && !opts.Bare {
		// Already inside a session of this tool that mi6 started. The
		// environment is set; build nothing and start the tool as is.
		return execFn(path, argv, os.Environ())
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logPath := audit.Path(build.StateDir(home))
	cwd := opts.Dir
	if cwd == "" {
		if cwd, err = os.Getwd(); err != nil {
			return err
		}
	}
	show := func(p string) string { return resolve.DisplayPath(p, home) }

	if opts.Bare {
		entry := audit.New(opts.Version, name, cwd)
		entry.Outcome = audit.Bare
		if err := audit.Append(logPath, entry); err != nil {
			return err
		}
		return execFn(path, argv, os.Environ())
	}

	st, err := resolve.Resolve(resolve.Options{Dir: opts.Dir, Home: home})
	if err != nil {
		return err
	}
	for _, n := range st.Notes {
		fmt.Fprintln(stderr, "mi6:", n)
	}
	for _, s := range st.Skipped {
		fmt.Fprintf(stderr, "mi6: skipped %s: %s\n", show(s.Path), s.Reason)
	}
	entry := audit.New(opts.Version, name, st.Dir)
	for _, l := range st.Layers {
		entry.Layers = append(entry.Layers, show(l.Path))
	}
	if len(st.Layers) == 0 {
		fmt.Fprintf(stderr, "mi6: no %s folder applies here. Starting %s with its plain config.\n", resolve.LayerDir, name)
		entry.Outcome = audit.Started
		if err := audit.Append(logPath, entry); err != nil {
			return err
		}
		return execFn(path, argv, os.Environ())
	}

	r, err := build.Build(st, build.Options{Home: home, Tools: []tool.Tool{t}})
	if err != nil {
		return err
	}
	for _, w := range r.Merged.Warnings {
		fmt.Fprintln(stderr, "mi6: warning:", w)
	}
	refusals := enforce.Check(enforce.Input{
		Tool: t, Layers: r.Layers, Merged: r.Merged, Dir: st.Dir, Checkout: st.Checkout, Args: args, Display: show,
	})
	entry.Set = show(r.Dir)
	var result *models.Result
	if e, ok := t.(tool.Enforcer); ok {
		result = r.Merged.Policy.Evaluate(name, e.ID())
	}
	entry.WithPolicy(r.Merged.Policy, result).WithDefault(enforce.Default(r.Merged.Settings(name)))
	if len(refusals) > 0 {
		entry.Outcome, entry.Reasons = audit.Refused, refusals
		if err := audit.Append(logPath, entry); err != nil {
			return err
		}
		return &Refused{Tool: name, Reasons: refusals}
	}
	entry.Outcome = audit.Started
	if err := audit.Append(logPath, entry); err != nil {
		return err
	}

	// The layers' variables first, then the tool's own, so a layer cannot
	// redirect the tool away from its set. Under a policy the variables
	// that would redirect the tool's model go too, wherever they came from.
	dir := r.ToolDir(t)
	env := os.Environ()
	if r.Merged.Policy.Active() {
		env = enforce.Env(env, t)
	}
	env = withVars(env, r.Env)
	env = withVars(env, append(t.Env(dir), ToolVar+"="+name, SetVar+"="+r.Dir))
	return execFn(path, argv, env)
}

// Refused is the error when the model policy does not let the tool start.
type Refused struct {
	Tool    string
	Reasons []string
}

func (r *Refused) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "not starting %s:", r.Tool)
	for _, reason := range r.Reasons {
		b.WriteString("\n  " + strings.ReplaceAll(reason, "\n", "\n  "))
	}
	return b.String()
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
