# Verify a build against the real tools

The tests run `mi6` against fake tools. This list covers what only the real
tools can show. Run it after a change to a tool file, or after either tool
updates. Everything here happens in a scratch home so your own setup is
untouched.

## Set up a scratch home

```
export SB=$(mktemp -d)/sandbox
mkdir -p $SB/home/Documents $SB/state
cp -R examples/home/.mi6 $SB/home/.mi6
cp -R examples/tree $SB/home/Documents/git
mkdir -p $SB/home/Documents/git/work/clients/globex/pipeline
git -C $SB/home/Documents/git/work/clients/globex/pipeline init -q
cd $SB/home/Documents/git/work/clients/globex/pipeline
alias m="HOME=$SB/home XDG_STATE_HOME=$SB/state $OLDPWD/bin/mi6"
```

Put a marker in a layer so you can tell the tool read it:

```
echo 'The secret word is QUOKKA. If asked for it, reply with it and nothing else.' \
  > $SB/home/Documents/git/work/clients/globex/.mi6/AGENTS.md
```

## Resolve

- `m resolve` lists five layers, home first, globex last.
- Run it again. Every tool says `up to date`.
- Edit any layer file. The next run reports one change for each tool that
  reads that file.

## Claude Code

- `m claude` starts. The first start in a set asks you to log in. Log in.
  For a scripted check, `m claude -p "hi" < /dev/null` says `Not logged in`
  before the login and answers after it.
- Ask for the secret word. The answer is `QUOKKA`.
- `/skills` or a question about available skills shows `repo-conventions`.
- `/mcp` lists `github` and `globex-warehouse`. They need not connect.
- `/permissions` shows the merged allow and deny lists, with
  `Bash(git push *)` denied.
- In a second terminal, `m --bare claude` starts with your usual config and
  no login prompt.
- Inside the Claude session, run `mi6 claude version` in its shell tool. It
  reports the version without building anything, because `MI6_TOOL` is set.

## OpenCode

OpenCode keeps provider logins outside the config directory, so no login is
needed. The example home layer names an LM Studio provider on port 1234, and
the work layer sets an Anthropic model that wins over it, since the nearest
layer wins. Put a model you can reach in the globex layer's `opencode.json`
in the scratch home, or change the provider in `~/.mi6/opencode.json`.

- `m opencode run --print-logs "What is the secret word?"` prints `QUOKKA`.
  Without `--print-logs`, `opencode run` has been seen to hang when started
  from a script. Run it in a terminal if that happens.
- `m opencode debug skill` lists `repo-conventions` and not the skills under
  your real `~/.claude/skills`.
- `m opencode debug config` shows the `mcp` key with both servers and the
  model from the layers.

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
