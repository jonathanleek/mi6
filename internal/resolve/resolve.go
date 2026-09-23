// Package resolve finds the stack of .mi6 layers that apply to a directory.
//
// The rules are in docs/design.md under "How mi6 finds the stack":
//
//  1. Find the main checkout with git rev-parse --git-common-dir, so a
//     worktree resolves like its main checkout. Outside a repo, use the
//     directory itself.
//  2. If git config mi6.parent is set, walk from that folder instead of the
//     checkout's parent.
//  3. ~/.mi6 is always the first layer. Then every .mi6 from the filesystem
//     root down to the walk's start, skipping ~/.mi6 if passed again.
//  4. The checkout's own .mi6, only if git config mi6.trust is true.
package resolve

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LayerDir is the folder name that marks a layer.
const LayerDir = ".mi6"

// Origin says where in the rules a layer came from.
type Origin string

const (
	// OriginHome is ~/.mi6.
	OriginHome Origin = "home"
	// OriginTree is a .mi6 between the filesystem root and the walk's start.
	OriginTree Origin = "tree"
	// OriginCheckout is the .mi6 inside the repo itself.
	OriginCheckout Origin = "checkout"
)

// Layer is one .mi6 directory in the stack.
type Layer struct {
	Path   string
	Origin Origin
}

// Skipped is a .mi6 directory that exists but was not used, with the reason.
type Skipped struct {
	Path   string
	Reason string
}

// Stack is the result of resolving a directory.
type Stack struct {
	// Dir is the directory the stack was resolved for.
	Dir string
	// Checkout is the main checkout's working directory. Empty outside a repo.
	Checkout string
	// Worktree is true when Dir is a linked worktree of Checkout.
	Worktree bool
	// Start is the folder the walk started from: the checkout's parent, the
	// folder named by mi6.parent, or Dir itself outside a repo.
	Start string
	// Layers in stacking order, first applied first.
	Layers []Layer
	// Skipped lists .mi6 directories that were found but not used.
	Skipped []Skipped
	// Notes are one-line facts worth showing, such as a missing checkout.
	Notes []string
}

// Options controls Resolve. Zero values mean the current directory and the
// current user's home.
type Options struct {
	Dir  string
	Home string
}

// Resolve returns the stack for opts.Dir.
func Resolve(opts Options) (*Stack, error) {
	dir, err := realDir(opts.Dir)
	if err != nil {
		return nil, err
	}
	home := opts.Home
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	home, err = realDir(home)
	if err != nil {
		return nil, fmt.Errorf("home directory: %w", err)
	}

	st := &Stack{Dir: dir}

	homeLayer := filepath.Join(home, LayerDir)
	if isDir(homeLayer) {
		st.Layers = append(st.Layers, Layer{Path: homeLayer, Origin: OriginHome})
	}

	commonDir, err := gitOutput(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	switch {
	case err != nil:
		if gitdir, ok := staleWorktree(dir); ok {
			// A linked worktree whose main checkout is gone. Git refuses to
			// treat it as a repository at all, so find out from the .git file.
			st.Notes = append(st.Notes, fmt.Sprintf("main checkout missing: %s. Using %s alone.", gitdir, displayPath(homeLayer, home)))
			return st, nil
		}
		// Not a repo. The directory itself is the bottom of the walk, and
		// there is no clone to distrust.
		st.Start = dir
	case !isDir(commonDir):
		st.Notes = append(st.Notes, fmt.Sprintf("main checkout missing: %s. Using %s alone.", commonDir, displayPath(homeLayer, home)))
		return st, nil
	case filepath.Base(commonDir) != ".git":
		st.Notes = append(st.Notes, fmt.Sprintf("bare repository at %s. Using %s alone.", commonDir, displayPath(homeLayer, home)))
		return st, nil
	default:
		st.Checkout = filepath.Dir(commonDir)
		if top, err := gitOutput(dir, "rev-parse", "--show-toplevel"); err == nil {
			if real, err := realDir(top); err == nil && real != st.Checkout {
				st.Worktree = true
			}
		}
		st.Start = filepath.Dir(st.Checkout)
		if parent, err := gitOutput(dir, "config", "--get", "mi6.parent"); err == nil && parent != "" {
			parent = expandHome(parent, home)
			real, err := realDir(parent)
			if err != nil {
				st.Notes = append(st.Notes, fmt.Sprintf("mi6.parent is %s, which does not exist. Walking from the checkout's parent.", parent))
			} else {
				st.Start = real
			}
		}
	}

	for _, a := range ancestors(st.Start) {
		p := filepath.Join(a, LayerDir)
		if p == homeLayer || !isDir(p) {
			continue
		}
		st.Layers = append(st.Layers, Layer{Path: p, Origin: OriginTree})
	}

	if st.Checkout != "" {
		p := filepath.Join(st.Checkout, LayerDir)
		if isDir(p) {
			trusted, _ := gitOutput(dir, "config", "--bool", "--get", "mi6.trust")
			if trusted == "true" {
				st.Layers = append(st.Layers, Layer{Path: p, Origin: OriginCheckout})
			} else {
				st.Skipped = append(st.Skipped, Skipped{
					Path:   p,
					Reason: "inside the checkout and not trusted. To use it: git config mi6.trust true",
				})
			}
		}
	}

	return st, nil
}

// staleWorktree reports whether dir is inside a linked worktree whose .git
// file points at a directory that no longer exists. Git itself reports such
// a directory as "not a git repository".
func staleWorktree(dir string) (string, bool) {
	for _, a := range ancestors(dir) {
		p := filepath.Join(a, ".git")
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		line := strings.TrimSpace(string(b))
		if !strings.HasPrefix(line, "gitdir:") {
			continue
		}
		gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(a, gitdir)
		}
		if !isDir(gitdir) {
			return gitdir, true
		}
	}
	return "", false
}

// ancestors returns dir and every parent of it, root first.
func ancestors(dir string) []string {
	var out []string
	for {
		out = append([]string{dir}, out...)
		parent := filepath.Dir(dir)
		if parent == dir {
			return out
		}
		dir = parent
	}
}

// realDir makes dir absolute and resolves symlinks, so paths from git and
// paths from the caller compare equal. Empty means the current directory.
func realDir(dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(real)
	if err != nil {
		return "", err
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	return real, nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// displayPath shortens a path under home to ~/... for messages.
func displayPath(p, home string) string {
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

// DisplayPath is displayPath for callers that print a Stack.
func DisplayPath(p, home string) string { return displayPath(p, home) }

var errGit = errors.New("git")

// gitOutput runs git in dir and returns trimmed stdout. A non-zero exit is an
// error, which for the commands used here means "not a repo" or "unset".
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w %s: %v", errGit, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
