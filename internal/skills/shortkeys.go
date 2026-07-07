package skills

import (
	"strings"
	"unicode"
)

const codexDashboardPrefix = "$agent-dashboard:"

// ExpandCodexShortkeys expands bare dashboard skill commands into Codex's
// plugin-qualified form. It only rewrites command tokens whose bare skill name
// is present in validSkills.
func ExpandCodexShortkeys(text string, validSkills []string) string {
	if text == "" || len(validSkills) == 0 || !strings.Contains(text, "$") {
		return text
	}
	valid := make(map[string]struct{}, len(validSkills))
	for _, skill := range validSkills {
		if skill == "" || skill == "(none)" {
			continue
		}
		valid[skill] = struct{}{}
	}
	if len(valid) == 0 {
		return text
	}

	var b strings.Builder
	b.Grow(len(text))
	runes := []rune(text)
	for i := 0; i < len(runes); {
		if runes[i] != '$' || (i > 0 && !unicode.IsSpace(runes[i-1])) {
			b.WriteRune(runes[i])
			i++
			continue
		}
		end := i + 1
		for end < len(runes) && isCodexCommandChar(runes[end]) {
			end++
		}
		if end == i+1 {
			b.WriteRune(runes[i])
			i++
			continue
		}
		word := string(runes[i+1 : end])
		tail := word
		if idx := strings.LastIndex(word, ":"); idx >= 0 {
			tail = word[idx+1:]
		}
		if _, ok := valid[tail]; ok && !strings.Contains(word, ":") {
			b.WriteString(codexDashboardPrefix)
			b.WriteString(tail)
		} else {
			b.WriteString(string(runes[i:end]))
		}
		i = end
	}
	return b.String()
}

func isCodexCommandChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == ':'
}
