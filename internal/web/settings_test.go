package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/stretchr/testify/mock"
)

func getSettings(t *testing.T, ts string) (*http.Response, domain.Settings) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts+"/api/settings", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/settings: %v", err)
	}
	var got domain.Settings
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
	}
	resp.Body.Close()
	return resp, got
}

func postSettings(t *testing.T, ts string, body string, withCSRF bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", ts+"/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if withCSRF {
		req.Header.Set("X-Requested-With", "dashboard")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/settings: %v", err)
	}
	return resp
}

// TestSettings_GET_Defaults verifies that on a fresh state dir (no
// settings.toml on disk) the GET handler returns the same defaults that
// LoadSettings would.
func TestSettings_GET_Defaults(t *testing.T) {
	ts, _ := createTestServer(t)
	resp, got := getSettings(t, ts.URL)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got.Harness.Default != "claude" {
		t.Errorf("Harness.Default = %q, want %q", got.Harness.Default, "claude")
	}
	if got.Effort.Default != "high" {
		t.Errorf("Effort.Default = %q, want %q", got.Effort.Default, "high")
	}
}

// TestSettings_POST_ValidCodex POSTs a settings struct with codex as the
// default harness, asserts 200, and verifies the file on disk round-trips.
func TestSettings_POST_ValidCodex(t *testing.T) {
	ts, stateDir := createTestServer(t)

	body := `{"Harness":{"Default":"codex"}}`
	resp := postSettings(t, ts.URL, body, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var errBody map[string]string
		json.NewDecoder(resp.Body).Decode(&errBody)
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, errBody)
	}

	// File on disk should reflect the new default.
	got := config.LoadSettings(stateDir)
	if got.Harness.Default != "codex" {
		t.Errorf("on-disk Harness.Default = %q, want %q", got.Harness.Default, "codex")
	}

	// GET should also reflect the new value.
	_, fetched := getSettings(t, ts.URL)
	if fetched.Harness.Default != "codex" {
		t.Errorf("GET after POST returned Harness.Default = %q, want %q", fetched.Harness.Default, "codex")
	}
}

// TestSettings_POST_InvalidHarness rejects an unknown harness name and
// leaves the on-disk file untouched.
func TestSettings_POST_InvalidHarness(t *testing.T) {
	ts, stateDir := createTestServer(t)

	body := `{"Harness":{"Default":"bogus"}}`
	resp := postSettings(t, ts.URL, body, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	// settings.toml must NOT exist after a rejected POST against a fresh dir.
	if _, err := os.Stat(filepath.Join(stateDir, "settings.toml")); !os.IsNotExist(err) {
		t.Errorf("settings.toml should not exist after rejected POST, stat err = %v", err)
	}
}

// TestSettings_POST_CSRFRequired denies POSTs without the X-Requested-With
// header, matching every other POST on the dashboard.
func TestSettings_POST_CSRFRequired(t *testing.T) {
	ts, _ := createTestServer(t)
	resp := postSettings(t, ts.URL, `{"Harness":{"Default":"codex"}}`, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// TestSettings_POST_NormalizesEmptyHarness asserts that posting
// {"Harness":{}} (Default omitted) normalizes to "claude" rather than
// persisting an empty string the UI dropdown can't match.
func TestSettings_POST_NormalizesEmptyHarness(t *testing.T) {
	ts, stateDir := createTestServer(t)

	resp := postSettings(t, ts.URL, `{"Harness":{}}`, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	got := config.LoadSettings(stateDir)
	if got.Harness.Default != "claude" {
		t.Errorf("Harness.Default = %q, want %q (empty must normalize to claude)", got.Harness.Default, "claude")
	}

	_, fetched := getSettings(t, ts.URL)
	if fetched.Harness.Default != "claude" {
		t.Errorf("GET Harness.Default = %q, want %q", fetched.Harness.Default, "claude")
	}
}

// TestSettings_POST_BodyTooLarge rejects bodies that exceed the
// MaxBytesReader cap. Acts as a DoS guard for an authenticated handler
// that otherwise decodes a JSON blob unbounded.
func TestSettings_POST_BodyTooLarge(t *testing.T) {
	ts, _ := createTestServer(t)
	huge := strings.Repeat("a", 70*1024)
	body := `{"Harness":{"Default":"claude"},"junk":"` + huge + `"}`

	resp := postSettings(t, ts.URL, body, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
}

// TestSettings_POST_RefreshesActiveHarness asserts that after a successful
// settings POST changing the default harness, a subsequent create with an
// EMPTY harness field actually spawns the new default — i.e. the in-memory
// cfg snapshot was refreshed, not just the file on disk.
func TestSettings_POST_RefreshesActiveHarness(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()
	existingAgent := domain.Agent{SessionID: "x", Session: "main", Window: 0, State: "running", Cwd: folder}

	mutate := func(c *domain.Config) {
		c.Settings.Harness.Codex.Model = "gpt-5.5"
		c.Settings.Harness.Codex.Approval = "on-request"
		c.Settings.Harness.Codex.Sandbox = "workspace-write"
	}
	ts, _ := createTestServerWithCfg(t, mutate, existingAgent)

	// 1) Flip default harness to codex via POST /api/settings.
	resp := postSettings(t, ts.URL, `{"Harness":{"Default":"codex","Codex":{"Model":"gpt-5.5","Approval":"on-request","Sandbox":"workspace-write"}}}`, true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings POST: expected 200, got %d", resp.StatusCode)
	}

	// 2) Create without specifying harness — should use the new default (codex).
	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:0", "-F", "#{pane_index}",
	).Return([]byte("0\n"), nil)

	var capturedCmd string
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:0", "-c", folder,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
		mock.MatchedBy(func(s string) bool { capturedCmd = s; return true }),
	).Return([]byte("main:0.1\n"), nil)
	m.On("Run", mock.Anything, "select-layout", "-t", "main:0", "tiled").Return(nil)

	cresp := postCreate(t, ts, `{"folder":"`+folder+`","message":"hi"}`)
	defer cresp.Body.Close()
	if cresp.StatusCode != http.StatusOK {
		t.Fatalf("create: expected 200, got %d", cresp.StatusCode)
	}

	if !strings.HasPrefix(capturedCmd, "codex ") {
		t.Errorf("captured cmd = %q, want codex spawn", capturedCmd)
	}
}
