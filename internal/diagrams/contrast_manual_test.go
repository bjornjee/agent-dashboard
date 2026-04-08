//go:build manual

package diagrams

import (
	"fmt"
	"testing"
)

// TestManualRenderContrastFixture writes a diagram with mixed light/dark
// classDef fills (yellow warn, cream note, pink error, dark default) and
// prints the resulting HTML path. Intended to be opened in a real browser
// (or Playwright) to eyeball the per-node text contrast. Not part of the
// normal test run — gated by `-tags manual`.
//
// Run: go test -tags manual -run TestManualRenderContrastFixture ./internal/diagrams/
func TestManualRenderContrastFixture(t *testing.T) {
	d := Diagram{
		Hash:  "contrast-fixture",
		Title: "Contrast fixture — mixed classDef fills",
		Source: `flowchart TD
  A[Default dark node] --> B[Another default]
  B --> C(Rounded default)
  C --> D{Decision?}
  D -->|yes| E[[Storage cylinder]]
  D -->|no| F((Circle))
  E --> G[/Input slash/]
  F --> H[\Output backslash\]
  G --> W[Warning state]
  H --> X[Error state]
  W --> S[Success state]
  X --> S

  classDef warn fill:#f9e2af,stroke:#f9c74f,color:#000
  classDef err  fill:#f38ba8,stroke:#f97583,color:#000
  classDef ok   fill:#a6e3a1,stroke:#40c057,color:#000
  classDef cream fill:#fef3c7,stroke:#fbbf24,color:#78350f

  class W warn
  class X err
  class S ok
  class E cream`,
	}
	path, err := WriteTempHTML(d)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	fmt.Printf("\n=== MANUAL CONTRAST FIXTURE ===\nfile://%s\n\n", path)
}
