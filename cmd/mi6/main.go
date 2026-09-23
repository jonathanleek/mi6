// Command mi6 launches an AI coding agent with the config stack for the
// current directory. See docs/design.md.
package main

import (
	"fmt"
	"os"

	"github.com/jonathanleek/mi6/internal/resolve"
)

// version is set by the release build with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage:
  mi6 <tool> [args...]     start a tool under the config stack for this directory
  mi6 --bare <tool> [args] start a tool with its plain user config
  mi6 resolve [dir]        print the stack for a directory
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
	case "--bare":
		if len(args) < 2 {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		return notYet("--bare " + args[1])
	default:
		return notYet(args[0])
	}
}

func notYet(what string) int {
	fmt.Fprintf(os.Stderr, "mi6 %s: launching a tool is not implemented yet\n", what)
	return 2
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
	return 0
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
