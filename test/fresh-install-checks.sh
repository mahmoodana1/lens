#!/usr/bin/env bash
# The fresh-install checks, run INSIDE a throwaway container by
# test/fresh-install.sh. Nothing here touches the host: HOME is made here, the
# repo is mounted read-only, and the capture logs live under this HOME.
#
# Usage: fresh-install-checks.sh source|binary
#
#   source  build and install the way ./install.sh does (needs Go)
#   binary  install a prebuilt binary the way the README's release path does
#
# It keeps going after a failure so one run reports everything that is wrong,
# and exits non-zero if anything did.

set -uo pipefail

mode="${1:?usage: fresh-install-checks.sh source|binary}"

# Where things are. In a container these are the mounts; run on a real machine
# by test/fresh-install.sh --local, they are pointed somewhere harmless.
REPO="${LENS_TEST_REPO:-/repo}"
ART="${LENS_TEST_ART:-/artifacts}"

# A tmux server of our own, on its own socket. Never the reader's: these checks
# start and stop a server, and doing that on the default socket would take down
# the session they are being run from.
TM=(tmux -L lens-fresh-test)

pass=0 fail=0
ok()   { printf '  \033[32mok\033[0m    %s\n' "$1"; pass=$((pass + 1)); }
bad()  { printf '  \033[31mFAIL\033[0m  %s\n' "$1"; shift; for l in "$@"; do printf '        %s\n' "$l"; done; fail=$((fail + 1)); }
head2() { printf '\n\033[1m%s\033[0m\n' "$1"; }

tally() { printf '\n\033[1m%s: %d passed, %d failed\033[0m\n' "$mode" "$pass" "$fail"; }

# stop is for a failure that makes every later check meaningless. Reporting
# those anyway buries the one thing that went wrong, and any that happen to
# pass without the binary even existing pass for no reason at all.
stop() {
  printf '  \033[31m----\033[0m  giving up here: %s\n' "$1"
  printf '        every later check needs a working install, so none were run.\n'
  tally
  exit 1
}

# want compares a value against what it should be, and shows both when it is not.
want() {
  local desc=$1 got=$2 expect=$3
  if [[ "$got" == "$expect" ]]; then ok "$desc"; else bad "$desc" "got:  $got" "want: $expect"; fi
}

# says checks that output mentions something, which is how the human-facing
# messages are covered: a fresh installer's error is only useful if it is read.
says() {
  local desc=$1 haystack=$2 needle=$3
  if [[ "$haystack" == *"$needle"* ]]; then ok "$desc"; else bad "$desc" "these words are missing: $needle" "in: ${haystack:0:300}"; fi
}

# ── a home with nothing in it ────────────────────────────────────────────────
# This is the whole point: no ~/.claude, no ~/.local, no settings, no logs.
export HOME="${LENS_TEST_HOME:-/home/tester}"
case "$HOME" in
  /home/tester|*lens-fresh*) : ;;
  *) echo "fresh-install-checks: refusing to wipe $HOME; it is not one of ours" >&2; exit 2 ;;
esac
rm -rf "$HOME"
mkdir -p "$HOME"
unset XDG_STATE_HOME
cd "$HOME" || exit 1

BIN="$HOME/.local/bin/lens"
SETTINGS="$HOME/.claude/settings.json"
STATE="$HOME/.local/state/lens"

# lens_hooks counts the hook entries that are lens's own.
lens_hooks() {
  python3 - "$1" <<'PY'
import json, sys
try:
    d = json.load(open(sys.argv[1]))
except Exception:
    print("unreadable"); raise SystemExit(0)
n = 0
for groups in (d.get("hooks") or {}).values():
    for g in groups:
        for h in g.get("hooks", []):
            if "lens hook " in h.get("command", ""):
                n += 1
print(n)
PY
}

# lens_binaries is every distinct lens the hooks would run, so an install that
# wired the wrong path is visible.
lens_binaries() {
  python3 - "$1" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
out = set()
for groups in (d.get("hooks") or {}).values():
    for g in groups:
        for h in g.get("hooks", []):
            c = h.get("command", "")
            if "lens hook " in c:
                out.add(c.split(" hook ")[0])
print(",".join(sorted(out)))
PY
}

# json_has answers a question about the settings file from a python expression,
# used to prove another tool's hooks and settings survived.
json_has() {
  python3 - "$1" "$2" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
print("yes" if eval(sys.argv[2], {"d": d, "json": json}) else "no")
PY
}

foreign='d.get("model") == "opus" and any("other-tool" in h.get("command","") for g in d["hooks"]["PreToolUse"] for h in g["hooks"])'

# ── 1. it refuses before it writes ───────────────────────────────────────────
# A friend without Claude Code installed should be told so, and should not be
# left with half an install.
if [[ "$mode" == source ]]; then
  head2 "1. refuses a machine without Claude Code, and writes nothing"
  out=$(cd "$REPO" && ./install.sh 2>&1); rc=$?
  [[ $rc -ne 0 ]] && ok "install.sh fails when ~/.claude is missing" || bad "install.sh should fail when ~/.claude is missing" "it exited 0"
  says "and says what to do about it" "$out" ".claude"
  [[ ! -e "$BIN" ]] && ok "no binary was left behind" || bad "a failed install left $BIN behind"
  [[ ! -e "$HOME/.claude" ]] && ok "it did not invent ~/.claude" || bad "it created ~/.claude itself"
fi

# ── 2. install into a machine that has Claude Code ───────────────────────────
head2 "2. installs, and leaves another tool's settings alone"

mkdir -p "$HOME/.claude"
# Somebody else's hook, and a setting that has nothing to do with hooks. Both
# have to survive: this file is not lens's to own.
cat >"$SETTINGS" <<'JSON'
{
  "model": "opus",
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "/usr/bin/other-tool guard"}]}
    ]
  }
}
JSON
cp "$SETTINGS" "$HOME/settings-before.json"

if [[ "$mode" == source ]]; then
  out=$(cd "$REPO" && ./install.sh 2>&1); rc=$?
else
  # The README's release path: drop the binary in and let it wire itself.
  mkdir -p "$HOME/.local/bin"
  install -m 0755 "$ART/lens" "$BIN"
  out=$("$BIN" hooks install --settings "$SETTINGS" 2>&1); rc=$?
fi
[[ $rc -eq 0 ]] && ok "the install succeeds" || bad "the install failed (exit $rc)" "$out"
if [[ -x "$BIN" ]]; then
  ok "$BIN is there and executable"
else
  bad "$BIN is missing"
  stop "the install produced no usable binary"
fi
"$BIN" --version >/dev/null 2>&1 || stop "$BIN will not run on this machine"

want "all five hooks are wired" "$(lens_hooks "$SETTINGS")" "5"
want "they run the lens that was just installed" "$(lens_binaries "$SETTINGS")" "$BIN"
want "the other tool's hook survived, and so did its settings" "$(json_has "$SETTINGS" "$foreign")" "yes"

# The settings file belongs to the reader, not to lens. Whatever was there
# before has to be recoverable, by either install path.
if [[ -f "$SETTINGS.lens-backup" ]]; then
  ok "the settings file as it was found was kept next to it"
  if diff -q "$HOME/settings-before.json" "$SETTINGS.lens-backup" >/dev/null 2>&1; then
    ok "and the copy is exactly what was there before"
  else
    bad "the backup is not what was there before" "$(diff "$HOME/settings-before.json" "$SETTINGS.lens-backup" | head -5)"
  fi
else
  bad "no copy of the original settings was kept" "expected $SETTINGS.lens-backup"
fi

# The installer's own account of what it did. A reader who is about to let a
# tool edit their Claude Code settings should be told the three paths it
# touches, without having to read the source.
head2 "2b. says what it changed"
says "it names the settings file it edited"  "$out" "$SETTINGS"
says "it names the hooks it added"           "$out" "PostToolUse"
says "it names the binary they run"          "$out" "$BIN"
says "it says the rest of the file was left alone" "$out" "left as it was"
says "it names where captured changes go"    "$out" "$STATE"
says "it says your code is only read"        "$out" "never written"
says "it says nothing is sent anywhere"      "$out" "no network connections"
if [[ "$mode" == source ]]; then
  says "and the installer said so before it wrote anything" "$out" "changes three things on this machine"
fi

# One copy, not a dated pile: ~/.claude should not silently fill up with
# settings.json.bak-20260101120000 every time a reader upgrades.
strays=$(find "$HOME/.claude" -name 'settings.json.bak-*' | wc -l)
want "no dated backup files were left in ~/.claude" "$strays" "0"

# ── 3. running it again is an upgrade, not a second copy ─────────────────────
head2 "3. installing again upgrades rather than duplicating"
if [[ "$mode" == source ]]; then
  (cd "$REPO" && ./install.sh) >/dev/null 2>&1
else
  "$BIN" hooks install --settings "$SETTINGS" >/dev/null 2>&1
fi
want "still exactly five hooks" "$(lens_hooks "$SETTINGS")" "5"
want "and the other tool is still there" "$(json_has "$SETTINGS" "$foreign")" "yes"

# ── 4. it can say what it is ─────────────────────────────────────────────────
head2 "4. reports its own version"
ver=$("$BIN" --version 2>&1)
[[ -n "$ver" ]] && ok "lens --version says: $ver" || bad "lens --version printed nothing"
if [[ "$mode" == binary && -f $ART/version ]]; then
  says "the version was stamped into the binary" "$ver" "$(cat $ART/version)"
fi

# ── 5. the doctor ────────────────────────────────────────────────────────────
head2 "5. the doctor tells the truth about this machine"
out=$("$BIN" doctor --project "$HOME" 2>&1); rc=$?
says "it reports the hooks it found" "$out" "hooks"
[[ $rc -eq 0 ]] && ok "outside tmux it warns without failing (exit 0)" || bad "it exited $rc outside tmux; a warning is not a failure" "$out"

# With no tmux on PATH the panel genuinely cannot open, so this one must fail.
mkdir -p "$HOME/emptybin"
out=$(PATH="$HOME/emptybin" "$BIN" doctor --project "$HOME" 2>&1); rc=$?
[[ $rc -eq 1 ]] && ok "with no tmux at all it fails (exit 1)" || bad "no tmux should be a failure, got exit $rc" "$out"
says "and says tmux is the problem" "$out" "tmux"

# ── 6. capture, end to end, through the real hooks ───────────────────────────
# The payloads below are the shape Claude Code actually sends, taken from
# internal/capture/testdata/probe-payloads.jsonl.
head2 "6. records edits when the hooks fire"

PROJ="$HOME/work"
mkdir -p "$PROJ/utils" "$PROJ/.secret"
printf 'line one\nline two\nline three\n' >"$PROJ/hello.py"
printf 'def greet(name):\n    return "hi " + name\n' >"$PROJ/utils/greeting.py"
printf 'token = "hunter2"\n' >"$PROJ/.secret/token.txt"
printf 'API_KEY=sekrit\n' >"$PROJ/.env"

SID="fresh-install-e2e"
hook() { "$BIN" hook "$1" >/dev/null 2>&1; }

hook session-start <<JSON
{"session_id":"$SID","cwd":"$PROJ","hook_event_name":"SessionStart","source":"startup"}
JSON

hook prompt <<JSON
{"session_id":"$SID","cwd":"$PROJ","prompt_id":"p1","hook_event_name":"UserPromptSubmit","prompt":"change line two of hello.py"}
JSON

# An Edit: the patch comes in the payload, so nothing has to be re-diffed.
printf 'line one\nline TWO CHANGED\nline three\n' >"$PROJ/hello.py"
hook post-tool-use <<JSON
{"session_id":"$SID","cwd":"$PROJ","prompt_id":"p1","hook_event_name":"PostToolUse","tool_name":"Edit",
 "tool_input":{"file_path":"$PROJ/hello.py","old_string":"line two","new_string":"line TWO CHANGED"},
 "tool_response":{"filePath":"$PROJ/hello.py","originalFile":"line one\nline two\nline three\n",
 "structuredPatch":[{"oldStart":1,"oldLines":3,"newStart":1,"newLines":3,
 "lines":[" line one","-line two","+line TWO CHANGED"," line three"]}],"userModified":false}}
JSON

# A Bash write: reports nothing about what it touched, so it has to be found by
# looking at the project. Hidden files are changed at the same time and must be
# ignored entirely.
sed -i 's/hi /hello, /' "$PROJ/utils/greeting.py"
printf 'token = "rotated"\n' >"$PROJ/.secret/token.txt"
printf 'API_KEY=rotated\n' >"$PROJ/.env"
hook post-tool-use <<JSON
{"session_id":"$SID","cwd":"$PROJ","prompt_id":"p1","hook_event_name":"PostToolUse","tool_name":"Bash",
 "tool_input":{"command":"sed -i 's/hi /hello, /' utils/greeting.py"},"tool_response":{"stdout":"","stderr":""}}
JSON

hook stop <<JSON
{"session_id":"$SID","cwd":"$PROJ","hook_event_name":"Stop"}
JSON

log="$STATE/$SID/events.jsonl"
if [[ -f "$log" ]]; then
  ok "the capture log was written to ~/.local/state/lens with no XDG_STATE_HOME set"
  body=$(cat "$log")
  says "the Edit was recorded" "$body" "hello.py"
  says "the line it changed is in the patch" "$body" "line TWO CHANGED"
  says "the shell write was found by watching the project" "$body" "greeting.py"
  says "the edit is joined to the prompt that caused it" "$body" '"prompt_id":"p1"'
  says "and the prompt text was recorded" "$(cat "$STATE/$SID/prompts.jsonl" 2>&1)" "change line two"
  # The security-relevant one: nothing hidden, by either route.
  [[ "$body" != *".secret"* ]] && ok "the hidden directory was never recorded" || bad ".secret leaked into the capture log"
  [[ "$body" != *".env"* ]]    && ok "the dotfile was never recorded" || bad ".env leaked into the capture log"
  [[ "$body" != *"hunter2"* && "$body" != *"sekrit"* && "$body" != *"rotated"* ]] \
    && ok "no hidden file's contents were recorded" || bad "a hidden file's contents leaked into the capture log"
else
  bad "nothing was captured: $log does not exist" "$(ls -R "$STATE" 2>&1 | head -5)"
fi

# ── 7. the panel itself, drawn in a real tmux ───────────────────────────────
head2 "7. the panel opens and draws the changes"
if command -v tmux >/dev/null 2>&1; then
  "${TM[@]}" kill-server >/dev/null 2>&1
  "${TM[@]}" new-session -d -s lens -x 200 -y 50 "cd '$PROJ' && '$BIN' --project '$PROJ' 2>$HOME/panel.err"
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    sleep 0.4
    screen=$("${TM[@]}" capture-pane -p -t lens 2>/dev/null)
    [[ "$screen" == *"hello.py"* ]] && break
  done
  screen=$("${TM[@]}" capture-pane -p -t lens 2>/dev/null)
  says "the changed file is on screen" "$screen" "hello.py"
  says "so is the one the shell command wrote" "$screen" "greeting.py"
  says "and the diff beside it" "$screen" "line TWO CHANGED"
  [[ "$screen" != *".secret"* && "$screen" != *".env"* ]] \
    && ok "nothing hidden is on screen either" || bad "a hidden file is drawn in the panel"

  # It opens ready to search, so a bare q would be typed into the filter. esc
  # leaves the filter first; then q closes the panel.
  "${TM[@]}" send-keys -t lens Escape >/dev/null 2>&1
  sleep 0.3
  "${TM[@]}" send-keys -t lens q >/dev/null 2>&1
  sleep 1
  if "${TM[@]}" has-session -t lens 2>/dev/null; then
    bad "q did not close the panel" "$("${TM[@]}" capture-pane -p -t lens 2>/dev/null | tail -3)"
  else
    ok "q closes it"
  fi
  err=$(cat "$HOME/panel.err" 2>/dev/null)
  [[ -z "$err" ]] && ok "it wrote nothing to stderr" || bad "the panel complained on stderr" "$err"
  "${TM[@]}" kill-server >/dev/null 2>&1
else
  bad "tmux is not installed in this container, so the panel was never drawn"
fi

# ── 8. the automatic popup can be muted per project ─────────────────────────
head2 "8. auto on/off, per project"
want "on by default"          "$("$BIN" auto status --project "$PROJ" 2>&1 | grep -c on)"  "1"
"$BIN" auto off --project "$PROJ" >/dev/null 2>&1
says "off after turning it off"    "$("$BIN" auto status --project "$PROJ" 2>&1)" "off"
says "and a relative path means the same project" "$(cd "$PROJ" && "$BIN" auto status --project . 2>&1)" "off"
says "the muted project is warned about by the doctor" "$("$BIN" doctor --project "$PROJ" 2>&1)" "auto-open"
[[ $("$BIN" doctor --project "$PROJ" >/dev/null 2>&1; echo $?) -eq 0 ]] \
  && ok "a muted project does not fail the doctor" || bad "a muted project should not fail the doctor"
"$BIN" auto toggle --project "$PROJ" >/dev/null 2>&1
says "toggle brings it back"       "$("$BIN" auto status --project "$PROJ" 2>&1)" "on"

# ── 9. XDG_STATE_HOME is honoured when it is set ────────────────────────────
head2 "9. honours XDG_STATE_HOME"
XDG_STATE_HOME="$HOME/xdg" "$BIN" hook session-start >/dev/null 2>&1 <<JSON
{"session_id":"xdg-session","cwd":"$PROJ","hook_event_name":"SessionStart","source":"startup"}
JSON
[[ -d "$HOME/xdg/lens/xdg-session" ]] && ok "the session landed under \$XDG_STATE_HOME" \
  || bad "XDG_STATE_HOME was ignored" "$(ls "$HOME/xdg" 2>&1)"

# ── 10. uninstall puts the machine back ─────────────────────────────────────
head2 "10. uninstall leaves the settings as it found them"
if [[ "$mode" == source ]]; then
  out=$(cd "$REPO" && ./uninstall.sh 2>&1); rc=$?
else
  out=$("$BIN" hooks remove --settings "$SETTINGS" 2>&1); rc=$?
  rm -f "$BIN"
fi
[[ $rc -eq 0 ]] && ok "uninstall succeeds" || bad "uninstall failed (exit $rc)" "$out"
want "no lens hooks are left"  "$(lens_hooks "$SETTINGS")" "0"
want "the other tool's hook is still there" "$(json_has "$SETTINGS" "$foreign")" "yes"
[[ ! -e "$BIN" ]] && ok "the binary is gone" || bad "$BIN is still there"
[[ -f "$log" ]] && ok "the capture logs were kept, as it promises" || bad "uninstall deleted the capture logs"

# ── the tally ───────────────────────────────────────────────────────────────
tally
[[ $fail -eq 0 ]]
