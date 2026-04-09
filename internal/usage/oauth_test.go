package usage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeKeychainRunner implements the runner interface for testing.
type fakeKeychainRunner struct {
	output []byte
	err    error
}

func (f *fakeKeychainRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return f.output, f.err
}

func TestKeychainReader_Success(t *testing.T) {
	creds := `{"claudeAiOauth":{"accessToken":"sk-ant-test-token","refreshToken":"rt","expiresAt":9999999999999,"scopes":["user:profile"],"rateLimitTier":"max"}}`
	r := &keychainReader{runner: &fakeKeychainRunner{output: []byte(creds)}}

	token, plan, err := r.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "sk-ant-test-token" {
		t.Errorf("token: got %q, want %q", token, "sk-ant-test-token")
	}
	if plan != "max" {
		t.Errorf("plan: got %q, want %q", plan, "max")
	}
}

func TestKeychainReader_EmptyOutput(t *testing.T) {
	r := &keychainReader{runner: &fakeKeychainRunner{output: []byte("")}}

	token, _, err := r.Read(context.Background())
	if err == nil {
		t.Fatal("expected error for empty output")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestKeychainReader_MalformedJSON(t *testing.T) {
	r := &keychainReader{runner: &fakeKeychainRunner{output: []byte("not json")}}

	token, _, err := r.Read(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestKeychainReader_MissingToken(t *testing.T) {
	creds := `{"claudeAiOauth":{"refreshToken":"rt"}}`
	r := &keychainReader{runner: &fakeKeychainRunner{output: []byte(creds)}}

	token, _, err := r.Read(context.Background())
	if err == nil {
		t.Fatal("expected error for missing access token")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestKeychainReader_CommandError(t *testing.T) {
	r := &keychainReader{runner: &fakeKeychainRunner{err: os.ErrNotExist}}

	token, _, err := r.Read(context.Background())
	if err == nil {
		t.Fatal("expected error for command failure")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestFileReader_Success(t *testing.T) {
	tmp := t.TempDir()
	creds := `{"claudeAiOauth":{"accessToken":"file-token","rateLimitTier":"pro"}}`
	if err := os.WriteFile(filepath.Join(tmp, ".credentials.json"), []byte(creds), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	r := &fileReader{path: filepath.Join(tmp, ".credentials.json")}
	token, plan, err := r.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "file-token" {
		t.Errorf("token: got %q, want %q", token, "file-token")
	}
	if plan != "pro" {
		t.Errorf("plan: got %q, want %q", plan, "pro")
	}
}

func TestFileReader_MissingFile(t *testing.T) {
	r := &fileReader{path: "/nonexistent/.credentials.json"}
	token, _, err := r.Read(context.Background())
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestAutoDiscoverToken_KeychainFirst(t *testing.T) {
	creds := `{"claudeAiOauth":{"accessToken":"keychain-token","rateLimitTier":"max"}}`
	orig := credReader
	credReader = &keychainReader{runner: &fakeKeychainRunner{output: []byte(creds)}}
	t.Cleanup(func() { credReader = orig })

	token, plan, err := AutoDiscoverToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "keychain-token" {
		t.Errorf("token: got %q, want %q", token, "keychain-token")
	}
	if plan != "max" {
		t.Errorf("plan: got %q, want %q", plan, "max")
	}
}

func TestAutoDiscoverToken_FallbackToFile(t *testing.T) {
	tmp := t.TempDir()
	creds := `{"claudeAiOauth":{"accessToken":"file-token","rateLimitTier":"team"}}`
	credPath := filepath.Join(tmp, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(creds), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	orig := credReader
	origFile := fileCredReader
	credReader = &keychainReader{runner: &fakeKeychainRunner{err: os.ErrNotExist}}
	fileCredReader = &fileReader{path: credPath}
	t.Cleanup(func() { credReader = orig; fileCredReader = origFile })

	token, plan, err := AutoDiscoverToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "file-token" {
		t.Errorf("token: got %q, want %q", token, "file-token")
	}
	if plan != "team" {
		t.Errorf("plan: got %q, want %q", plan, "team")
	}
}

func TestAutoDiscoverToken_NoneAvailable(t *testing.T) {
	orig := credReader
	origFile := fileCredReader
	credReader = &keychainReader{runner: &fakeKeychainRunner{err: os.ErrNotExist}}
	fileCredReader = &fileReader{path: "/nonexistent/.credentials.json"}
	t.Cleanup(func() { credReader = orig; fileCredReader = origFile })

	token, plan, err := AutoDiscoverToken(context.Background())
	if err != nil {
		t.Fatal("expected no error when both sources unavailable")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
	if plan != "" {
		t.Errorf("expected empty plan, got %q", plan)
	}
}
