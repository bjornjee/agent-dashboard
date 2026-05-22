package conversation

import (
	"bufio"
	"encoding/json"
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

func FindSubagents(sessionsRoot, parentSessionID string) []domain.SubagentInfo {
	if parentSessionID == "" {
		return nil
	}
	key := cacheKey{root: sessionsRoot, sessionID: parentSessionID}
	if subs, ok := pkgCache.getSubagentList(key); ok {
		return subs
	}
	if _, err := os.Stat(sessionsRoot); err != nil {
		return nil
	}

	var agents []domain.SubagentInfo
	walkSubagentSessionMetas(sessionsRoot, func(path string, meta subagentSessionMeta) {
		if meta.Source.Subagent.ThreadSpawn.ParentThreadID != parentSessionID {
			return
		}
		agents = append(agents, domain.SubagentInfo{
			AgentID:     meta.ID,
			AgentType:   firstNonEmpty(meta.AgentRole, meta.Source.Subagent.ThreadSpawn.AgentRole),
			Description: firstNonEmpty(meta.AgentNickname, meta.Source.Subagent.ThreadSpawn.AgentNickname, meta.AgentRole, meta.Source.Subagent.ThreadSpawn.AgentRole, meta.ID),
			Completed:   rolloutCompleted(path),
			StartedAt:   meta.Timestamp,
		})
	})

	sort.Slice(agents, func(i, j int) bool {
		return agents[i].StartedAt > agents[j].StartedAt
	})
	pkgCache.putSubagentList(key, agents)
	return agents
}

func ParentThreadID(sessionsRoot, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	key := cacheKey{root: sessionsRoot, sessionID: sessionID}
	if entry, ok := pkgCache.getRollout(key); ok && entry.MetaRead {
		return entry.Meta.Source.Subagent.ThreadSpawn.ParentThreadID
	}

	path, err := LocateRollout(sessionsRoot, sessionID)
	if err != nil || path == "" {
		return ""
	}
	// Cache the result of the meta read regardless of success — a file
	// that doesn't yet have a session_meta line shouldn't be re-opened
	// every poll until it does.
	meta, _ := readSubagentSessionMeta(path)
	pkgCache.putRollout(key, rolloutEntry{Path: path, Meta: meta, MetaRead: true})
	return meta.Source.Subagent.ThreadSpawn.ParentThreadID
}

func walkSubagentSessionMetas(sessionsRoot string, visit func(string, subagentSessionMeta)) {
	_ = filepath.WalkDir(sessionsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		meta, ok := readSubagentSessionMeta(path)
		if ok {
			visit(path, meta)
		}
		return nil
	})
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
