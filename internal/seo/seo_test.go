package seo

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot walks up from the test working directory until it finds the
// directory containing go.mod. The seo tests intentionally read project-wide
// static artifacts (docs/, README.md, .claude-plugin/) so they need a stable
// anchor independent of Go's per-package working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not locate repo root (no go.mod found walking up)")
	return ""
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// userAgentBlock returns the slice of robots.txt covering one User-agent
// directive: from "User-agent: <bot>\n" up to the next "User-agent:" line or
// EOF. The trailing newline anchor prevents bot prefix collisions
// (e.g. "Applebot" matching "Applebot-Extended").
func userAgentBlock(robots, bot string) string {
	header := "User-agent: " + bot + "\n"
	idx := strings.Index(robots, header)
	if idx < 0 {
		return ""
	}
	rest := robots[idx:]
	if next := strings.Index(rest[len(header):], "\nUser-agent:"); next >= 0 {
		return rest[:len(header)+next]
	}
	return rest
}

func TestRobotsBlocksTrainingBots(t *testing.T) {
	robots := readFile(t, filepath.Join(repoRoot(t), "docs", "robots.txt"))
	blocked := []string{
		"GPTBot",
		"Google-Extended",
		"CCBot",
		"anthropic-ai",
		"Applebot-Extended",
		"Bytespider",
		"Meta-ExternalAgent",
		"cohere-ai",
		"Diffbot",
	}
	for _, bot := range blocked {
		block := userAgentBlock(robots, bot)
		if block == "" {
			t.Errorf("missing User-agent: %s block", bot)
			continue
		}
		if !strings.Contains(block, "Disallow: /") {
			t.Errorf("User-agent: %s must Disallow: / (got block: %q)", bot, block)
		}
	}
}

func TestRobotsAllowsLiveAnswerBots(t *testing.T) {
	robots := readFile(t, filepath.Join(repoRoot(t), "docs", "robots.txt"))
	allowed := []string{
		"OAI-SearchBot",
		"ChatGPT-User",
		"PerplexityBot",
		"Perplexity-User",
		"Claude-Web",
		"Claude-User",
		"Applebot",
		"Googlebot",
		"Bingbot",
		"DuckDuckBot",
	}
	for _, bot := range allowed {
		block := userAgentBlock(robots, bot)
		if block == "" {
			t.Errorf("missing User-agent: %s block", bot)
			continue
		}
		if !strings.Contains(block, "Allow: /") {
			t.Errorf("User-agent: %s must Allow: / (got block: %q)", bot, block)
		}
	}
}

func TestRobotsReferencesSitemap(t *testing.T) {
	robots := readFile(t, filepath.Join(repoRoot(t), "docs", "robots.txt"))
	want := "Sitemap: https://bjornjee.github.io/agent-dashboard/sitemap.xml"
	if !strings.Contains(robots, want) {
		t.Errorf("robots.txt missing %q", want)
	}
}

func TestLLMsTxtStructure(t *testing.T) {
	llms := readFile(t, filepath.Join(repoRoot(t), "docs", "llms.txt"))
	lines := strings.Split(llms, "\n")
	var firstNonBlank string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			firstNonBlank = l
			break
		}
	}
	if !strings.HasPrefix(firstNonBlank, "# agent-dashboard") {
		t.Errorf("first non-blank line should start with %q, got %q", "# agent-dashboard", firstNonBlank)
	}
	if !strings.Contains(llms, "\n> ") && !strings.HasPrefix(llms, "> ") {
		t.Error("llms.txt missing blockquote summary line starting with '> '")
	}
	headingCount := strings.Count(llms, "\n## ")
	if strings.HasPrefix(llms, "## ") {
		headingCount++
	}
	if headingCount < 3 {
		t.Errorf("llms.txt should have at least 3 H2 sections, got %d", headingCount)
	}
	if got := strings.Count(llms, "https://"); got < 4 {
		t.Errorf("llms.txt should link to at least 4 URLs, got %d", got)
	}
}

func TestHeadCustomHasOpenGraph(t *testing.T) {
	head := readFile(t, filepath.Join(repoRoot(t), "docs", "_includes", "head_custom.html"))
	must := []string{
		`property="og:image"`,
		`property="og:url"`,
		`property="og:type"`,
		`property="og:site_name"`,
		`rel="canonical"`,
	}
	for _, m := range must {
		if !strings.Contains(head, m) {
			t.Errorf("head_custom.html missing %q", m)
		}
	}
}

func TestHeadCustomHasTwitterCard(t *testing.T) {
	head := readFile(t, filepath.Join(repoRoot(t), "docs", "_includes", "head_custom.html"))
	if !strings.Contains(head, `name="twitter:card"`) {
		t.Error("head_custom.html missing twitter:card")
	}
	if !strings.Contains(head, `content="summary_large_image"`) {
		t.Error("head_custom.html twitter:card should be summary_large_image")
	}
	if !strings.Contains(head, `name="twitter:image"`) {
		t.Error("head_custom.html missing twitter:image")
	}
}

func TestHeadCustomHasJSONLD(t *testing.T) {
	head := readFile(t, filepath.Join(repoRoot(t), "docs", "_includes", "head_custom.html"))
	must := []string{
		`application/ld+json`,
		`"@type": "SoftwareApplication"`,
		`"name": "agent-dashboard"`,
		`"applicationCategory"`,
		`"operatingSystem"`,
	}
	for _, m := range must {
		if !strings.Contains(head, m) {
			t.Errorf("head_custom.html missing JSON-LD field %q", m)
		}
	}
}

func TestOGCardSVGValid(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docs", "assets", "images", "og-card.svg")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	s := string(data)
	if !strings.HasPrefix(s, "<?xml") && !strings.HasPrefix(s, "<svg") {
		t.Error("og-card.svg should start with <?xml or <svg")
	}
	if !strings.Contains(s, `width="1200"`) {
		t.Error("og-card.svg should declare width=\"1200\"")
	}
	if !strings.Contains(s, `height="630"`) {
		t.Error("og-card.svg should declare height=\"630\"")
	}
}

func TestOGCardPNGExists(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docs", "assets", "images", "og-card.png")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.HasPrefix(data, pngMagic) {
		t.Error("og-card.png does not start with PNG magic bytes")
	}
}

func TestConfigDeclaresSitemapPlugin(t *testing.T) {
	cfg := readFile(t, filepath.Join(repoRoot(t), "docs", "_config.yml"))
	// jekyll-sitemap must appear under a top-level plugins: list (any whitespace, dash form).
	re := regexp.MustCompile(`(?m)^plugins:[\s\S]*?-\s+jekyll-sitemap`)
	if !re.MatchString(cfg) {
		t.Error("_config.yml plugins: list should include jekyll-sitemap")
	}
}

func TestGemfileBundlesSitemap(t *testing.T) {
	gf := readFile(t, filepath.Join(repoRoot(t), "docs", "Gemfile"))
	re := regexp.MustCompile(`(?m)^gem\s+"jekyll-sitemap"`)
	if !re.MatchString(gf) {
		t.Error("docs/Gemfile should declare gem \"jekyll-sitemap\"")
	}
}

func TestMarketplaceHasAIMetadata(t *testing.T) {
	raw := readFile(t, filepath.Join(repoRoot(t), ".claude-plugin", "marketplace.json"))
	var doc struct {
		Metadata struct {
			Homepage   string   `json:"homepage"`
			Repository string   `json:"repository"`
			License    string   `json:"license"`
			Keywords   []string `json:"keywords"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("marketplace.json invalid JSON: %v", err)
	}
	if len(doc.Metadata.Keywords) == 0 {
		t.Error("metadata.keywords must be a non-empty array")
	}
	if doc.Metadata.Homepage != "https://bjornjee.github.io/agent-dashboard" {
		t.Errorf("metadata.homepage = %q, want %q", doc.Metadata.Homepage, "https://bjornjee.github.io/agent-dashboard")
	}
	if doc.Metadata.Repository != "https://github.com/bjornjee/agent-dashboard" {
		t.Errorf("metadata.repository = %q, want %q", doc.Metadata.Repository, "https://github.com/bjornjee/agent-dashboard")
	}
	if doc.Metadata.License == "" {
		t.Error("metadata.license must be set")
	}
}

func TestREADMEHasWhyAndFAQ(t *testing.T) {
	readme := readFile(t, filepath.Join(repoRoot(t), "README.md"))
	if !strings.Contains(readme, "\n## Why agent-dashboard?") {
		t.Error("README.md missing '## Why agent-dashboard?' section")
	}
	if !strings.Contains(readme, "\n## FAQ") {
		t.Error("README.md missing '## FAQ' section")
	}
}
