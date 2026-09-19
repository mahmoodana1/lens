package capture

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ErrNoPatch means the payload carried no structured patch. Some edit tools
// report no diff at all; those edits are skipped rather than treated as errors.
var ErrNoPatch = errors.New("capture: payload has no structuredPatch")

// postToolUse mirrors the subset of the PostToolUse payload we rely on. The
// shape was captured from a live session; see the design doc.
type postToolUse struct {
	SessionID string `json:"session_id"`
	PromptID  string `json:"prompt_id"`
	CWD       string `json:"cwd"`
	ToolName  string `json:"tool_name"`

	ToolInput struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	} `json:"tool_input"`

	ToolResponse struct {
		FilePath        string  `json:"filePath"`
		Type            string  `json:"type"`
		Content         string  `json:"content"`
		OriginalFile    *string `json:"originalFile"`
		StructuredPatch []Hunk  `json:"structuredPatch"`
	} `json:"tool_response"`
}

// ParsePostToolUse converts a raw PostToolUse payload into a Record.
// It returns ErrNoPatch when the payload describes no diff.
func ParsePostToolUse(raw []byte) (*Record, error) {
	var p postToolUse
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("capture: unmarshal payload: %w", err)
	}

	// Creating a file reports an empty patch — there is nothing to diff against.
	// Synthesize one so new files are readable in the panel like any other edit.
	if len(p.ToolResponse.StructuredPatch) == 0 {
		content := p.ToolResponse.Content
		if content == "" {
			content = p.ToolInput.Content
		}
		if p.ToolResponse.Type != "create" || content == "" {
			return nil, ErrNoPatch
		}
		p.ToolResponse.StructuredPatch = []Hunk{wholeFileHunk(content)}
	}

	path := p.ToolResponse.FilePath
	if path == "" {
		path = p.ToolInput.FilePath
	}

	kind := p.ToolResponse.Type
	if kind == "" {
		// Older or unfamiliar payloads: infer from whether prior content exists.
		if p.ToolResponse.OriginalFile == nil {
			kind = "create"
		} else {
			kind = "update"
		}
	}

	hunks := p.ToolResponse.StructuredPatch
	var original string
	if p.ToolResponse.OriginalFile != nil {
		original = *p.ToolResponse.OriginalFile
	}
	added, removed := 0, 0
	for i := range hunks {
		SliceContext(original, &hunks[i], MaxContext)
		for _, ln := range hunks[i].Lines {
			switch {
			case strings.HasPrefix(ln, "+"):
				added++
			case strings.HasPrefix(ln, "-"):
				removed++
			}
		}
	}

	return &Record{
		SessionID: p.SessionID,
		CWD:       p.CWD,
		Event: Event{
			Time:     time.Now(),
			Tool:     p.ToolName,
			Path:     path,
			Rel:      RelativeTo(p.CWD, path),
			Kind:     kind,
			PromptID: p.PromptID,
			Hunks:    hunks,
			Added:    added,
			Removed:  removed,
		},
	}, nil
}

// RelativeTo shortens an absolute path against the project root, falling back
// to the original path when the file lives outside it. The root must be the
// session's, not the agent's current directory: a path measured from a moving
// point names the same file two different ways.
func RelativeTo(root, path string) string {
	if root == "" || path == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
