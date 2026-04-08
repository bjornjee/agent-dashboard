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
)

//go:embed template.html
var mermaidTemplateHTML string

//go:embed mermaid.min.js
var mermaidJS []byte

// tempDirName is the subdirectory under tempDirRoot() that holds the rendered
// HTML files plus the shared mermaid.min.js asset. Using a dedicated dir
// lets each .html reference mermaid.min.js as a same-origin sibling, which
// browsers allow over file:// (unlike a https:// CDN reference, which Safari
// and Chrome can block when loaded from a file:// origin).
const tempDirName = "agent-dashboard-diagrams"

// tempDirRoot returns the OS temp-dir parent under which tempDirName lives.
// It is a var so tests can swap it for a hermetic t.TempDir().
var tempDirRoot = os.TempDir

// mermaidJSFilename is content-addressed to the embedded bundle bytes so
// any stale or wrong copy sitting in the cache dir cannot collide with it.
// This is the fix for the regression where a hand-rolled stub (or older
// bundled version) stayed cached forever because the writer only ran when
// the file was missing.
var mermaidJSFilename = func() string {
	sum := sha256.Sum256(mermaidJS)
	return fmt.Sprintf("mermaid-%s.min.js", hex.EncodeToString(sum[:4]))
}()

// WriteTempHTML emits a self-contained mermaid HTML file into the OS temp
// directory and returns its absolute path. The filename is content-addressed
// by the diagram hash so reopening the same diagram reuses the same file.
//
// The mermaid runtime is bundled with the binary and written once to a
// content-addressed sibling file (mermaid-<sha8>.min.js) in the same temp
// directory. The HTML references it via a relative path so the browser can
// load it over `file://` without CORS or mixed-content restrictions. A new
// bundled version (different bytes) gets a new filename automatically, so
// older cached copies are harmlessly orphaned instead of poisoning the cache.
func WriteTempHTML(d Diagram) (string, error) {
	hash := d.Hash
	if hash == "" {
		hash = Hash(d.Source)
	}
	dir := filepath.Join(tempDirRoot(), tempDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
