# lens

A panel that shows what Claude Code changes in your code, as it happens, so you
can read it rather than scroll past it.

It opens by itself in a tmux pane the first time Claude edits a file, lists
every change on the left and the diff on the right, and moves under vim motions.
Each diff carries the prompt that caused it, so you see the request and the code
that satisfied it side by side.

## Install

```sh
./install.sh
```

This builds `~/.local/bin/lens` and adds three hooks to `~/.claude/settings.json`
(backing it up first, and preserving anything already there). Restart Claude Code
to load them. The panel then appears on its own.

## Keys

| Key | Action |
|---|---|
| `j` / `k` | move down / up |
| `ctrl-d` / `ctrl-u` | half page |
| `gg` / `G` | first / last |
| `n` / `N` | next / previous hunk |
| `J` / `K` | next / previous file |
| `t` | toggle timeline ⇄ grouped by file |
| `+` / `-` | more / less surrounding context |
| `Tab` | focus the list or the diff |
| `Enter` | open the file in `$EDITOR` at the change |
| `?` | help |
| `q` | quit, clearing this session's log |

## How it works

Claude Code's `PostToolUse` hook already carries a `structuredPatch` and the
file's prior contents, so `lens` never snapshots or re-diffs anything. Each edit
is appended to `$XDG_STATE_HOME/lens/<session>/events.jsonl`; the panel tails it.
`UserPromptSubmit` records prompts, joined to edits by `prompt_id`.

Storage is per-session and ephemeral: quitting the panel deletes the log, and
sessions left behind are swept after 24 hours.

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/` for
the implementation plan.
