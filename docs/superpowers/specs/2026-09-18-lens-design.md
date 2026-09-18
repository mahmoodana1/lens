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
2. **Panel auto-spawns as a tmux popup** when Claude finishes a turn that
   changed files. A popup holds the client's keyboard, so the panel is
   scrollable the moment it appears, with no pane to switch to first — and
   waiting for the end of the turn is what keeps it from taking the keyboard
   while Claude is still working. A key bound to `lens popup` reopens it.
   Outside tmux, capture still happens silently and the panel can be opened
   by hand.
3. **Unread edits are visible at a glance.** Landing on an edit reads it — the
   panel is for reading, so looking at one is reading it — and the read set is
   persisted per session in `seen`, since the popup is opened and closed
   repeatedly and read state that reset each time would be worthless.
4. **`/` filters the list by file name**, fuzzy and as you type. The order stays
   the view's own, so narrowing a timeline does not reshuffle it; the ranking
   decides where the cursor lands instead.
5. **The diff wears the editor's colours.** tokyonight-moon, expressed as a
   chroma style, on every line including added and removed ones; what marks a
   change is a wash of the diff colour behind the code, not flattening it to a
   single green or red. The diff has its own cursor when focused, and scrolls
   from either pane, so reading a long hunk never needs a focus change.
6. **Two views over the same data**, toggled with `t`: a chronological
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

## Capture: changes no payload describes

A tool-based hook only sees what the Edit and Write tools do. In practice a
large share of file changes arrive through the Bash tool — heredocs, `sed -i`,
code generators, formatters — and a `Bash` payload carries a command string and
stdout, nothing about which files moved. A session that wrote an entire project
with `cat > file <<'EOF'` produced prompts and no edits at all.

So lens also watches the project. The `SessionStart` hook records what the
project looked like before Claude touched anything; after each Bash call, a
stat-walk finds files whose mtime or size moved, and their contents are diffed
against what was stored. Only changed files are read.

Bounds that keep this cheap and quiet:

- Roots are the session's working directory, plus directories named by the
  commands themselves — which is how a session started in `$HOME` still finds
  the project it just created. A home directory or `/` is never walked.
- Adding a parent root absorbs roots nested inside it, so no file is walked or
  reported twice.
- Build output, dependency and VCS directories are skipped, as are binaries and
  files over 256 KB.
- A root adopted mid-session reports only files modified since the session
  began, so `cd`-ing into an existing project does not flood the panel.
- Deletions are not reported: there is nothing to read in a file that is gone.

## Architecture

One Go binary, `lens`, wearing two hats:

| Invocation | Role |
|---|---|
| `lens hook session-start` | records the project's pre-session state |
| `lens hook post-tool-use` | appends an event (or scans, for Bash) |
| `lens hook prompt` | records prompt text by `prompt_id` |
| `lens hook stop` | opens the popup, if the turn changed anything |
| `lens hook session-end` | marks the session ended |
| `lens popup` | opens the popup over the current tmux pane |
| `lens` | the TUI, as the popup runs it |

A single static binary means the hooks have no runtime dependency — no shell
parsing, no `jq`, and a fast path for something that runs on every edit.

### Storage

Per session, under `$XDG_STATE_HOME/lens/<session_id>/`:

- `meta.json` — project cwd, start time, ended flag
- `events.jsonl` — one record per edit, append-only
- `prompts.jsonl` — `prompt_id` → prompt text
- `seen` — sequence numbers of edits already read, one per line
- `announced` — the highest edit the panel has been opened for

Beside the session directories, `auto-open-off` lists the projects whose panel
must not open by itself, one path per line.

Append-only JSONL is what makes the live panel simple: the TUI tails the file
and never contends with the writer.

Each event stores the hunks plus a bounded context window (up to 50 lines
either side, sliced from `originalFile` at capture time). Full file copies are
never written, so a long session's log stays small.

### Lifecycle

1. First `post-tool-use` of a session creates the session directory.
2. `Stop` opens the popup over the session's pane, if the turn recorded an edit
   the panel has not already been opened for — the test is against `announced`,
   not against the session having any edits at all, or the first edit of a
   session would reopen the panel at the end of every later turn. Auto-open can
   be turned off per project with `lens auto off` — a repo you want quiet should
   not silence the others — and capture continues regardless, so nothing is lost
   while it is off.
   `display-popup` does not return until the popup is dismissed, so the spawn
   is detached — a hook that waited on it would freeze the session for as long
   as the panel stayed open.

   The pane is located by controlling terminal, not by `$TMUX`. Hook processes
   do not reliably inherit the terminal's environment variables — a session
   that captured edits perfectly still never opened a panel, because `$TMUX`
   was empty in the hook. The controlling terminal does survive, and tmux
   reports which pane owns which tty, so `#{pane_tty}` is matched against it.
   `$TMUX_PANE` is still used as a fast path when present.

   A popup also needs an attached client; with none, tmux refuses it.

   When no pane can be found, the reason is written once per session to
   `debug.log` and to a `pane-unavailable` marker, so an invisible panel is
   never a silent one.
3. The TUI tails `events.jsonl` and re-renders on change.
4. Closing the panel closes the popup and nothing else: the log survives so the
   `lens popup` binding can bring it back. Logs are swept after 24 hours.
5. `SessionEnd` writes an ended marker. The TUI keeps rendering, so the
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
- **A slow hook slows every command.** The Bash path stat-walks the project,
  measured at ~13 ms on a 35-file repo, and reads only files that changed. The
  tool path stays a parse-and-append. Neither blocks on the TUI.
- **A hook that errors could disrupt a session.** It always exits 0.
