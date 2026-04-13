package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchRateLimit_FullResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization: got %q, want %q", got, "Bearer test-token")
		}
		if got := r.Header.Get("anthropic-beta"); got != oauthBetaHeader {
			t.Errorf("anthropic-beta: got %q, want %q", got, oauthBetaHeader)
		}
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("User-Agent should not be empty")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{
			"five_hour": {"utilization": 42.5, "resets_at": "2026-04-09T14:00:00Z"},
			"seven_day": {"utilization": 30.0, "resets_at": "2026-04-12T00:00:00Z"},
			"seven_day_opus": {"utilization": 10.0, "resets_at": "2026-04-12T00:00:00Z"},
			"seven_day_sonnet": {"utilization": 25.0, "resets_at": "2026-04-12T00:00:00Z"},
			"extra_usage": {
				"is_enabled": true,
				"monthly_limit": 5000,
				"used_credits": 420,
				"utilization": 8.4,
				"currency": "usd"
			}
		}`))
	}))
	defer srv.Close()

	fetcher := &httpRateLimitFetcher{baseURL: srv.URL}
	rl, err := fetcher.Fetch(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rl.Session == nil {
		t.Fatal("Session should not be nil")
	}
	if rl.Session.UsedPercent != 42.5 {
		t.Errorf("Session.UsedPercent: got %f, want 42.5", rl.Session.UsedPercent)
	}
	expectedReset := time.Date(2026, 4, 9, 14, 0, 0, 0, time.UTC)
	if !rl.Session.ResetsAt.Equal(expectedReset) {
		t.Errorf("Session.ResetsAt: got %v, want %v", rl.Session.ResetsAt, expectedReset)
	}

	if rl.Weekly == nil {
		t.Fatal("Weekly should not be nil")
	}
	if rl.Weekly.UsedPercent != 30.0 {
		t.Errorf("Weekly.UsedPercent: got %f, want 30.0", rl.Weekly.UsedPercent)
	}

	if rl.Opus == nil {
		t.Fatal("Opus should not be nil")
	}
	if rl.Opus.UsedPercent != 10.0 {
		t.Errorf("Opus.UsedPercent: got %f, want 10.0", rl.Opus.UsedPercent)
	}

	if rl.Sonnet == nil {
		t.Fatal("Sonnet should not be nil")
	}
	if rl.Sonnet.UsedPercent != 25.0 {
		t.Errorf("Sonnet.UsedPercent: got %f, want 25.0", rl.Sonnet.UsedPercent)
	}

	if rl.Extra == nil {
		t.Fatal("Extra should not be nil")
	}
	if !rl.Extra.Enabled {
		t.Error("Extra.Enabled should be true")
	}
	if rl.Extra.MonthlyLimit != 50.0 {
		t.Errorf("Extra.MonthlyLimit: got %f, want 50.0 (cents/100)", rl.Extra.MonthlyLimit)
	}
	if rl.Extra.UsedCredits != 4.20 {
		t.Errorf("Extra.UsedCredits: got %f, want 4.20 (cents/100)", rl.Extra.UsedCredits)
	}
}

func TestFetchRateLimit_PartialResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{
			"five_hour": {"utilization": 50.0}
		}`))
	}))
	defer srv.Close()

	fetcher := &httpRateLimitFetcher{baseURL: srv.URL}
	rl, err := fetcher.Fetch(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rl.Session == nil {
		t.Fatal("Session should not be nil")
	}
	if rl.Session.UsedPercent != 50.0 {
		t.Errorf("Session.UsedPercent: got %f, want 50.0", rl.Session.UsedPercent)
	}
	if rl.Weekly != nil {
		t.Error("Weekly should be nil for partial response")
	}
	if rl.Extra != nil {
		t.Error("Extra should be nil for partial response")
	}
}

func TestFetchRateLimit_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	fetcher := &httpRateLimitFetcher{baseURL: srv.URL}
	_, err := fetcher.Fetch(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestFetchRateLimit_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	fetcher := &httpRateLimitFetcher{baseURL: srv.URL}
	_, err := fetcher.Fetch(context.Background(), "token")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestMapWindow(t *testing.T) {
	tests := []struct {
		name        string
		utilization float64
		wantPercent float64
	}{
		{"normal percent", 64.0, 64.0},
		{"zero", 0.0, 0.0},
		{"full", 100.0, 100.0},
		{"fractional", 42.5, 42.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := tt.utilization
			rw := mapWindow(&oauthUsageWindow{Utilization: &u})
			if rw.UsedPercent != tt.wantPercent {
				t.Errorf("UsedPercent = %f, want %f", rw.UsedPercent, tt.wantPercent)
			}
		})
	}
}

func TestFetchRateLimit_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	fetcher := &httpRateLimitFetcher{baseURL: srv.URL}
	rl, err := fetcher.Fetch(context.Background(), "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rl.Session != nil || rl.Weekly != nil || rl.Opus != nil || rl.Sonnet != nil || rl.Extra != nil {
		t.Error("all windows should be nil for empty response")
	}
}
