package translation_test

import (
	"testing"

	"AGoTTHT/internal/translation"
)

func TestGenerateForeignWord(t *testing.T) {
	t.Run("Empty Or Whitespace Input", func(t *testing.T) {
		if res := translation.GenerateForeignWord(""); res != "" {
			t.Errorf("expected empty string for empty input, got %q", res)
		}
		if res := translation.GenerateForeignWord("   "); res != "" {
			t.Errorf("expected empty string for whitespace input, got %q", res)
		}
	})

	t.Run("Deterministic Output", func(t *testing.T) {
		word := "apple"
		res1 := translation.GenerateForeignWord(word)
		res2 := translation.GenerateForeignWord(word)

		if res1 == "" {
			t.Fatalf("expected non-empty foreign word for %q", word)
		}

		if res1 != res2 {
			t.Errorf("expected deterministic output: got %q and %q", res1, res2)
		}
	})

	t.Run("Case Insensitivity", func(t *testing.T) {
		resLower := translation.GenerateForeignWord("hello")
		resUpper := translation.GenerateForeignWord("HELLO")
		resMixed := translation.GenerateForeignWord("HeLLo")

		if resLower != resUpper || resLower != resMixed {
			t.Errorf("expected case insensitive output: lower=%q, upper=%q, mixed=%q", resLower, resUpper, resMixed)
		}
	})

	t.Run("Syllable Length Bounds", func(t *testing.T) {
		res := translation.GenerateForeignWord("golang")

		// All syllables in the list are 2 characters long.
		// The syllable count is between 1 and 7, so string length must be between 2 and 14 characters.
		if len(res)%2 != 0 {
			t.Errorf("expected even length string, got length %d (%q)", len(res), res)
		}

		if len(res) < 2 || len(res) > 14 {
			t.Errorf("expected length between 2 and 14, got %d (%q)", len(res), res)
		}
	})
}
