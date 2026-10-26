package translation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/html"
)

// DBQuerier abstracts *sql.DB and *sql.Tx for shared database queries.
type DBQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

type DictionaryEntry struct {
	ForeignWord         string   `json:"foreign_word"`
	EnglishTranslations []string `json:"english_translations"`
}

type datamuseResult struct {
	Word string `json:"word"`
}

// ExtractWords extracts readable body text, ignoring script and style tags.
func ExtractWords(r io.Reader) ([]string, error) {
	tokenizer := html.NewTokenizer(r)
	var words []string
	var ignoreDepth int

	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			err := tokenizer.Err()
			if err == io.EOF {
				return words, nil
			}
			return nil, err

		case html.StartTagToken:
			token := tokenizer.Token()
			tagName := strings.ToLower(token.Data)
			if tagName == "script" || tagName == "style" {
				ignoreDepth++
			}

		case html.EndTagToken:
			token := tokenizer.Token()
			tagName := strings.ToLower(token.Data)
			if tagName == "script" || tagName == "style" {
				if ignoreDepth > 0 {
					ignoreDepth--
				}
			}

		case html.TextToken:
			if ignoreDepth > 0 {
				continue
			}

			token := tokenizer.Token()
			text := strings.TrimSpace(token.Data)
			if text != "" {
				extracted := strings.Fields(text)
				for _, w := range extracted {
					cleaned := strings.TrimFunc(w, func(r rune) bool {
						return unicode.IsPunct(r) || unicode.IsSymbol(r)
					})
					if cleaned != "" {
						words = append(words, cleaned)
					}
				}
			}
		}
	}
}

func TranslationExists(db DBQuerier, searchWord string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 
			FROM server_dictionary, json_each(english_translations)
			WHERE LOWER(json_each.value) = LOWER(?)
		)
	`
	var exists bool
	err := db.QueryRow(query, strings.TrimSpace(searchWord)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check translation existence: %w", err)
	}
	return exists, nil
}

// FetchSynonyms fetches candidate synonyms from the Datamuse API (Network I/O only).
func FetchSynonyms(word string) ([]string, error) {
	apiURL := fmt.Sprintf("https://api.datamuse.com/words?ml=%s", url.QueryEscape(word))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to reach datamuse API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("datamuse API returned status: %d", resp.StatusCode)
	}

	var results []datamuseResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}

	maxChecks := 10
	if len(results) < maxChecks {
		maxChecks = len(results)
	}

	candidates := make([]string, 0, maxChecks)
	for _, res := range results[:maxChecks] {
		candidate := strings.TrimSpace(res.Word)
		if candidate != "" && !strings.EqualFold(candidate, word) {
			candidates = append(candidates, candidate)
		}
	}

	return candidates, nil
}

// SearchSemanticMeaning checks if any candidate synonyms exist in the database.
func SearchSemanticMeaning(db DBQuerier, candidates []string) (foreignWord string, found bool, err error) {
	query := `
		SELECT server_dictionary.foreign_word 
		FROM server_dictionary, json_each(server_dictionary.english_translations)
		WHERE LOWER(json_each.value) = LOWER(?) 
		LIMIT 1
	`

	for _, candidate := range candidates {
		var existingForeignWord string
		err := db.QueryRow(query, candidate).Scan(&existingForeignWord)
		if err == nil {
			return existingForeignWord, true, nil
		}
		if err != sql.ErrNoRows {
			return "", false, fmt.Errorf("database query error: %w", err)
		}
	}

	return "", false, nil
}

// AddtoDB processes candidate words using a transaction to minimize SQLite locks.
func AddtoDB(words []string, db *sql.DB) {
	// Deduplicate and lower-case inputs
	uniqueWords := make(map[string]bool)
	for _, w := range words {
		cleaned := strings.ToLower(strings.TrimSpace(w))
		if cleaned != "" {
			uniqueWords[cleaned] = true
		}
	}

	type wordPayload struct {
		input      string
		synonyms   []string
		fetchError error
	}

	// Fetch synonyms concurrently (bounded to 10 parallel HTTP workers)
	payloads := make([]wordPayload, len(uniqueWords))
	semaphore := make(chan struct{}, 10)
	var wg sync.WaitGroup

	idx := 0
	for input := range uniqueWords {
		wg.Add(1)
		go func(i int, word string) {
			defer wg.Done()
			semaphore <- struct{}{}        // acquire slot
			defer func() { <-semaphore }() // release slot

			synonyms, err := FetchSynonyms(word)
			payloads[i] = wordPayload{
				input:      word,
				synonyms:   synonyms,
				fetchError: err,
			}
		}(idx, input)
		idx++
	}
	wg.Wait()

	// Begin DB Transaction for writes
	tx, err := db.Begin()
	if err != nil {
		fmt.Printf("failed to start transaction: %v\n", err)
		return
	}
	defer tx.Rollback()

	for _, item := range payloads {
		input := item.input

		exists, err := TranslationExists(tx, input)
		if err != nil {
			fmt.Printf("error checking existence for %q: %v\n", input, err)
			continue
		}
		if exists {
			continue
		}

		if item.fetchError != nil {
			fmt.Printf("error fetching semantic meaning for %q: %v\n", input, item.fetchError)
			continue
		}

		foreignWord, found, err := SearchSemanticMeaning(tx, item.synonyms)
		if err != nil {
			fmt.Printf("error searching database for synonyms of %q: %v\n", input, err)
			continue
		}

		if found {
			updateQuery := `
				UPDATE server_dictionary 
				SET english_translations = json_insert(english_translations, '$[#]', ?) 
				WHERE foreign_word = ?
			`
			_, err = tx.Exec(updateQuery, input, foreignWord)
			if err != nil {
				fmt.Printf("failed to update translation array for %q: %v\n", foreignWord, err)
			}
		} else {
			generatedForeignWord := GenerateForeignWord(input)

			insertQuery := `
				INSERT INTO server_dictionary (foreign_word, english_translations) 
				VALUES (?, json_array(?))
				ON CONFLICT(foreign_word) DO UPDATE SET 
					english_translations = json_insert(english_translations, '$[#]', ?)
			`
			_, err = tx.Exec(insertQuery, generatedForeignWord, input, input)
			if err != nil {
				fmt.Printf("failed to insert new dictionary entry for %q: %v\n", generatedForeignWord, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		fmt.Printf("failed to commit transaction: %v\n", err)
	}
}
