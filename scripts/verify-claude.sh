#!/bin/sh
# Run the scripted part of docs/verify.md for Claude Code against a built set
# that already has a login. Prints one report. Interactive checks are listed
# at the end for a person to do.
#
# usage: scripts/verify-claude.sh <set dir>
# The set dir is the path "mi6 resolve" prints after "set".
set -u

SET=${1:?usage: verify-claude.sh <set dir>}
MI6=$(cd "$(dirname "$0")/.." && pwd)/bin/mi6
CFG=$SET/claude
export CLAUDE_CONFIG_DIR=$CFG MI6_TOOL=claude

hr() { printf '\n=== %s\n' "$1"; }
ask() { claude -p "$1" < /dev/null 2>&1; }

hr "set"
echo "$SET"
[ -d "$CFG" ] || { echo "no claude directory in the set"; exit 1; }
cat "$SET/layers"

hr "1. secret word (expect QUOKKA)"
ask "What is the secret word?"

hr "2. skills (expect repo-conventions; Claude Code's built-ins also appear)"
ask "List the names of the skills available to you, comma separated, nothing else."

hr "3. mcp servers (expect github and globex-warehouse; connecting may fail)"
echo "Servers named claude.ai come with your account, not the set."
claude mcp list 2>&1 | grep -E '^(github|globex-warehouse|claude\.ai)'

hr "4. permissions in the set's settings.json (expect git push denied, make allowed)"
python3 - "$CFG/settings.json" <<'PY'
import json, sys
p = json.load(open(sys.argv[1])).get("permissions", {})
print("allow:", p.get("allow"))
print("deny: ", p.get("deny"))
PY

hr "5. nested launch guard (expect a version, no rebuild)"
"$MI6" claude --version 2>&1

hr "6. bare mode, scripted (expect a version, then an answer from your usual login)"
unset CLAUDE_CONFIG_DIR MI6_TOOL
"$MI6" --bare claude --version; echo "exit $?"
"$MI6" --bare claude -p "Reply with the single word OK." < /dev/null 2>&1; echo "exit $?"

hr "7. bare mode, interactive: run these two yourself in this directory"
cat <<EOF
  $MI6 --bare claude; echo "exit \$?"
  claude; echo "exit \$?"
Each should start your usual Claude Code. Exit it. If either returns to the
prompt with no output, paste the exit code.
EOF
