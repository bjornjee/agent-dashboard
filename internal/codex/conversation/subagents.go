package conversation

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

type subagentSessionMeta struct {
	ID            string            `json:"id"`
	Timestamp     string            `json:"timestamp"`
	Originator    string            `json:"originator"`
	Source        sessionMetaSource `json:"source"`
	AgentNickname string            `json:"agent_nickname"`
	AgentRole     string            `json:"agent_role"`
}

// sessionMetaSource is polymorphic in codex's JSONL schema: user threads
// write a string ("cli", "vscode"), subagent threads write a nested
// {subagent: {thread_spawn: {...}}} object. The custom UnmarshalJSON
// accepts both forms so a string source doesn't fail the whole meta
// decode — without this, payload.originator would never be populated
// for top-level (non-subagent) sessions.
type sessionMetaSource struct {
	Subagent struct {
		ThreadSpawn struct {
			ParentThreadID string `json:"parent_thread_id"`
			AgentNickname  string `json:"agent_nickname"`
			AgentRole      string `json:"agent_role"`
		} `json:"thread_spawn"`
	} `json:"subagent"`
}

func (s *sessionMetaSource) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return nil
	}
	type alias sessionMetaSource
	return json.Unmarshal(data, (*alias)(s))
}

// subagentRolloutDetails carries the per-rollout signals we extract in a
// single pass while building the sessions index — completion status, the
// first meaningful instruction line for the description, and the
// collaboration/sandbox mode summary.
type subagentRolloutDetails struct {
	InstructionHead string
	Mode            string
	Completed       bool
}

// FindSubagents returns the subagents whose session_meta names parentSessionID
// as their parent_thread_id. Results come from the shared sessions index,
// so the first cold call within a TTL window walks the tree once and all
// subsequent callers (this one or others) read from the in-memory map.
func FindSubagents(sessionsRoot, parentSessionID string) []domain.SubagentInfo {
	if parentSessionID == "" {
		return nil
	}
	idx := getOrBuildIndex(sessionsRoot)
	if idx == nil {
		return nil
	}
	return idx.children[parentSessionID]
}

// ParentThreadID returns the parent_thread_id recorded in sessionID's
// rollout file, or "" when the session has no parent (a top-level agent),
// has no session_meta line yet, or doesn't have a rollout file under
// sessionsRoot.
//
// Resolution goes through the per-session cache to avoid the
// session-tree-wide index build on the main bubbletea goroutine —
// TopLevelAgents calls this once per codex agent on every stateUpdatedMsg.
// On a cache miss the lookup walks directory entries by filename only
// (no rollout opens) to find the matching file, then opens that single
// file to read its first session_meta line.
func ParentThreadID(sessionsRoot, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	entry, _ := resolveSessionEntry(sessionsRoot, sessionID, true)
	return entry.Parent
}

// OriginatorDesktopApp is the payload.originator string codex writes in
// session_meta when the rollout was created by the Codex desktop app
// (as opposed to "codex-tui" for the codex CLI). The dashboard filters
// these rollouts out — they're not the user's CLI work.
const OriginatorDesktopApp = "Codex Desktop"

// Originator returns the payload.originator recorded in sessionID's
// rollout file (e.g. "codex-tui", "Codex Desktop"), or "" when the
// session has no session_meta line yet or no rollout file under
// sessionsRoot. Shares the same per-session cache as ParentThreadID, so
// a TopLevelAgents pass that consults both fields opens at most one
// file per agent within the TTL window.
func Originator(sessionsRoot, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	entry, _ := resolveSessionEntry(sessionsRoot, sessionID, true)
	return entry.Originator
}

// resolveSessionEntry returns the per-session cache entry for
// (sessionsRoot, sessionID), populating fields lazily:
//   - Path: located by directory-only walk (no rollout file opens).
//   - Parent/MetaRead: populated only when readMeta is true.
//
// The entry is cached under the package cache's TTL so subsequent callers
// (LocateRollout, ParentThreadID, or repeat calls) share the work.
func resolveSessionEntry(sessionsRoot, sessionID string, readMeta bool) (sessionEntry, bool) {
	if sessionID == "" {
		return sessionEntry{}, false
	}
	if entry, ok := pkgCache.getSession(sessionsRoot, sessionID); ok {
		if !readMeta || entry.MetaRead {
			return entry, true
		}
		// Cached path is still good; we just need the meta on top of it.
		if entry.Path != "" {
			meta, _ := readSubagentSessionMeta(entry.Path)
			entry.Parent = meta.Source.Subagent.ThreadSpawn.ParentThreadID
			entry.Originator = meta.Originator
			entry.MetaRead = true
			pkgCache.putSession(sessionsRoot, sessionID, entry)
			return entry, true
		}
	}

	entry := sessionEntry{Path: locateRolloutFile(sessionsRoot, sessionID)}
	if readMeta && entry.Path != "" {
		meta, _ := readSubagentSessionMeta(entry.Path)
		entry.Parent = meta.Source.Subagent.ThreadSpawn.ParentThreadID
		entry.Originator = meta.Originator
		entry.MetaRead = true
	}
	pkgCache.putSession(sessionsRoot, sessionID, entry)
	return entry, true
}

// locateRolloutFile walks sessionsRoot's directory tree and returns the
// path of the rollout file whose filename trails with `-<sessionID>.jsonl`.
// Crucially it inspects only directory entries (d.Name()) — no rollout
// JSONL is opened — so the per-session lookup is cheap enough to run on
// the bubbletea main goroutine even for sessions trees with thousands of
// files. Empty string when nothing matches or the root doesn't exist.
//
// Applies the same "lexicographically greatest path wins" rule as
// buildSessionsIndex: when `codex resume <sid>` produces a second rollout
// for the same sessionID under a later YYYY/MM/DD directory, the greatest
// path (== newest by ISO8601 prefix) is returned.
func locateRolloutFile(sessionsRoot, sessionID string) string {
	if sessionsRoot == "" || sessionID == "" {
		return ""
	}
	suffix := "-" + sessionID + ".jsonl"
	var found string
	_ = filepath.WalkDir(sessionsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, suffix) {
			return nil
		}
		if path > found {
			found = path
		}
		return nil
	})
	return found
}

// getOrBuildIndex returns the cached sessions index for sessionsRoot or,
// on a cold cache, builds it under singleflight so concurrent callers
// share one filesystem walk.
//
// A nil return means the sessions root does not exist (codex isn't
// installed yet). The empty result is still cached so the next caller
// doesn't re-stat.
func getOrBuildIndex(sessionsRoot string) *sessionsIndex {
	if idx, ok := pkgCache.getIndex(sessionsRoot); ok {
		return idx
	}
	v, _, _ := pkgCache.callGroup().Do(sessionsRoot, func() (any, error) {
		// Recheck after the singleflight wait — a sibling caller may have
		// populated the cache while we were queued.
		if idx, ok := pkgCache.getIndex(sessionsRoot); ok {
			return idx, nil
		}
		idx := buildSessionsIndex(sessionsRoot)
		pkgCache.putIndex(sessionsRoot, idx)
		return idx, nil
	})
	idx, _ := v.(*sessionsIndex)
	return idx
}

// buildSessionsIndex walks sessionsRoot once and produces the per-root
// index. Missing roots produce an empty (non-nil) index so callers cache
// the "codex not installed" outcome.
func buildSessionsIndex(sessionsRoot string) *sessionsIndex {
	idx := &sessionsIndex{
		builtAt:  nowFunc(),
		children: map[string][]domain.SubagentInfo{},
	}
	if _, err := os.Stat(sessionsRoot); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return idx // empty index, cached negative result
		}
		return idx
	}

	walkSessionsRootFn(sessionsRoot, func(path string, meta subagentSessionMeta) {
		idByMeta := meta.ID
		parentID := meta.Source.Subagent.ThreadSpawn.ParentThreadID
		if parentID == "" || idByMeta == "" {
			return
		}
		agentType := firstNonEmpty(meta.AgentRole, meta.Source.Subagent.ThreadSpawn.AgentRole)
		fallback := firstNonEmpty(meta.AgentNickname, meta.Source.Subagent.ThreadSpawn.AgentNickname, meta.AgentRole, meta.Source.Subagent.ThreadSpawn.AgentRole, idByMeta)
		details := readSubagentRolloutDetails(path)
		instructionHead := firstNonEmpty(details.InstructionHead, fallback)
		idx.children[parentID] = append(idx.children[parentID], domain.SubagentInfo{
			AgentID:         idByMeta,
			AgentType:       agentType,
			Description:     instructionHead,
			InstructionHead: instructionHead,
			Mode:            details.Mode,
			Completed:       details.Completed,
			StartedAt:       meta.Timestamp,
		})
	})

	for parentID, subs := range idx.children {
		sort.Slice(subs, func(i, j int) bool { return subs[i].StartedAt > subs[j].StartedAt })
		idx.children[parentID] = subs
	}
	return idx
}

// walkSessionsRootFn is the swappable walker used to traverse the codex
// sessions tree. Tests replace this to count walks and verify
// single-flight behaviour.
var walkSessionsRootFn = walkSessionsRootImpl

func walkSessionsRootImpl(sessionsRoot string, visit func(string, subagentSessionMeta)) {
	_ = filepath.WalkDir(sessionsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		meta, _ := readSubagentSessionMeta(path)
		visit(path, meta)
		return nil
	})
}

func readSubagentSessionMeta(path string) (subagentSessionMeta, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return subagentSessionMeta{}, false
	}
	mtime := info.ModTime()
	if meta, ok, hit := pkgCache.getMetaForPath(path, mtime); hit {
		return meta, ok
	}

	f, err := os.Open(path)
	if err != nil {
		return subagentSessionMeta{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	for scanner.Scan() {
		var line struct {
			Type    string              `json:"type"`
			Payload subagentSessionMeta `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil || line.Type != "session_meta" {
			continue
		}
		if line.Payload.ID == "" {
			pkgCache.putMetaForPath(path, mtime, subagentSessionMeta{}, false)
			return subagentSessionMeta{}, false
		}
		pkgCache.putMetaForPath(path, mtime, line.Payload, true)
		return line.Payload, true
	}
	pkgCache.putMetaForPath(path, mtime, subagentSessionMeta{}, false)
	return subagentSessionMeta{}, false
}

// readSubagentRolloutDetails scans the rollout JSONL once and extracts:
//   - InstructionHead: the first meaningful line of the first user_message
//     event (the prompt codex received), used as the subagent description.
//   - Mode: a short summary derived from the first turn_context payload
//     (collaboration mode / approval policy / sandbox type).
//   - Completed: true if any event_msg/task_complete line is present.
//
// One file open per rollout — buildSessionsIndex was previously calling
// rolloutCompleted (a second open) on top of readSubagentSessionMeta.
func readSubagentRolloutDetails(path string) subagentRolloutDetails {
	info, err := os.Stat(path)
	if err != nil {
		return subagentRolloutDetails{}
	}
	mtime := info.ModTime()
	if details, hit := pkgCache.getDetailsForPath(path, mtime); hit {
		return details
	}

	f, err := os.Open(path)
	if err != nil {
		return subagentRolloutDetails{}
	}
	defer f.Close()

	var details subagentRolloutDetails
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	for scanner.Scan() {
		var line struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		switch line.Type {
		case "event_msg":
			var payload struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}
			if json.Unmarshal(line.Payload, &payload) != nil {
				continue
			}
			if payload.Type == "user_message" && details.InstructionHead == "" {
				details.InstructionHead = firstMeaningfulLine(payload.Message)
			}
			if payload.Type == "task_complete" {
				details.Completed = true
			}
		case "turn_context":
			if details.Mode == "" {
				details.Mode = codexModeFromTurnContext(line.Payload)
			}
		}
	}
	pkgCache.putDetailsForPath(path, mtime, details)
	return details
}

func firstMeaningfulLine(message string) string {
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func codexModeFromTurnContext(raw json.RawMessage) string {
	var payload struct {
		ApprovalPolicy string `json:"approval_policy"`
		SandboxPolicy  struct {
			Type string `json:"type"`
		} `json:"sandbox_policy"`
		CollaborationMode struct {
			Mode string `json:"mode"`
		} `json:"collaboration_mode"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	parts := []string{
		payload.CollaborationMode.Mode,
		payload.ApprovalPolicy,
		payload.SandboxPolicy.Type,
	}
	nonEmpty := parts[:0]
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, " / ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
