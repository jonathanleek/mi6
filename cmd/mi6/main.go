// Command mi6 launches an AI coding agent with the config stack for the
// current directory. See docs/design.md.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jonathanleek/mi6/internal/build"
	"github.com/jonathanleek/mi6/internal/doctor"
	"github.com/jonathanleek/mi6/internal/launch"
	"github.com/jonathanleek/mi6/internal/resolve"
)

// version is set by the release build with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage:
  mi6 <tool> [args...]     start a tool under the config stack for this directory
  mi6 --bare <tool> [args] start a tool with its plain user config
  mi6 resolve [dir]        print the stack for a directory and build its set
  mi6 doctor [dir]         check that a launch from a directory would work
  mi6 help
  mi6 version
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	case "version", "--version":
		fmt.Println("mi6", version)
		return 0
	case "resolve":
		return runResolve(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "--bare":
		if len(args) < 2 {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		return runLaunch(args[1], args[2:], true)
	default:
		if strings.HasPrefix(args[0], "-") {
			fmt.Fprintf(os.Stderr, "mi6: unknown option %s\n%s", args[0], usage)
			return 2
		}
		return runLaunch(args[0], args[1:], false)
	}
}

// runLaunch returns only on failure: a successful launch replaces the
// process.
func runLaunch(name string, args []string, bare bool) int {
	err := launch.Run(name, args, launch.Options{Bare: bare})
	fmt.Fprintln(os.Stderr, "mi6:", err)
	return 2
}

func runDoctor(args []string) int {
	if len(args) > 1 {
		fmt.Fprint(os.Stderr, "usage: mi6 doctor [dir]\n")
		return 2
	}
	dir := ""
	if len(args) == 1 {
		dir = args[0]
	}
	r, err := doctor.Run(doctor.Options{Dir: dir})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mi6:", err)
		return 1
	}
	for _, c := range r.Checks {
		fmt.Printf("%-5s %-8s %s\n", c.Status, c.Name, c.Detail)
	}
	if r.Failed() {
		return 1
	}
	return 0
}

func runResolve(args []string) int {
	if len(args) > 1 {
		fmt.Fprint(os.Stderr, "usage: mi6 resolve [dir]\n")
		return 2
	}
	dir := ""
	if len(args) == 1 {
		dir = args[0]
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "mi6:", err)
		return 1
	}
	st, err := resolve.Resolve(resolve.Options{Dir: dir, Home: home})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mi6:", err)
		return 1
	}
	printStack(st, home)

	r, err := build.Build(st, build.Options{Home: home})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mi6:", err)
		return 1
	}
	printBuild(r, home)
	return 0
}

func printBuild(r *build.Result, home string) {
	fmt.Println()
	fmt.Printf("set        %s\n", resolve.DisplayPath(r.Dir, home))
	for _, name := range append([]string{"set"}, toolNames(r)...) {
		changes := r.Changes[name]
		if name == "set" && len(changes) == 0 {
			continue
		}
		label := name
		if name == "set" {
			label = "layers"
		}
		switch len(changes) {
		case 0:
			fmt.Printf("  %-9s up to date\n", label)
		case 1:
			rel, _ := filepath.Rel(r.Dir, changes[0].Path)
			fmt.Printf("  %-9s %s %s\n", label, changes[0].What, rel)
		default:
			fmt.Printf("  %-9s %d changes\n", label, len(changes))
		}
	}
	if n := len(r.Merged.Skills); n > 0 {
		fmt.Printf("  skills    %d\n", n)
	}
	if len(r.Env) > 0 {
		fmt.Println()
		fmt.Println("env")
		for _, kv := range r.Env {
			fmt.Printf("  %s\n", kv)
		}
	}
	for _, w := range r.Merged.Warnings {
		fmt.Printf("warning  %s\n", w)
	}
}

func toolNames(r *build.Result) []string {
	var names []string
	for n := range r.Changes {
		if n != "set" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func printStack(st *resolve.Stack, home string) {
	show := func(p string) string { return resolve.DisplayPath(p, home) }

	fmt.Printf("directory  %s\n", show(st.Dir))
	switch {
	case st.Checkout == "":
		fmt.Println("checkout   none, not a git repository")
	case st.Worktree:
		fmt.Printf("checkout   %s  (worktree of it)\n", show(st.Checkout))
	default:
		fmt.Printf("checkout   %s\n", show(st.Checkout))
	}
	if st.Start != "" && st.Start != st.Dir {
		fmt.Printf("walk from  %s\n", show(st.Start))
	}

	fmt.Println()
	if len(st.Layers) == 0 {
		fmt.Println("layers     none")
	} else {
		fmt.Println("layers")
		for i, l := range st.Layers {
			fmt.Printf("  %d  %-50s %s\n", i+1, show(l.Path), l.Origin)
		}
	}
	if len(st.Skipped) > 0 {
		fmt.Println()
		fmt.Println("skipped")
		for _, s := range st.Skipped {
			fmt.Printf("  %s\n     %s\n", show(s.Path), s.Reason)
		}
	}
	if len(st.Notes) > 0 {
		fmt.Println()
		for _, n := range st.Notes {
			fmt.Printf("note  %s\n", n)
		}
	}
}
