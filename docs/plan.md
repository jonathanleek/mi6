# Build plan

Six milestones. Each ends in a commit that is reviewed before the next
starts. [The design](domains.md) is the spec. When the two disagree, fix the
design first.

Decided on 2026-09-23, before milestone 1:

- Audience: solo consultants. Teams later, maybe.
- Go, macOS and Linux. Homebrew tap once it works.
- Built state under `$XDG_STATE_HOME/mi6`.
- Config repo at `MI6_CONFIG`, default `~/.config/mi6`. Roots in `mi6.json`.
- Layers mirror the folder tree. `git config mi6.domain` for repos outside a root.
- Tools are Go files in `mi6`. Claude Code and OpenCode in v1.
- Instructions are `AGENTS.md` in config, generated per launch, named per tool.
- One merge rule: instructions concatenate, skills union, JSON deep-merges with lists unioned.
- MCP servers written straight into each tool's file. Plugins out.
- A project override is a layer under `projects/`, giving that repo its own set.
- Skill source repos clone into the state directory unless a path is given.
- Pull cadence is opt-in in `mi6.json`.
- Out of v1: accounts, network contexts, Astro home, model policy, plugins.

## 1. Design and example config

Docs only. Done when the design doc reflects every decision above, the
dropped features sit under Later with a sketch each, and
`examples/config/` is a config repo someone can copy and use.

- Rewrite `docs/domains.md`.
- Write this plan.
- Write `examples/config/` with fake names in the shape of the real one.
- Update the README.

## 2. Resolve

`mi6 resolve` works and is tested. Done when it prints the right stack for a
repo under a root, a worktree of that repo, a repo with `mi6.domain` set, a
repo under no root, and a plain directory.

- Go module, `cmd/mi6`, a `config` package that reads `mi6.json`.
- A `resolve` package: find the main checkout, match roots, walk the layer
  tree, apply the overrides.
- Fixture config trees under `testdata/`. Tests that create temporary git
  repos and worktrees.

## 3. Build

`mi6 build` writes a set for both tools and is tested. Done when a second
run with no config change writes nothing.

- A `layer` package: load a layer folder into a struct.
- A `merge` package: instructions, skills, JSON. Table tests for each rule.
- A `tool` package with the interface and the `claude` and `opencode` files.
- A `set` package: compute the target tree, diff it against disk, apply.
- Golden-file tests: fixture layers in, expected set out.

## 4. Launch

`mi6 <tool>` starts the real tool. Done when Claude Code and OpenCode each
start under a set and show the domain's instructions and skills.

- Reserved subcommand names. `--no-domain`. Argument pass-through. `exec`.
- A fake tool under `testdata/bin/` that prints its environment and exits, so
  the launch path is tested without either real tool.
- A manual checklist in `docs/verify.md` for the two real tools, with the
  items from the design's last section.

## 5. Sources, sync, doctor

- Skill sources: clone into the state directory, or use a given path. Link
  skills under the collision rule.
- Pull cadence with timeout, offline tolerance, and the one-line report on a
  failed fast-forward.
- Project layer and machine layer.
- `mi6 doctor`.

## 6. Bootstrap handoff

- Move `development-tools-bootstrap` into the `meta` domain.
- Replace the skill symlinking in `scripts/claude-env.sh` with an `mi6`
  install, a config repo clone, and one `mi6 build`.
- Install the shell aliases.
- Create the private config repo from the real setup.
