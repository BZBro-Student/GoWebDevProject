package translation

import (
	"fmt"
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
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "’", "'")
	h := fnv.New64a()
	h.Write([]byte(normalized))
	return int64(h.Sum64())
}

// sampleSyllableCount picks a length from 2 to 7 centered around ~3.1 syllables.
// Minimum length is 2 (10,000 combinations) to avoid rapid 1-syllable collisions.
func sampleSyllableCount(r *rand.Rand) int {
	weights := []struct {
		length int
		weight int
	}{
		{2, 25}, // 25% chance
		{3, 50}, // 50% chance (peak / average)
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
	return GenerateForeignWordWithSalt(englishWord, 0)
}

// GenerateForeignWordWithSalt deterministically generates a conlang word.
// Incrementing attempt produces an alternate word if a collision occurs in the DB.
func GenerateForeignWordWithSalt(englishWord string, attempt int) string {
	englishWord = strings.TrimSpace(englishWord)
	if englishWord == "" {
		return ""
	}

	seedInput := englishWord
	if attempt > 0 {
		seedInput = fmt.Sprintf("%s#%d", englishWord, attempt)
	}

	r := rand.New(rand.NewSource(hashWord(seedInput)))
	syllableCount := sampleSyllableCount(r)

	var sb strings.Builder
	prevIdx := -1
	for i := 0; i < syllableCount; i++ {
		idx := r.Intn(len(syllables))
		for idx == prevIdx {
			idx = r.Intn(len(syllables))
		}
		prevIdx = idx
		sb.WriteString(syllables[idx])
	}

	return sb.String()
}

func DeriveSynonymWord(rootForeignWord, englishWord string) string {
	return DeriveSynonymWordWithSalt(rootForeignWord, englishWord, 0)
}

// DeriveSynonymWordWithSalt appends a deterministic suffix syllable to rootForeignWord,
// avoiding immediate syllable repetition and supporting collision retries via attempt.
func DeriveSynonymWordWithSalt(rootForeignWord, englishWord string, attempt int) string {
	seedInput := fmt.Sprintf("%s:%s", rootForeignWord, englishWord)
	if attempt > 0 {
		seedInput = fmt.Sprintf("%s:%s#%d", rootForeignWord, englishWord, attempt)
	}

	r := rand.New(rand.NewSource(hashWord(seedInput)))
	idx := r.Intn(len(syllables))
	for strings.HasSuffix(rootForeignWord, syllables[idx]) {
		idx = r.Intn(len(syllables))
	}
	return rootForeignWord + syllables[idx]
}
