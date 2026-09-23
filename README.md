# mi6

A launcher that gives AI coding agents per-folder configuration.

Put a `.mi6/` folder anywhere in your tree of repos. Run `mi6 claude` or
`mi6 opencode` instead of the bare command. `mi6` walks up from the repo,
stacks every `.mi6/` it passes, builds a config directory from them, and
points the agent at it through the tool's own config-directory variable.
Nothing is written into the repo, so client repos stay clean, and one edit to
a folder's instructions or skills reaches every repo below it.

Status: design. The launcher does not exist yet. Read
[docs/design.md](docs/design.md) for the problem and the design, and
[docs/plan.md](docs/plan.md) for the build order.

## Layout

- `docs/design.md`: the design.
- `docs/plan.md`: the milestones and the decisions behind them.
- `examples/`: a home layer and a tree of layers to copy from.
