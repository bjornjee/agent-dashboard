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
func Read(path string, limit int) ([]domain.ConversationEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

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
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
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
		return nil, err
	}

	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}
