// Package conversation parses codex CLI 0.130.0 rollout JSONL files into
// the dashboard's domain.ConversationEntry shape.
//
// Codex emits two flavors of conversation lines per turn:
//
//  1. response_item entries — the raw OpenAI Responses API turn payload
//     (role: developer/user/assistant; content arrays with input_text,
//     output_text, reasoning, function_call, etc.). These carry envelope
//     text like <environment_context>...</environment_context>.
//
//  2. event_msg entries with payload.type "user_message" / "agent_message"
//     — codex's own clean view of each turn's user-visible message.
//
// For dashboard rendering we use (2): the text is already stripped of
// system envelope content and matches what the user sees in the codex
// TUI. Tool calls, reasoning, and other metadata are out of scope here —
// they belong on a separate activity log surface (follow-up).
package conversation

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// Read parses path as a codex rollout JSONL and returns the conversation
// entries in original order. A missing file returns an empty slice and
// nil error — codex sessions may appear in the index before their
// rollout is flushed. Malformed or unrecognized lines are skipped.
//
// If limit > 0, only the most recent limit entries are returned (with
// the oldest of those first, matching domain.ConversationEntry ordering
// elsewhere). limit == 0 returns everything.
//
// Read is a thin wrapper for ReadIncremental(path, limit, nil, 0) — use
// ReadIncremental directly when you want to skip re-decoding bytes you've
// already seen on a previous call to the same path.
func Read(path string, limit int) ([]domain.ConversationEntry, error) {
	entries, _, err := ReadIncremental(path, limit, nil, 0)
	return entries, err
}

// ReadIncremental returns the codex rollout's conversation entries,
// resuming from prevOffset when prev is non-nil. Codex rollouts are
// append-only per session, so seeking past previously-decoded bytes is
// safe: the caller passes back the previous entries and the byte offset
// returned by the prior call, and only new bytes are decoded.
//
// On a file shrink (truncation/rewrite) the function falls back to a
// full re-read from offset 0. Missing files return (nil, 0, nil) to
// preserve Read's contract.
func ReadIncremental(path string, limit int, prev []domain.ConversationEntry, prevOffset int64) ([]domain.ConversationEntry, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	fileSize := stat.Size()

	// File shrank (rotation/rewrite) or offset is corrupt — full re-read.
	if prevOffset > fileSize || prevOffset < 0 {
		prevOffset = 0
		prev = nil
	}

	// Nothing new since the last call — return prev as-is.
	if prevOffset > 0 && prevOffset == fileSize && prev != nil {
		return prev, prevOffset, nil
	}

	if prevOffset > 0 {
		if _, err := f.Seek(prevOffset, 0); err != nil {
			// Seek failed — fall back to a full re-read.
			prevOffset = 0
			prev = nil
			if _, err := f.Seek(0, 0); err != nil {
				return nil, 0, err
			}
		}
	}

	type payload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	type line struct {
		Timestamp string  `json:"timestamp"`
		Type      string  `json:"type"`
		Payload   payload `json:"payload"`
	}

	var out []domain.ConversationEntry
	if prevOffset > 0 && prev != nil {
		out = make([]domain.ConversationEntry, len(prev))
		copy(out, prev)
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 4096), 8*1024*1024)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var l line
		if json.Unmarshal(raw, &l) != nil {
			continue
		}
		if l.Type != "event_msg" {
			continue
		}
		var role string
		switch l.Payload.Type {
		case "user_message":
			role = "human"
		case "agent_message":
			role = "assistant"
		default:
			continue
		}
		if l.Payload.Message == "" {
			continue
		}
		out = append(out, domain.ConversationEntry{
			Role:      role,
			Content:   l.Payload.Message,
			Timestamp: l.Timestamp,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, fileSize, nil
}
