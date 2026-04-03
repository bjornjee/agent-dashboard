package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// acquireLock attempts to acquire an exclusive lock on a file in stateDir.
// Returns the lock file (caller must defer Close) or an error if another
// dashboard instance is already running.
func acquireLock(stateDir string) (*os.File, error) {
	lockPath := filepath.Join(stateDir, "dashboard.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	// Try non-blocking exclusive lock
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Read PID from lock file for a helpful error message
		pid := readLockPID(f)
		f.Close()
		if pid > 0 {
			return nil, fmt.Errorf("another dashboard is already running (pid %d)", pid)
		}
		return nil, fmt.Errorf("another dashboard is already running")
	}

	// Write our PID to the lock file
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Sync()

	return f, nil
}

func readLockPID(f *os.File) int {
	_, _ = f.Seek(0, 0)
	buf := make([]byte, 32)
	n, _ := f.Read(buf)
	if n == 0 {
		return 0
	}
	// Trim newline
	s := string(buf[:n])
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	pid, _ := strconv.Atoi(s)
	return pid
}
