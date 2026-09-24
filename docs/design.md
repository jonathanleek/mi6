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
main checkout, walks up from it, and every `.mi6/` it passes is a layer.
`~/.mi6/` is always the first layer, whether or not the walk passes through
home. The layers stack, top of the tree first, and the stack is built into
one config directory per tool. That is the whole design.

```
~/.mi6/                                  every repo on this machine
~/Documents/git/.mi6/                    every repo in the tree
~/Documents/git/work/.mi6/
~/Documents/git/work/clients/.mi6/       stricter permissions
~/Documents/git/work/clients/globex/.mi6/
```

A repo under `globex/` gets all five. A repo under `personal/` gets the first
two and `personal/.mi6/` if there is one. A repo outside the tree, or on
another volume, gets `~/.mi6/` and whatever lies between it and the root of
its own filesystem.

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
| `env.json` | Variables to export to the tool. A flat object of names to strings. A leading `~` in a value expands to your home directory. |

## How layers merge

One rule covers every file, so a new key in a tool's settings needs no code:

- **Instructions** concatenate, top of the tree first, each block under a
  comment line that names its layer.
- **Skills** union by name. On a collision the layer nearest the repo wins.
- **JSON** deep-merges. An object merges key by key. A list unions, keeping
  order and dropping duplicates. Anything else takes the value from the layer
  nearest the repo.
- **Variables** merge by name. The layer nearest the repo wins.

A layer nearer the repo can add to a list but not remove from it. That is
deliberate. A `permissions.deny` entry at the top of the tree reaches every
set, and no client layer can lift it. The flip side: a deny at the top also
binds your own repos, since Claude Code lets deny win over allow. Put a deny
at the level where every repo below it should have it, and nowhere higher.

There is one exception, for allowlists of models: `availableModels` in
`claude.json` and `enabled_providers` in `opencode.json`. Under the union
rule a nearer layer could only widen them, which is backwards for an
allowlist. So when two layers set one of these keys, the result is the
entries of the outer list that the nearer list also names, in the outer
list's order. A client layer can allow fewer models than the tree, never
more. Matching is by exact string. If nothing matches, the outer list stays
and `mi6 resolve` warns, since an empty allowlist would block every model
and a mismatch such as `sonnet` against `claude-sonnet-5` is more likely a
typo than an intent.

Claude Code honors `availableModels` from a user-level settings file, which
is where a set puts it: a `--model` outside the list is replaced at start
and `/model` refuses the switch. `enforceAvailableModels: true` in the same
file makes the default model obey the list too. Both are verified below.

## How `mi6` finds the stack

1. Run `git rev-parse --git-common-dir`. Inside a worktree this returns the
   main checkout's `.git` directory, so a worktree resolves to the same stack
   as its main checkout. Outside a git repo, use the current directory.
2. If `git config mi6.parent` is set, walk up from that folder instead of the
   checkout's parent. This is how a repo outside the tree joins it. The
   setting is per clone, lives in `.git/config`, and is never committed.
3. Start with `~/.mi6/`. Then add every `.mi6/` from the filesystem root down
   to the checkout's parent, skipping `~/.mi6/` if the walk passes it again.
4. Add the `.mi6/` inside the checkout, if there is one and the clone is
   trusted. See the next section. Outside a repo there is no clone to
   distrust, so a `.mi6/` in the directory itself is an ordinary layer.

### A layer inside the checkout is untrusted until you say otherwise

Every layer above the repo is a folder you made. The checkout is the one
place in the stack that someone else writes to. A cloned repo's
`.mi6/mcp.json` is a command that runs on your machine when the tool starts,
its `claude.json` can widen permissions, and its `AGENTS.md` is text the
agent follows. Claude Code shows you a repo's `.mcp.json` and asks before
using it. `mi6` would merge the file into the set before the tool ever ran.

So a checkout's `.mi6/` counts only when the clone says so:

```
git config mi6.trust true
```

The setting lives in the clone's `.git/config`, so it never arrives with a
clone and is never committed. Without it, `mi6` skips the folder and
`mi6 resolve` prints one line saying it did. Set it once in your own repos.
In a client repo, read the folder first, then decide.

In your own repos, commit the folder or not as you like. In a client repo,
list it in `.git/info/exclude`, which is per clone. `mi6` reads a checkout
and never writes into one.

## Built sets

The build writes one directory per stack and per tool under
`$XDG_STATE_HOME/mi6`, which defaults to `~/.local/state/mi6`. The directory
is named by a hash of the stack's layer paths and holds a `layers` file that
lists them and an `env` file with the merged variables. You never need to
look inside it. `mi6 resolve` prints its location.

Inside a set:

- **Skills are symlinks** into the layer. Editing the skill changes the
  running setup.
- **Instructions, settings, and MCP files are generated** on every launch,
  because they merge several layers. An edit is live on the next launch.
- **Everything the tool writes for itself** stays in the set: session history,
  the sign-in, auto-memory, caches. Each set is a separate install of the tool
  as far as the tool can tell. Claude Code asks you to log in once per set.
  OpenCode keeps its provider logins outside the config directory, so they
  carry over.

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
and `skills/<name>` links.

OpenCode differs from Claude Code in two ways that the launcher has to know:

- `OPENCODE_CONFIG_DIR` adds a directory. It does not replace
  `~/.config/opencode`, which OpenCode keeps reading. So an OpenCode session
  under `mi6` sees your global config with the set merged on top. Keep the
  global file small, or move its contents into `~/.mi6/`.
- OpenCode scans `~/.claude/skills` and `~/.agents/skills` on its own. `mi6`
  sets `OPENCODE_DISABLE_EXTERNAL_SKILLS=1` so the set's skills are the
  whole list, the same as for Claude Code.

The `mcp.json` shape is Claude Code's. The OpenCode tool file translates it:
a `command` entry becomes `type: local` with the command and arguments as one
list, a `url` entry becomes `type: remote`.

## The command

`mi6 <tool> [args]` resolves the stack, builds or refreshes the set, exports
the layers' variables and then the tool's own, and replaces itself with the
tool, passing the arguments through. The tool's variables go last, so a
layer's `env.json` cannot point the tool away from its set. Two more
variables go with them: `MI6_TOOL` with the tool's name and `MI6_SET` with
the set's directory.

`mi6 resolve` prints the stack for the current directory, where each layer
came from, the set's location, and the merged variables. It builds the set
too, so a setup script can call it.

`mi6 --bare <tool>` skips the stack and starts the tool with its plain user
config. Use it to repair a broken layer from inside the tool.

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

A worktree whose main checkout has been moved or deleted cannot resolve.
`mi6` then uses `~/.mi6/` alone and says why.

## Later

These were in earlier designs and are out on purpose. Each has a reason and a
sketch, so it can come back without re-deciding it.

- **A config repo `mi6` knows about.** The earlier design mirrored the tree
  in a separate repo, with roots, a config path, and an hourly pull. Symlinks
  from the tree into a private repo give the same result with no `mi6`
  concepts. Revisit if the symlink setup turns out to be a burden.
- **Skill sources.** A layer that names a git repo, cloned and pulled by
  `mi6`. A symlink under `skills/` to a checkout you already have covers it.
- **Accounts.** Each set has its own Claude Code login. That is verified:
  a fresh config directory starts logged out, and on macOS the login is a
  Keychain item keyed to the config directory, not a file in it. So there
  is no credential file to share between sets. Claude Code can take a
  long-lived token from `claude setup-token` through the environment, so a
  layer that names an account could have `mi6` export that account's token.
  Not needed until logging in per set becomes a burden.
- **Network contexts.** A layer that applies because of where the machine is:
  on the home network, on its VPN, or away. The general form is a layer plus
  a probe script that exits 0 when the context applies. Dropped because it is
  specific to one setup.
- **Plugins.** Claude Code installs plugins into the config directory with its
  own command, so a set would have to run that command at build time.
- **`mi6 doctor`.** A check of the tools on the path and the shell aliases.
  Add it when there is something to diagnose.
- **Teams.** A shared layer repo with per-user layers on top. The stack does
  not preclude it.

## Verified on this machine

Checked on 2026-09-23 with Claude Code 2.1.281 and OpenCode 1.18.30.

- Claude Code under a fresh `CLAUDE_CONFIG_DIR` starts logged out. The login
  does not follow from `~/.claude`.
- Claude Code reads `mcpServers` from a `.claude.json` that `mi6` wrote
  before the first start, and merges its own keys into that file without
  dropping the servers.
- OpenCode reads `AGENTS.md` from `OPENCODE_CONFIG_DIR` as instructions, with
  no `instructions` key. A model run answered with the marker from that file.
- OpenCode reads `skills/` from `OPENCODE_CONFIG_DIR`.
- OpenCode keeps reading `~/.config/opencode` with `OPENCODE_CONFIG_DIR`
  set, and scans `~/.claude/skills` unless told not to.
- `git rev-parse --git-common-dir` from a Termic task returns the main
  checkout's `.git` under `~/Documents/git`. A worktree also reads the main
  clone's `git config`, so `mi6.parent` and `mi6.trust` carry over.
- `mi6 opencode run` in a scratch home answers with the marker from the
  nearest layer's `AGENTS.md`, using the model from that layer.
- `mi6 claude -p` in a scratch home reaches Claude Code's login prompt for
  the set, which shows the real binary ran under `CLAUDE_CONFIG_DIR`.
- Logging in under a set works with the real `HOME` and fails with
  "Keychain Not Found" when `HOME` is overridden, because macOS finds the
  login keychain through `HOME`. `mi6` never sets `HOME`.
- A full interactive Claude Code session under a set, on 2026-09-24: it
  reads the set's instructions, lists the set's skills and none from
  `~/.claude/skills`, lists the set's MCP servers, and applies the merged
  permissions. `mi6 claude --version` inside the session returns at once.
  `mi6 --bare claude` starts the usual config with no login prompt.
- What follows the login rather than the config directory: the claude.ai
  connectors, and skills from plugins tied to the account. They appear in
  every set. Claude Code's built-in skills appear in every set too.
- `availableModels` in a set's `settings.json` is enforced, on 2026-09-24:
  with `["sonnet"]` and `enforceAvailableModels: true` in a layer,
  `claude --model opus -p` answered as Sonnet, and so did a start with no
  model named. `scripts/verify-models.sh` repeats the check.
- Unreachable MCP servers do not slow OpenCode's start. Four starts through
  `mi6 opencode run` with no servers, a local server that exits at once, a
  remote server whose host does not resolve, and both, took between 20 and
  91 seconds with no MCP error in the logs. The variation, and the
  six-minute stall seen during v1, was the local model treating the prompt
  as work to do and editing files in the scratch repo with its tools.

Not yet verified, because it needs a login in a fresh config directory:

- Claude Code loads `CLAUDE.md` and `skills/` from `CLAUDE_CONFIG_DIR`. The
  docs say every `~/.claude` path moves there. Check after the first login
  under a set.
