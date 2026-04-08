package diagrams

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed template.html
var mermaidTemplateHTML string

//go:embed mermaid.min.js
var mermaidJS []byte

// mermaidJSFilename is content-addressed to the embedded bundle bytes.
// Combined with per-process session directories this is defense in depth:
// even if two runs somehow shared a directory, different bundles wouldn't
// collide on the JS filename.
var mermaidJSFilename = func() string {
	sum := sha256.Sum256(mermaidJS)
	return fmt.Sprintf("mermaid-%s.min.js", hex.EncodeToString(sum[:4]))
}()

// Session directory strategy
// ─────────────────────────────────────────────────────────────────────────
// Rendered HTMLs and the bundled mermaid.min.js are written into a
// per-process temp directory created lazily via os.MkdirTemp on the first
// WriteTempHTML call. This matches the pattern used by `go tool cover
// -html` and `pprof -web` — both generate one-shot HTML that the user's
// browser loads and forgets. Advantages over a fixed shared directory:
//
//   1. No cross-process collision: a stale bundle left by an older binary
//      cannot poison a new process because each process gets its own dir.
//      This was the origin of the "mermaid renders as plain text" bug.
//   2. No custom reaper: unique dirs under the OS temp are swept by the
//      system's normal temp-cleanup policy.
//   3. No cache coherence problem: content-addressing and atomic writes
//      are defense in depth, not the primary correctness mechanism.
//
// Reusing the same dir for every diagram within a single process keeps
// repeated opens fast (write the 3MB bundle once) and lets the browser
// dedupe tabs for the same diagram hash.
var (
	sessionDirMu sync.Mutex
	sessionDir   string
)

// newSessionDir creates the per-process temp directory. It's a var so
// tests can swap it for a hermetic t.TempDir().
var newSessionDir = func() (string, error) {
	return os.MkdirTemp("", "agent-dashboard-diagrams-")
}

func getOrCreateSessionDir() (string, error) {
	sessionDirMu.Lock()
	defer sessionDirMu.Unlock()
	if sessionDir != "" {
		if _, err := os.Stat(sessionDir); err == nil {
			return sessionDir, nil
		}
		// Dir was swept out from under us (unlikely, but possible on
		// long-running processes on aggressive tmpfs). Fall through and
		// recreate so we stay usable.
	}
	dir, err := newSessionDir()
	if err != nil {
		return "", err
	}
	sessionDir = dir
	return sessionDir, nil
}

// WriteTempHTML emits a self-contained mermaid HTML file into the
// per-process session directory and returns its absolute path. The
// filename is content-addressed by the diagram hash so reopening the
// same diagram within a single process reuses the same file.
//
// The mermaid runtime is bundled with the binary and written once per
// process to a content-addressed sibling file (mermaid-<sha8>.min.js) in
// the same directory. The HTML references it via a relative path so the
// browser can load it over `file://` without CORS or mixed-content
// restrictions.
func WriteTempHTML(d Diagram) (string, error) {
	hash := d.Hash
	if hash == "" {
		hash = Hash(d.Source)
	}
	dir, err := getOrCreateSessionDir()
	if err != nil {
		return "", err
	}
	jsPath := filepath.Join(dir, mermaidJSFilename)
	if _, err := os.Stat(jsPath); os.IsNotExist(err) {
		// Atomic write so a crashed mid-write doesn't leave a truncated file
		// that future runs would skip (stat-then-skip).
		tmp := jsPath + ".tmp"
		if err := os.WriteFile(tmp, mermaidJS, 0o644); err != nil {
			return "", err
		}
		if err := os.Rename(tmp, jsPath); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
	}
	name := fmt.Sprintf("agent-dashboard-diagram-%s.html", hash)
	path := filepath.Join(dir, name)

	// Title goes inside <title> and <h1> as HTML text — full escape.
	// Source goes inside <pre class="mermaid">, which mermaid.js reads via
	// textContent. The browser must round-trip the source unchanged so
	// mermaid sees the original characters. We must escape &, <, > so the
	// HTML parser doesn't choke on them (e.g. `User->>API` mid-tag), but
	// we must NOT escape `"` or `'` — html.EscapeString turns `"` into
	// `&#34;`, which the browser decodes back to `&#34;` in textContent
	// (not `"`), breaking node labels like `A["label"]`.
	sourceEscaper := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	body := mermaidTemplateHTML
	body = strings.ReplaceAll(body, "{{TITLE}}", html.EscapeString(d.Title))
	body = strings.ReplaceAll(body, "{{SOURCE}}", sourceEscaper.Replace(d.Source))
	body = strings.ReplaceAll(body, "{{MERMAID_JS}}", mermaidJSFilename)

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
