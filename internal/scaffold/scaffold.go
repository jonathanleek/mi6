// Package scaffold creates a .mi6 layer with every file mi6 reads, each
// minimal and valid, so a new layer starts from something that works.
package scaffold

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jonathanleek/mi6/internal/resolve"
)

// Result says what Create did.
type Result struct {
	// Dir is the layer directory.
	Dir string
	// Created and Kept are paths relative to Dir. A file is kept when it
	// already existed; Create never overwrites.
	Created, Kept []string
	// Hint is a line to show the user, or empty.
	Hint string
}

// Create makes dir/.mi6 and fills it. home is used to name the folder in
// the generated text.
func Create(dir, home string) (*Result, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	layer := filepath.Join(abs, resolve.LayerDir)
	r := &Result{Dir: layer}

	if err := os.MkdirAll(filepath.Join(layer, "skills"), 0o755); err != nil {
		return nil, err
	}

	name := resolve.DisplayPath(abs, home)
	files := []struct{ path, content string }{
		{"README.md", readme(name)},
		{"AGENTS.md", fmt.Sprintf("# Rules for %s\n\n", name)},
		{"mcp.json", "{\n\t\"mcpServers\": {}\n}\n"},
		{"claude.json", "{\n\t\"permissions\": {\n\t\t\"allow\": [],\n\t\t\"deny\": []\n\t}\n}\n"},
		{"opencode.json", "{\n\t\"$schema\": \"https://opencode.ai/config.json\"\n}\n"},
		{"env.json", "{}\n"},
		{filepath.Join("skills", "README.md"), skillsReadme},
	}
	for _, f := range files {
		p := filepath.Join(layer, f.path)
		if _, err := os.Stat(p); err == nil {
			r.Kept = append(r.Kept, f.path)
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := os.WriteFile(p, []byte(f.content), 0o644); err != nil {
			return nil, err
		}
		r.Created = append(r.Created, f.path)
	}

	if top, err := gitTopLevel(abs); err == nil && top == abs {
		r.Hint = "This folder is a git checkout. Its layer counts only after: git config mi6.trust true"
	}
	return r, nil
}

func gitTopLevel(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	top := strings.TrimSpace(string(out))
	real, err := filepath.EvalSymlinks(top)
	if err != nil {
		return top, nil
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err == nil && real == realDir {
		return dir, nil
	}
	return real, nil
}

func readme(name string) string {
	return fmt.Sprintf(`# Layer for %s

Every repo below this folder gets what is in here, on top of the layers
above it. Every file is optional. mi6 ignores this README.

| File | What it is |
|---|---|
| AGENTS.md | Instructions for the agent. Layers concatenate, top of the tree first. |
| skills/ | One folder per skill, each with a SKILL.md. See skills/README.md. |
| mcp.json | MCP servers, in Claude Code's mcpServers shape. |
| claude.json | Claude Code settings, the same shape as settings.json. Lists union with the layers above. |
| opencode.json | OpenCode settings. |
| env.json | Variables to export to the tool, names to strings. |

Run mi6 resolve in any repo below to see the stack it gets.
`, name)
}

const skillsReadme = `A skill is a folder here with a SKILL.md in it:

	skills/my-skill/SKILL.md

An entry may also be a symlink to a folder of skills in a checkout you
already have, for example:

	ln -s ~/Documents/git/personal/claude-skills/skills claude-skills

mi6 looks one level inside such a link. This README is ignored.
`
