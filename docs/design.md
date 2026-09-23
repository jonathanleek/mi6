# Layers: per-folder agent configuration

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

Every repo under a folder should get the same agent setup with no per-project
work, and a sub-folder should be able to add to that setup. The setup cannot
be committed to the project's repo, because many repos belong to clients. It
must work for Claude Code and for OpenCode. And it must work when a tool
checks the repo out somewhere else as a git worktree, which Termic, Conductor,
and similar tools do.

The setup covers four things:

- instructions, in `AGENTS.md` form
- skills
- MCP servers
- tool settings, including permissions, since your own repos need write
  access that no client repo gets

## Why not a `CLAUDE.md` in the folder

Claude Code loads `CLAUDE.md` from every ancestor of the working directory, so
a `CLAUDE.md` in `~/Documents/git/work/` reaches every repo below it. Three
things rule it out as the whole answer:

- A git worktree checked out elsewhere is not below the folder, so the file
  never loads there.
- It carries instructions only. Skills, MCP servers, and settings in an
  ancestor folder are not read.
- OpenCode reads one `AGENTS.md`, the first it finds walking up, so a repo that
  ships its own hides the folder's.

The mechanism that both tools support, and that reaches a worktree, is an
alternate config directory chosen by environment variable. Claude Code reads
`CLAUDE_CONFIG_DIR`. OpenCode reads `OPENCODE_CONFIG` and
`OPENCODE_CONFIG_DIR`. So `mi6` builds that directory from the folders above
the repo and points the tool at it.

## The idea

A `.mi6/` folder anywhere in the tree is a **layer**. `mi6` finds the repo's
main checkout, walks up from it to your home directory, and every `.mi6/` it
passes is a layer. The layers stack, top of the tree first, and the stack is
built into one config directory per tool. That is the whole design.

```
~/.mi6/                                  every repo on this machine
~/Documents/git/.mi6/                    every repo in the tree
~/Documents/git/work/.mi6/
~/Documents/git/work/clients/.mi6/       stricter permissions
~/Documents/git/work/clients/globex/.mi6/
```

A repo under `globex/` gets all five. A repo under `personal/` gets the first
two and `personal/.mi6/` if there is one. A repo outside the tree gets
`~/.mi6/` alone.

## Terms

- **Layer**: a `.mi6/` folder.
- **Stack**: the layers above a repo, top of the tree first.
- **Set**: the stack built into one directory per tool.

## What a layer holds

Any of these, all optional. There is no file that belongs to `mi6` itself.

| File | Purpose |
|---|---|
| `AGENTS.md` | Instructions. |
| `skills/<name>/SKILL.md` | Skills. An entry under `skills/` may also be a symlink to a folder of skills, and `mi6` looks one level deeper. |
| `mcp.json` | MCP servers, in Claude Code's `mcpServers` shape. |
| `claude.json` | Claude Code settings, including permissions. The same shape as `settings.json`. |
| `opencode.json` | OpenCode settings. |

## How layers merge

One rule covers every file, so a new key in a tool's settings needs no code:

- **Instructions** concatenate, top of the tree first, each block under a
  heading that names its layer.
- **Skills** union by name. On a collision the layer nearest the repo wins.
- **JSON** deep-merges. An object merges key by key. A list unions, keeping
  order and dropping duplicates. Anything else takes the value from the layer
  nearest the repo.

A layer nearer the repo can add to a list but not remove from it. That is
deliberate. A `permissions.deny` entry at the top of the tree reaches every
set, and no client layer can lift it. If a case turns up where a deeper layer
must remove something, that is the moment to add a per-key rule, not before.

## How `mi6` finds the stack

1. Run `git rev-parse --git-common-dir`. Inside a worktree this returns the
   main checkout's `.git` directory, so a worktree resolves to the same stack
   as its main checkout. Outside a git repo, use the current directory.
2. If `git config mi6.parent` is set, walk up from that folder instead of the
   checkout's parent. This is how a repo outside the tree joins it. The
   setting is per clone, lives in `.git/config`, and is never committed.
3. Take the `.mi6/` inside the checkout, if there is one, then every `.mi6/`
   from the parent up to the home directory.

A `.mi6/` inside a repo is a layer like any other. In your own repos, commit
it or not as you like. In a client repo, list it in `.git/info/exclude`, which
is per clone. `mi6` reads a checkout and never writes into one.

## Built sets

The build writes one directory per stack and per tool under
`$XDG_STATE_HOME/mi6`, which defaults to `~/.local/state/mi6`. The directory
is named by a hash of the stack's layer paths. You never need to look inside
it. `mi6 resolve` prints its location.

Inside a set:

- **Skills are symlinks** into the layer. Editing the skill changes the
  running setup.
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
merged `claude.json` files, `skills/<name>` links, and the `mcpServers` key of
`.claude.json`. That last file is the tool's own state file, so the build
merges the key in and leaves the rest alone.

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

## The command

`mi6 <tool> [args]` resolves the stack, builds or refreshes the set, exports
the tool's variables, and replaces itself with the tool, passing the
arguments through.

`mi6 resolve` prints the stack for the current directory, where each layer
came from, and the set's location. It builds the set too, so a setup script
can call it.

`mi6 --no-domain <tool>` skips the stack and starts the tool with its plain
user config. Use it to repair a broken layer from inside the tool.

`resolve`, `help`, and `version` are reserved names. Any other first argument
is a tool.

## What happens without `mi6`

Bare `claude` reads `~/.claude` as it always did. Nothing `mi6` built applies,
and nothing `mi6` built is touched. The two setups share no history,
settings, or login. The risk is a client repo opened with your personal
permissions because you forgot the prefix. The fix is a shell alias, `claude`
to `mi6 claude`.

## Two machines

`mi6` never syncs anything. Where your layers live and how they travel
between machines is yours to decide. Two patterns:

- **Symlinks into a private repo.** Keep the layers in one git repo, and make
  each `.mi6/` in the tree a symlink into it. A setup script creates the
  links once. Client names stay in a private repo, and a pull on either
  machine updates every layer.
- **`~/.mi6/` is the machine layer.** The home directory is not synced, so
  the layer there holds what differs per machine, such as a local model
  server's address.

Do not put the state directory in iCloud Drive, Dropbox, or Syncthing. It
holds symlink farms and session databases, and file sync corrupts both.

## Worktree tools

Termic, Conductor, and similar tools check a repo out as a git worktree in
their own directory and run an agent there. Step 1 of the resolution handles
this: the worktree resolves to its main checkout's stack. Each such tool
needs two things from you:

- Register `mi6 claude` and `mi6 opencode` in place of the bare commands.
- Make sure `mi6` is on the `PATH` of whatever shell the tool spawns. Termic
  runs a login shell, so `.zprofile` is the file that matters there.

## Later

These were in earlier designs and are out on purpose. Each has a reason and a
sketch, so it can come back without re-deciding it.

- **A config repo `mi6` knows about.** The earlier design mirrored the tree
  in a separate repo, with roots, a config path, and an hourly pull. Symlinks
  from the tree into a private repo give the same result with no `mi6`
  concepts. Revisit if the symlink setup turns out to be a burden.
- **Skill sources.** A layer that names a git repo, cloned and pulled by
  `mi6`. A symlink under `skills/` to a checkout you already have covers it.
- **Accounts.** Each set has its own login today. Whether Claude Code's macOS
  login lives in a file or in the Keychain, and whether the Keychain entry
  follows `CLAUDE_CONFIG_DIR`, is unverified. Find out on a real machine
  before designing this.
- **Network contexts.** A layer that applies because of where the machine is:
  on the home network, on its VPN, or away. The general form is a layer plus
  a probe script that exits 0 when the context applies. Dropped because it is
  specific to one setup.
- **Per-tool environment.** A layer that exports variables to the tool, such
  as `ASTRO_HOME` for a client's Astronomer organization. Dropped because it
  serves one user.
- **Model policy.** A layer listing allowed models, turned into
  `availableModels` for Claude Code and `enabled_providers` for OpenCode.
  Claude Code merges `availableModels` from user settings rather than
  replacing it, so enforcement is advisory. Put the keys in `claude.json` by
  hand until this matters.
- **Plugins.** Claude Code installs plugins into the config directory with its
  own command, so a set would have to run that command at build time.
- **`mi6 doctor`.** A check of the tools on the path and the shell aliases.
  Add it when there is something to diagnose.
- **Teams.** A shared layer repo with per-user layers on top. The stack does
  not preclude it.

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
