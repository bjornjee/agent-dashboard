package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	lockSuffix     = ".lock"
	lockStaleAfter = 5 * time.Second
	lockRetryWait  = 5 * time.Millisecond
	lockMaxRetries = 400
)

// withFileLock acquires an exclusive sidecar lock for filePath, runs fn,
// then releases the lock. The lock file is filePath + ".lock" and is
// created via O_CREATE|O_EXCL — the same atomic primitive the JS writer
// uses, so Go and Node writers on the same agent file rendezvous on the
// same kernel object.
//
// A lock older than lockStaleAfter is treated as abandoned (the writer
// crashed) and unlinked. Retry budget is bounded; if the lock cannot be
// acquired within ~2s, withFileLock returns an error rather than block
// the caller indefinitely.
func withFileLock(filePath string, fn func() error) error {
	lockPath := filePath + lockSuffix
	for attempt := 0; attempt < lockMaxRetries; attempt++ {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d", os.Getpid())
			_ = f.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("open lock %s: %w", lockPath, err)
		}
		if info, statErr := os.Stat(lockPath); statErr == nil {
			if time.Since(info.ModTime()) > lockStaleAfter {
				_ = os.Remove(lockPath)
				continue
			}
		}
		time.Sleep(lockRetryWait)
	}
	return fmt.Errorf("acquire lock for %s: timed out", filePath)
}

// writeJSONAtomic writes data to filePath via tmp + rename. The rename is
// atomic on POSIX, so a concurrent reader sees either the old contents or
// the new — never a truncated/empty file. Fixes the torn-read race where
// os.WriteFile's truncate-then-write window let JS hooks observe an empty
// file and merge against {} (wiping every existing field).
func writeJSONAtomic(filePath string, data []byte) error {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d", base, os.Getpid()))
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, filePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, filePath, err)
	}
	return nil
}
