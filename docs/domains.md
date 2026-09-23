# Domains: per-folder agent configuration

Status: design. Nothing in this document is implemented yet. [The plan](plan.md)
says in what order it gets built.

## The problem

You keep repos in one tree, grouped by who the work is for:

```
~/Documents/git/
	personal/
	work/
	work/clients/globex/
	workshop/
	meta/
```

Each top-level folder is a *domain*. Every repo in a domain should get the same
agent setup with no per-project work, and a sub-folder or a single repo should
be able to add to that setup. The setup cannot be committed to the project's
repo, because many repos belong to clients. It must work for Claude Code and
for OpenCode. And it must work when a tool checks the repo out somewhere else
as a git worktree, which Termic, Conductor, and similar tools do.

The setup covers four things:

- instructions, in `AGENTS.md` form
- skills
- MCP servers
- tool settings, including permissions, since your own repos need write access
  that no client repo gets

## Why not a `CLAUDE.md` in the domain folder

Claude Code loads `CLAUDE.md` from every ancestor of the working directory, so
a `CLAUDE.md` in `~/Documents/git/work/` reaches every repo below it. Three
things rule it out as the whole answer:

- A git worktree checked out elsewhere is not below the domain folder, so the
  file never loads there.
- It carries instructions only. Skills, MCP servers, and settings in an
  ancestor folder are not read.
- OpenCode reads one `AGENTS.md`, the first it finds walking up, so a repo that
  ships its own hides the domain's.

The mechanism that both tools support, and that reaches a worktree, is an
alternate config directory chosen by environment variable. Claude Code reads
`CLAUDE_CONFIG_DIR`. OpenCode reads `OPENCODE_CONFIG` and
`OPENCODE_CONFIG_DIR`. So a domain is a config directory, and a launcher picks
it.

## Terms

- **Root**: a folder whose sub-folders are domains. `~/Documents/git` above.
- **Domain**: a top-level folder under a root.
- **Layer**: a folder on the path from the domain down to the repo that has a
  matching folder in the config repo. `work`, `work/clients`, and
  `work/clients/globex` are three layers.
- **Config set**: the merged result of `shared` plus every layer on the path,
  built as one directory per tool.
- **Launcher**: the `mi6` binary that resolves the layers, builds the set, and
  starts the tool.

## Repos

| Repo | Visibility | Holds |
|---|---|---|
| `mi6` | public | the launcher, this document, an example config tree |
| your config repo | private | your real `domains/`, `machines/`, and `projects/` |

`mi6` finds the config repo through `MI6_CONFIG`, which defaults to
`~/.config/mi6`. Clone your config repo there, or set the variable. The tool
and the config are separate because the config holds client names and the tool
is meant to be shared. Start a config repo by copying [examples/config](../examples/config).

## How the launcher finds the domain

The launcher resolves the domain from the path of the repo's main checkout,
not from its git remote. A remote does not identify the domain. Some client
repos are cloned from the client, some are created in your own org, and some
have no remote.

1. If `MI6_DOMAIN` is set, use it as the layer path and stop.
2. Run `git config mi6.domain`. If it is set, use it as the layer path and
   stop. This is how a repo outside every root joins a domain. The setting is
   per clone, lives in `.git/config`, and is never committed.
3. Run `git rev-parse --git-common-dir`. Inside a worktree this returns the
   main checkout's `.git` directory, so a worktree resolves to the same domain
   as its main checkout.
4. Find the root that contains that checkout. Take the path relative to the
   root, for example `work/clients/globex/globex-pipeline`.
5. Walk that path from the top. Each folder with a matching folder under
   `domains/` in the config repo is a layer.
6. If the directory is not a git repo, or the checkout is under no root, the
   layer path is empty. The set is `shared` plus `default`.

`default` is optional. Without it, a repo outside every root gets `shared`
alone, so a fresh install with one layer works.

## Layout of the config repo

```
mi6.json                     roots, pull cadence
domains/
	shared/                    applied to every set
	default/                   repos under no root
	meta/
	personal/
	work/
	work/clients/              stricter permissions
	work/clients/globex/       one client
	workshop/
machines/
	desktop/                   applied on that machine only
	laptop/
projects/
	globex-pipeline/           one repo, by the name of its checkout folder
```

`mi6.json` holds what the launcher needs before it can resolve anything:

```json
{
	"roots": ["~/Documents/git"],
	"pull": {"every": "1h", "timeout": "10s"}
}
```

Every folder under `domains/`, `machines/`, and `projects/` is a layer. A
layer holds any of these, all optional:

| File | Purpose |
|---|---|
| `AGENTS.md` | Instructions. |
| `skills/<name>/SKILL.md` | Skills kept in the config repo. |
| `layer.json` | Skill repos to pull skills from. See Skill sources. |
| `mcp.json` | MCP servers, in Claude Code's `mcpServers` shape. |
| `claude/settings.json` | Claude Code settings, including permissions. |
| `opencode.json` | OpenCode settings. |

Layers stack in this order, later on top: `shared`, the domain path from the
top down, the project layer named after the checkout folder, the machine
layer. `default` takes the domain path's place when there is none.

## How layers merge

One rule covers every file, so a new key in a tool's settings needs no code:

- **Instructions** concatenate, top layer first, each block under a heading
  that names its layer.
- **Skills** union by name. On a collision the layer on top wins.
- **JSON** deep-merges. An object merges key by key. A list unions, keeping
  order and dropping duplicates. Anything else takes the value from the layer
  on top.

A layer on top can add to a list but not remove from it. That is deliberate.
A `permissions.deny` entry in `shared` reaches every set, and no client layer
can lift it. If a case turns up where a deeper layer must remove something,
that is the moment to add a per-key rule, not before.

## Built config sets

The build writes one directory per set and per tool under the XDG state
directory, `$XDG_STATE_HOME/mi6`, which defaults to `~/.local/state/mi6`:

```
~/.local/state/mi6/
	sets/
		work/clients/globex/
			claude/                CLAUDE_CONFIG_DIR for this set
			opencode/              OPENCODE_CONFIG_DIR for this set
		_default/                the empty layer path
	sources/                   cloned skill repos
```

A path that adds no layer of its own uses its nearest ancestor's set. A client
folder with no `domains/` entry runs as `work/clients`. A repo with a
`projects/` layer or a machine with a `machines/` layer gets its own set, named
after the full stack, because the set differs from its neighbours'.

Inside a set:

- **Skills are symlinks** into the config repo or a source clone. Editing the
  skill changes the running setup.
- **Instructions, settings, and MCP files are generated** on every launch,
  because they merge several layers. An edit is live on the next launch.
- **Everything the tool writes for itself** stays in the set: session history,
  the sign-in, auto-memory, caches. Each set is a separate install of the tool
  as far as the tool can tell. Log in once per set.

The refresh compares what is on disk with what it would write and touches
nothing that matches, so a launch with no config change costs a few file
reads.

## Tools

A tool is a Go file in `mi6` that knows four things: the environment variable
that moves the tool's config, the files the tool reads from that directory,
where MCP servers go, and how to start the tool. Two ship in v1. Adding a
third is a pull request.

**Claude Code.** `CLAUDE_CONFIG_DIR` points at `<set>/claude/`. The build
writes `CLAUDE.md` from the merged instructions, `settings.json` from the
merged `claude/settings.json` files, `skills/<name>` links, and the
`mcpServers` key of `.claude.json`. That last file is the tool's own state
file, so the build merges the key in and leaves the rest alone.

**OpenCode.** `OPENCODE_CONFIG_DIR` points at `<set>/opencode/` and
`OPENCODE_CONFIG` at `<set>/opencode/opencode.json`. The build writes
`AGENTS.md` from the merged instructions, `opencode.json` from the merged
`opencode.json` files with the `mcp` key filled from the merged `mcp.json`,
and `skills/<name>` links. The generated `opencode.json` also lists the
generated `AGENTS.md` under `instructions`, since the docs describe the global
rules file only at the default location.

The `mcp.json` shape is Claude Code's. The OpenCode tool file translates it:
a `command` entry becomes `type: local` with the command and arguments as one
list, a `url` entry becomes `type: remote`.

## The launcher

`mi6 <tool> [args]` does this on every run:

1. Pull the config repo and every skill repo the layers use, if `mi6.json`
   sets a cadence and the last pull is older than it. See Keeping machines in
   sync.
2. Resolve the layers.
3. Add the project layer, if `projects/<checkout-name>/` exists.
4. Add the machine layer, if `machines/<hostname>/` exists. See Machines.
5. Build or refresh the config set.
6. Export the tool's environment variables.
7. Replace itself with the real tool, passing the remaining arguments through.

`mi6 --no-domain <tool>` skips steps 1 to 6 and starts the tool with its
plain user config. Use it to repair a broken `meta` layer from inside the
tool.

The subcommands `resolve`, `build`, `doctor`, `help`, and `version` are
reserved names. Any other first argument is a tool.

| Command | Does |
|---|---|
| `mi6 resolve` | Prints the layer stack for the current directory and how each layer was chosen. |
| `mi6 build` | Builds or refreshes the set for the current directory without starting a tool. |
| `mi6 doctor` | Checks the config repo, the roots, the hostname, each tool on the path, and the shell aliases. |

## What happens without `mi6`

Bare `claude` reads `~/.claude` as it always did. Nothing mi6 built applies,
and nothing mi6 built is touched. The two setups share no history, settings,
or login. The risk is a client repo opened with your personal permissions
because you forgot the prefix. The fix is a shell alias, `claude` to
`mi6 claude`, and `mi6 doctor` warns when the alias is missing.

## Machines

Two machines run the same config repo but are not the same machine. One runs
large local models, the other cannot. `machines/<hostname>/` is a layer for
those differences, stacked last. The name is the short hostname, lowercased.
`mi6 doctor` prints it. A machine with no folder gets no machine layer.

```
machines/
	desktop/opencode.json      local models at 127.0.0.1
	laptop/opencode.json       cloud providers only
```

## Skill sources

A layer can pull skills from a git repo. `layer.json`:

```json
{
	"skill_sources": [
		"github.com/someone/claude-skills",
		{"repo": "github.com/someone/workshop-skills", "path": "~/Documents/git/workshop/workshop-skills"}
	]
}
```

Every `skills/<name>/` folder with a `SKILL.md` in the repo joins the layer's
skills, under the same collision rule. A plain entry clones into
`sources/` under the state directory. An entry with `path` uses a checkout you
already have, for a repo you edit. Put a source that every domain should have
in `shared`.

## Keeping machines in sync

Sync the source through git, rebuild what is derived on each machine, and
never file-sync the derived state.

| Thing | Lives in | Synced by |
|---|---|---|
| Layer config | the config repo | git |
| Skills | the config repo or a skill repo | git |
| Built config sets | the state directory | nothing; `mi6` rebuilds them |
| Session history, auto-memory, logins | inside each config set | nothing; per machine |

An edit to config is a commit. Change a domain's `AGENTS.md` on the laptop,
commit, push. On the desktop, the next launch pulls it and regenerates the
set.

The pull is the sync, and it is opt-in. When `mi6.json` sets `pull.every`,
`mi6` runs `git pull --ff-only` on the config repo and on each cloned skill
source at most that often, gives up after `pull.timeout`, and carries on
offline. If a pull cannot fast-forward, `mi6` leaves that repo alone, prints
one line naming it, and launches with what is on disk. Pushing stays a
deliberate act. Without `pull.every`, `mi6` never touches the network.

Do not put the state directory in iCloud Drive, Dropbox, or Syncthing. It
holds symlink farms and session databases, and file sync corrupts both.

## The `meta` domain

`meta` is an ordinary layer. It holds `mi6` and the config repo, and its
`claude/settings.json` grants write permission to the state directory and the
config repo, which `shared` denies to everything else.

The agent running under `meta` edits the files that define its own config. If
an edit breaks the `meta` set, `mi6 --no-domain claude` starts without it.

## Worktree tools

Termic, Conductor, and similar tools check a repo out as a git worktree in
their own directory and run an agent there. Step 3 of the domain resolution
handles this: the worktree resolves to its main checkout's domain. Each such
tool needs two things from you:

- Register `mi6 claude` and `mi6 opencode` in place of the bare commands.
- Make sure `mi6` is on the `PATH` of whatever shell the tool spawns. Termic
  runs a login shell, so `.zprofile` is the file that matters there.

## Later

These were in the first design and are out of v1 on purpose. Each has a
reason and a sketch, so it can come back without re-deciding it.

- **Accounts.** Each set has its own login today. The first design shared a
  login between sets with the same account through symlinked credential
  files. Whether Claude Code's macOS login lives in a file or in the Keychain,
  and whether the Keychain entry follows `CLAUDE_CONFIG_DIR`, is unverified.
  Find out on a real machine before designing this.
- **Network contexts.** A layer that applies because of where the machine is:
  on the home network, on its VPN, or away. The general form is a named layer
  plus a probe script that exits 0 when the context applies, with the probe
  kept in the config repo, not in `mi6`. The first design's probe checked the
  default gateway's MAC address and the VPN's `utun` route. Dropped because it
  is specific to one setup and the per-prompt refresh doubled the surface.
- **Astro home per layer.** The Astro CLI keeps one active organization per
  `ASTRO_HOME`. A layer that names an organization could get its own
  `astro/` directory in the set. The general form is a per-layer environment
  block, applied to any tool. Dropped because it serves one user.
- **Model policy.** A layer listing allowed models, turned into
  `availableModels` for Claude Code and `enabled_providers` for OpenCode, with
  a deeper layer able to narrow but not widen. Claude Code applies
  `availableModels` from user settings by merging, not replacing, so
  enforcement is advisory. Put the keys in the tool settings by hand until
  this matters.
- **Plugins.** Claude Code installs plugins into the config directory with its
  own command, so a set would have to run that command at build time. Not
  needed yet.
- **Teams.** A shared config repo with per-user overrides on top. The layer
  model does not preclude it.

## Checked before relying on it

- OpenCode reads `AGENTS.md` from `OPENCODE_CONFIG_DIR`. The docs name only
  `~/.config/opencode/AGENTS.md`. The `instructions` key is the fallback.
- OpenCode reads `skills/` from `OPENCODE_CONFIG_DIR`. The docs list agents,
  commands, modes, and plugins for that variable and the plural folders for
  `.opencode`.
- Claude Code writes `.claude.json` under `CLAUDE_CONFIG_DIR` and keeps the
  `mcpServers` key that the build merges in.
- `git rev-parse --git-common-dir` returns the main checkout's `.git` from
  inside a Termic task and a Conductor workspace.
