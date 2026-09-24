#!/bin/sh
# Check that Claude Code honors availableModels from a set's settings.json.
# Writes a model allowlist into the globex layer of a scratch home, rebuilds
# the set, starts Claude Code with a model outside the list, and shows what
# happens. Restores the layer afterward.
#
# usage: scripts/verify-models.sh <scratch home root>
# The root is the folder that holds home/ and state/, the $SB of verify.md.
set -u

SB=${1:?usage: verify-models.sh <scratch home root>}
MI6=$(cd "$(dirname "$0")/.." && pwd)/bin/mi6
LAYER=$SB/home/Documents/git/work/clients/globex/.mi6
PROJECT=$SB/home/Documents/git/work/clients/globex/pipeline
BACKUP=$LAYER/claude.json.before-models

m() { HOME=$SB/home XDG_STATE_HOME=$SB/state "$MI6" "$@"; }
hr() { printf '\n=== %s\n' "$1"; }

cd "$PROJECT" || exit 1
[ -f "$LAYER/claude.json" ] && cp "$LAYER/claude.json" "$BACKUP"

hr "1. write an allowlist into the globex layer"
cat > "$LAYER/claude.json" <<'EOF'
{"availableModels": ["sonnet"], "enforceAvailableModels": true}
EOF
cat "$LAYER/claude.json"

hr "2. rebuild the set (expect: claude patched or wrote settings.json)"
m resolve | sed -n '/^set/,$p'
SET=$(m resolve | awk '/^set/ {print $2}')

hr "3. the generated settings.json"
cat "$SET/claude/settings.json"

hr "4. start with --model opus (expect a warning that opus was replaced, then an answer from Sonnet)"
CLAUDE_CONFIG_DIR=$SET/claude claude --model opus -p "Which model are you? One line." < /dev/null 2>&1

hr "5. start with no --model (expect an answer from Sonnet, since enforceAvailableModels is on)"
CLAUDE_CONFIG_DIR=$SET/claude claude -p "Which model are you? One line." < /dev/null 2>&1

hr "6. restore the layer"
if [ -f "$BACKUP" ]; then mv "$BACKUP" "$LAYER/claude.json"; else rm -f "$LAYER/claude.json"; fi
m resolve | sed -n '/^set/,$p'
