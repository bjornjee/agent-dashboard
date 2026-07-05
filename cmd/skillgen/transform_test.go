package main

import (
	"strings"
	"testing"
)

func TestTransformConditionals(t *testing.T) {
	tests := []struct {
		name    string
		target  target
		input   string
		want    string
		wantErr string
	}{
		{
			name:   "keeps matching block and strips markers for claude",
			target: targetClaude,
			input: strings.Join([]string{
				"before",
				"<!-- claude-only -->",
				"claude",
				"<!-- /claude-only -->",
				"after",
				"",
			}, "\n"),
			want: "before\nclaude\nafter\n",
		},
		{
			name:   "drops nonmatching block for claude",
			target: targetClaude,
			input: strings.Join([]string{
				"before",
				"<!-- codex-only -->",
				"codex",
				"<!-- /codex-only -->",
				"after",
				"",
			}, "\n"),
			want: "before\nafter\n",
		},
		{
			name:   "keeps matching block and strips markers for codex",
			target: targetCodex,
			input: strings.Join([]string{
				"before",
				"<!-- codex-only -->",
				"codex",
				"<!-- /codex-only -->",
				"after",
				"",
			}, "\n"),
			want: "before\ncodex\nafter\n",
		},
		{
			name:   "drops nonmatching block for codex",
			target: targetCodex,
			input: strings.Join([]string{
				"before",
				"<!-- claude-only -->",
				"claude",
				"<!-- /claude-only -->",
				"after",
				"",
			}, "\n"),
			want: "before\nafter\n",
		},
		{
			name:    "unbalanced marker errors",
			target:  targetClaude,
			input:   "before\n<!-- claude-only -->\nmissing close\n",
			wantErr: "unclosed",
		},
		{
			name:   "nested marker errors",
			target: targetCodex,
			input: strings.Join([]string{
				"before",
				"<!-- claude-only -->",
				"outer",
				"<!-- codex-only -->",
				"inner",
				"<!-- /codex-only -->",
				"<!-- /claude-only -->",
				"after",
				"",
			}, "\n"),
			wantErr: "nested",
		},
		{
			name:   "rewrites agent-dashboard slash prefix only for codex",
			target: targetCodex,
			input:  "run /agent-dashboard:feature then /agent-dashboard:pr\n",
			want:   "run $agent-dashboard:feature then $agent-dashboard:pr\n",
		},
		{
			name:   "keeps agent-dashboard slash prefix for claude",
			target: targetClaude,
			input:  "run /agent-dashboard:feature then /agent-dashboard:pr\n",
			want:   "run /agent-dashboard:feature then /agent-dashboard:pr\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := transform([]byte(tt.input), tt.target)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("transform() error = nil, want substring %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("transform() error = %q, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("transform() unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("transform() = %q, want %q", got, tt.want)
			}
		})
	}
}
