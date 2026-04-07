package diagrams

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Load returns all diagrams stored for a given session, sorted latest-first
// by the timestamp encoded in each filename. A missing session directory
// is not an error — an empty slice is returned.
func Load(stateDir, sessionID string) ([]Diagram, error) {
	dir := Dir(stateDir, sessionID)
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var out []Diagram
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ts, hash, ok := ParseFilename(name)
		if !ok {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		d := NewDiagramFromBytes(sessionID, ts, data)
		d.Hash = hash // prefer hash from filename (canonical key)
		d.Path = path
		out = append(out, d)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out, nil
}

// Exists reports whether a diagram with the given hash exists in the
// session's directory (any timestamp).
func Exists(stateDir, sessionID, hash string) bool {
	dir := Dir(stateDir, sessionID)
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		_, h, ok := ParseFilename(e.Name())
		if ok && h == hash {
			return true
		}
	}
	return false
}

// Delete removes a diagram file. Missing files are not an error.
func Delete(d Diagram) error {
	if d.Path == "" {
		return nil
	}
	if err := os.Remove(d.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// CleanupSession removes the entire per-session diagrams directory.
// A missing directory is not an error.
func CleanupSession(stateDir, sessionID string) error {
	dir := Dir(stateDir, sessionID)
	if dir == "" {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
