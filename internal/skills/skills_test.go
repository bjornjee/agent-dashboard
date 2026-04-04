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

func TestDiscoverSkills_ValidDir(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "agent-dashboard", "agent-dashboard", "0.22.1", "skills")
	for _, name := range []string{"feature", "fix", "chore"} {
		mkdirAll(t, filepath.Join(skillsDir, name))
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
	mkdirAll(t, filepath.Join(tmp, "agent-dashboard", "agent-dashboard", "0.9.0", "skills", "old"))
	mkdirAll(t, filepath.Join(tmp, "agent-dashboard", "agent-dashboard", "0.22.1", "skills", "new"))

	got := DiscoverSkills(tmp)
	want := []string{"new"}
	if len(got) != 1 || got[0] != "new" {
		t.Errorf("expected %v from latest version, got %v", want, got)
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
