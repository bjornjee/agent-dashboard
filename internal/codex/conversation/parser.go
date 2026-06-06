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

// requestUserInputArgs is the parsed shape of codex's request_user_input
// function_call arguments — a JSON-encoded string carried on the wire as
// payload.arguments. Mirrors the codex CLI 0.130+ schema. Used by
// ReadPendingQuestion to normalize codex's question shape into the
// dashboard's domain.PendingQuestion.
type requestUserInputArgs struct {
	Questions []struct {
		ID       string `json:"id"`
		Question string `json:"question"`
		Header   string `json:"header"`
		Options  []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
	} `json:"questions"`
}

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

	type item struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type payload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Item    item   `json:"item"`
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
		// Plan finalization: codex emits item_completed with item.type
		// "Plan" when an agent commits a <proposed_plan>. Mirrors claude's
		// ExitPlanMode handling — frontend renders a chat-stream "View
		// plan" card at this timeline position.
		if l.Payload.Type == "item_completed" && l.Payload.Item.Type == "Plan" {
			out = append(out, domain.ConversationEntry{
				Role:      "plan-saved",
				Timestamp: l.Timestamp,
			})
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

// ReadPlanContent returns the text of the most recent finalized plan in a
// codex rollout — the item.text of the latest event_msg whose
// payload.type is "item_completed" and payload.item.type is "Plan".
// Missing files and sessions without a plan return "" without error.
func ReadPlanContent(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	type item struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type payload struct {
		Type string `json:"type"`
		Item item   `json:"item"`
	}
	type line struct {
		Type    string  `json:"type"`
		Payload payload `json:"payload"`
	}

	var latest string
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
		if l.Payload.Type != "item_completed" || l.Payload.Item.Type != "Plan" {
			continue
		}
		if l.Payload.Item.Text != "" {
			latest = l.Payload.Item.Text
		}
	}
	return latest
}

// LastPendingBlockingToolCodex returns "question" when the codex rollout
// at path contains an unanswered request_user_input function_call, and
// "" otherwise. It is the codex symmetric of
// conversation.LastPendingBlockingTool (which handles claude's
// AskUserQuestion / ExitPlanMode) and is consumed by
// state.ApplyIdleOverrides to promote codex agents from state="done" or
// "idle_prompt" up to state="question" when the rollout shows an
// outstanding picker.
//
// Implementation is a thin wrapper over ReadPendingQuestion: same scan,
// just discard the payload. Codex has no rollout-side ExitPlanMode
// equivalent today, so this only returns "question" or "". Missing
// files return "" without error.
func LastPendingBlockingToolCodex(path string) string {
	if ReadPendingQuestion(path) != nil {
		return "question"
	}
	return ""
}

// ReadPendingQuestion returns the parsed payload of the most recent
// unanswered request_user_input function_call in a codex rollout JSONL,
// or nil if no such question is pending. A function_call_output with the
// same call_id means the question was answered — we drop it.
//
// Missing files return nil without error so the handler can fall through
// to the empty state.
func ReadPendingQuestion(path string) *domain.PendingQuestion {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	type payload struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		CallID    string `json:"call_id"`
	}
	type line struct {
		Type    string  `json:"type"`
		Payload payload `json:"payload"`
	}

	// Scan once; track the last unanswered request_user_input. Answered
	// status is "saw a function_call_output with the same call_id later
	// in the file" — codex appends both in order.
	type pending struct {
		callID string
		args   string
	}
	var open []pending
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
		if l.Type != "response_item" {
			continue
		}
		switch l.Payload.Type {
		case "function_call":
			if l.Payload.Name != "request_user_input" {
				continue
			}
			open = append(open, pending{callID: l.Payload.CallID, args: l.Payload.Arguments})
		case "function_call_output":
			for i := range open {
				if open[i].callID == l.Payload.CallID {
					open = append(open[:i], open[i+1:]...)
					break
				}
			}
		}
	}
	if len(open) == 0 {
		return nil
	}

	latest := open[len(open)-1]
	var args requestUserInputArgs
	if err := json.Unmarshal([]byte(latest.args), &args); err != nil {
		return nil
	}
	out := &domain.PendingQuestion{ToolUseID: latest.callID}
	for _, q := range args.Questions {
		prompt := domain.PendingQuestionPrompt{
			ID:       q.ID,
			Question: q.Question,
			Header:   q.Header,
		}
		for _, o := range q.Options {
			prompt.Options = append(prompt.Options, domain.PendingQuestionOption{
				Label:       o.Label,
				Description: o.Description,
			})
		}
		out.Questions = append(out.Questions, prompt)
	}
	return out
}
