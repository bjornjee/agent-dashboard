package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// codexPricingTable maps Codex model names to per-million-token pricing.
// Ported from CodexBar's CostUsagePricing.
var codexPricingTable = map[string]domain.CodexModelPricing{
	"gpt-5":         {Input: 1.25, Output: 10.0, CacheRead: 0.125},
	"gpt-5-codex":   {Input: 1.25, Output: 10.0, CacheRead: 0.125},
	"gpt-5-mini":    {Input: 0.25, Output: 2.0, CacheRead: 0.025},
	"gpt-5-nano":    {Input: 0.05, Output: 0.40, CacheRead: 0.005},
	"gpt-5-pro":     {Input: 15.0, Output: 120.0, CacheRead: 0.0},
	"gpt-5.1":       {Input: 1.25, Output: 10.0, CacheRead: 0.125},
	"gpt-5.1-codex": {Input: 1.25, Output: 10.0, CacheRead: 0.125},
	"gpt-5.2":       {Input: 1.75, Output: 14.0, CacheRead: 0.175},
	"gpt-5.2-codex": {Input: 1.75, Output: 14.0, CacheRead: 0.175},
	"gpt-5.3-codex": {Input: 1.75, Output: 14.0, CacheRead: 0.175},
	"gpt-5.4":       {Input: 2.50, Output: 15.0, CacheRead: 0.25},
	"gpt-5.4-codex": {Input: 2.50, Output: 15.0, CacheRead: 0.25},
	"gpt-5.4-mini":  {Input: 0.75, Output: 4.50, CacheRead: 0.075},
	"gpt-5.4-nano":  {Input: 0.20, Output: 1.25, CacheRead: 0.02},
	"gpt-5.4-pro":   {Input: 30.0, Output: 180.0, CacheRead: 0.0},
}

// lookupCodexPricing finds pricing for a Codex model string.
// Falls back to gpt-5.4 pricing for unknown models.
func lookupCodexPricing(model string) domain.CodexModelPricing {
	normalized := strings.ToLower(strings.TrimSpace(model))
	// Strip "openai/" prefix if present
	normalized = strings.TrimPrefix(normalized, "openai/")
	// Strip dated suffix like -2026-04-03
	if idx := len(normalized); idx > 11 {
		// Check if last 11 chars match -YYYY-MM-DD pattern
		suffix := normalized[idx-11:]
		if len(suffix) == 11 && suffix[0] == '-' && suffix[5] == '-' && suffix[8] == '-' {
			base := normalized[:idx-11]
			if p, ok := codexPricingTable[base]; ok {
				return p
			}
		}
	}
	if p, ok := codexPricingTable[normalized]; ok {
		return p
	}
	return codexPricingTable["gpt-5.4"]
}

// codexCost calculates the USD cost for a Codex session given pricing and token counts.
func codexCost(pricing domain.CodexModelPricing, inputTokens, cachedInputTokens, outputTokens int) float64 {
	cached := cachedInputTokens
	if cached > inputTokens {
		cached = inputTokens
	}
	nonCached := inputTokens - cached
	cost := float64(nonCached) / 1_000_000 * pricing.Input
	cost += float64(cached) / 1_000_000 * pricing.CacheRead
	cost += float64(outputTokens) / 1_000_000 * pricing.Output
	return cost
}

// codexJSONLEntry is the top-level structure of a Codex session JSONL line.
type codexJSONLEntry struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexTurnContext struct {
	Model string `json:"model"`
}

type codexEventMsg struct {
	Type string          `json:"type"`
	Info *codexTokenInfo `json:"info"`
}

type codexTokenInfo struct {
	TotalTokenUsage codexTokenUsage `json:"total_token_usage"`
}

type codexTokenUsage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

// CodexSession holds parsed usage data from a single Codex session file.
type CodexSession struct {
	Model             string
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
	CostUSD           float64
}

// readCodexSession parses a single Codex JSONL file and returns the session's usage.
func readCodexSession(path string) CodexSession {
	f, err := os.Open(path)
	if err != nil {
		return CodexSession{}
	}
	defer f.Close()

	var sess CodexSession
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry codexJSONLEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "turn_context":
			var tc codexTurnContext
			if err := json.Unmarshal(entry.Payload, &tc); err == nil && tc.Model != "" {
				sess.Model = tc.Model
			}
		case "event_msg":
			var em codexEventMsg
			if err := json.Unmarshal(entry.Payload, &em); err != nil {
				continue
			}
			if em.Type == "token_count" && em.Info != nil {
				tu := em.Info.TotalTokenUsage
				sess.InputTokens = tu.InputTokens
				sess.CachedInputTokens = tu.CachedInputTokens
				sess.OutputTokens = tu.OutputTokens
			}
		}
	}

	if sess.Model != "" && (sess.InputTokens > 0 || sess.OutputTokens > 0) {
		pricing := lookupCodexPricing(sess.Model)
		sess.CostUSD = codexCost(pricing, sess.InputTokens, sess.CachedInputTokens, sess.OutputTokens)
	}

	return sess
}

// CodexDayUsage holds aggregated Codex usage for a single day.
type CodexDayUsage struct {
	Date              string
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
	CostUSD           float64
}

// ReadCodexDailyUsage scans Codex session files in date-partitioned directories
// and returns aggregated usage per day, only including days on or after `since`.
// The sessions directory is expected to be structured as: <root>/<yyyy>/<mm>/<dd>/*.jsonl
func ReadCodexDailyUsage(sessionsDir string, since time.Time) []CodexDayUsage {
	sinceStr := since.Format("2006-01-02")
	dayMap := make(map[string]*CodexDayUsage)

	// Walk year dirs
	yearDirs, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	for _, yd := range yearDirs {
		if !yd.IsDir() {
			continue
		}
		year, err := strconv.Atoi(yd.Name())
		if err != nil || year < since.Year() {
			continue
		}

		yearPath := filepath.Join(sessionsDir, yd.Name())
		monthDirs, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}

		for _, md := range monthDirs {
			if !md.IsDir() {
				continue
			}
			month, err := strconv.Atoi(md.Name())
			if err != nil || month < 1 || month > 12 {
				continue
			}
			monthPath := filepath.Join(yearPath, md.Name())
			dayDirs, err := os.ReadDir(monthPath)
			if err != nil {
				continue
			}

			for _, dd := range dayDirs {
				if !dd.IsDir() {
					continue
				}
				day, err := strconv.Atoi(dd.Name())
				if err != nil || day < 1 || day > 31 {
					continue
				}
				dateStr := yd.Name() + "-" + md.Name() + "-" + dd.Name()
				if dateStr < sinceStr {
					continue
				}

				dayPath := filepath.Join(monthPath, dd.Name())
				files, err := os.ReadDir(dayPath)
				if err != nil {
					continue
				}

				for _, f := range files {
					if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
						continue
					}
					sess := readCodexSession(filepath.Join(dayPath, f.Name()))
					if sess.OutputTokens == 0 && sess.InputTokens == 0 {
						continue
					}

					d, ok := dayMap[dateStr]
					if !ok {
						d = &CodexDayUsage{Date: dateStr}
						dayMap[dateStr] = d
					}
					d.InputTokens += sess.InputTokens
					d.CachedInputTokens += sess.CachedInputTokens
					d.OutputTokens += sess.OutputTokens
					d.CostUSD += sess.CostUSD
				}
			}
		}
	}

	// Sort by date ascending
	result := make([]CodexDayUsage, 0, len(dayMap))
	for _, d := range dayMap {
		result = append(result, *d)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date < result[j].Date
	})
	return result
}
