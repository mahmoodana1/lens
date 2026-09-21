#!/usr/bin/env bash
# Build lens and wire it into Claude Code's hooks.
#
# Existing settings are backed up first and merged into, never replaced.
# Run it again to upgrade: lens's own hook entries are replaced, and anything
# else in the file is left alone.
set -euo pipefail

BIN="${HOME}/.local/bin/lens"
SETTINGS="${HOME}/.claude/settings.json"
GO_MIN="1.25"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

die() {
  echo "lens: $1" >&2
  shift
  for line in "$@"; do echo "     $line" >&2; done
  exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }

# Everything is checked before anything is written, so a missing tool leaves
# the machine as it was rather than half-installed.

have go || die "needs Go $GO_MIN or newer to build, and go is not installed." \
  "arch:    sudo pacman -S go" \
  "debian:  sudo apt install golang" \
  "macos:   brew install go" \
  "or take a prebuilt binary instead:" \
  "  https://github.com/mahmoodana1/lens/releases"

# Sort -V puts the lower version first; if that is not the minimum, go is older.
go_version="$(go env GOVERSION 2>/dev/null || echo unknown)"
go_version="${go_version#go}"
if [[ "$go_version" != unknown ]]; then
  lowest="$(printf '%s\n%s\n' "$GO_MIN" "$go_version" | sort -V | head -1)"
  [[ "$lowest" == "$GO_MIN" ]] || die \
    "needs Go $GO_MIN or newer; this is go$go_version." \
    "upgrade Go, or take a prebuilt binary:" \
    "  https://github.com/mahmoodana1/lens/releases"
fi

have tmux || die "needs tmux: the panel is a tmux popup over your session." \
  "arch:    sudo pacman -S tmux" \
  "debian:  sudo apt install tmux" \
  "macos:   brew install tmux"

[[ -d "${HOME}/.claude" ]] || die \
  "cannot find ~/.claude, so Claude Code does not look installed." \
  "Install Claude Code first, run it once, then try again."

cat <<NOTICE
lens changes three things on this machine, and nothing else:

  $BIN
      a binary built from this checkout

  $SETTINGS
      five Claude Code hooks added. Other tools' hooks and your own
      settings in that file are left alone, and a copy of it as it is
      now is kept beside it.

  ${XDG_STATE_HOME:-${HOME}/.local/state}/lens
      what Claude changes, and the prompts that caused it, so the panel
      can show them. Swept 24 hours after a session stops changing.

Your code is only ever read, never written. Nothing hidden — anything under
a dot, like .git or .env — is recorded at all. lens makes no network
connections. To undo all of it: ./uninstall.sh

NOTICE

echo "building $BIN"
mkdir -p "$(dirname "$BIN")"
# -buildvcs=false: the stamped commit is never read back, and asking git for it
# fails outright on a clone owned by another user — a repo cloned as root, a
# shared machine, a mounted volume in a container. That would stop the build
# with "error obtaining VCS status" and nothing lost by skipping it.
(cd "$here" && go build -buildvcs=false -o "$BIN" .)

restore=""
if [[ -f "$SETTINGS" ]]; then
  restore="$(mktemp)"
  cp "$SETTINGS" "$restore"
fi

# lens edits the settings itself: the hooks it needs are its own business, it
# reports exactly what it changed, and it can put them back the same way when
# uninstalling.
if ! "$BIN" hooks install --settings "$SETTINGS"; then
  [[ -n "$restore" ]] && cp "$restore" "$SETTINGS"
  rm -f "$restore"
  die "could not install the hooks; your settings have been put back."
fi
rm -f "$restore"

echo "The panel pops up by itself when Claude finishes a turn that changed files."
echo
echo "Add these to ~/.tmux.conf to reopen it and to mute a project:"
echo "  bind e run-shell \"cd '#{pane_current_path}' && $BIN popup\""
echo "  bind E run-shell \"cd '#{pane_current_path}' && $BIN auto toggle\""
echo
echo "If anything looks wrong, run:  lens doctor"

if ! [[ ":$PATH:" == *":${HOME}/.local/bin:"* ]]; then
  echo
  echo "Note: ~/.local/bin is not on your PATH, so \`lens\` will not be found by"
  echo "name. The hooks use the full path and work regardless. To fix it, add:"
  echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
fi
