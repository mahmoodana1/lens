#!/usr/bin/env bash
# Build lens and wire it into Claude Code's hooks.
#
# Existing settings are backed up first and merged into, never replaced.
set -euo pipefail

BIN="${HOME}/.local/bin/lens"
SETTINGS="${HOME}/.claude/settings.json"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "building $BIN"
mkdir -p "$(dirname "$BIN")"
(cd "$here" && go build -o "$BIN" .)

if [[ ! -f "$SETTINGS" ]]; then
  echo '{}' > "$SETTINGS"
fi

backup="${SETTINGS}.bak-$(date +%Y%m%d%H%M%S)"
cp "$SETTINGS" "$backup"
echo "backed up settings to $backup"

python3 - "$SETTINGS" "$BIN" <<'PY'
import json, sys

settings_path, binary = sys.argv[1], sys.argv[2]
with open(settings_path) as f:
    settings = json.load(f)

hooks = settings.setdefault("hooks", {})

wanted = {
    "PostToolUse":      ("Edit|Write|MultiEdit|NotebookEdit", f"{binary} hook post-tool-use"),
    "UserPromptSubmit": (None,                                f"{binary} hook prompt"),
    "SessionEnd":       (None,                                f"{binary} hook session-end"),
}

for event, (matcher, command) in wanted.items():
    entries = hooks.setdefault(event, [])

    # Drop any lens entry from a previous install, leaving other tools alone.
    for entry in entries:
        entry["hooks"] = [h for h in entry.get("hooks", []) if "lens hook" not in h.get("command", "")]
    entries[:] = [e for e in entries if e.get("hooks")]

    entry = {"hooks": [{"type": "command", "command": command}]}
    if matcher:
        entry["matcher"] = matcher
    entries.append(entry)

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)
    f.write("\n")
print("hooks installed")
PY

python3 -c "import json,sys; json.load(open('$SETTINGS')); print('settings.json is valid')"
echo
echo "Done. Restart Claude Code (or start a new session) for the hooks to load."
echo "The panel opens by itself the first time Claude edits a file inside tmux."
