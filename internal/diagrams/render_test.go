package diagrams

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteTempHTML_Deterministic(t *testing.T) {
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

func TestWriteTempHTML_EscapesHTML(t *testing.T) {
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
