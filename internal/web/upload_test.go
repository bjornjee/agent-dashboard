package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// uploadTestPNG is a minimal valid PNG (8-byte signature + IHDR), enough for
// http.DetectContentType to return image/png.
var uploadTestPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

// writeAgentStateFile drops an agent state file in the given dir so the
// server's lookupAgent finds it.
func writeAgentStateFile(t *testing.T, stateDir string, agent domain.Agent) {
	t.Helper()
	agentsDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentsDir, 0o700); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	data, _ := json.Marshal(agent)
	if err := os.WriteFile(filepath.Join(agentsDir, agent.SessionID+".json"), data, 0o600); err != nil {
		t.Fatalf("write agent file: %v", err)
	}
}

// buildMultipart returns a multipart body and the content-type header containing
// a single file field named "file" with the given filename + payload.
func buildMultipart(t *testing.T, fieldName, filename string, payload []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return body, w.FormDataContentType()
}

func postUpload(t *testing.T, ts *httptest.Server, agentID string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/agents/"+agentID+"/upload", body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func newUploadTestServer(t *testing.T, agent domain.Agent) *httptest.Server {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	if agent.SessionID != "" {
		writeAgentStateFile(t, cfg.Profile.StateDir, agent)
	}
	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestUpload_HappyPath_WritesIntoAgentWorkdir(t *testing.T) {
	workdir := t.TempDir()
	agent := domain.Agent{SessionID: "agent-happy", Cwd: workdir}
	ts := newUploadTestServer(t, agent)

	body, ct := buildMultipart(t, "file", "screenshot.png", uploadTestPNG)
	resp := postUpload(t, ts, agent.SessionID, body, ct)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(raw))
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	path := out["path"]
	if path == "" {
		t.Fatalf("expected non-empty path in response")
	}
	if !strings.HasPrefix(path, filepath.Join(workdir, ".uploads")+string(filepath.Separator)) {
		t.Fatalf("expected path under %s/.uploads/, got %s", workdir, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if !bytes.Equal(contents, uploadTestPNG) {
		t.Fatalf("saved file contents differ from upload")
	}
	// Filename is preserved (with timestamp prefix).
	base := filepath.Base(path)
	if !strings.HasSuffix(base, "-screenshot.png") {
		t.Fatalf("expected basename to end with -screenshot.png, got %s", base)
	}
}

func TestUpload_PrefersWorktreeCwdOverCwd(t *testing.T) {
	cwd := t.TempDir()
	worktree := t.TempDir()
	agent := domain.Agent{SessionID: "agent-worktree", Cwd: cwd, WorktreeCwd: worktree}
	ts := newUploadTestServer(t, agent)

	body, ct := buildMultipart(t, "file", "img.png", uploadTestPNG)
	resp := postUpload(t, ts, agent.SessionID, body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if !strings.HasPrefix(out["path"], filepath.Join(worktree, ".uploads")+string(filepath.Separator)) {
		t.Fatalf("expected upload under worktree, got %s", out["path"])
	}
}

func TestUpload_UnknownAgent_404(t *testing.T) {
	ts := newUploadTestServer(t, domain.Agent{})

	body, ct := buildMultipart(t, "file", "x.png", uploadTestPNG)
	resp := postUpload(t, ts, "nope", body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestUpload_AgentWithoutDir_400(t *testing.T) {
	agent := domain.Agent{SessionID: "agent-nodir"} // no Cwd, no WorktreeCwd
	ts := newUploadTestServer(t, agent)

	body, ct := buildMultipart(t, "file", "x.png", uploadTestPNG)
	resp := postUpload(t, ts, agent.SessionID, body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestUpload_MissingFile_400(t *testing.T) {
	workdir := t.TempDir()
	agent := domain.Agent{SessionID: "agent-no-file", Cwd: workdir}
	ts := newUploadTestServer(t, agent)

	// Multipart body with a different field name → no "file" field.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("notthefile", "x")
	_ = w.Close()

	resp := postUpload(t, ts, agent.SessionID, body, w.FormDataContentType())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestUpload_NonImage_415(t *testing.T) {
	workdir := t.TempDir()
	agent := domain.Agent{SessionID: "agent-non-image", Cwd: workdir}
	ts := newUploadTestServer(t, agent)

	body, ct := buildMultipart(t, "file", "notes.txt", []byte("hello world this is plain text\n"))
	resp := postUpload(t, ts, agent.SessionID, body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", resp.StatusCode)
	}
}

func TestUpload_FilenameSanitization_NoEscape(t *testing.T) {
	workdir := t.TempDir()
	agent := domain.Agent{SessionID: "agent-evil-name", Cwd: workdir}
	ts := newUploadTestServer(t, agent)

	body, ct := buildMultipart(t, "file", "../../../etc/passwd", uploadTestPNG)
	resp := postUpload(t, ts, agent.SessionID, body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	path := out["path"]
	uploads := filepath.Join(workdir, ".uploads")
	if !strings.HasPrefix(path, uploads+string(filepath.Separator)) {
		t.Fatalf("path escapes .uploads/: %s", path)
	}
	// Basename must not contain path separators.
	base := filepath.Base(path)
	if strings.ContainsAny(base, "/\\") {
		t.Fatalf("basename contains separator: %s", base)
	}
	// "etc/passwd" should not appear as a subpath.
	if strings.Contains(path, filepath.Join("etc", "passwd")) {
		t.Fatalf("path contains etc/passwd: %s", path)
	}
}

func TestUpload_Oversize_413(t *testing.T) {
	workdir := t.TempDir()
	agent := domain.Agent{SessionID: "agent-oversize", Cwd: workdir}
	ts := newUploadTestServer(t, agent)

	// 51 MB of zeroes prefixed with a PNG signature so the sniff would
	// pass; the size cap should fire first.
	big := make([]byte, 51<<20)
	copy(big, uploadTestPNG)
	body, ct := buildMultipart(t, "file", "huge.png", big)
	resp := postUpload(t, ts, agent.SessionID, body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
}

func TestUpload_MissingCSRF_403(t *testing.T) {
	workdir := t.TempDir()
	agent := domain.Agent{SessionID: "agent-csrf", Cwd: workdir}
	ts := newUploadTestServer(t, agent)

	body, ct := buildMultipart(t, "file", "x.png", uploadTestPNG)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/agents/"+agent.SessionID+"/upload", body)
	req.Header.Set("Content-Type", ct)
	// no X-Requested-With
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 from missing CSRF, got %d", resp.StatusCode)
	}
}
