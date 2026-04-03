package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestMakefileVersionNotEmpty(t *testing.T) {
	// Verify the Makefile VERSION variable resolves to a non-empty value.
	// This catches the bug where `git describe | sed` succeeds with empty
	// output, causing the `|| cat VERSION` fallback to never trigger.
	cmd := exec.Command("make", "-n", "-p")
	cmd.Dir = "../../" // project root from cmd/dashboard/
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("make not available: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "VERSION :=") || strings.HasPrefix(line, "VERSION =") {
			val := strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
			if val == "" {
				t.Fatal("Makefile VERSION variable resolved to empty string; ldflags will inject an empty version")
			}
			t.Logf("VERSION = %q", val)
			return
		}
	}
	t.Skip("VERSION variable not found in Makefile output")
}
