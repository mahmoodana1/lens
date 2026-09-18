# lens

A panel that shows what Claude Code changes in your code, as it happens, so you
can read it rather than scroll past it.

It pops up by itself in a tmux popup when Claude finishes a turn that changed
files — a turn that only answered a question leaves it closed, lists every change on the left and the diff on the right, and moves under
vim motions. A popup holds the keyboard, so it is scrollable the moment it
appears — there is no pane to switch to first. Each diff carries the prompt that
caused it, so you see the request and the code that satisfied it side by side.

It opens ready to search: type to filter the list by file name, `⏎` to keep the
filter, `esc` to clear it. Edits you have not looked at carry a `●` and stay
bright; once you land on one it dims. Read state is kept per session, so closing
the popup and reopening it does not present everything as new again.

The diff is coloured with the same tokyonight-moon palette as the editor, syntax
and all, including added and removed lines — what marks those is a wash of green
or red behind the code rather than flattening it to one colour. `tab` gives the
diff a cursor of its own; `ctrl-e`/`ctrl-y` and `ctrl-f`/`ctrl-b` scroll it from
either pane, so a long hunk never needs a focus change to read.

Edits are recorded silently while Claude works and shown once, at the end:
opening a popup mid-turn would take the session away from you while you are
still talking to it.

## Install

```sh
./install.sh
```

This builds `~/.local/bin/lens` and adds five hooks to `~/.claude/settings.json`
(backing it up first, and preserving anything already there). Restart Claude Code
to load them. The panel then appears on its own.

To bring it back after you have closed it, bind a key to `lens popup`:

```tmux
bind e run-shell "~/.local/bin/lens popup"
bind E run-shell "cd '#{pane_current_path}' && ~/.local/bin/lens auto toggle"
```

The second binding turns the automatic popup off and on **for the project you
are in** — `lens auto [--project DIR] [on|off|toggle|status]`, defaulting to the
working directory. That is why the binding runs it from the pane's own path: a
key binding's command otherwise inherits the tmux server's directory.

Capture keeps running while it is off, so the next `lens popup` still shows
everything that happened, and turning it back on catches up rather than skipping
what it missed. Muted projects are listed in
`$XDG_STATE_HOME/lens/auto-open-off`, one path per line.

## Keys

| Key | Action |
|---|---|
| `j` / `k` | move down / up |
| `/` | filter by file name; `⏎` keeps it, `esc` clears it |
| `ctrl-j` / `ctrl-k` | move through the results while the prompt is open |
| `ctrl-e` / `ctrl-y` | scroll the diff a line, from either pane |
| `ctrl-f` / `ctrl-b` | scroll the diff a page, from either pane |
| `ctrl-d` / `ctrl-u` | half page |
| `gg` / `G` | first / last |
| `n` / `N` | next / previous hunk |
| `J` / `K` | next / previous file |
| `t` | toggle timeline ⇄ grouped by file |
| `+` / `-` | more / less surrounding context |
| `Tab` | focus the list or the diff; the diff gets its own cursor |
| `●` | marks an edit you have not looked at yet |
| `?` | help |
| `q` | close the popup (the capture stays; reopen with your `lens popup` key) |

## How it works

Claude Code's `PostToolUse` hook already carries a `structuredPatch` and the
file's prior contents, so edits made with the Edit and Write tools need no
snapshotting or re-diffing.

Files written by shell commands — heredocs, `sed -i`, code generators — report
nothing about what they touched, so those are found by watching the project:
`SessionStart` records what it looked like beforehand, and after each Bash call
a stat-walk finds what moved and diffs it. Build output, dependencies, binaries
and large files are skipped; `$HOME` and `/` are never walked.

Each change is appended to `$XDG_STATE_HOME/lens/<session>/events.jsonl`; the
panel tails it. `UserPromptSubmit` records prompts, joined to edits by
`prompt_id`.

The panel is opened for edits, not for turns: `announced` records the highest
edit it has been opened for, and a turn whose edits are all below that opens
nothing.

Storage is per-session and ephemeral. Closing the popup leaves the log alone so
you can reopen it, and `SessionEnd` only marks the session finished — the panel
stays readable afterwards. Logs are swept 24 hours after their last change.

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/` for
the implementation plan.
