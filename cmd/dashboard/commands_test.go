package main

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
