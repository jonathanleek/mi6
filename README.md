# mi6

Per-folder configuration for AI coding agents.

Put a `.mi6/` folder anywhere in your tree of repos. Run `mi6 claude` or
`mi6 opencode` instead of the bare command. `mi6` walks up from the repo,
stacks every `.mi6/` it passes, builds a config directory from them, and
points the agent at it through the tool's own config-directory variable.
Nothing is written into the repo, so client repos stay clean, and one edit to
a folder's instructions or skills reaches every repo below it.

```
~/.mi6/                                  every repo on this machine
~/Documents/git/.mi6/                    every repo in the tree
~/Documents/git/work/.mi6/
~/Documents/git/work/clients/.mi6/       stricter permissions, fewer models
~/Documents/git/work/clients/globex/.mi6/
```

A repo under `globex/` gets all five, top of the tree first. A repo under
`personal/` gets the first two and its own folder's, if there is one. A
worktree checked out somewhere else by Termic, Conductor, or a similar tool
gets the same stack as its main checkout.

## Install

```
go install github.com/jonathanleek/mi6/cmd/mi6@latest
```

A Homebrew tap comes with the first tagged release. Then `mi6 doctor` says
whether a launch from the current directory would work.

## Start

Make a layer and put instructions in it:

```
mkdir -p ~/.mi6
echo '# Every repo' > ~/.mi6/AGENTS.md
```

Then, in any repo:

```
mi6 resolve    # which layers apply here, and the set they built
mi6 claude     # Claude Code, under that set
mi6 opencode   # OpenCode, under that set
```

The first Claude Code start under a set asks you to log in. Each set is a
separate login. `mi6 --bare claude` starts the tool with its plain config.

## Where the layers live

In your tree. A layer is a folder, and for one machine that is the whole
setup: make the folders, put files in them, done. There is no config repo
and nothing to clone. `mi6` finds the folders by walking up from the repo
you are in.

If you work on two machines, keep the layers in a private git repo of your
own, laid out like the tree, and symlink each `.mi6/` in the tree to it. A
five-line script does it once per machine. `mi6` never knows the repo
exists. [The design](docs/design.md#where-the-layers-live) has the
details.

## What a layer holds

Any of these, all optional. There is no file that belongs to `mi6` itself.

| File | Purpose |
|---|---|
| `AGENTS.md` | Instructions. Layers concatenate, top of the tree first. |
| `skills/<name>/SKILL.md` | Skills. Layers union, nearest wins a name. An entry may be a symlink to a folder of skills. |
| `mcp.json` | MCP servers, in Claude Code's `mcpServers` shape. |
| `claude.json` | Claude Code settings, the same shape as `settings.json`. |
| `opencode.json` | OpenCode settings. |
| `env.json` | Variables to export to the tool. |

JSON merges key by key. Lists union, so a `permissions.deny` at the top of
the tree reaches every repo and no layer below can lift it. The one
exception is a model allowlist, which a nearer layer can narrow and never
widen. [examples/](examples/) is a full tree to copy from.

A `.mi6/` inside a cloned repo counts only after `git config mi6.trust true`
in that clone, because its `mcp.json` is a command that runs on your
machine. A repo outside your tree joins it with `git config mi6.parent
<folder>`.

## Compared with other tools

Two families of tools sit near this one.

Claude Code profile switchers, such as
[claude-profile-manager](https://github.com/JakubKontra/claude-profile-manager)
and [claude-profile](https://quinnjr.github.io/claude-code-profiles/), use the
same `CLAUDE_CONFIG_DIR` mechanism with named profiles, a marker file per
project, and a shell hook that switches on `cd`. Cross-tool config syncers,
such as [agentsmesh](https://samplexbro.github.io/agentsmesh/),
[ai-rules-sync](https://github.com/lbb00/ai-rules-sync),
[rulesync](https://github.com/dyoshikawa/rulesync), and
[agent-rules-sync](https://github.com/dhruv-anand-aintech/agent-rules-sync),
write one source of truth into each tool's native files, across many tools.

| | profile switchers | config syncers | `mi6` |
|---|---|---|---|
| Layers from parent folders | no, one flat profile | no, one config everywhere | yes |
| Worktree inherits the main checkout's config | no | no | yes |
| Nothing written into the repo | yes | no, native files in the project | yes |
| Claude Code and OpenCode from one source | Claude only | yes, many tools | yes, two tools |
| Skills, MCP, permissions | credentials and env per profile | yes | yes |

The gap is the combination in the last column. It matters to people with a
folder tree of client work and worktree-based tools, and it is narrow. The
syncers are complementary: author skills with one of them, select them with
`mi6`.

Two one-person proposals, [agentsstandard.com](https://agentsstandard.com/)
and the [.agents Protocol](https://dotagentsprotocol.com/), describe a
cascade of `AGENTS.md` files under a `~/.agents/` directory. Neither has
been adopted by a tool. `mi6` builds on the conventions that have been:
[agents.md](https://agents.md/) for the instruction format and
[Agent Skills](https://agentskills.io/) for skills.

## More

- [docs/design.md](docs/design.md): the design, and what was verified
  against the real tools.
- [docs/verify.md](docs/verify.md): how to check a build against Claude
  Code and OpenCode.
- [docs/plan.md](docs/plan.md): what was built, in what order, and what
  waits.

## Build from source

```
go build -o bin/mi6 ./cmd/mi6
go test ./...
```
