package translation

import (
	"hash/fnv"
	"math/rand"
	"strings"
)

var syllables = []string{
	"na", "ni", "nu", "ne", "no",
	"ma", "mi", "mu", "me", "mo",
	"la", "li", "lu", "le", "lo",
	"ra", "ri", "ru", "re", "ro",
	"pa", "pi", "pu", "pe", "po",
	"ta", "ti", "tu", "te", "to",
	"ka", "ki", "ku", "ke", "ko",
	"fa", "fi", "fu", "fe", "fo",
	"sa", "si", "su", "se", "so",
	"ba", "bi", "bu", "be", "bo",
	"da", "di", "du", "de", "do",
	"ga", "gi", "gu", "ge", "go",
	"ha", "hi", "hu", "he", "ho",
	"wa", "wi", "wu", "we", "wo",
	"ya", "yi", "yu", "ye", "yo",
	"va", "vi", "vu", "ve", "vo",
	"za", "zi", "zu", "ze", "zo",
	"cha", "chi", "chu", "che", "cho",
	"sha", "shi", "shu", "she", "sho",
	"tha", "thi", "thu", "the", "tho",
}

func hashWord(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(strings.ToLower(s)))
	return int64(h.Sum64())
}

// sampleSyllableCount picks a length from 1 to 7 centered around an average of 3
func sampleSyllableCount(r *rand.Rand) int {
	// Weights sum to 100 for percentage-based distribution
	// Weighted average: (1*5 + 2*25 + 3*45 + 4*15 + 5*7 + 6*2 + 7*1) / 100 = 3.04 syllables
	weights := []struct {
		length int
		weight int
	}{
		{1, 5},  // 5% chance
		{2, 25}, // 25% chance
		{3, 45}, // 45% chance (peak / average)
		{4, 15}, // 15% chance
		{5, 7},  // 7% chance
		{6, 2},  // 2% chance
		{7, 1},  // 1% chance
	}

	roll := r.Intn(100)
	cumulative := 0
	for _, w := range weights {
		cumulative += w.weight
		if roll < cumulative {
			return w.length
		}
	}
	return 3
}

func GenerateForeignWord(englishWord string) string {
	englishWord = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == ':' {
			return -1 // Returning -1 drops the character
		}
		return r
	}, englishWord)

	englishWord = strings.TrimSpace(englishWord)
	if englishWord == "" {
		return ""
	}
	r := rand.New(rand.NewSource(hashWord(englishWord)))
	syllableCount := sampleSyllableCount(r)

	var sb strings.Builder
	for i := 0; i < syllableCount; i++ {
		idx := r.Intn(len(syllables))
		sb.WriteString(syllables[idx])
	}

	return sb.String()
}
