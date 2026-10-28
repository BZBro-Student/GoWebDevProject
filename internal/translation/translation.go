package translation

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
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

var commonSentenceStarters = map[string]bool{
	"a": true, "an": true, "the": true, "in": true, "on": true,
	"at": true, "and": true, "but": true, "or": true, "for": true,
	"as": true, "to": true, "if": true, "when": true, "then": true,
	"he": true, "she": true, "it": true, "they": true, "we": true,
	"now": true, "so": true, "by": true, "with": true, "from": true,
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"been": true, "being": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true,
	"shall": true, "should": true, "may": true, "might": true, "must": true,
	"can": true, "could": true, "this": true, "that": true, "these": true,
	"those": true, "there": true, "here": true, "where": true, "what": true,
	"who": true, "whom": true, "whose": true, "which": true, "why": true,
	"how": true, "all": true, "any": true, "both": true, "each": true,
	"few": true, "more": true, "most": true, "other": true, "some": true,
	"such": true, "no": true, "nor": true, "not": true, "only": true,
	"own": true, "same": true, "than": true, "too": true, "very": true,
}

func isNumberOrReference(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	for _, r := range s {
		if !unicode.IsDigit(r) && r != ':' {
			return false
		}
	}
	return true
}

func isCapitalized(s string) bool {
	runes := []rune(strings.TrimSpace(s))
	return len(runes) > 0 && unicode.IsUpper(runes[0])
}

func analyzeCorpus(words []string) (properNouns map[string]bool, canonicalCasing map[string]string) {
	lowercaseCounts := make(map[string]int)
	canonicalCasing = make(map[string]string)
	properNouns = make(map[string]bool)

	for _, w := range words {
		cleaned := strings.TrimSpace(w)
		if cleaned == "" || isNumberOrReference(cleaned) {
			continue
		}

		lower := strings.ToLower(cleaned)

		if !isCapitalized(cleaned) {
			lowercaseCounts[lower]++
		}

		if existing, exists := canonicalCasing[lower]; !exists || (isCapitalized(cleaned) && !isCapitalized(existing)) {
			canonicalCasing[lower] = cleaned
		}
	}

	for lower, cased := range canonicalCasing {
		if !isCapitalized(cased) || commonSentenceStarters[lower] || lowercaseCounts[lower] > 0 {
			continue
		}
		properNouns[lower] = true
	}

	return properNouns, canonicalCasing
}

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

// AddtoDB processes candidate words with full console progress tracking.
func AddtoDB(words []string, db *sql.DB) {
	startTime := time.Now()
	fmt.Printf("\n========================================\n")
	fmt.Printf("[Translation] Starting batch processing for %d raw word tokens...\n", len(words))

	// Step 1: Corpus Analysis
	properNouns, canonicalCasing := analyzeCorpus(words)

	uniqueWords := make(map[string]string)
	for lower, cased := range canonicalCasing {
		if properNouns[lower] {
			uniqueWords[lower] = cased
		} else {
			uniqueWords[lower] = lower
		}
	}

	totalUnique := len(uniqueWords)
	totalProper := len(properNouns)
	fmt.Printf("[Step 1/3] Corpus analysis completed in %v\n", time.Since(startTime).Round(time.Millisecond))
	fmt.Printf("           -> %d unique words identified (%d proper nouns, %d standard words)\n",
		totalUnique, totalProper, totalUnique-totalProper)

	if totalUnique == 0 {
		fmt.Printf("[Translation] No valid words to process. Aborting.\n")
		return
	}

	// Step 2: Concurrent Synonym Fetching
	type wordPayload struct {
		input      string
		isProper   bool
		synonyms   []string
		fetchError error
	}

	fetchStart := time.Now()
	payloads := make([]wordPayload, totalUnique)
	semaphore := make(chan struct{}, 10)
	var wg sync.WaitGroup
	var completedCount int32

	fmt.Printf("[Step 2/3] Fetching synonyms from Datamuse (10 worker pool)...\n")

	idx := 0
	for lower, input := range uniqueWords {
		isProper := properNouns[lower]

		wg.Add(1)
		go func(i int, word string, isProperNoun bool) {
			defer wg.Done()

			if isProperNoun {
				payloads[i] = wordPayload{input: word, isProper: true}
				curr := atomic.AddInt32(&completedCount, 1)
				if curr%25 == 0 || int(curr) == totalUnique {
					fmt.Printf("           -> Progress: [%d/%d] words processed\n", curr, totalUnique)
				}
				return
			}

			semaphore <- struct{}{}
			synonyms, err := FetchSynonyms(word)
			<-semaphore

			payloads[i] = wordPayload{
				input:      word,
				isProper:   false,
				synonyms:   synonyms,
				fetchError: err,
			}

			curr := atomic.AddInt32(&completedCount, 1)
			if curr%25 == 0 || int(curr) == totalUnique {
				fmt.Printf("           -> Progress: [%d/%d] words processed\n", curr, totalUnique)
			}
		}(idx, input, isProper)
		idx++
	}
	wg.Wait()
	fmt.Printf("           -> Synonym fetching completed in %v\n", time.Since(fetchStart).Round(time.Millisecond))

	// Step 3: Database Writes inside Transaction
	dbStart := time.Now()
	fmt.Printf("[Step 3/3] Writing entries to SQLite database...\n")

	tx, err := db.Begin()
	if err != nil {
		fmt.Printf("[ERROR] Failed to start database transaction: %v\n", err)
		return
	}
	defer tx.Rollback()

	var countExisting, countUpdated, countInserted, countSkipped int

	for _, item := range payloads {
		input := item.input

		exists, err := TranslationExists(tx, input)
		if err != nil {
			fmt.Printf("  [WARN] Error checking existence for %q: %v\n", input, err)
			countSkipped++
			continue
		}
		if exists {
			countExisting++
			continue
		}

		if item.fetchError != nil {
			fmt.Printf("  [WARN] Skipping %q due to synonym fetch error: %v\n", input, item.fetchError)
			countSkipped++
			continue
		}

		foreignWord, found, err := SearchSemanticMeaning(tx, item.synonyms)
		if err != nil {
			fmt.Printf("  [WARN] Error searching synonyms for %q: %v\n", input, err)
			countSkipped++
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
				fmt.Printf("  [ERROR] Failed to map %q to existing foreign word %q: %v\n", input, foreignWord, err)
				countSkipped++
			} else {
				countUpdated++
			}
		} else {
			var generatedForeignWord string

			if item.isProper {
				generatedForeignWord = input
			} else {
				generatedForeignWord = GenerateForeignWord(input)
			}

			if strings.TrimSpace(generatedForeignWord) == "" {
				countSkipped++
				continue
			}

			insertQuery := `
                INSERT INTO server_dictionary (foreign_word, english_translations) 
                VALUES (?, json_array(?))
                ON CONFLICT(foreign_word) DO UPDATE SET 
                    english_translations = json_insert(english_translations, '$[#]', ?)
            `
			_, err = tx.Exec(insertQuery, generatedForeignWord, input, input)
			if err != nil {
				fmt.Printf("  [ERROR] Failed to insert new entry %q -> %q: %v\n", input, generatedForeignWord, err)
				countSkipped++
			} else {
				countInserted++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		fmt.Printf("[ERROR] Failed to commit database transaction: %v\n", err)
		return
	}

	// Completion Summary
	fmt.Printf("           -> DB transaction committed in %v\n", time.Since(dbStart).Round(time.Millisecond))
	fmt.Printf("\n--- Batch Summary ---\n")
	fmt.Printf(" Already in DB : %d\n", countExisting)
	fmt.Printf(" Mapped to Syn : %d\n", countUpdated)
	fmt.Printf(" Newly Created : %d\n", countInserted)
	fmt.Printf(" Errors/Skipped: %d\n", countSkipped)
	fmt.Printf(" Total Elapsed : %v\n", time.Since(startTime).Round(time.Millisecond))
	fmt.Printf("========================================\n\n")
}

// LoadDictionary loads English -> Foreign mappings into an in-memory map.
func LoadDictionary(db *sql.DB) (map[string]string, error) {
	dict := make(map[string]string)

	// Unnest the JSON array of english_translations
	query := `
		SELECT LOWER(json_each.value) AS english_word, foreign_word
		FROM server_dictionary, json_each(server_dictionary.english_translations)
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var english, foreign string
		if err := rows.Scan(&english, &foreign); err == nil {
			dict[english] = foreign
		}
	}

	return dict, nil
}

// TranslateHTML processes raw English HTML and translates text nodes into your conlang.
func TranslateHTML(englishHTML string, dict map[string]string) (string, error) {
	doc, err := html.Parse(strings.NewReader(englishHTML))
	if err != nil {
		return "", err
	}

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		// Ignore script and style tags
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}

		// Process only text nodes
		if n.Type == html.TextNode {
			n.Data = translateTextString(n.Data, dict)
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// translateTextString translates words while preserving spaces, punctuation, and capitalization
func translateTextString(text string, dict map[string]string) string {
	var builder strings.Builder
	var wordBuf strings.Builder

	flushWord := func() {
		if wordBuf.Len() == 0 {
			return
		}
		original := wordBuf.String()
		lower := strings.ToLower(original)

		if foreign, found := dict[lower]; found {
			// Match capitalization of original word
			if len(original) > 0 && unicode.IsUpper([]rune(original)[0]) {
				runes := []rune(foreign)
				runes[0] = unicode.ToUpper(runes[0])
				foreign = string(runes)
			}
			builder.WriteString(foreign)
		} else {
			builder.WriteString(original) // Fallback to original if unmapped
		}
		wordBuf.Reset()
	}

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			wordBuf.WriteRune(r)
		} else {
			flushWord()
			builder.WriteRune(r) // Preserve punctuation, spaces, and newlines
		}
	}
	flushWord()

	return builder.String()
}
