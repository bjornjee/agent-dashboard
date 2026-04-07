package diagrams

import (
	"strings"
	"testing"
	"time"
)

func TestHash_DeterministicAndShort(t *testing.T) {
	src := "flowchart TD\n  A --> B\n"
	h1 := Hash(src)
	h2 := Hash(src)
	if h1 != h2 {
		t.Fatalf("Hash not deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 8 {
		t.Fatalf("expected 8-char hash, got %q (len=%d)", h1, len(h1))
	}
}

func TestHash_DistinctForDifferentSources(t *testing.T) {
	a := Hash("flowchart TD\n  A --> B")
	b := Hash("flowchart TD\n  A --> C")
	if a == b {
		t.Fatalf("expected distinct hashes for different sources, got %q", a)
	}
}

func TestHash_SensitiveToWhitespace(t *testing.T) {
	// Unlike "normalized" hashes, we want byte-identical dedup only.
	// A trailing newline difference should yield a new hash.
	a := Hash("flowchart TD\n  A --> B")
	b := Hash("flowchart TD\n  A --> B\n")
	if a == b {
		t.Fatalf("expected whitespace-sensitive hash, got same: %q", a)
	}
}

func TestFilename_TimestampAndHash(t *testing.T) {
	ts := time.Unix(1733601245, 0)
	got := Filename(ts, "a1b2c3d4")
	want := "1733601245-a1b2c3d4.mmd"
	if got != want {
		t.Fatalf("Filename: got %q want %q", got, want)
	}
}

func TestParseType(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"flowchart", "flowchart TD\n  A --> B", "flowchart"},
		{"sequence", "sequenceDiagram\n  A->>B: Hi", "sequenceDiagram"},
		{"state", "stateDiagram-v2\n  [*] --> S1", "stateDiagram-v2"},
		{"class", "classDiagram\n  Foo <|-- Bar", "classDiagram"},
		{"er", "erDiagram\n  CUSTOMER ||--o{ ORDER : places", "erDiagram"},
		{"gantt", "gantt\n  title A", "gantt"},
		{"title-line-skipped", "%% title: My Diagram\nflowchart TD\n  A --> B", "flowchart"},
		{"frontmatter-skipped", "---\ntitle: My\n---\nflowchart TD\n  A", "flowchart"},
		{"empty", "", "unknown"},
		{"whitespace-leading", "   \n\nflowchart TD", "flowchart"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseType(tc.src)
			if got != tc.want {
				t.Errorf("ParseType(%q): got %q want %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestParseTitle(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "comment-title",
			src:  "%% title: Request Lifecycle\nflowchart TD\n  A --> B",
			want: "Request Lifecycle",
		},
		{
			name: "comment-title-with-extra-spaces",
			src:  "%%  title:   Spaced Out  \nflowchart TD",
			want: "Spaced Out",
		},
		{
			name: "frontmatter-title",
			src:  "---\ntitle: Frontmatter Title\n---\nflowchart TD",
			want: "Frontmatter Title",
		},
		{
			name: "no-title-fallback",
			src:  "flowchart TD\n  A --> B",
			want: "",
		},
		{
			name: "comment-not-title",
			src:  "%% some comment\nflowchart TD",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseTitle(tc.src)
			if got != tc.want {
				t.Errorf("ParseTitle(%q): got %q want %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestDir(t *testing.T) {
	got := Dir("/tmp/state", "abc-123")
	want := "/tmp/state/diagrams/abc-123"
	if got != want {
		t.Fatalf("Dir: got %q want %q", got, want)
	}
}

func TestParseFilename(t *testing.T) {
	ts, hash, ok := ParseFilename("1733601245-a1b2c3d4.mmd")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if hash != "a1b2c3d4" {
		t.Errorf("hash: got %q want %q", hash, "a1b2c3d4")
	}
	if ts.Unix() != 1733601245 {
		t.Errorf("ts: got %d want %d", ts.Unix(), 1733601245)
	}

	for _, bad := range []string{
		"",
		"noprefix.mmd",
		"1733601245.mmd",
		"abc-xyz.mmd",
		"1733601245-a1b2c3d4.txt",
	} {
		if _, _, ok := ParseFilename(bad); ok {
			t.Errorf("ParseFilename(%q) expected ok=false", bad)
		}
	}
}

func TestNewDiagramFromBytes(t *testing.T) {
	src := "%% title: My Flow\nflowchart TD\n  A --> B\n"
	ts := time.Unix(1733600000, 0)
	d := NewDiagramFromBytes("sess-1", ts, []byte(src))

	if d.SessionID != "sess-1" {
		t.Errorf("SessionID: got %q", d.SessionID)
	}
	if !strings.HasPrefix(d.Hash, "") || len(d.Hash) != 8 {
		t.Errorf("Hash: got %q", d.Hash)
	}
	if d.Title != "My Flow" {
		t.Errorf("Title: got %q want %q", d.Title, "My Flow")
	}
	if d.Type != "flowchart" {
		t.Errorf("Type: got %q want %q", d.Type, "flowchart")
	}
	if d.Source != src {
		t.Errorf("Source mismatch")
	}
	if d.Timestamp.Unix() != ts.Unix() {
		t.Errorf("Timestamp: got %d want %d", d.Timestamp.Unix(), ts.Unix())
	}
}
