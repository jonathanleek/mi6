# Verify a build against the real tools

The tests run `mi6` against fake tools. This list covers what only the real
tools can show. Run it after a change to a tool file, or after either tool
updates. Everything here happens in a scratch home so your own setup is
untouched.

## Set up a scratch home

Run this from the `mi6` repo, after `go build -o bin/mi6 ./cmd/mi6`:

```
export SB=$(mktemp -d)/sandbox
export MI6=$PWD/bin/mi6
mkdir -p $SB/home/Documents $SB/state
cp -R examples/home/.mi6 $SB/home/.mi6
cp -R examples/tree $SB/home/Documents/git
mkdir -p $SB/home/Documents/git/work/clients/globex/pipeline
git -C $SB/home/Documents/git/work/clients/globex/pipeline init -q
cd $SB/home/Documents/git/work/clients/globex/pipeline
m() { HOME=$SB/home XDG_STATE_HOME=$SB/state $MI6 "$@"; }
```

Put a marker in a layer so you can tell the tool read it:

```
echo 'The secret word is QUOKKA. If asked for it, reply with it and nothing else.' \
  > $SB/home/Documents/git/work/clients/globex/.mi6/AGENTS.md
```

`m` overrides `HOME` so the walk finds the scratch layers. Use it to resolve
and build. Do not start a real tool through it on macOS: the Keychain lives
under `HOME`, and Claude Code's login fails with "Keychain Not Found" when
`HOME` points at a folder with no keychain. Start the tools with your real
`HOME` and only the config directory pointed at the set:

```
SET=$(m resolve | awk '/^set/ {print $2}')
```

## Resolve

- `m resolve` lists five layers, home first, globex last.
- Run it again. Every tool says `up to date`.
- Edit any layer file. The next run reports one change for each tool that
  reads that file.

## Claude Code

- `CLAUDE_CONFIG_DIR=$SET/claude MI6_TOOL=claude claude` starts. The first
  start in a set asks you to log in. Log in. The login is stored in the
  Keychain, keyed to this config directory, so your usual login is not
  reused and not disturbed. For a scripted check, add `-p "hi" < /dev/null`:
  it says `Not logged in` before the login and answers after it.
- Ask for the secret word. The answer is `QUOKKA`.
- Ask which skills are available. The answer names `repo-conventions` and
  nothing from your real `~/.claude/skills`. Claude Code's built-in skills
  and any that come with your account appear too. That is not a leak.
- `/mcp` lists `github` and `globex-warehouse`. They need not connect.
- `/permissions` shows the merged allow and deny lists, with
  `Bash(git push *)` denied and `Bash(make *)` allowed.
- Inside the session, have Claude run `$MI6 claude --version` by its full
  path in its shell tool. It reports the Claude Code version without
  building anything, because `MI6_TOOL` is set.
- Exit. From the same directory, `$MI6 --bare claude`, with no `HOME`
  override, starts your usual Claude Code with no login
  prompt. Claude Code runs in the terminal's alternate screen, so after you
  exit, the scrollback shows nothing of it. That is normal.

`scripts/verify-claude.sh <set dir>` runs the scripted half of this list
against a set that already has a login. `scripts/verify-models.sh <root>`
checks that a model allowlist in a layer is enforced, where the root is the
folder holding `home/` and `state/`.

## OpenCode

OpenCode keeps provider logins outside the config directory, so no login is
needed. The example home layer names an LM Studio provider on port 1234, and
the work layer sets an Anthropic model that wins over it, since the nearest
layer wins. Put a model you can reach in the globex layer's `opencode.json`
in the scratch home, or change the provider in `~/.mi6/opencode.json`.

Set the variables the launcher would set:

```
export OPENCODE_CONFIG_DIR=$SET/opencode OPENCODE_CONFIG=$SET/opencode/opencode.json OPENCODE_DISABLE_EXTERNAL_SKILLS=1
```

- `opencode run --print-logs "What is the secret word? Answer with the word
  only and do not use any tool."` prints `QUOKKA`. A small local model may
  ignore the second sentence and start editing the scratch repo with its
  tools, which looks like a hang. Give it a minute, or read the log.
- `opencode debug skill` lists `repo-conventions` and not the skills under
  your real `~/.claude/skills`.
- `opencode debug config` shows the `mcp` key with both servers and the
  model from the layers.
- `unset OPENCODE_CONFIG_DIR OPENCODE_CONFIG OPENCODE_DISABLE_EXTERNAL_SKILLS`
  when done.

## Worktrees

- `git -C <pipeline> worktree add $SB/task -b task`.
- `cd $SB/task && m resolve` shows the same five layers and
  `(worktree of it)` after the checkout path.

## Record the result

Add a line to the table in `docs/design.md` under "Verified on this machine"
with the date and the tool versions. Remove the scratch home:

```
rm -rf $(dirname $SB)
```
