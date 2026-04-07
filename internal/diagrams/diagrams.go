// Package diagrams captures, stores, and renders mermaid diagrams
// extracted from Claude Code assistant messages on a per-session basis.
//
// The dashboard is a pure reader of this package's filesystem layout.
// The sole writer is the mermaid-extractor Claude Code hook.
package diagrams

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Diagram represents a single captured mermaid diagram stored on disk.
type Diagram struct {
	SessionID string
	Hash      string    // 8-char sha256 prefix of the raw source bytes
	Timestamp time.Time // parsed from filename prefix
	Source    string    // raw mermaid source (may include %% title: line)
	Title     string    // parsed from %% title: comment or frontmatter
	Type      string    // flowchart, sequenceDiagram, stateDiagram, ...
	Path      string    // absolute path to the .mmd file
}

// safeSessionIDRe restricts session IDs to alnum/dash/underscore so they
// can be used safely as a single path component (no traversal, no slashes).
var safeSessionIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// IsValidSessionID reports whether s is safe to use as a directory name.
func IsValidSessionID(s string) bool {
	return safeSessionIDRe.MatchString(s)
}

// Dir returns the per-session diagrams directory path. Returns "" if the
// session ID does not match the safe pattern (defense in depth against
// path traversal via crafted session IDs).
func Dir(stateDir, sessionID string) string {
	if !IsValidSessionID(sessionID) {
		return ""
	}
	return filepath.Join(stateDir, "diagrams", sessionID)
}

// Filename returns the canonical <unix-ts>-<hash>.mmd filename.
func Filename(ts time.Time, hash string) string {
	return fmt.Sprintf("%d-%s.mmd", ts.Unix(), hash)
}

// Hash returns the 8-character hex prefix of sha256(src) over the raw
// source bytes. No normalization is performed — byte-identical sources
// dedup, anything else produces a new hash.
func Hash(src string) string {
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:4])
}

var knownTypes = []string{
	"flowchart",
	"sequenceDiagram",
	"stateDiagram-v2",
	"stateDiagram",
	"classDiagram",
	"erDiagram",
	"gantt",
	"pie",
	"journey",
	"gitGraph",
	"mindmap",
	"timeline",
	"quadrantChart",
	"requirementDiagram",
	"C4Context",
}

// ParseType returns the mermaid diagram type (first token of the first
// non-metadata line), or "unknown" if it can't be determined.
func ParseType(src string) string {
	lines := strings.Split(src, "\n")
	i := 0
	// Skip frontmatter block (--- ... ---).
	if i < len(lines) && strings.TrimSpace(lines[i]) == "---" {
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) != "---" {
			i++
		}
		if i < len(lines) {
			i++ // consume closing ---
		}
	}
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "%%") {
			continue
		}
		for _, t := range knownTypes {
			if line == t || strings.HasPrefix(line, t+" ") || strings.HasPrefix(line, t+"\t") {
				return t
			}
		}
		// Unknown first non-metadata line.
		return "unknown"
	}
	return "unknown"
}

var (
	commentTitleRe     = regexp.MustCompile(`^%%\s*title:\s*(.+?)\s*$`)
	frontmatterTitleRe = regexp.MustCompile(`^title:\s*(.+?)\s*$`)
)

// ParseTitle extracts the title from a mermaid source. It understands
// `%% title: ...` comment lines and `---\ntitle: ...\n---` frontmatter.
// Returns an empty string if no title is declared.
func ParseTitle(src string) string {
	lines := strings.Split(src, "\n")
	// Frontmatter.
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "---" {
				break
			}
			if m := frontmatterTitleRe.FindStringSubmatch(line); m != nil {
				return m[1]
			}
		}
	}
	// %% title: ... line.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if m := commentTitleRe.FindStringSubmatch(trimmed); m != nil {
			return m[1]
		}
		// Stop at the first non-comment, non-blank line.
		if !strings.HasPrefix(trimmed, "%%") && trimmed != "---" {
			break
		}
	}
	return ""
}

var filenameRe = regexp.MustCompile(`^(\d+)-([0-9a-f]{8})\.mmd$`)

// ParseFilename parses a canonical <unix-ts>-<hash>.mmd filename.
func ParseFilename(name string) (time.Time, string, bool) {
	m := filenameRe.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, "", false
	}
	secs, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}
	return time.Unix(secs, 0), m[2], true
}

// NewDiagramFromBytes builds a Diagram from raw source bytes, computing
// Hash/Title/Type but leaving Path empty (callers set it after writing).
func NewDiagramFromBytes(sessionID string, ts time.Time, src []byte) Diagram {
	s := string(src)
	return Diagram{
		SessionID: sessionID,
		Hash:      Hash(s),
		Timestamp: ts,
		Source:    s,
		Title:     ParseTitle(s),
		Type:      ParseType(s),
	}
}
