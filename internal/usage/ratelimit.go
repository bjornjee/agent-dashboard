package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

const (
	oauthBaseURL    = "https://api.anthropic.com"
	oauthUsagePath  = "/api/oauth/usage"
	oauthBetaHeader = "oauth-2025-04-20"
	oauthUserAgent  = "claude-code/2.1.0"
)

// RateLimitFetcher fetches rate-limit data from the Anthropic OAuth API.
type RateLimitFetcher interface {
	Fetch(ctx context.Context, accessToken string) (domain.RateLimit, error)
}

// oauthUsageResponse matches the Anthropic OAuth usage API response.
type oauthUsageResponse struct {
	FiveHour       *oauthUsageWindow `json:"five_hour"`
	SevenDay       *oauthUsageWindow `json:"seven_day"`
	SevenDayOpus   *oauthUsageWindow `json:"seven_day_opus"`
	SevenDaySonnet *oauthUsageWindow `json:"seven_day_sonnet"`
	ExtraUsage     *oauthExtraUsage  `json:"extra_usage"`
}

type oauthUsageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

type oauthExtraUsage struct {
	IsEnabled    bool     `json:"is_enabled"`
	MonthlyLimit *float64 `json:"monthly_limit"` // cents
	UsedCredits  *float64 `json:"used_credits"`  // cents
}

// httpRateLimitFetcher is the production fetcher that calls the real API.
type httpRateLimitFetcher struct {
	baseURL string // overridden in tests
}

var rateLimitFetcher RateLimitFetcher = &httpRateLimitFetcher{baseURL: oauthBaseURL}

// oauthHTTPClient is a dedicated client for OAuth API calls with explicit timeouts.
var oauthHTTPClient = &http.Client{Timeout: 30 * time.Second}

func (f *httpRateLimitFetcher) Fetch(ctx context.Context, accessToken string) (domain.RateLimit, error) {
	url := f.baseURL + oauthUsagePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.RateLimit{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-beta", oauthBetaHeader)
	req.Header.Set("User-Agent", oauthUserAgent)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return domain.RateLimit{}, fmt.Errorf("oauth usage request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return domain.RateLimit{}, fmt.Errorf("oauth usage: HTTP %d", resp.StatusCode)
	}

	var apiResp oauthUsageResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&apiResp); err != nil {
		return domain.RateLimit{}, fmt.Errorf("decode oauth usage: %w", err)
	}

	return mapResponse(apiResp), nil
}

func mapResponse(r oauthUsageResponse) domain.RateLimit {
	rl := domain.RateLimit{
		FetchedAt: time.Now(),
	}

	if w := r.FiveHour; w != nil && w.Utilization != nil {
		rl.Session = mapWindow(w)
	}
	if w := r.SevenDay; w != nil && w.Utilization != nil {
		rl.Weekly = mapWindow(w)
	}
	if w := r.SevenDayOpus; w != nil && w.Utilization != nil {
		rl.Opus = mapWindow(w)
	}
	if w := r.SevenDaySonnet; w != nil && w.Utilization != nil {
		rl.Sonnet = mapWindow(w)
	}
	if e := r.ExtraUsage; e != nil && e.IsEnabled {
		extra := domain.ExtraUsage{
			Enabled: true,
		}
		if e.MonthlyLimit != nil {
			extra.MonthlyLimit = *e.MonthlyLimit / 100 // cents → USD
		}
		if e.UsedCredits != nil {
			extra.UsedCredits = *e.UsedCredits / 100 // cents → USD
		}
		rl.Extra = &extra
	}

	return rl
}

func mapWindow(w *oauthUsageWindow) *domain.RateWindow {
	rw := &domain.RateWindow{}
	if w.Utilization != nil {
		rw.UsedPercent = *w.Utilization * 100
	}
	if w.ResetsAt != "" {
		rw.ResetsAt = parseISO8601(w.ResetsAt)
	}
	return rw
}

func parseISO8601(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// FetchRateLimit is the public entry point used by the TUI.
// Returns (nil, nil) when no credentials are available.
func FetchRateLimit(ctx context.Context) (*domain.RateLimit, error) {
	token, plan, err := AutoDiscoverToken(ctx)
	if err != nil || token == "" {
		return nil, nil
	}

	rl, err := rateLimitFetcher.Fetch(ctx, token)
	if err != nil {
		return nil, err
	}
	rl.Plan = plan
	return &rl, nil
}
