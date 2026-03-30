package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
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

// fetchAndCacheQuotes fetches 10 quotes from API Ninjas and caches them in the DB.
func fetchAndCacheQuotes(db *DB) {
	key := os.Getenv("API_NINJAS_KEY")
	if key == "" {
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", "https://api.api-ninjas.com/v1/quotes?limit=30", nil)
	if err != nil {
		return
	}
	req.Header.Set("X-Api-Key", key)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var results []apiNinjasQuote
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil || len(results) == 0 {
		return
	}
	rows := make([]QuoteRow, len(results))
	for i, r := range results {
		rows[i] = QuoteRow{Quote: r.Quote, Author: r.Author}
	}
	if err := db.InsertQuotes(rows); err != nil {
		return
	}
	db.SetLastQuoteFetch(time.Now().Format("2006-01-02"))
}

// refreshQuotesIfNeeded checks if quotes need refreshing (daily) and fetches if so.
func refreshQuotesIfNeeded(db *DB) {
	today := time.Now().Format("2006-01-02")
	if db.LastQuoteFetch() == today {
		return
	}
	fetchAndCacheQuotes(db)
}

// pickQuote returns a random quote from the DB cache, falling back to hardcoded.
func pickQuote(db *DB) string {
	if db != nil {
		refreshQuotesIfNeeded(db)
		q, a := db.RandomQuote()
		if q != "" {
			return fmt.Sprintf("%s — %s", q, a)
		}
	}
	return fallbackQuote()
}

func fallbackQuote() string {
	if len(quotes) == 0 {
		return ""
	}
	return quotes[rand.Intn(len(quotes))]
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

// Pixel art colors — soft kawaii palette (no black outlines)
var (
	pxHotPink    = lipgloss.Color("#D946A8")
	pxMedPink    = lipgloss.Color("#F472B6")
	pxLightPink  = lipgloss.Color("#FBCFE8")
	pxPalePink   = lipgloss.Color("#FDF2F8")
	pxDarkPurple = lipgloss.Color("#312E81")
	pxLavender   = lipgloss.Color("#D8B4FE")
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
	1: lipgloss.Color("#E882B4"), // Dark Pink
	2: lipgloss.Color("#F8C8E8"), // Light Pink
	3: lipgloss.Color("#3D0E61"), // Dark Purple (Eyes)
	4: lipgloss.Color("#D1A1C4"), // Muted Pink (Nose)
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
	Foreground(lipgloss.Color("230"))

var quoteStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("242")).
	Italic(true)

func (m model) renderBanner() string {
	icon := renderAxolotl()
	greet := greetingStyle.Render(greeting(m.nowFunc()))
	q := quoteStyle.Render(m.quote)

	left := lipgloss.JoinHorizontal(lipgloss.Center, "  ", icon, "  ", greet)

	leftWidth := lipgloss.Width(left)
	rightWidth := m.width - leftWidth - 2
	if rightWidth < 0 {
		rightWidth = 0
	}

	right := lipgloss.NewStyle().
		Width(rightWidth).
		Align(lipgloss.Right).
		Render(q)

	return lipgloss.JoinHorizontal(lipgloss.Center, left, "  ", right)
}
