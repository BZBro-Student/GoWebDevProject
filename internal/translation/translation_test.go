package translation_test

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"AGoTTHT/internal/translation"

	_ "github.com/mattn/go-sqlite3"
)

// Helper to set up an isolated in-memory SQLite database per test
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	db.SetMaxOpenConns(1)

	schema := `
    CREATE TABLE IF NOT EXISTS server_dictionary (
        foreign_word TEXT PRIMARY KEY,
        english_translations TEXT NOT NULL
    );
    `
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

// Custom round tripper to mock HTTP transport for FetchSynonyms
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
	origClientTransport := http.DefaultClient.Transport
	defer func() {
		http.DefaultTransport = origTransport
		http.DefaultClient.Transport = origClientTransport
	}()

	t.Run("Successful API fetch and filtering", func(t *testing.T) {
		mock := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			jsonResp := `[{"word":"fast"},{"word":"quick"},{"word":"rapid"}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(jsonResp)),
			}, nil
		})

		http.DefaultTransport = mock
		http.DefaultClient.Transport = mock

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
		mock := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("internal error")),
			}, nil
		})

		http.DefaultTransport = mock
		http.DefaultClient.Transport = mock

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
	origClientTransport := http.DefaultClient.Transport
	defer func() {
		http.DefaultTransport = origTransport
		http.DefaultClient.Transport = origClientTransport
	}()

	mock := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var jsonResp string
		if strings.Contains(req.URL.String(), "swift") {
			jsonResp = `[{"word":"fast"}]`
		} else {
			jsonResp = `[]`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(jsonResp)),
		}, nil
	})

	http.DefaultTransport = mock
	http.DefaultClient.Transport = mock

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

	// Process words: "fast" (already in DB), "swift" (synonym of "fast")
	words := []string{"fast", "swift"}
	translation.AddtoDB(words, db)

	// 1. Verify "swift" now exists in server_dictionary
	exists, err := translation.TranslationExists(db, "swift")
	if err != nil {
		t.Fatalf("failed to check existence for 'swift': %v", err)
	}
	if !exists {
		t.Errorf("expected 'swift' to be mapped and exist in server_dictionary")
	}

	// 2. Query the derived entry created for 'swift'
	var derivedForeignWord, translationsJSON string
	err = db.QueryRow(`
        SELECT server_dictionary.foreign_word, server_dictionary.english_translations 
        FROM server_dictionary, json_each(server_dictionary.english_translations) 
        WHERE LOWER(json_each.value) = 'swift'
    `).Scan(&derivedForeignWord, &translationsJSON)
	if err != nil {
		t.Fatalf("failed to query entry for 'swift': %v", err)
	}

	if !strings.Contains(translationsJSON, "swift") {
		t.Errorf("expected translation array to contain 'swift', got: %s", translationsJSON)
	}
}
