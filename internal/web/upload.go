package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	uploadMaxBytes   = 50 << 20
	uploadMemBytes   = 10 << 20
	uploadFieldName  = "file"
	uploadSubdir     = ".uploads"
	uploadFallbackFN = "upload.bin"
)

// handleUpload accepts a multipart image upload from the mobile client and
// stores it under <agent-effective-dir>/.uploads/<unixnano>-<sanitized-name>.
// Returns {"path": "<absolute path>"} on success — same shape as
// /api/file-picker so the client-side insertion code is identical.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.lookupAgent(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	dir := agent.EffectiveDir()
	if dir == "" || !filepath.IsAbs(dir) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent has no valid working directory"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, uploadMaxBytes)
	if err := r.ParseMultipartForm(uploadMemBytes); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "upload exceeds 50 MB limit"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form: " + err.Error()})
		return
	}

	file, hdr, err := r.FormFile(uploadFieldName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing form field \"file\""})
		return
	}
	defer file.Close()

	sniff := make([]byte, 512)
	n, err := io.ReadFull(file, sniff)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read upload: " + err.Error()})
		return
	}
	contentType := http.DetectContentType(sniff[:n])
	if !strings.HasPrefix(contentType, "image/") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "only image uploads are accepted"})
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "rewind upload: " + err.Error()})
		return
	}

	uploadsDir := filepath.Join(dir, uploadSubdir)
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mkdir uploads: " + err.Error()})
		return
	}

	name := sanitizeUploadFilename(hdr.Filename)
	dest := filepath.Join(uploadsDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), name))

	out, err := os.Create(dest)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create file: " + err.Error()})
		return
	}

	if _, err := io.Copy(out, file); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write file: " + err.Error()})
		return
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "close file: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": dest})
}

// sanitizeUploadFilename keeps only the basename, strips characters outside
// [A-Za-z0-9._-], and falls back to a placeholder if nothing usable remains.
// Always returns a single path segment — never escapes the uploads dir.
func sanitizeUploadFilename(name string) string {
	name = filepath.Base(name)
	if name == "." || name == "/" || name == "\\" {
		return uploadFallbackFN
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return uploadFallbackFN
	}
	return out
}
