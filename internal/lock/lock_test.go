package lock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireLock_Success(t *testing.T) {
	dir := t.TempDir()
	f, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer f.Close()

	// Lock file should exist with our PID
	lockPath := filepath.Join(dir, "dashboard.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	if !strings.Contains(string(data), "\n") {
		t.Error("lock file should contain PID followed by newline")
	}
}

func TestAcquireLock_SecondInstanceBlocked(t *testing.T) {
	dir := t.TempDir()

	// First instance acquires lock
	f1, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer f1.Close()

	// Second instance should fail
	f2, err := AcquireLock(dir)
	if err == nil {
		f2.Close()
		t.Fatal("expected error for second instance, got nil")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("expected 'already running' error, got: %v", err)
	}
}

func TestAcquireLock_ReleasedAfterClose(t *testing.T) {
	dir := t.TempDir()

	// Acquire and release
	f1, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	f1.Close()

	// Should succeed after release
	f2, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("second lock after release: %v", err)
	}
	defer f2.Close()
}
