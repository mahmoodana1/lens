# lens — a live panel for reading what Claude changes

**Date:** 2026-09-18
**Status:** approved, in implementation

## Purpose

Every edit Claude Code makes to a file should appear, as it happens, in a
panel the user can read with vim motions. The user's goal is learning: seeing
the request, the code that satisfied it, and the surrounding context tightens
the loop between "Claude wrote something" and "I understand why".

This is a reading tool, not a review tool. It does not approve, reject, or
revert anything.

## Requirements

Settled with the user during brainstorming:

1. **Automatic in every Claude Code session**, in any directory, with no
   per-project setup.
2. **Panel auto-spawns as a tmux pane** on the first edit of a session, and is
   reused afterward. Outside tmux, capture still happens silently and the
   panel can be opened by hand.
3. **Two views over the same data**, toggled with `t`: a chronological
   timeline of edits, and a file-grouped view with cumulative counts.
4. **Learning aids:** syntax highlighting, the prompt that caused each edit,
   and expandable surrounding context.
5. **Current session only.** Nothing persists once the session's panel is
   closed.
6. A `lens` CLI to open the panel manually. No keybindings.

Explicitly out of scope: asking Claude to explain a hunk (considered,
declined — token cost per press), cross-session history, dotfiles tracking.

## Capture: what the hooks actually give us

The design originally assumed a `PreToolUse`/`PostToolUse` pair, with
`PreToolUse` snapshotting each file's bytes so the post-edit state could be
diffed against it. A headless probe (`claude -p` against a throwaway settings
file whose hooks dumped their raw stdin) showed that snapshot to be
unnecessary. `PostToolUse` already carries everything:

```jsonc
{
  "hook_event_name": "PostToolUse",
  "session_id": "...", "prompt_id": "...", "cwd": "/abs/project",
  "tool_name": "Edit",
  "tool_input":  { "file_path": "...", "old_string": "...", "new_string": "..." },
  "tool_response": {
    "filePath": "...",
    "type": "create" | "update",
    "originalFile": "<full prior contents, null on create>",
    "userModified": false,
    "structuredPatch": [
      { "oldStart": 1, "oldLines": 3, "newStart": 1, "newLines": 3,
        "lines": [" line one", "-line two", "+line TWO CHANGED", " line three"] }
    ]
  }
}
```

`structuredPatch` is a standard unified-diff hunk list, computed for us.
`originalFile` supplies the pre-edit text needed to widen context.
`type` distinguishes file creation from modification.

**Consequence:** capture is a single `PostToolUse` hook. This removes the
snapshot directory, the pre/post correlation, and a class of races where an
edit completes before its snapshot lands. The approved approach was the
pre/post pair; this is that approach minus a step that turned out to be dead
weight, verified against real payloads rather than recollection.

The same probe confirmed `prompt_id` is shared between `UserPromptSubmit` and
every `PostToolUse` it causes. That is the join key for the intent line.

## Architecture

One Go binary, `lens`, wearing two hats:

| Invocation | Role |
|---|---|
| `lens hook post-tool-use` | called by the hook; appends an event, spawns the pane if needed |
| `lens hook prompt` | called by the hook; records prompt text by `prompt_id` |
| `lens hook session-end` | called by the hook; marks the session ended |
| `lens` | the TUI |

A single static binary means the hooks have no runtime dependency — no shell
parsing, no `jq`, and a fast path for something that runs on every edit.

### Storage

Per session, under `$XDG_STATE_HOME/lens/<session_id>/`:

- `meta.json` — project cwd, start time, ended flag
- `events.jsonl` — one record per edit, append-only
- `prompts.jsonl` — `prompt_id` → prompt text
- `pane` — the tmux pane id, once spawned (also the spawn lock)

Append-only JSONL is what makes the live panel simple: the TUI tails the file
and never contends with the writer.

Each event stores the hunks plus a bounded context window (up to 50 lines
either side, sliced from `originalFile` at capture time). Full file copies are
never written, so a long session's log stays small.

### Lifecycle

1. First `post-tool-use` of a session creates the session directory.
2. If `$TMUX` is set and `pane` does not yet exist, the hook splits a pane
   running `lens` and records the pane id. The file is created with `O_EXCL`,
   so concurrent edits cannot spawn two panes.
3. The TUI tails `events.jsonl` and re-renders on change.
4. `SessionEnd` writes an ended marker. The TUI keeps rendering, so the
   session can still be read after Claude exits.
5. Quitting the TUI deletes the session directory. A sweep at startup removes
   directories orphaned for more than 24 hours.

### TUI

Bubble Tea and Lipgloss, Chroma for highlighting. Two panes: a list on the
left (timeline or file-grouped), the diff on the right.

Motions: `j`/`k`, `gg`/`G`, `ctrl-d`/`ctrl-u`, `n`/`N` for hunks, `J`/`K` for
files, `t` to toggle view, `+`/`-` to widen and narrow context, `Tab` to move
focus between panes, `Enter` to open the file in `$EDITOR` at the hunk's line,
`?` for help, `q` to quit.

## Testing

The capture layer is pure data transformation and is tested against the real
payloads recorded by the probe, which are committed as fixtures. Parsing,
context slicing, and the timeline/file groupings are unit-tested. The tmux
spawn is tested against a real detached tmux server. The TUI's rendering is
tested through Bubble Tea's test harness at fixed terminal sizes.

## Risks

- **Hook payload shape is not a public contract.** It could change. The
  parser tolerates missing fields and logs anything it cannot understand to a
  debug file rather than failing the session.
- **A slow hook slows every edit.** The hook does bounded work — parse,
  append, return — and never blocks on the TUI.
- **A hook that errors could disrupt a session.** It always exits 0.
