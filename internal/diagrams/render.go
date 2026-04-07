package diagrams

import (
	_ "embed"
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

// tempDirName is the subdirectory under os.TempDir() that holds the rendered
// HTML files plus the shared mermaid.min.js asset. Using a dedicated dir
// lets each .html reference mermaid.min.js as a same-origin sibling, which
// browsers allow over file:// (unlike a https:// CDN reference, which Safari
// and Chrome can block when loaded from a file:// origin).
const tempDirName = "agent-dashboard-diagrams"

// WriteTempHTML emits a self-contained mermaid HTML file into the OS temp
// directory and returns its absolute path. The filename is content-addressed
// by the diagram hash so reopening the same diagram reuses the same file.
//
// The mermaid runtime is bundled with the binary and written once to a
// sibling `mermaid.min.js` file in the same temp directory. The HTML
// references it via a relative path so the browser can load it over
// `file://` without CORS or mixed-content restrictions.
func WriteTempHTML(d Diagram) (string, error) {
	hash := d.Hash
	if hash == "" {
		hash = Hash(d.Source)
	}
	dir := filepath.Join(os.TempDir(), tempDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	jsPath := filepath.Join(dir, "mermaid.min.js")
	if _, err := os.Stat(jsPath); os.IsNotExist(err) {
		if err := os.WriteFile(jsPath, mermaidJS, 0o644); err != nil {
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

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
