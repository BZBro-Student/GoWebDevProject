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
}
