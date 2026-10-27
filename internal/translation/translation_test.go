package translation_test

import (
	"database/sql"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"AGoTTHT/internal/translation"

	_ "github.com/mattn/go-sqlite3"
)

// Helper to set up an in-memory SQLite database with JSON support
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	schema := `
	CREATE TABLE server_dictionary (
		foreign_word TEXT PRIMARY KEY,
		english_translations TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

// Custom round tripper to mock http.DefaultTransport for FetchSynonyms
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// ==========================================
// 1. ExtractWords Tests
// ==========================================

func TestExtractWords(t *testing.T) {
	htmlInput := `
		<!DOCTYPE html>
		<html>
		<head>
			<style>body { color: red; }</style>
			<script>console.log("skip me!");</script>
		</head>
		<body>
			<h1>Hello, World!</h1>
			<p>This is a test... with punctuation!!</p>
		</body>
		</html>
	`

	words, err := translation.ExtractWords(strings.NewReader(htmlInput))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"Hello", "World", "This", "is", "a", "test", "with", "punctuation"}
	if !reflect.DeepEqual(words, expected) {
		t.Errorf("got %v, want %v", words, expected)
	}
}

// ==========================================
// 2. TranslationExists Tests
// ==========================================

func TestTranslationExists(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed dictionary entry with JSON array of translations
	_, err := db.Exec(
		`INSERT INTO server_dictionary (foreign_word, english_translations) VALUES (?, ?)`,
		"hola", `["hello", "hi"]`,
	)
	if err != nil {
		t.Fatalf("failed to seed database: %v", err)
	}

	t.Run("Existing word (case insensitive)", func(t *testing.T) {
		exists, err := translation.TranslationExists(db, "HELLO")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exists {
			t.Errorf("expected translation to exist")
		}
	})

	t.Run("Non-existing word", func(t *testing.T) {
		exists, err := translation.TranslationExists(db, "goodbye")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exists {
			t.Errorf("expected translation NOT to exist")
		}
	})
}

// ==========================================
// 3. FetchSynonyms Tests
// ==========================================

func TestFetchSynonyms(t *testing.T) {
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()

	t.Run("Successful API fetch and filtering", func(t *testing.T) {
		http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			jsonResp := `[{"word":"fast"},{"word":"quick"},{"word":"rapid"}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(jsonResp)),
			}, nil
		})

		// Query for "rapid" -> "rapid" should be filtered out from results
		synonyms, err := translation.FetchSynonyms("rapid")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []string{"fast", "quick"}
		if !reflect.DeepEqual(synonyms, expected) {
			t.Errorf("got %v, want %v", synonyms, expected)
		}
	})

	t.Run("API Error status", func(t *testing.T) {
		http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("internal error")),
			}, nil
		})

		_, err := translation.FetchSynonyms("rapid")
		if err == nil {
			t.Errorf("expected error on 500 response, got nil")
		}
	})
}

// ==========================================
// 4. SearchSemanticMeaning Tests
// ==========================================

func TestSearchSemanticMeaning(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(
		`INSERT INTO server_dictionary (foreign_word, english_translations) VALUES (?, ?)`,
		"rapido", `["fast", "quick"]`,
	)
	if err != nil {
		t.Fatalf("failed to seed database: %v", err)
	}

	t.Run("Candidate match found", func(t *testing.T) {
		candidates := []string{"slow", "quick", "sluggish"}
		foreignWord, found, err := translation.SearchSemanticMeaning(db, candidates)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Errorf("expected candidate match to be found")
		}
		if foreignWord != "rapido" {
			t.Errorf("expected foreign_word 'rapido', got '%s'", foreignWord)
		}
	})

	t.Run("No candidate matches", func(t *testing.T) {
		candidates := []string{"slow", "sluggish"}
		_, found, err := translation.SearchSemanticMeaning(db, candidates)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Errorf("expected found to be false")
		}
	})
}

// ==========================================
// 5. AddtoDB Integration Tests
// ==========================================

func TestAddtoDB(t *testing.T) {
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()

	// Mock API to return synonyms based on input word
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var jsonResp string
		if strings.Contains(req.URL.String(), "ml=swift") {
			jsonResp = `[{"word":"fast"}]`
		} else {
			jsonResp = `[]`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(jsonResp)),
		}, nil
	})

	db := setupTestDB(t)
	defer db.Close()

	// Seed existing foreign word "rapido" with translation "fast"
	_, err := db.Exec(
		`INSERT INTO server_dictionary (foreign_word, english_translations) VALUES (?, ?)`,
		"rapido", `["fast"]`,
	)
	if err != nil {
		t.Fatalf("failed to seed database: %v", err)
	}

	// Process new words: "fast" (already exists), "swift" (synonym of "fast"), "unique" (new word)
	words := []string{"fast", "swift"}
	translation.AddtoDB(words, db)

	// "swift" should be appended to "rapido"'s english_translations array via synonym match
	var translationsJSON string
	err = db.QueryRow("SELECT english_translations FROM server_dictionary WHERE foreign_word = ?", "rapido").Scan(&translationsJSON)
	if err != nil {
		t.Fatalf("failed to query updated record: %v", err)
	}

	if !strings.Contains(translationsJSON, "swift") {
		t.Errorf("expected translation array to contain 'swift', got: %s", translationsJSON)
	}
}
