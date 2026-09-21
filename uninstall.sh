#!/usr/bin/env bash
# Take lens out of Claude Code's hooks and remove the binary.
#
# Your capture logs are left alone: they are under
# ${XDG_STATE_HOME:-~/.local/state}/lens and are yours to delete.
set -euo pipefail

BIN="${HOME}/.local/bin/lens"
SETTINGS="${HOME}/.claude/settings.json"

if [[ -f "$SETTINGS" ]]; then
  backup="${SETTINGS}.bak-$(date +%Y%m%d%H%M%S)"
  cp "$SETTINGS" "$backup"
  echo "backed up settings to $backup"

  # lens removes its own hooks, so other tools' entries are never touched.
  if [[ -x "$BIN" ]]; then
    "$BIN" hooks remove --settings "$SETTINGS" || {
      cp "$backup" "$SETTINGS"
      echo "lens: could not edit the hooks; your settings have been put back" >&2
      exit 1
    }
  else
    echo "lens: $BIN is already gone, so the hooks were left as they are." >&2
    echo "     Remove the lines mentioning 'lens hook' from $SETTINGS by hand." >&2
  fi
fi

if [[ -e "$BIN" ]]; then
  rm -f "$BIN"
  echo "removed $BIN"
fi

state="${XDG_STATE_HOME:-${HOME}/.local/state}/lens"
echo
echo "Done. Restart Claude Code for the hooks to stop running."
if [[ -d "$state" ]]; then
  echo "Your capture logs are still in $state ($(du -sh "$state" 2>/dev/null | cut -f1))."
  echo "Delete them with:  rm -rf $state"
fi
