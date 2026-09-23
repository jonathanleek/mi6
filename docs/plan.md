# Build plan

Four milestones. Each ends in a commit that is reviewed before the next
starts. [The design](design.md) is the spec. When the two disagree, fix the
design first.

Decided on 2026-09-23:

- Audience: solo consultants. Teams later, maybe.
- Go, macOS and Linux. Homebrew tap once it works.
- A `.mi6/` folder anywhere from the repo up to the filesystem root is a
  layer, and `~/.mi6/` always is. No config repo, no roots, no config file of
  `mi6`'s own.
- Built state under `$XDG_STATE_HOME/mi6`, named by a hash of the stack.
- `git config mi6.parent` for repos outside the tree. `git config mi6.trust`
  before a checkout's own `.mi6/` counts.
- Tools are Go files in `mi6`. Claude Code and OpenCode in v1.
- Instructions are `AGENTS.md` in a layer, generated per launch, named per tool.
- One merge rule: instructions concatenate, skills union, JSON deep-merges
  with lists unioned.
- MCP servers written straight into each tool's file. Plugins out.
- `mi6` never syncs, clones, or pulls. Symlinks into a private repo do that.
- Out of v1: a known config repo, skill sources, accounts, network contexts,
  per-tool environment, model policy, plugins, `doctor`.

## 1. Design and example (done)

Docs only. Done when the design reflects every decision above, the dropped
features sit under Later with a sketch each, and `examples/` shows a tree
someone can copy.

## 2. Resolve (done)

`mi6 resolve` prints the stack and is tested. Done when it is right for a
repo in the tree, a worktree of that repo, a repo with `mi6.parent` set, a
repo outside the tree, a plain directory, and a repo with its own `.mi6/`
both with and without `mi6.trust`.

- Go module, `cmd/mi6`.
- A `resolve` package: find the main checkout, apply `mi6.parent`, walk up,
  collect layers.
- Tests that create temporary trees, git repos, and worktrees.

## 3. Build (done)

`mi6 resolve` also writes a set for both tools, and is tested. Done when a
second run with no config change writes nothing.

- A `layer` package: load a `.mi6/` folder into a struct, following skill
  symlinks one level.
- A `merge` package: instructions, skills, JSON. Table tests for each rule.
- A `tool` package with the interface and the `claude` and `opencode` files.
- A `set` package: compute the target tree, diff it against disk, apply.
- Golden-file tests: fixture layers in, expected set out.

## 4. Launch

`mi6 <tool>` starts the real tool. Done when Claude Code and OpenCode each
start under a set and show the stack's instructions and skills.

- Reserved subcommand names. `--bare`. Argument pass-through. `exec`.
- A fake tool under `testdata/bin/` that prints its environment and exits, so
  the launch path is tested without either real tool.
- A manual checklist in `docs/verify.md` for the two real tools, with the
  items from the design's last section.

After v1, in the bootstrap repo, not here: install `mi6`, clone the private
layer repo, create the `.mi6` symlinks, install the shell aliases.
