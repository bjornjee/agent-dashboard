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
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Source    struct {
		Subagent struct {
			ThreadSpawn struct {
				ParentThreadID string `json:"parent_thread_id"`
				AgentNickname  string `json:"agent_nickname"`
				AgentRole      string `json:"agent_role"`
			} `json:"thread_spawn"`
		} `json:"subagent"`
	} `json:"source"`
	AgentNickname string `json:"agent_nickname"`
	AgentRole     string `json:"agent_role"`
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
func ParentThreadID(sessionsRoot, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	idx := getOrBuildIndex(sessionsRoot)
	if idx == nil {
		return ""
	}
	return idx.rollouts[sessionID].Meta.Source.Subagent.ThreadSpawn.ParentThreadID
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
		rollouts: map[string]rolloutEntry{},
		children: map[string][]domain.SubagentInfo{},
	}
	if _, err := os.Stat(sessionsRoot); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return idx // empty index, cached negative result
		}
		return idx
	}

	walkSessionsRootFn(sessionsRoot, func(path string, meta subagentSessionMeta) {
		// Two sessionID sources: the filename (always present) and the
		// parsed session_meta (only present once codex has written a meta
		// line). Both can disagree in theory; in practice they don't, but
		// indexing by the filename ensures LocateRollout works for brand
		// new rollouts whose session_meta hasn't been flushed yet.
		idByFile := sessionIDFromFilename(path)
		idByMeta := meta.ID

		// LocateRollout invariant: when `codex resume <sid>` produces a
		// second rollout for the same sessionID, keep the lexicographically
		// greatest path (== newest by ISO8601 prefix). Apply both to the
		// rollouts map and to subagent placement.
		recordRollout := func(id string) {
			if id == "" {
				return
			}
			if existing, ok := idx.rollouts[id]; ok && existing.Path >= path {
				return
			}
			idx.rollouts[id] = rolloutEntry{Path: path, Meta: meta}
		}
		recordRollout(idByFile)
		if idByMeta != "" && idByMeta != idByFile {
			recordRollout(idByMeta)
		}

		parentID := meta.Source.Subagent.ThreadSpawn.ParentThreadID
		if parentID == "" || idByMeta == "" {
			return
		}
		info := domain.SubagentInfo{
			AgentID:     idByMeta,
			AgentType:   firstNonEmpty(meta.AgentRole, meta.Source.Subagent.ThreadSpawn.AgentRole),
			Description: firstNonEmpty(meta.AgentNickname, meta.Source.Subagent.ThreadSpawn.AgentNickname, meta.AgentRole, meta.Source.Subagent.ThreadSpawn.AgentRole, idByMeta),
			Completed:   rolloutCompleted(path),
			StartedAt:   meta.Timestamp,
		}
		idx.children[parentID] = append(idx.children[parentID], info)
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
		// Report every rollout, even ones that don't yet have a session_meta
		// line — LocateRollout still needs to find them by sessionID.
		meta, _ := readSubagentSessionMeta(path)
		visit(path, meta)
		return nil
	})
}

// sessionIDFromFilename extracts the trailing UUID from a rollout filename
// like `rollout-YYYY-MM-DDTHH-MM-SS-<sessionID>.jsonl`. The "session ID"
// itself can be a UUID with hyphens, so we strip the fixed-length ISO8601
// prefix rather than splitting on hyphens.
//
// Returns "" for any filename that doesn't match the expected shape.
func sessionIDFromFilename(path string) string {
	name := filepath.Base(path)
	const prefix = "rollout-"
	const suffix = ".jsonl"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return ""
	}
	core := name[len(prefix) : len(name)-len(suffix)]
	// ISO8601 prefix codex uses: YYYY-MM-DDTHH-MM-SS = 19 chars plus a
	// trailing "-" → 20 chars.
	const isoLen = len("2026-05-21T14-44-03") + 1 // 20
	if len(core) <= isoLen {
		return ""
	}
	return core[isoLen:]
}

func readSubagentSessionMeta(path string) (subagentSessionMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return subagentSessionMeta{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var line struct {
			Type    string              `json:"type"`
			Payload subagentSessionMeta `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil || line.Type != "session_meta" {
			continue
		}
		if line.Payload.ID == "" {
			return subagentSessionMeta{}, false
		}
		return line.Payload, true
	}
	return subagentSessionMeta{}, false
}

func rolloutCompleted(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var line struct {
			Type    string `json:"type"`
			Payload struct {
				Type string `json:"type"`
			} `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		if line.Type == "event_msg" && line.Payload.Type == "task_complete" {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
