package web

import (
	"encoding/json"
	"net/http"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness"
)

// handleGetSettings returns the active settings struct as JSON.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	snap := s.snapshotCfg()
	writeJSON(w, http.StatusOK, snap.Settings)
}

// maxSettingsBodyBytes caps POST /api/settings payloads. domain.Settings is
// a small flat struct; 64 KiB is generous and bounds the memory a single
// request can buffer in the JSON decoder.
const maxSettingsBodyBytes = 64 * 1024

// handleSaveSettings validates the incoming settings, persists them to
// $stateDir/settings.toml, and refreshes the in-memory cfg.Settings +
// cfg.Harness so the next request sees the new default without a restart.
//
// Validation rule (today): the only field we type-check is
// Settings.Harness.Default — we resolve it through harness.Resolve so
// unknown names are rejected with 400. Other fields (banner, notifications,
// etc.) round-trip as-is; the read path already tolerates anything that
// decodes into the domain.Settings struct.
func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsBodyBytes)

	var req domain.Settings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		status := http.StatusBadRequest
		if _, ok := err.(*http.MaxBytesError); ok {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": "invalid JSON body"})
		return
	}

	snap := s.snapshotCfg()

	// Validate the requested default harness against the registry. We
	// always validate (even if the field was omitted and decoded to "")
	// because "" routes to claude via harness.Resolve — same fallback
	// the boot path uses, so behavior stays consistent.
	newHarness, hErr := harness.Resolve(req.Harness.Default, snap.Profile)
	if hErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": hErr.Error()})
		return
	}
	// Normalize empty default to the resolved harness name so the stored
	// file and subsequent GETs return a canonical value the create-form
	// dropdown can match against — otherwise `{"Harness":{}}` would
	// persist Default="" and leave the UI dropdown un-highlighted.
	req.Harness.Default = newHarness.Name()

	if err := config.SaveSettings(snap.Profile.StateDir, req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.cfgMu.Lock()
	s.cfg.Settings = req
	s.cfg.Harness = newHarness
	s.cfgMu.Unlock()

	writeJSON(w, http.StatusOK, req)
}
