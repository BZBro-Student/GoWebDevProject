package censor

import (
	"regexp"
	"sort"
	"strings"
)

// Words that frequently appear inside innocent English words (e.g., "class", "document", "title")
// require strict word boundaries (\b) to prevent false positives.
var exactOnlyWords = map[string]bool{
	"ass":   true,
	"tit":   true,
	"cum":   true,
	"anal":  true,
	"crap":  true,
	"damn":  true,
	"piss":  true,
	"porn":  true,
	"penis": true,
}

var rawBadWords = []string{
	// Compound / Explicit words
	"motherfucker", "cocksucker", "fuckhead", "fuckface", "shithead",
	"bullshit", "dickhead", "dumbass", "jackass", "asshole", "asswipe",

	// Unambiguous bad words (substring matching catches combinations like "fuckcunt")
	"fuck", "fucker", "fucking", "shit", "shitting", "shitty",
	"bitch", "bitches", "bitching", "cunt", "cunts", "dick",
	"cock", "pussy", "pussies", "bastard", "whore", "slut", "sluts",
	"fag", "faggot", "nigger", "nigga", "retard", "retarded",
	"spic", "chink", "kike", "dyke", "blowjob", "dildo", "ejaculate", "jizz",

	// Words requiring strict word boundaries (\b)
	"ass", "tit", "tits", "cum", "anal", "crap", "damn", "piss", "porn", "penis",
}

type patternItem struct {
	re      *regexp.Regexp
	wordLen int
}

var compiledPatterns []patternItem

var leetReplacer = strings.NewReplacer(
	"@", "a",
	"4", "a",
	"1", "i",
	"!", "i",
	"0", "o",
	"3", "e",
	"$", "s",
	"5", "s",
)

func init() {
	// Sort words by length descending so longer compound words match before short ones
	words := append([]string{}, rawBadWords...)
	sort.Slice(words, func(i, j int) bool {
		return len(words[i]) > len(words[j])
	})

	for _, word := range words {
		if word == "" {
			continue
		}

		var pattern string
		if exactOnlyWords[word] {
			// Enforce word boundaries for words prone to false positives
			pattern = `(?i)\b` + regexp.QuoteMeta(word) + `\b`
		} else {
			// Substring match for unambiguous bad words (catches "fuckcunt", "shitbitch", etc.)
			pattern = `(?i)` + regexp.QuoteMeta(word)
		}

		if re, err := regexp.Compile(pattern); err == nil {
			compiledPatterns = append(compiledPatterns, patternItem{
				re:      re,
				wordLen: len(word),
			})
		}
	}
}

// Sanitize censors bad words, compound combinations, and leetspeak variations
func Sanitize(input string) string {
	if strings.TrimSpace(input) == "" {
		return input
	}

	// 1. Normalize leetspeak (e.g., "f!ckcunt" -> "fuckcunt")
	normalized := leetReplacer.Replace(input)
	hasLeet := normalized != input

	cleaned := input

	// 2. Filter against pre-compiled regexes
	for _, item := range compiledPatterns {
		stars := strings.Repeat("*", item.wordLen)

		// Replace directly in cleaned input
		cleaned = item.re.ReplaceAllString(cleaned, stars)

		// If input contained leetspeak, also filter normalized matches out
		if hasLeet && item.re.MatchString(normalized) {
			cleaned = item.re.ReplaceAllString(normalized, stars)
		}
	}

	return cleaned
}
