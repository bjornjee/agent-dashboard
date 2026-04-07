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

// WriteTempHTML emits a self-contained mermaid HTML file into the OS temp
// directory and returns its absolute path. The filename is content-addressed
// by the diagram hash so reopening the same diagram reuses the same file.
func WriteTempHTML(d Diagram) (string, error) {
	hash := d.Hash
	if hash == "" {
		hash = Hash(d.Source)
	}
	name := fmt.Sprintf("agent-dashboard-diagram-%s.html", hash)
	path := filepath.Join(os.TempDir(), name)

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
