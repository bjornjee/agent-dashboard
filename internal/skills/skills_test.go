package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeSkill(t *testing.T, skillsDir, name string) {
	t.Helper()
	dir := filepath.Join(skillsDir, name)
	mkdirAll(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverSkills_ValidDir(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "agent-dashboard", "agent-dashboard", "0.22.1", "skills")
	for _, name := range []string{"feature", "fix", "chore"} {
		writeSkill(t, skillsDir, name)
	}
	got := DiscoverSkills(tmp)
	want := []string{"chore", "feature", "fix"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverSkills_NoDir(t *testing.T) {
	got := DiscoverSkills("/nonexistent/path")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestDiscoverSkills_EmptySkillsDir(t *testing.T) {
	tmp := t.TempDir()
	mkdirAll(t, filepath.Join(tmp, "agent-dashboard", "agent-dashboard", "1.0.0", "skills"))
	got := DiscoverSkills(tmp)
	if got != nil {
		t.Errorf("expected nil for empty skills dir, got %v", got)
	}
}

func TestDiscoverSkills_VersionOrdering(t *testing.T) {
	tmp := t.TempDir()
	writeSkill(t, filepath.Join(tmp, "agent-dashboard", "agent-dashboard", "0.9.0", "skills"), "old")
	writeSkill(t, filepath.Join(tmp, "agent-dashboard", "agent-dashboard", "0.22.1", "skills"), "new")

	got := DiscoverSkills(tmp)
	want := []string{"new"}
	if len(got) != 1 || got[0] != "new" {
		t.Errorf("expected %v from latest version, got %v", want, got)
	}
}

func TestDiscoverSkills_RequiresSkillManifest(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "agent-dashboard", "agent-dashboard", "0.22.1", "skills")
	writeSkill(t, skillsDir, "feature")
	mkdirAll(t, filepath.Join(skillsDir, "_shared"))
	mkdirAll(t, filepath.Join(skillsDir, ".cache"))
	mkdirAll(t, filepath.Join(skillsDir, "notes"))

	got := DiscoverSkills(tmp)
	want := []string{"feature"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestBuildSkillList(t *testing.T) {
	got := BuildSkillList([]string{"chore", "feature", "fix"})
	want := []string{"(none)", "chore", "feature", "fix"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildSkillList_Empty(t *testing.T) {
	got := BuildSkillList(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// Codex agents must surface skills installed under the codex plugin
// cache (~/.codex/plugins/cache), and must hide skills that the dashboard
// blocks for codex via SupportsHarness — implement and rca rely on
// claude-only orchestration primitives.
func TestDiscoverSkillsForHarness_CodexScansCodexCache(t *testing.T) {
	claudeCache := t.TempDir()
	codexCache := t.TempDir()
	writeSkill(t, filepath.Join(claudeCache, "agent-dashboard", "agent-dashboard", "0.32.0", "skills"), "claude-only-skill")
	codexSkills := filepath.Join(codexCache, "agent-dashboard", "agent-dashboard", "0.32.0", "skills")
	for _, name := range []string{"feature", "fix", "implement", "rca", "pr"} {
		writeSkill(t, codexSkills, name)
	}

	got := DiscoverSkillsForHarness(claudeCache, codexCache, "codex")
	// implement + rca are blocked for codex; pr/feature/fix must remain.
	want := []string{"feature", "fix", "pr"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverSkillsForHarness_ClaudeUsesClaudeCache(t *testing.T) {
	claudeCache := t.TempDir()
	codexCache := t.TempDir()
	for _, name := range []string{"feature", "fix", "implement"} {
		writeSkill(t, filepath.Join(claudeCache, "agent-dashboard", "agent-dashboard", "0.32.0", "skills"), name)
	}
	writeSkill(t, filepath.Join(codexCache, "agent-dashboard", "agent-dashboard", "0.32.0", "skills"), "codex-only-skill")

	got := DiscoverSkillsForHarness(claudeCache, codexCache, "claude")
	// implement stays — only codex blocks it.
	want := []string{"feature", "fix", "implement"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverSkillsForHarness_EmptyHarnessFallsBackToClaude(t *testing.T) {
	claudeCache := t.TempDir()
	writeSkill(t, filepath.Join(claudeCache, "agent-dashboard", "agent-dashboard", "0.32.0", "skills"), "feature")
	got := DiscoverSkillsForHarness(claudeCache, "", "")
	if len(got) != 1 || got[0] != "feature" {
		t.Errorf("got %v, want [feature]", got)
	}
}

func Test_compareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.9.0", "0.22.1", -1},
		{"0.22.1", "0.9.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.99.99", 1},
	}
	for _, tt := range tests {
		got := compareSemver(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
