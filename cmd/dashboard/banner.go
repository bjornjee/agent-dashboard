package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var quotes = []string{
	"Be yourself; everyone else is already taken.",
	"I'm selfish, impatient and a little insecure. I make mistakes, I am out of control and at times hard to handle. But if you can't handle me at my worst, then you sure as hell don't deserve me at my best.",
	"Two things are infinite: the universe and human stupidity; and I'm not sure about the universe.",
	"Be who you are and say what you feel, because those who mind don't matter, and those who matter don't mind.",
	"You only live once, but if you do it right, once is enough.",
	"Be the change that you wish to see in the world.",
	"To live is the rarest thing in the world. Most people exist, that is all.",
	"Without music, life would be a mistake.",
	"It is better to be hated for what you are than to be loved for what you are not.",
	"It is our choices, Harry, that show what we truly are, far more than our abilities.",
	"There are only two ways to live your life. One is as though nothing is a miracle. The other is as though everything is a miracle.",
	"For every minute you are angry you lose sixty seconds of happiness.",
	"And, when you want something, all the universe conspires in helping you to achieve it.",
	"You may say I'm a dreamer, but I'm not the only one. I hope someday you'll join us. And the world will live as one.",
	"Who controls the past controls the future. Who controls the present controls the past.",
}

type apiNinjasQuote struct {
	Quote  string `json:"quote"`
	Author string `json:"author"`
}

// maxDailyRetries is the max API calls per day to find one new unique quote
// that fits the length constraint. ~37% of quotes fit maxQuoteLen, so on
// average ~3 attempts needed; 10 gives comfortable margin for duplicates.
const maxDailyRetries = 10

// fetchAndCacheQuote retries up to maxDailyRetries times to find one new
// unique quote that fits within maxQuoteLen.
func fetchAndCacheQuote(db *DB) {
	key := os.Getenv("API_NINJAS_KEY")
	if key == "" {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	baseURL := "https://api.api-ninjas.com/v2/randomquotes"

	params := url.Values{}
	for _, cat := range []string{"wisdom", "philosophy", "life", "humor", "inspirational"} {
		params.Add("categories", cat)
	}
	fullURL := baseURL + "?" + params.Encode()

	for attempt := range maxDailyRetries {
		if attempt > 0 {
			time.Sleep(200 * time.Millisecond)
		}

		req, err := http.NewRequest(http.MethodGet, fullURL, nil)
		if err != nil {
			return
		}
		req.Header.Set("X-Api-Key", key)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		var results []apiNinjasQuote
		json.NewDecoder(resp.Body).Decode(&results)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK || len(results) == 0 {
			continue
		}

		r := results[0]
		if len(r.Quote)+len(r.Author)+3 > maxQuoteLen {
			continue
		}
		if db.QuoteExists(r.Quote) {
			continue
		}

		if err := db.InsertQuotes([]QuoteRow{{Quote: r.Quote, Author: r.Author}}); err != nil {
			return
		}
		db.SetLastQuoteFetch(time.Now().Format("2006-01-02"))
		return
	}

	// Exhausted retries — still mark the day so we don't retry on every render.
	db.SetLastQuoteFetch(time.Now().Format("2006-01-02"))
}

// refreshQuotesIfNeeded checks if quotes need refreshing (daily) and fetches if so.
func refreshQuotesIfNeeded(db *DB) {
	today := time.Now().Format("2006-01-02")
	if db.LastQuoteFetch() == today {
		return
	}
	fetchAndCacheQuote(db)
}

// maxQuoteLen is the hard character limit for the full "quote — author" string.
// Based on ~1/3 terminal width (40 chars) * 2 lines = 80 chars.
const maxQuoteLen = 80

// pickQuote returns the quote text and author (empty for fallback).
func pickQuote(db *DB) (string, string) {
	if db != nil {
		refreshQuotesIfNeeded(db)
		q, a := db.RandomQuote(maxQuoteLen)
		if q != "" {
			return q, a
		}
	}
	return fallbackQuoteText(), ""
}

func fallbackQuoteText() string {
	var eligible []string
	for _, q := range quotes {
		if len(q) <= maxQuoteLen {
			eligible = append(eligible, q)
		}
	}
	if len(eligible) == 0 {
		return ""
	}
	return eligible[rand.Intn(len(eligible))]
}

// wrapWords word-wraps text into lines that fit within width.
// prefix is prepended to the first line only.
func wrapWords(words []string, width int, prefix string) []string {
	if len(words) == 0 {
		return nil
	}
	line := prefix + words[0]
	var lines []string
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = w
		} else {
			line += " " + w
		}
	}
	lines = append(lines, line)
	return lines
}

// formatQuote formats the quote for display within the given width.
// For authored quotes: quote text is word-wrapped, then author goes on its own
// line with padding — unless it all fits on one line.
// For fallback quotes: word-wrapped with at least 2 words on the last line.
func formatQuote(text, author string, width int) string {
	if width <= 0 || text == "" {
		return ""
	}

	if author != "" {
		oneLine := fmt.Sprintf("\" %s — %s \"", text, author)
		if len(oneLine) <= width {
			return oneLine
		}
		// Word-wrap the quote text, then put author on the last line.
		words := strings.Fields(text)
		lines := wrapWords(words, width, "\" ")
		lines = append(lines, fmt.Sprintf("    — %s \"", author))
		return strings.Join(lines, "\n")
	}

	// Fallback quote (no author)
	oneLine := "\" " + text + " \""
	if len(oneLine) <= width {
		return oneLine
	}

	// Word-wrap, but ensure at least 2 words on the last line.
	words := strings.Fields(text)
	if len(words) <= 2 {
		return "\" " + text + " \""
	}
	lines := wrapWords(words, width, "\" ")
	// If the last line has only 1 word, pull a word from the previous line.
	for len(lines) > 1 {
		lastWords := strings.Fields(lines[len(lines)-1])
		if len(lastWords) >= 2 {
			break
		}
		// Pull the last word from the previous line down.
		prevWords := strings.Fields(lines[len(lines)-2])
		if len(prevWords) <= 1 {
			break
		}
		pulled := prevWords[len(prevWords)-1]
		lines[len(lines)-2] = strings.Join(prevWords[:len(prevWords)-1], " ")
		lines[len(lines)-1] = pulled + " " + lines[len(lines)-1]
	}
	lines[len(lines)-1] += " \""
	return strings.Join(lines, "\n")
}

func greeting(now time.Time) string {
	hour := now.Hour()
	switch {
	case hour < 12:
		return "Good Morning, Bjorn"
	case hour < 17:
		return "Good Afternoon, Bjorn"
	default:
		return "Good Evening, Bjorn"
	}
}

// Pixel art colors — Catppuccin Frappé palette
var (
	pxHotPink    = themePink
	pxMedPink    = themeFlamingo
	pxLightPink  = themeRosewater
	pxPalePink   = themeLavender
	pxDarkPurple = themeCrust
	pxLavender   = themeMauve
)

// Color Palette based on image:
// 0: Empty/Transparent
// 1: Dark Pink (Gills/Outline)
// 2: Light Pink (Face/Body)
// 3: Dark Purple (Eyes)
// 4: Muted Pink (Nose/Blush)
var axolotlPixels = [][]int{
	{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	{0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0},
	{0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0},
	{0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0},
	{0, 0, 0, 0, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 0, 0, 0, 0},
	{0, 0, 0, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 0, 0, 0},
	{0, 1, 0, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 0, 1, 0},
	{0, 1, 1, 2, 2, 3, 2, 2, 4, 4, 2, 2, 3, 2, 2, 1, 1, 0},
	{0, 1, 0, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 0, 1, 0},
	{0, 0, 0, 1, 1, 2, 2, 2, 2, 2, 2, 2, 2, 1, 1, 0, 0, 0},
	{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
}

var pxColors = map[int]lipgloss.Color{
	1: themePink,      // Gills/outline
	2: themeRosewater, // Face/body
	3: themeCrust,     // Eyes
	4: themeFlamingo,  // Nose/blush
}

// renderAxolotl renders the pixel art using half-block characters with true colors.
// Each terminal row encodes 2 pixel rows via ▀ (fg=top, bg=bottom).
func renderAxolotl() string {
	var lines []string
	for y := 0; y < len(axolotlPixels); y += 2 {
		var line strings.Builder
		topRow := axolotlPixels[y]
		var botRow []int
		if y+1 < len(axolotlPixels) {
			botRow = axolotlPixels[y+1]
		}
		for x := 0; x < len(topRow); x++ {
			top := topRow[x]
			bot := 0
			if botRow != nil && x < len(botRow) {
				bot = botRow[x]
			}
			switch {
			case top == 0 && bot == 0:
				line.WriteString(" ")
			case top == 0:
				line.WriteString(lipgloss.NewStyle().Foreground(pxColors[bot]).Render("▄"))
			case bot == 0:
				line.WriteString(lipgloss.NewStyle().Foreground(pxColors[top]).Render("▀"))
			case top == bot:
				line.WriteString(lipgloss.NewStyle().Foreground(pxColors[top]).Render("█"))
			default:
				line.WriteString(lipgloss.NewStyle().
					Foreground(pxColors[top]).
					Background(pxColors[bot]).
					Render("▀"))
			}
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

var greetingStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(themeRosewater)

var quoteStyle = lipgloss.NewStyle().
	Foreground(themeOverlay1).
	Italic(true)

func (m model) renderBanner() string {
	icon := renderAxolotl()
	greet := greetingStyle.Render(greeting(m.nowFunc()))

	// 1. Build the left side first to calculate its footprint
	left := lipgloss.JoinHorizontal(lipgloss.Center, "  ", icon, "  ", greet)
	leftWidth := lipgloss.Width(left)

	// 2. Calculate remaining space.
	// We want the right side to take up every single pixel left.
	rightWidth := m.width - leftWidth
	if rightWidth < 0 {
		rightWidth = 0
	}

	// 3. Define a max width for the quote text itself so it doesn't
	// wrap awkwardly if the terminal is huge.
	maxQuoteWidth := m.width / 3

	// 4. Format the quote with wrapping awareness, then style it
	q := quoteStyle.Render(formatQuote(m.quote, m.quoteAuthor, maxQuoteWidth))

	// 4b. Version label
	ver := helpStyle.Render("v" + Version)

	// 5. Wrap that quote + version in a container that fills the remaining width
	// and pushes the content to the Right.
	rightContent := lipgloss.JoinVertical(lipgloss.Right, ver, "", q)
	right := lipgloss.NewStyle().
		Width(rightWidth).
		Align(lipgloss.Right).
		Render(rightContent)

	// 6. Join them. No extra spacers needed here because 'right'
	// already fills the gap.
	return lipgloss.JoinHorizontal(lipgloss.Center, left, right)
}
