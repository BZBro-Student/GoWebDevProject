package translation

import (
	"hash/fnv"
	"math/rand"
	"strings"
)

var syllables = []string{
	"na", "ni", "nu", "ne", "no",
	"ta", "ti", "tu", "te", "to",
	"fa", "fi", "fu", "fe", "fo",
	"la", "li", "lu", "le", "lo",
	"sa", "si", "su", "se", "so",
	"ma", "mi", "mu", "me", "mo",
	"ka", "ki", "ku", "ke", "ko",
	"pa", "pi", "pu", "pe", "po",
	"ra", "ri", "ru", "re", "ro",
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

// GenerateForeignWord deterministically produces a syllable-based word
func GenerateForeignWord(englishWord string) string {
	englishWord = strings.TrimSpace(englishWord)
	if englishWord == "" {
		return ""
	}

	// Deterministic seed ensures the same English word always yields the exact same foreign word
	r := rand.New(rand.NewSource(hashWord(englishWord)))
	syllableCount := sampleSyllableCount(r)

	var sb strings.Builder
	for i := 0; i < syllableCount; i++ {
		idx := r.Intn(len(syllables))
		sb.WriteString(syllables[idx])
	}

	return sb.String()
}
