package tui

import (
	"os"
	"strings"
	"testing"
)

func TestVersionFileNotEmpty(t *testing.T) {
	// Verify the VERSION file at the project root is non-empty.
	// The Makefile uses: git describe ... || cat VERSION
	// so this file is the fallback that must always contain a valid version.
	data, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Skipf("VERSION file not readable: %v", err)
	}
	val := strings.TrimSpace(string(data))
	if val == "" {
		t.Fatal("VERSION file is empty; ldflags will inject an empty version")
	}
	t.Logf("VERSION = %q", val)
}
