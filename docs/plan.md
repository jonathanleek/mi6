# Build plan

Each milestone ends in a commit that is reviewed before the next starts.
[The design](design.md) is the spec. When the two disagree, fix the design
first.

Installing `mi6`, creating the `.mi6` symlinks into a private layer repo, and
wiring the shell so a bare `claude` goes through `mi6` are the user's own
setup. None of that lives in this repo.

## v1: done on 2026-09-23

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

### 1. Design and example (done)

Docs only. Done when the design reflects every decision above, the dropped
features sit under Later with a sketch each, and `examples/` shows a tree
someone can copy.

### 2. Resolve (done)

`mi6 resolve` prints the stack and is tested. Done when it is right for a
repo in the tree, a worktree of that repo, a repo with `mi6.parent` set, a
repo outside the tree, a plain directory, and a repo with its own `.mi6/`
both with and without `mi6.trust`.

- Go module, `cmd/mi6`.
- A `resolve` package: find the main checkout, apply `mi6.parent`, walk up,
  collect layers.
- Tests that create temporary trees, git repos, and worktrees.

### 3. Build (done)

`mi6 resolve` also writes a set for both tools, and is tested. Done when a
second run with no config change writes nothing.

- A `layer` package: load a `.mi6/` folder into a struct, following skill
  symlinks one level.
- A `merge` package: instructions, skills, JSON. Table tests for each rule.
- A `tool` package with the interface and the `claude` and `opencode` files.
- A `set` package: compute the target tree, diff it against disk, apply.
- Golden-file tests: fixture layers in, expected set out.

### 4. Launch (done)

`mi6 <tool>` starts the real tool. Done when Claude Code and OpenCode each
start under a set and show the stack's instructions and skills.

- Reserved subcommand names. `--bare`. Argument pass-through. `exec`.
- A fake tool under `testdata/bin/` that prints its environment and exits, so
  the launch path is tested without either real tool.
- A manual checklist in `docs/verify.md` for the two real tools, with the
  items from the design's last section.

## v2

Decided on 2026-09-23. Five milestones, in this order. 5 comes before 6
because the model check needs a set with a login. 7 and 8 are independent.
9 is last because everything before it changes the README.

### 5. Close the loose ends (done)

- Isolate the OpenCode stall seen in v1. Build a set with one unreachable
  local MCP server, then one unreachable remote, and time each start. Write
  the finding into the design's verified list and the OpenCode tool file.
- The interactive Claude Code check in [verify.md](verify.md): log in under
  a set and run the list. Only a person can do this. It gates v2.

### 6. Model policy (done)

- Verify that Claude Code honors `availableModels` from a user-level
  `settings.json`. Needs a set with a login.
- An intersection rule for `availableModels` in `claude.json` and
  `enabled_providers` in `opencode.json`, so a nearer layer narrows. Table
  tests. The design's merge section gets its first per-key exception, with
  the reason.

### 7. Per-layer environment (done)

- `env.json` in a layer: a flat object of variable names to strings. Merges
  like any object, nearest wins per key. Exported on launch after the tool's
  own variables, so it cannot override them. A leading `~` in a value
  expands.
- `mi6 resolve` prints the merged variables. The golden test and the example
  get one.

### 8. `mi6 doctor` (done)

- Checks each tool on the PATH with its version, the state directory is
  writable, every layer in the current stack parses, no skill collisions,
  and `MI6_TOOL` is not already set. Non-zero exit on any failure so a setup
  script can gate on it.
- Nothing about shell hooks or aliases.

### 9. Ship it (done, except the first release)

The tap repo, its token, and the first tag are on hold.

- CI: `go test` and `go vet` on every pull request.
- GoReleaser with version stamping, a tag-driven release, and a Homebrew
  tap. Unsigned binaries to start.
- README polish, including the comparison below.

### Not in v2

- **Accounts.** Per-set login is the daily cost. The fix needs a token
  source. Decide after living with it.
- **More layer content.** `rules/`, `agents/`, `commands/`. Wait until one
  is missed.
- **Plugins.** Nobody has asked.
- **A third tool.** When someone runs one.

## v3: model policy

Decided on 2026-09-29. [Model policy](design.md#model-policy) is the spec.
Eight milestones, in this order. 10 is first because if OpenCode does not
enforce its lists, 12 changes shape.

- Tags on models and providers, allow and deny by tag, in `models.json`.
- Enforced when launched through `mi6`. Fail closed: an uncataloged model is
  never allowed, and an empty allowed list refuses the launch.
- Hand-written model lists in a tool's settings become an error.
- An audit log of every launch, always on.

### 10. Verify OpenCode (done)

Done on 2026-09-29 against fake servers, so no login was needed:
`scripts/verify-enforcement.sh`. OpenCode enforces its lists everywhere
the design needs, and so does Claude Code, including the background
request of an interactive session. Five ways around the lists were found
and the design now refuses each: the checkout's own tool settings in both
tools, a trusted checkout's `ANTHROPIC_BASE_URL`, `OPENCODE_CONFIG_CONTENT`,
and a passed-through `--settings`. Milestone 12 builds those refusals.

### 11. Policy engine (done)

- A `models` package: load and merge `models.json`, compute each tool's
  allowed list with the reason each other model was removed, and report
  the refusals that come from the catalog itself: an undefined tag, a
  model without a provider entry, a file with the wrong shape. Table
  tests. The refusals that need a tool's settings or the checkout are
  milestone 12.
- `layer` loads `models.json`. `mi6 init` scaffolds an empty one.

### 12. Enforcement

- A tool declares how it enforces a model list. Claude Code writes
  `availableModels` and `enforceAvailableModels`. OpenCode writes
  `enabled_providers` and each provider's `whitelist`.
- Launch refuses on any refusal from 11, checks every model a tool's
  settings name, and removes the redirecting variables from the environment,
  `OPENCODE_CONFIG_CONTENT` among them.
- Launch reads the checkout's own tool settings, `.claude/settings.json`,
  `.claude/settings.local.json`, and OpenCode's `opencode.json` and
  `.opencode/opencode.json` up to the checkout root, and refuses if any
  touches models. It refuses `--settings` and `--setting-sources` in the
  arguments passed to Claude Code.
- Remove the v2 narrowing rule from `merge`.
- Convert the example tree and the golden test.

### 13. Audit and reporting

- `audit.jsonl`: one line per launch, started, refused, or bare. A launch
  that cannot write it is refused.
- The models section in `mi6 resolve`. The new checks in `mi6 doctor`.

### 14. Catalog commands

- `mi6 models`, `models add`, `models export`, `models import`, `tags`,
  `tags add`, `tag`, `untag`. Writes default to `~/.mi6/models.json`,
  `--layer` names another.

### 15. `mi6 models discover`

- `/v1/models` from each OpenAI-compatible provider in the merged
  `opencode.json`, and `opencode models`. Lists what the catalog lacks.

### 16. Docs

- The README's layer table and merge paragraph. `docs/verify.md`.

### 17. Tagging skill

- Not in this repo. A skill in `~/.mi6/skills/` that runs `discover`,
  researches each new model's maker and origin from real sources, proposes
  tags, waits for approval, and runs `mi6 tag`.

## README: compare with other tools

Done in milestone 9. The comparison lives in the README.
