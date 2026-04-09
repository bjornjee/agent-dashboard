package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// execOAuthRunner is the production runner that shells out to real commands.
type execOAuthRunner struct{}

func (r *execOAuthRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// OAuthRunner abstracts command execution for credential discovery.
type OAuthRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// credentialEnvelope is the top-level JSON structure of ~/.claude/.credentials.json
// and the macOS keychain blob.
type credentialEnvelope struct {
	ClaudeAiOauth struct {
		AccessToken   string `json:"accessToken"`
		RateLimitTier string `json:"rateLimitTier"`
	} `json:"claudeAiOauth"`
}

// keychainReader reads OAuth credentials from the macOS keychain via the
// security CLI, avoiding the macOS "allow access" dialog.
type keychainReader struct {
	runner OAuthRunner
}

func (k *keychainReader) Read(ctx context.Context) (token, plan string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	out, err := k.runner.Output(ctx, "/usr/bin/security",
		"find-generic-password", "-s", "Claude Code-credentials", "-w")
	if err != nil {
		return "", "", fmt.Errorf("keychain read: %w", err)
	}

	return parseCredentials(out)
}

// fileReader reads OAuth credentials from ~/.claude/.credentials.json.
type fileReader struct {
	path string
}

func (f *fileReader) Read(_ context.Context) (token, plan string, err error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return "", "", fmt.Errorf("credentials file: %w", err)
	}
	return parseCredentials(data)
}

// parseCredentials extracts the access token and plan from raw credential JSON.
func parseCredentials(data []byte) (token, plan string, err error) {
	if len(data) == 0 {
		return "", "", fmt.Errorf("empty credentials data")
	}

	var env credentialEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return "", "", fmt.Errorf("parse credentials: %w", err)
	}
	if env.ClaudeAiOauth.AccessToken == "" {
		return "", "", fmt.Errorf("no access token in credentials")
	}

	return env.ClaudeAiOauth.AccessToken, env.ClaudeAiOauth.RateLimitTier, nil
}

// Package-level readers for test swapping.
var credReader interface {
	Read(ctx context.Context) (token, plan string, err error)
}

var fileCredReader interface {
	Read(ctx context.Context) (token, plan string, err error)
}

func init() {
	homeDir, _ := os.UserHomeDir()
	credReader = &keychainReader{runner: &execOAuthRunner{}}
	fileCredReader = &fileReader{path: filepath.Join(homeDir, ".claude", ".credentials.json")}
}

// AutoDiscoverToken tries keychain first, then credentials file.
// Returns ("", "", nil) if no credentials are available — this is not an error.
func AutoDiscoverToken(ctx context.Context) (token, plan string, err error) {
	token, plan, err = credReader.Read(ctx)
	if err == nil && token != "" {
		return token, plan, nil
	}

	token, plan, err = fileCredReader.Read(ctx)
	if err == nil && token != "" {
		return token, plan, nil
	}

	// Neither source available — graceful skip
	return "", "", nil
}
