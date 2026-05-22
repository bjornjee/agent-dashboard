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

type subagentRolloutDetails struct {
	InstructionHead string
	Mode            string
	Completed       bool
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
		agentType := firstNonEmpty(meta.AgentRole, meta.Source.Subagent.ThreadSpawn.AgentRole)
		fallback := firstNonEmpty(meta.AgentNickname, meta.Source.Subagent.ThreadSpawn.AgentNickname, meta.AgentRole, meta.Source.Subagent.ThreadSpawn.AgentRole, meta.ID)
		details := readSubagentRolloutDetails(path)
		instructionHead := firstNonEmpty(details.InstructionHead, fallback)
		agents = append(agents, domain.SubagentInfo{
			AgentID:         meta.ID,
			AgentType:       agentType,
			Description:     instructionHead,
			InstructionHead: instructionHead,
			Mode:            details.Mode,
			Completed:       details.Completed,
			StartedAt:       meta.Timestamp,
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

func readSubagentRolloutDetails(path string) subagentRolloutDetails {
	f, err := os.Open(path)
	if err != nil {
		return subagentRolloutDetails{}
	}
	defer f.Close()

	var details subagentRolloutDetails
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
