package main

import (
	"bytes"
	"fmt"
	"strings"
)

type target string

const (
	targetClaude target = "claude"
	targetCodex  target = "codex"
)

type markerKind string

const (
	markerNone       markerKind = ""
	markerClaudeOnly markerKind = "claude-only"
	markerCodexOnly  markerKind = "codex-only"
)

func transform(src []byte, tgt target) ([]byte, error) {
	var out bytes.Buffer
	active := markerNone

	lines := bytes.SplitAfter(src, []byte("\n"))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}

	for i, line := range lines {
		kind, closing, ok := parseMarkerLine(line)
		if ok {
			if closing {
				if active == markerNone {
					return nil, fmt.Errorf("line %d: closing marker %q without opener", i+1, kind)
				}
				if active != kind {
					return nil, fmt.Errorf("line %d: closing marker %q does not match open marker %q", i+1, kind, active)
				}
				active = markerNone
				continue
			}
			if active != markerNone {
				return nil, fmt.Errorf("line %d: nested marker %q inside %q", i+1, kind, active)
			}
			active = kind
			continue
		}

		if active == markerNone || markerMatchesTarget(active, tgt) {
			out.Write(line)
		}
	}

	if active != markerNone {
		return nil, fmt.Errorf("unclosed marker %q", active)
	}

	result := out.Bytes()
	if tgt == targetCodex {
		result = bytes.ReplaceAll(result, []byte("/agent-dashboard:"), []byte("$agent-dashboard:"))
	}
	return result, nil
}

func parseMarkerLine(line []byte) (markerKind, bool, bool) {
	text := strings.TrimSpace(string(line))
	switch text {
	case "<!-- claude-only -->":
		return markerClaudeOnly, false, true
	case "<!-- /claude-only -->":
		return markerClaudeOnly, true, true
	case "<!-- codex-only -->":
		return markerCodexOnly, false, true
	case "<!-- /codex-only -->":
		return markerCodexOnly, true, true
	default:
		return markerNone, false, false
	}
}

func markerMatchesTarget(kind markerKind, tgt target) bool {
	switch kind {
	case markerClaudeOnly:
		return tgt == targetClaude
	case markerCodexOnly:
		return tgt == targetCodex
	default:
		return true
	}
}
