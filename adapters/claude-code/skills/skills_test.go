package skills_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// skillsDir returns the absolute path to the skills directory adjacent to this test file.
func skillsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Dir(thisFile)
}

// TestImplementationSkills_ContainDelegationGate verifies that skills with
// implementation phases include a Codex delegation gate that references
// /codex-delegate rather than duplicating the delegation protocol inline.
func TestImplementationSkills_ContainDelegationGate(t *testing.T) {
	dir := skillsDir(t)

	skills := []string{"fix", "feature", "refactor"}

	for _, name := range skills {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name, "SKILL.md")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("cannot read %s: %v", path, err)
			}
			content := string(data)

			if !strings.Contains(content, "codex --version") {
				t.Errorf("%s/SKILL.md missing Codex availability check (codex --version)", name)
			}
			if !strings.Contains(content, "/codex-delegate") {
				t.Errorf("%s/SKILL.md missing delegation reference (/codex-delegate)", name)
			}
		})
	}
}
