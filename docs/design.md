# Layers: per-folder agent configuration

This is the design behind `mi6`, and the record of what was checked against
the real tools. For how to install and start, read the [README](../README.md).
For the order things were built in, read [the plan](plan.md).

## The problem

You keep repos in one tree, grouped by who the work is for:

```
~/Documents/git/
	personal/
	work/
	work/clients/globex/
	workshop/
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
| `models.json` | Model tags and the rules that allow or deny them. See [Model policy](#model-policy). |

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

There is one exception, `allow` in `models.json`, which narrows instead of
widening. The next section says why.

## Model policy

Decided on 2026-09-29, built in v3. Until v3 ships, the code has the older
rule: `availableModels` in `claude.json` and `enabled_providers` in
`opencode.json` narrow across layers by exact string.

### The problem

An employer policy bans models from Chinese companies. Some private
projects should use only models served on the home network. Both are
rules about groups of models, and a model list per layer, named one model
at a time, does not say either one. And both are enforcement: a model that
nobody has classified must not slip through.

So models get **tags**, and layers allow or deny tags. Enforcement holds
when the tool is launched through `mi6`, which is the bar the employer
policy needs. `mi6 --bare` and a bare tool bypass it. The audit log records
the first.

### `models.json`

```json
{
  "tags": {
    "chinese": "Developed by a company based in China",
    "network": "Served from hardware on the home network",
    "cloud": "Served by a third-party API",
    "anthropic": "Made by Anthropic"
  },
  "providers": {
    "anthropic": { "tags": ["cloud"] },
    "lmstudio": { "tags": ["network"] }
  },
  "models": {
    "anthropic/claude-sonnet-5": { "tags": ["anthropic"], "claude": "sonnet" },
    "lmstudio/qwen3-coder-30b": { "tags": ["chinese"] },
    "lmstudio/gpt-oss-120b": { "tags": [] }
  },
  "deny": ["chinese"]
}
```

A tag appears in three places, each with its own job:

- **`tags` defines it**, with one line saying what it means. A tag used
  anywhere else but defined nowhere in the stack is an error, because a
  deny on a misspelled tag would deny nothing.
- **`providers` and `models` attach it.** A model key is `provider/model`,
  which is also OpenCode's name for it, and the provider must have an entry.
  A model's tags are its own plus its provider's, so the same weights served
  locally and from a cloud API are two entries with different tags. The
  `claude` key is Claude Code's name for the model. A model without one is
  not offered to Claude Code.
- **`allow` and `deny` use it.** Each is a list of tags. Rules name tags
  only, never a model. A tag with one model in it names one model.

The catalog, meaning the definitions and the attached tags, normally lives
in `~/.mi6/models.json`. A rule lives in the layer where the restriction
should start. A private project's layer holds `{"allow": ["network"]}` and
nothing else.

### How it merges

`tags`, `providers`, `models`, and `deny` follow the ordinary rule. A tag
list unions, so a layer can attach a tag and never remove one that a layer
above attached. A deny at the top reaches every set and nothing below lifts
it.

`allow` is the exception. Under the union rule a nearer layer could only
widen it, which is backwards. So each layer's `allow` is kept apart, and a
model must pass every one of them. A nearer layer can only narrow.

### Which models are allowed

The policy applies when any layer in the stack has an `allow` or a `deny`.
With no rule anywhere, `mi6` restricts nothing. For each tool:

1. Start with the catalog models the tool can use: every model for
   OpenCode, those with a `claude` name for Claude Code.
2. Remove every model with a denied tag.
3. For each layer with an `allow`, keep only the models with at least one of
   its tags.

What is left is the tool's allowed list. A model outside the catalog is
never allowed. Tagging a new model is a chore that comes with failing closed.

### When a launch is refused

Each refusal names the file and layer at fault.

| Problem | Refuses |
|---|---|
| A tag is used but never defined | every tool |
| A model's provider has no `providers` entry | every tool |
| A hand-written `availableModels`, `enabled_providers`, `whitelist`, or `blacklist` | that tool |
| The allowed list is empty | that tool |
| A model the tool's settings name is not allowed | that tool |
| A layer sets a variable that redirects the tool's model | that tool |
| The checkout's own tool settings touch models | that tool |
| A passed-through argument replaces the tool's settings | that tool |
| The tool has no way to enforce a model list | that tool |
| The audit log cannot be written | every tool |

A private project that leaves Claude Code no model still lets
`mi6 opencode` start there. The message lists the rules in force and each
catalog model with the reason it was removed:

```
mi6: not starting claude: no model is allowed here
  deny   chinese   from ~/.mi6
  allow  network   from ~/Documents/git/personal/secret-proj/.mi6
  claude models in the catalog:
    anthropic/claude-sonnet-5   tags: anthropic, cloud   removed: not in allow (secret-proj)
```

The model lists belong to `models.json` alone, so a hand-written list in a
tool's settings is an error rather than a second place to look.

The checkout is the one place in the stack that someone else writes to, and
both tools read settings from it that outrank the set's. So under a policy
`mi6` reads the checkout's tool settings before the launch and refuses if
any of them touches models: `.claude/settings.json` and
`.claude/settings.local.json` for Claude Code, and `opencode.json` and
`.opencode/opencode.json` from the working directory up to the checkout's
root for OpenCode. Touching models means a model list, a model key, a
redirecting variable, or a provider entry. Anything else in those files,
and the checkout's `CLAUDE.md` and `AGENTS.md`, is left alone. Each tool
has a switch that ignores the checkout's settings outright,
`--setting-sources user` and `OPENCODE_DISABLE_PROJECT_CONFIG`, but the
Claude Code one drops the checkout's `CLAUDE.md` too, so `mi6` refuses
instead.

### How each tool enforces it

The tool does the enforcing. `mi6` writes the allowed list into the tool's
settings in the set.

**Claude Code.** `availableModels` gets the `claude` names of the allowed
models, and `enforceAvailableModels` is `true`. A `--model` outside the list
is replaced at start, `/model` refuses the switch, and the default model
obeys the list. All three are verified below. `model` in the merged settings
must be allowed.

Variables can point Claude Code's model names somewhere else:
`ANTHROPIC_MODEL`, `ANTHROPIC_DEFAULT_OPUS_MODEL`,
`ANTHROPIC_DEFAULT_SONNET_MODEL`, `ANTHROPIC_DEFAULT_HAIKU_MODEL`,
`ANTHROPIC_SMALL_FAST_MODEL`, `CLAUDE_CODE_SUBAGENT_MODEL`, and
`ANTHROPIC_BASE_URL`. Under a policy a layer that sets one, in `env.json` or
in the `env` key of `claude.json`, is refused, and `mi6` removes them from
the environment it passes on, so one exported in your shell does not reach
the tool either. `--settings` and `--setting-sources` in the arguments
passed through are refused, since the first outranks the set's settings and
the second changes what is read. Verified on 2026-09-29: a `--model`
outside the list is replaced, the default model obeys the list, and so
does the one background request an interactive session makes.

**OpenCode.** `enabled_providers` gets the providers with an allowed model,
which also shuts out any provider defined in `~/.config/opencode`.
`provider.<id>.whitelist` gets each provider's allowed models. `model`,
`small_model`, and every `agent.*.model` in the merged settings must be
allowed. Verified on 2026-09-29: a model outside the lists is refused
through `-m`, through the API the `/model` picker uses, as an agent's
model, and as the default, with no request sent. A `small_model` outside
the lists falls back to an allowed one.

OpenCode reads its config in a fixed order, and the set's file is read
last of the files on disk because `mi6` sets `OPENCODE_CONFIG_DIR`. So
the set's lists win over the global file and over the checkout's. What the
set does not write, the checkout can still set, which is why a provider
entry in the checkout's config is refused. `OPENCODE_CONFIG_CONTENT` is
read after the set and overrides it, so `mi6` removes it from the
environment. Two sources are read after the set and are out of reach: the
config of an OpenCode console account's organization, and config an
administrator installs on the machine. `mi6 doctor` reports both.

**A third tool** declares how it enforces a model list. Under a policy, a
tool that cannot is refused.

### The audit log

Every launch appends one line to `$XDG_STATE_HOME/mi6/audit.jsonl`, with or
without a policy, whether it starts, is refused, or is `--bare`. If the line
cannot be written the launch is refused.

```json
{"time": "2026-09-29T14:02:11-04:00", "mi6": "0.4.0", "tool": "opencode",
 "dir": "/Users/you/Documents/git/work/clients/globex/pipeline",
 "set": "~/.local/state/mi6/sets/0851a02eab9b",
 "layers": ["~/.mi6", "~/Documents/git/.mi6"],
 "policy": "sha256:3f9a…",
 "deny": [{"tag": "chinese", "from": "~/.mi6"}], "allow": [],
 "allowed": ["anthropic/claude-sonnet-5", "lmstudio/gpt-oss-120b"],
 "removed": [{"model": "lmstudio/qwen3-coder-30b", "why": "deny chinese (~/.mi6)"}],
 "default": "anthropic/claude-sonnet-5", "outcome": "started"}
```

`policy` is a hash of the merged `models.json`, so two launches can be shown
to run under the same rules without copying the catalog into every line.
The log says what was allowed. What was used is in the tools' own history:
Claude Code's transcripts in the set, OpenCode's sessions by directory. `set`
and `dir` join the two.

### Commands

`mi6 resolve` prints the rules in force, each tool's allowed list, and each
removed model with its reason. `mi6 doctor` reports every refusal above, and
warns about a redirecting variable in the current shell and about model or
provider keys in `~/.config/opencode`.

The catalog has its own commands. Rules do not: a policy change is a
hand-edit, so it is deliberate and shows in a diff.

| Command | Does |
|---|---|
| `mi6 models` | The catalog, each model's tags, and whether it is allowed here and why |
| `mi6 models discover` | Models the providers serve that the catalog lacks |
| `mi6 models add <provider/model> [--tag t…] [--claude name]` | Add a model |
| `mi6 models export [file]` | The merged catalog as one file: definitions and attached tags, no rules |
| `mi6 models import <file>` | Merge a catalog file in. Adds only; a conflict is reported |
| `mi6 tags` | The defined tags, their meaning, their layer, and how many models carry each |
| `mi6 tags add <tag> "<meaning>"` | Define a tag |
| `mi6 tag <model> <tag>…` | Attach tags |
| `mi6 untag <model> <tag>…` | Remove tags, from the layer that attached them only |

A command that writes edits `~/.mi6/models.json` unless `--layer <dir>`
names another layer, since a tag is a fact about a model, not about the
folder you are standing in. It rewrites the file with sorted keys.

`discover` asks each OpenAI-compatible provider in the merged
`opencode.json` for its `/v1/models`, and runs `opencode models`. Claude
Code has no command that lists its models, so those are added by hand.

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
where MCP servers go, and how to start the tool. Two ship: Claude Code and
OpenCode. Adding a third is a pull request.

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

`mi6 init [dir]` creates `.mi6/` in a directory with every file `mi6`
reads, each empty but valid, plus a README that says what each one is and
that `mi6` ignores. It never overwrites: a second run reports every file as
kept. At a git checkout's root it prints the `mi6.trust` hint, since that
layer does not count until the clone is trusted.

`mi6 resolve` prints the stack for the current directory, where each layer
came from, the set's location, and the merged variables. It builds the set
too, so a setup script can call it.

`mi6 --bare <tool>` skips the stack and starts the tool with its plain user
config. Use it to repair a broken layer from inside the tool. It skips the
model policy too, and the audit log records that it did.

`mi6 doctor` checks that a launch from the current directory would work:
git and each tool on the path with their versions, the state directory
writable, every layer in the stack parses, no skill collisions, and whether
this shell is already inside a tool that `mi6` started. It exits non-zero
on a failure, so a setup script can gate on it. Warnings do not fail it.

`init`, `resolve`, `doctor`, `models`, `tags`, `tag`, `untag`, `help`, and
`version` are reserved names. Any other first argument is a tool. The model
commands are under [Model policy](#model-policy).

## What happens without `mi6`

Bare `claude` reads `~/.claude` as it always did. Nothing `mi6` built applies,
and nothing `mi6` built is touched. The two setups share no history,
settings, or login. The risk is a client repo opened with your personal
permissions because you forgot the prefix. The fix is a shell alias, `claude`
to `mi6 claude`.

## Where the layers live

A layer is a folder in your tree. That is the whole answer: make the
folders, put files in them, done. There is no config repo, no setting that
names one, and nothing to clone. `mi6` finds the folders by walking up from
the repo you are in.

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

- **Syncing layers between machines.** The tree is not a git repo, so the
  folders do not travel on their own. The earlier design mirrored the tree
  in a separate repo that `mi6` pulled, and that was dropped for the
  complexity it brought. Whatever comes back should be a first-class
  feature, designed on its own, not a pattern the user assembles.
- **Skill sources.** A layer that names a git repo, cloned and pulled by
  `mi6`. A symlink under `skills/` to a checkout you already have covers it.
- **A model gateway.** Under a policy, `ANTHROPIC_BASE_URL` is refused,
  since it can serve any model under the name `sonnet`. A layer that sets it
  could instead name the provider it points at, so that provider's tags
  apply. Wait until a gateway is in use.
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
- **Teams.** A shared layer repo with per-user layers on top. The stack does
  not preclude it.

## Verified on this machine

Checked on 2026-09-23 and 2026-09-24 with Claude Code 2.1.281 and OpenCode
1.18.30.

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
- Model enforcement in both tools, on 2026-09-29 with Claude Code 2.1.285
  and OpenCode 1.18.30, against fake servers that record the model of every
  request, so no login is needed. `scripts/verify-enforcement.sh` repeats
  the checks. The findings are in the model policy section, in short: the
  set's lists hold against `--model`, `-m`, the `/model` picker, agents,
  defaults, and background requests, and against the global config. They
  do not hold against the checkout's own tool settings, which outrank the
  set in both tools, against `OPENCODE_CONFIG_CONTENT`, or against a
  passed-through `--settings`. The design refuses each of those.
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
