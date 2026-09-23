# mi6

A launcher that gives AI coding agents per-domain configuration.

Run `mi6 claude` or `mi6 opencode` instead of the bare command. `mi6` works out
which *domain* the repo belongs to from its path, assembles a config directory
from layers (shared, domain, sub-folder, this repo, this machine), and points
the agent at it through the tool's own config-directory variable. Nothing is
written into the repo, so client repos stay clean, and one edit to a domain's
instructions or skills reaches every project in that domain.

Status: design. The launcher does not exist yet. Read
[docs/domains.md](docs/domains.md) for the problem and the design, and
[docs/plan.md](docs/plan.md) for the build order.

## Layout

- `docs/domains.md`: the design.
- `docs/plan.md`: the milestones and the decisions behind them.
- `examples/config/`: a config repo to copy. Your real one is private and
  lives wherever `MI6_CONFIG` points, `~/.config/mi6` by default.
