package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ModelPricing holds per-million-token rates in USD.
type ModelPricing struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

// pricingTable maps model family keywords to pricing.
// Updated lazily — add new models as they appear.
var pricingTable = map[string]ModelPricing{
	"opus":   {Input: 15.0, Output: 75.0, CacheRead: 1.50, CacheWrite: 18.75},
	"sonnet": {Input: 3.0, Output: 15.0, CacheRead: 0.30, CacheWrite: 3.75},
	"haiku":  {Input: 0.80, Output: 4.0, CacheRead: 0.08, CacheWrite: 1.00},
}

// Usage holds aggregated token counts and estimated cost for a session.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CostUSD          float64
	Model            string // last seen model
}

// lookupPricing finds the pricing for a model string like "claude-opus-4-6".
func lookupPricing(model string) ModelPricing {
	normalized := strings.ToLower(model)
	for key, pricing := range pricingTable {
		if strings.Contains(normalized, key) {
			return pricing
		}
	}
	// Default to sonnet pricing for unknown models
	return pricingTable["sonnet"]
}

// usageEntry is the minimal structure we need from assistant JSONL entries.
type usageEntry struct {
	Message struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ReadUsage reads a Claude session JSONL and sums token usage + estimated cost.
func ReadUsage(projDir, sessionID string) Usage {
	path := filepath.Join(projDir, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return Usage{}
	}
	defer f.Close()

	var u Usage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry usageEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		tok := entry.Message.Usage
		if tok.InputTokens == 0 && tok.OutputTokens == 0 {
			continue
		}

		u.InputTokens += tok.InputTokens
		u.OutputTokens += tok.OutputTokens
		u.CacheReadTokens += tok.CacheReadInputTokens
		u.CacheWriteTokens += tok.CacheCreationInputTokens

		if entry.Message.Model != "" {
			u.Model = entry.Message.Model
			pricing := lookupPricing(entry.Message.Model)
			u.CostUSD += float64(tok.InputTokens) / 1_000_000 * pricing.Input
			u.CostUSD += float64(tok.OutputTokens) / 1_000_000 * pricing.Output
			u.CostUSD += float64(tok.CacheReadInputTokens) / 1_000_000 * pricing.CacheRead
			u.CostUSD += float64(tok.CacheCreationInputTokens) / 1_000_000 * pricing.CacheWrite
		}
	}
	// scanner.Err() intentionally ignored — partial usage is acceptable for dashboard display

	return u
}

// ReadAllUsage reads usage for all agents and returns per-agent + total.
func ReadAllUsage(agents []Agent) (map[string]Usage, Usage) {
	perAgent := make(map[string]Usage)
	var total Usage

	for _, agent := range agents {
		if agent.Cwd == "" {
			continue
		}
		slug := ProjectSlug(agent.Cwd)
		projDir := filepath.Join(ConversationsDir(), slug)

		sessionID := agent.SessionID
		if sessionID == "" {
			sessionID = FindSessionID(agent.Cwd)
		}
		if sessionID == "" {
			continue
		}

		u := ReadUsage(projDir, sessionID)
		perAgent[agent.Target] = u
		total.InputTokens += u.InputTokens
		total.OutputTokens += u.OutputTokens
		total.CacheReadTokens += u.CacheReadTokens
		total.CacheWriteTokens += u.CacheWriteTokens
		total.CostUSD += u.CostUSD
	}

	return perAgent, total
}

// FormatCost returns a human-readable cost string.
func FormatCost(costUSD float64) string {
	if costUSD < 0.01 {
		return fmt.Sprintf("$%.4f", costUSD)
	}
	return fmt.Sprintf("$%.2f", costUSD)
}

// FormatTokens returns a compact token count like "12.3k" or "1.2M".
func FormatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}
