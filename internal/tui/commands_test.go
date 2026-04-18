package tui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFolder_ValidDir(t *testing.T) {
	dir := t.TempDir()
	absPath, err := validateFolder(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if absPath != dir {
		t.Errorf("expected %q, got %q", dir, absPath)
	}
}

func TestValidateFolder_Missing(t *testing.T) {
	_, err := validateFolder("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "folder not found") {
		t.Errorf("expected 'folder not found' in error, got: %v", err)
	}
	// Should contain the underlying OS error (wrapped via %w)
	if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("expected underlying OS error in message, got: %v", err)
	}
	// Verify error wrapping works with errors.Is
	if !errors.Is(err, fs.ErrNotExist) {
		t.Error("expected error to unwrap to fs.ErrNotExist")
	}
}

func TestValidateFolder_NotDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile.txt")
	if err := os.WriteFile(file, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := validateFolder(file)
	if err == nil {
		t.Fatal("expected error for file path")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected 'not a directory' in error, got: %v", err)
	}
}

func TestParseGitHubRepo(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{
			name:      "SSH URL",
			remoteURL: "git@github.com:bjornjee/agent-dashboard.git",
			wantOwner: "bjornjee",
			wantRepo:  "agent-dashboard",
			wantOK:    true,
		},
		{
			name:      "HTTPS URL",
			remoteURL: "https://github.com/bjornjee/agent-dashboard.git",
			wantOwner: "bjornjee",
			wantRepo:  "agent-dashboard",
			wantOK:    true,
		},
		{
			name:      "HTTPS without .git suffix",
			remoteURL: "https://github.com/bjornjee/agent-dashboard",
			wantOwner: "bjornjee",
			wantRepo:  "agent-dashboard",
			wantOK:    true,
		},
		{
			name:      "SSH without .git suffix",
			remoteURL: "git@github.com:bjornjee/agent-dashboard",
			wantOwner: "bjornjee",
			wantRepo:  "agent-dashboard",
			wantOK:    true,
		},
		{
			name:      "non-GitHub SSH",
			remoteURL: "git@gitlab.com:bjornjee/agent-dashboard.git",
			wantOK:    false,
		},
		{
			name:      "non-GitHub HTTPS",
			remoteURL: "https://gitlab.com/bjornjee/agent-dashboard.git",
			wantOK:    false,
		},
		{
			name:      "empty string",
			remoteURL: "",
			wantOK:    false,
		},
		{
			name:      "malformed URL",
			remoteURL: "not-a-url",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := parseGitHubRepo(tt.remoteURL)
			if owner != tt.wantOwner || repo != tt.wantRepo || ok != tt.wantOK {
				t.Errorf("parseGitHubRepo(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.remoteURL, owner, repo, ok, tt.wantOwner, tt.wantRepo, tt.wantOK)
			}
		})
	}
}

func TestBuildPRURL(t *testing.T) {
	tests := []struct {
		name   string
		owner  string
		repo   string
		base   string
		branch string
		want   string
	}{
		{
			name:   "simple branch",
			owner:  "bjornjee",
			repo:   "agent-dashboard",
			base:   "main",
			branch: "feat/auto-open-pr",
			want:   "https://github.com/bjornjee/agent-dashboard/compare/main...feat%2Fauto-open-pr?expand=1",
		},
		{
			name:   "master base",
			owner:  "bjornjee",
			repo:   "agent-dashboard",
			base:   "master",
			branch: "fix-bug",
			want:   "https://github.com/bjornjee/agent-dashboard/compare/master...fix-bug?expand=1",
		},
		{
			name:   "branch with special chars",
			owner:  "bjornjee",
			repo:   "agent-dashboard",
			base:   "main",
			branch: "feat/hello world",
			want:   "https://github.com/bjornjee/agent-dashboard/compare/main...feat%2Fhello%20world?expand=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPRURL(tt.owner, tt.repo, tt.base, tt.branch)
			if got != tt.want {
				t.Errorf("buildPRURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePRURL(t *testing.T) {
	tests := []struct {
		name    string
		owner   string
		repo    string
		base    string
		branch  string
		ghPRURL string // non-empty means a PR exists
		wantURL string
	}{
		{
			name:    "no existing PR opens compare page",
			owner:   "bjornjee",
			repo:    "agent-dashboard",
			base:    "main",
			branch:  "fix/my-bug",
			ghPRURL: "",
			wantURL: "https://github.com/bjornjee/agent-dashboard/compare/main...fix%2Fmy-bug?expand=1",
		},
		{
			name:    "existing PR opens files page",
			owner:   "bjornjee",
			repo:    "agent-dashboard",
			base:    "main",
			branch:  "fix/my-bug",
			ghPRURL: "https://github.com/bjornjee/agent-dashboard/pull/42",
			wantURL: "https://github.com/bjornjee/agent-dashboard/pull/42/files",
		},
		{
			name:    "existing PR URL with trailing slash",
			owner:   "bjornjee",
			repo:    "agent-dashboard",
			base:    "main",
			branch:  "fix/my-bug",
			ghPRURL: "https://github.com/bjornjee/agent-dashboard/pull/42/",
			wantURL: "https://github.com/bjornjee/agent-dashboard/pull/42/files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePRURL(tt.owner, tt.repo, tt.base, tt.branch, tt.ghPRURL)
			if got != tt.wantURL {
				t.Errorf("resolvePRURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestValidateFolder_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	absPath, err := validateFolder("~")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if absPath != home {
		t.Errorf("expected %q, got %q", home, absPath)
	}
}

func TestContainsTrustPrompt_Positive(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
	}{
		{"exact match", []string{"Do you trust the files in this folder?"}},
		{"surrounded by other text", []string{"", "  Do you trust the files in this folder?  ", ""}},
		{"mixed with other lines", []string{"Claude Code", "Do you trust the files in this folder?", "Yes / No"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !containsTrustPrompt(tt.lines) {
				t.Errorf("expected trust prompt to be detected in %v", tt.lines)
			}
		})
	}
}

func TestContainsTrustPrompt_Negative(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
	}{
		{"empty", nil},
		{"no match", []string{"Hello world", "Running..."}},
		{"partial match", []string{"trust the files"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if containsTrustPrompt(tt.lines) {
				t.Errorf("did not expect trust prompt to be detected in %v", tt.lines)
			}
		})
	}
}
