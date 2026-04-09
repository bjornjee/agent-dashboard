package diagrams

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTempDirRoot points WriteTempHTML at a hermetic t.TempDir() for the
// duration of the test and resets the process-wide session dir so tests
// don't leak state into each other.
func withTempDirRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sessionDirMu.Lock()
	prevDir := sessionDir
	prevFn := newSessionDir
	sessionDir = dir
	newSessionDir = func() (string, error) { return dir, nil }
	sessionDirMu.Unlock()
	t.Cleanup(func() {
		sessionDirMu.Lock()
		sessionDir = prevDir
		newSessionDir = prevFn
		sessionDirMu.Unlock()
	})
	return dir
}

func TestWriteTempHTML_Deterministic(t *testing.T) {
	withTempDirRoot(t)
	d := Diagram{
		SessionID: "s",
		Hash:      "abc12345",
		Title:     "My Diagram",
		Type:      "flowchart",
		Source:    "flowchart TD\n  A --> B",
		Timestamp: time.Unix(100, 0),
	}

	p1, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write1: %v", err)
	}
	p2, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write2: %v", err)
	}
	if p1 != p2 {
		t.Errorf("expected deterministic path, got %q and %q", p1, p2)
	}
	if !strings.Contains(filepath.Base(p1), "abc12345") {
		t.Errorf("expected hash in filename, got %q", filepath.Base(p1))
	}
	if !strings.HasSuffix(p1, ".html") {
		t.Errorf("expected .html suffix, got %q", p1)
	}
	t.Cleanup(func() { os.Remove(p1) })
}

func TestWriteTempHTML_EmbedsSourceAndTitle(t *testing.T) {
	withTempDirRoot(t)
	d := Diagram{
		SessionID: "s",
		Hash:      "def67890",
		Title:     "Request Lifecycle",
		Type:      "sequenceDiagram",
		Source:    "sequenceDiagram\n  User->>API: POST /foo",
		Timestamp: time.Unix(200, 0),
	}
	p, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Cleanup(func() { os.Remove(p) })

	content, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "Request Lifecycle") {
		t.Errorf("title missing from HTML")
	}
	if !strings.Contains(s, "User-&gt;&gt;API: POST /foo") && !strings.Contains(s, "User->>API: POST /foo") {
		t.Errorf("source body missing from HTML")
	}
	if !strings.Contains(s, "mermaid") {
		t.Errorf("mermaid.js hook missing from HTML")
	}
}

// TestWriteTempHTML_BundlesMermaidJS verifies the rendered HTML references
// a same-origin sibling mermaid.min.js file (not an https:// CDN URL) and
// that the file is actually written to disk. This catches the original
// regression where the temp HTML loaded mermaid via https://, which Safari
// and some Chrome configurations block when the page is opened over file://.
func TestWriteTempHTML_BundlesMermaidJS(t *testing.T) {
	withTempDirRoot(t)
	d := Diagram{
		Hash:   "bundle01",
		Title:  "bundle test",
		Source: "flowchart TD\n  A --> B",
	}
	p, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	content, _ := os.ReadFile(p)
	s := string(content)
	if strings.Contains(s, "https://") || strings.Contains(s, "http://") {
		t.Errorf("HTML must not reference any remote URL (file:// origin would block it). got:\n%s", s)
	}
	if !strings.Contains(s, `src="`+mermaidJSFilename+`"`) {
		t.Errorf("HTML must reference sibling %s, got:\n%s", mermaidJSFilename, s)
	}
	jsPath := filepath.Join(filepath.Dir(p), mermaidJSFilename)
	info, err := os.Stat(jsPath)
	if err != nil {
		t.Fatalf("expected sibling %s at %s, got: %v", mermaidJSFilename, jsPath, err)
	}
	// Real mermaid bundle is ~3MB; a stub that only declares the global
	// is well under 1KB. Require the real thing.
	if info.Size() < 1_000_000 {
		t.Errorf("%s is suspiciously small (%d bytes) — probably a stub", mermaidJSFilename, info.Size())
	}
	// Must match the embed byte-for-byte.
	got, _ := os.ReadFile(jsPath)
	wantSum := sha256.Sum256(mermaidJS)
	gotSum := sha256.Sum256(got)
	if wantSum != gotSum {
		t.Errorf("on-disk %s does not match embedded mermaidJS", mermaidJSFilename)
	}
}

// TestWriteTempHTML_SessionDirIsProcessScoped documents the invariant that
// replaced the stale-cache bug: each process creates its own temp directory
// via os.MkdirTemp (matching `go tool cover -html` / `pprof -web`), so a
// stale or wrong mermaid.min.js left anywhere else on disk cannot poison
// the rendered HTML. Clearing the package-level session dir simulates a
// fresh process invocation; the two runs must land in different directories.
func TestWriteTempHTML_SessionDirIsProcessScoped(t *testing.T) {
	// Bypass withTempDirRoot — we want the real newSessionDir (os.MkdirTemp)
	// so we're testing the production path. Redirect TMPDIR into t.TempDir()
	// so we don't pollute the user's real /tmp.
	t.Setenv("TMPDIR", t.TempDir())
	sessionDirMu.Lock()
	prevDir := sessionDir
	sessionDir = ""
	sessionDirMu.Unlock()
	t.Cleanup(func() {
		sessionDirMu.Lock()
		sessionDir = prevDir
		sessionDirMu.Unlock()
	})

	d := Diagram{Hash: "procscp1", Source: "flowchart TD\n  A --> B"}
	p1, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write1: %v", err)
	}
	dir1 := filepath.Dir(p1)

	// Simulate a second process invocation.
	sessionDirMu.Lock()
	sessionDir = ""
	sessionDirMu.Unlock()

	p2, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write2: %v", err)
	}
	dir2 := filepath.Dir(p2)

	if dir1 == dir2 {
		t.Errorf("expected per-process session dirs, both runs landed in %s", dir1)
	}
	if !strings.Contains(filepath.Base(dir1), "agent-dashboard-diagrams-") {
		t.Errorf("session dir name should include agent-dashboard-diagrams- prefix, got %s", dir1)
	}

	// Both runs must produce the real embedded bundle, independent of any
	// file that happens to exist elsewhere. This is defense in depth — the
	// fresh dir already makes pollution impossible.
	for _, p := range []string{p1, p2} {
		body, _ := os.ReadFile(p)
		scriptIdx := strings.Index(string(body), `<script src="`)
		if scriptIdx == -1 {
			t.Fatalf("HTML missing <script src=...>:\n%s", body)
		}
		rest := string(body)[scriptIdx+len(`<script src="`):]
		end := strings.Index(rest, `"`)
		scriptName := rest[:end]
		jsPath := filepath.Join(filepath.Dir(p), scriptName)
		got, err := os.ReadFile(jsPath)
		if err != nil {
			t.Fatalf("expected sibling JS at %s: %v", jsPath, err)
		}
		if sha256.Sum256(got) != sha256.Sum256(mermaidJS) {
			t.Errorf("sibling JS at %s does not match embedded mermaidJS", jsPath)
		}
	}

	// The embedded bundle itself must export `globalThis["mermaid"]` —
	// otherwise mermaid.initialize is undefined in the browser.
	if !strings.Contains(string(mermaidJS), `globalThis["mermaid"]`) {
		t.Errorf("embedded mermaid.min.js does not assign globalThis[\"mermaid\"]; the bundle is broken")
	}
}

// TestWriteTempHTML_FitToViewportAndNoInnerScroll asserts the rendered
// HTML opens the diagram pre-scaled to fit the viewport (so wide diagrams
// aren't cropped) and that the diagram card has no internal scroll — the
// user pans/zooms the whole page instead. This guards against regressing
// back to the base template that rendered at native size behind a
// scrolling card.
func TestWriteTempHTML_FitToViewportAndNoInnerScroll(t *testing.T) {
	withTempDirRoot(t)
	d := Diagram{
		Hash:   "fit00001",
		Title:  "fit test",
		Source: "flowchart TD\n  A --> B",
	}
	p, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	body, _ := os.ReadFile(p)
	s := string(body)

	// Must have the zoom wrappers: outer tracks scaled layout size so the
	// body's scroll region matches the visible diagram; inner applies the
	// CSS transform at the SVG's intrinsic size. The two-wrapper split
	// prevents double-scaling (layout × transform) that otherwise creates
	// phantom margins on large diagrams.
	if !strings.Contains(s, `id="zoom-outer"`) {
		t.Errorf("expected #zoom-outer in rendered HTML")
	}
	if !strings.Contains(s, `id="zoom-inner"`) {
		t.Errorf("expected #zoom-inner in rendered HTML")
	}
	// Must compute fit-to-viewport on initial render and use it as default.
	if !strings.Contains(s, "computeFitZoom") {
		t.Errorf("expected computeFitZoom() in rendered HTML")
	}
	if !strings.Contains(s, "fitDefault") {
		t.Errorf("expected fitDefault assignment in rendered HTML")
	}
	// Must have zoom toolbar buttons.
	if !strings.Contains(s, `id="zoom-in"`) || !strings.Contains(s, `id="zoom-out"`) {
		t.Errorf("expected zoom toolbar buttons in rendered HTML")
	}
	// Diagram card must NOT have inner scroll: no `overflow: auto` on it.
	if strings.Contains(s, "overflow: auto") {
		t.Errorf("diagram-card must not use overflow: auto — the page itself scrolls")
	}
	// Drag-to-pan support.
	if !strings.Contains(s, "grabbing") {
		t.Errorf("expected drag-to-pan (grabbing) support in rendered HTML")
	}
	// Fit must be width-only, not min(width, height). A tall flowchart in
	// a landscape window gets squashed to unreadable 10-12% zoom when the
	// height constraint binds; GitHub-style vertical scroll is the fix.
	// Guard by asserting the computeFitZoom body references availW but
	// not availH, and does not use Math.min on two dimensions.
	fitStart := strings.Index(s, "function computeFitZoom()")
	if fitStart == -1 {
		t.Fatalf("computeFitZoom not found")
	}
	fitEnd := strings.Index(s[fitStart:], "\n    }")
	if fitEnd == -1 {
		t.Fatalf("computeFitZoom body not terminated")
	}
	fitBody := s[fitStart : fitStart+fitEnd]
	if !strings.Contains(fitBody, "availW") {
		t.Errorf("computeFitZoom must compute width availability, got:\n%s", fitBody)
	}
	if strings.Contains(fitBody, "availH") {
		t.Errorf("computeFitZoom must NOT use height availability (fit-by-width only), got:\n%s", fitBody)
	}
	if strings.Contains(fitBody, "baseH") {
		t.Errorf("computeFitZoom must NOT reference baseH (fit-by-width only), got:\n%s", fitBody)
	}
}

func TestWriteTempHTML_EscapesHTML(t *testing.T) {
	withTempDirRoot(t)
	d := Diagram{
		Hash:   "esc00001",
		Title:  "<script>alert(1)</script>",
		Source: "flowchart TD\n  A[\"<img src=x>\"]",
	}
	p, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Cleanup(func() { os.Remove(p) })

	content, _ := os.ReadFile(p)
	s := string(content)
	if strings.Contains(s, "<script>alert(1)</script>") {
		t.Errorf("title not escaped")
	}
	// The mermaid source inside <pre class="mermaid"> needs to be
	// present for mermaid.js to parse, but HTML-sensitive chars must
	// be escaped so the browser does not interpret them as tags.
	if strings.Contains(s, "<img src=x>") {
		t.Errorf("source not escaped")
	}
}
