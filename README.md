# mi6

A launcher that gives AI coding agents per-folder configuration.

Put a `.mi6/` folder anywhere in your tree of repos. Run `mi6 claude` or
`mi6 opencode` instead of the bare command. `mi6` walks up from the repo,
stacks every `.mi6/` it passes, builds a config directory from them, and
points the agent at it through the tool's own config-directory variable.
Nothing is written into the repo, so client repos stay clean, and one edit to
a folder's instructions or skills reaches every repo below it.

Status: v1 works. Read [docs/design.md](docs/design.md) for the problem and
the design, [docs/plan.md](docs/plan.md) for what was built and what comes
next, and [docs/verify.md](docs/verify.md) to check a build against the real
tools.

## Build

```
go build -o bin/mi6 ./cmd/mi6
go test ./...
```

`bin/mi6 resolve` prints the layers that apply to the current directory and
builds their set under `~/.local/state/mi6`. `bin/mi6 claude` and
`bin/mi6 opencode` start the tool under that set. Put `bin/` on your `PATH`,
or install with `go install ./cmd/mi6`.

## Layout

- `docs/design.md`: the design.
- `docs/plan.md`: the milestones and the decisions behind them.
- `examples/`: a home layer and a tree of layers to copy from.
- `cmd/mi6`: the command.
- `internal/resolve`: finds the stack of layers for a directory.
- `internal/layer`, `internal/merge`: load a layer, merge a stack.
- `internal/tool`: one file per supported tool.
- `internal/set`, `internal/build`: write a set and refresh it in place.
- `internal/launch`: export the tool's variables and replace the process.
