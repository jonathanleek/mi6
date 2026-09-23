# mi6

A launcher that gives AI coding agents per-domain configuration.

Run `mi6 claude` or `mi6 opencode` instead of the bare command. `mi6` works out
which *domain* the repo belongs to from its path, assembles a config directory
from layers (shared, domain, sub-folder, this machine, the current network), and
points the agent at it through the tool's own config-directory variable. Nothing
is written into the repo, so client repos stay clean, and one edit to a domain's
instructions or skills reaches every project in that domain.

Status: design. The launcher does not exist yet. Read
[docs/domains.md](docs/domains.md) for the problem, the design, and what is
still to be verified.

## Layout

- `docs/domains.md`: the design.
- `domains/`, `contexts/`, `machines/`: not here. They live in a separate,
  private config repo that `MI6_CONFIG` points at. An example tree will land
  here as a starting point.
