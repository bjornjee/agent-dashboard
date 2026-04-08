package diagrams

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTempDirRoot redirects WriteTempHTML's temp-dir parent to t.TempDir()
// for the duration of the test, so each test gets a hermetic dir and tests
// cannot pollute the user's real /tmp/agent-dashboard-diagrams.
func withTempDirRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := tempDirRoot
	tempDirRoot = func() string { return dir }
	t.Cleanup(func() { tempDirRoot = prev })
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

// TestWriteTempHTML_RecoversFromStaleMermaidJS is the regression test for
// the bug where a stale or wrong mermaid.min.js sitting in the temp dir
// (e.g. from an older binary or a hand-rolled stub) was reused forever
// because the writer only ran when the file was missing. After the fix,
// the on-disk file referenced by the rendered HTML must be byte-identical
// to the embedded mermaidJS.
func TestWriteTempHTML_RecoversFromStaleMermaidJS(t *testing.T) {
	dir := withTempDirRoot(t)
	cacheDir := filepath.Join(dir, tempDirName)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Plant a stub at the legacy path to simulate the broken cache.
	stub := []byte("/* fake mermaid bundle for tests */ var mermaid = {};\n")
	legacyPath := filepath.Join(cacheDir, "mermaid.min.js")
	if err := os.WriteFile(legacyPath, stub, 0o644); err != nil {
		t.Fatal(err)
	}

	d := Diagram{
		Hash:   "stale001",
		Title:  "stale cache test",
		Source: "flowchart TD\n  A --> B",
	}
	htmlPath, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	body, _ := os.ReadFile(htmlPath)
	bodyStr := string(body)

	// HTML must reference some sibling JS file (the exact name may be
	// content-addressed; we discover it from the script tag).
	scriptIdx := strings.Index(bodyStr, `<script src="`)
	if scriptIdx == -1 {
		t.Fatalf("HTML missing <script src=...>:\n%s", bodyStr)
	}
	rest := bodyStr[scriptIdx+len(`<script src="`):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		t.Fatalf("HTML script tag malformed:\n%s", bodyStr)
	}
	scriptName := rest[:end]
	if scriptName == "" {
		t.Fatalf("empty script src in HTML")
	}

	// The referenced sibling file must be the real bundle.
	jsPath := filepath.Join(filepath.Dir(htmlPath), scriptName)
	got, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("expected sibling JS at %s, got: %v", jsPath, err)
	}
	wantSum := sha256.Sum256(mermaidJS)
	gotSum := sha256.Sum256(got)
	if wantSum != gotSum {
		t.Errorf("sibling JS does not match embedded mermaidJS\n want sha256=%s (%d bytes)\n  got sha256=%s (%d bytes)",
			hex.EncodeToString(wantSum[:]), len(mermaidJS),
			hex.EncodeToString(gotSum[:]), len(got))
	}

	// Defense in depth: the embed itself must export `globalThis["mermaid"]`,
	// otherwise mermaid.initialize will be undefined in the browser even with
	// a fresh cache.
	if !strings.Contains(string(mermaidJS), `globalThis["mermaid"]`) {
		t.Errorf("embedded mermaid.min.js does not assign globalThis[\"mermaid\"]; the bundle is broken")
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
