package translation

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	_ "github.com/mattn/go-sqlite3"
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// DBQuerier abstracts *sql.DB and *sql.Tx for shared database queries.
type DBQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

type DictionaryEntry struct {
	ForeignWord         string   `json:"foreign_word"`
	EnglishTranslations []string `json:"english_translations"`
}

type datamuseResult struct {
	Word string `json:"word"`
}

// Titles and honorifics that should remain untranslated when attached to names
var commonTitles = map[string]bool{
	"don": true, "sir": true, "lady": true, "lord": true,
	"count": true, "duke": true, "king": true, "queen": true,
	"prince": true, "princess": true, "friar": true, "saint": true,
}

var romanNumerals = map[string]bool{
	"i": true, "ii": true, "iii": true, "iv": true, "v": true,
	"vi": true, "vii": true, "viii": true, "ix": true, "x": true,
	"xi": true, "xii": true, "xiii": true, "xiv": true, "xv": true,
}

var commonSentenceStarters = map[string]bool{
	"i": true, "you": true, "me": true, "my": true, "we": true,
	"us": true, "our": true, "he": true, "him": true, "his": true,
	"she": true, "her": true, "it": true, "its": true, "they": true,
	"them": true, "their": true, "a": true, "an": true, "the": true,
	"in": true, "on": true, "at": true, "and": true, "but": true,
	"or": true, "for": true, "as": true, "to": true, "if": true,
	"when": true, "then": true, "now": true, "so": true, "by": true,
	"with": true, "from": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true,
	"has": true, "had": true, "do": true, "does": true, "did": true,
	"will": true, "would": true, "shall": true, "should": true, "may": true,
	"might": true, "must": true, "can": true, "could": true, "this": true,
	"that": true, "these": true, "those": true, "there": true, "here": true,
	"where": true, "what": true, "who": true, "whom": true, "whose": true,
	"which": true, "why": true, "how": true, "all": true, "any": true,
	"both": true, "each": true, "few": true, "more": true, "most": true,
	"other": true, "some": true, "such": true, "no": true, "nor": true,
	"not": true, "only": true, "own": true, "same": true, "than": true,
	"too": true, "very": true,
}

func isNumberOrReference(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if romanNumerals[strings.ToLower(s)] {
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

func isAllCaps(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
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
		if isCapitalized(cased) && (commonTitles[lower] || (lowercaseCounts[lower] == 0 && !commonSentenceStarters[lower])) {
			properNouns[lower] = true
		}
	}

	return properNouns, canonicalCasing
}

func ExtractWords(r io.Reader) ([]string, error) {
	tokenizer := nethtml.NewTokenizer(r)
	var words []string
	var ignoreDepth int

	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case nethtml.ErrorToken:
			err := tokenizer.Err()
			if err == io.EOF {
				return words, nil
			}
			return nil, err

		case nethtml.StartTagToken:
			token := tokenizer.Token()
			tagName := strings.ToLower(token.Data)
			if tagName == "script" || tagName == "style" {
				ignoreDepth++
			}

		case nethtml.EndTagToken:
			token := tokenizer.Token()
			tagName := strings.ToLower(token.Data)
			if tagName == "script" || tagName == "style" {
				if ignoreDepth > 0 {
					ignoreDepth--
				}
			}

		case nethtml.TextToken:
			if ignoreDepth > 0 {
				continue
			}

			token := tokenizer.Token()
			text := strings.TrimSpace(stdhtml.UnescapeString(token.Data))
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

func AddtoDB(words []string, db *sql.DB) {
	startTime := time.Now()
	fmt.Printf("\n========================================\n")
	fmt.Printf("[Translation] Starting batch processing for %d raw word tokens...\n", len(words))

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

	fmt.Printf("[Step 2/3] Processing synonyms & proper nouns (10 worker pool)...\n")

	idx := 0
	for lower, input := range uniqueWords {
		isProper := properNouns[lower]

		wg.Add(1)
		go func(i int, word string, isProperNoun bool) {
			defer wg.Done()

			if isProperNoun {
				payloads[i] = wordPayload{
					input:    word,
					isProper: true,
				}
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
	fmt.Printf("           -> Synonym processing completed in %v\n", time.Since(fetchStart).Round(time.Millisecond))

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
		input := strings.ToLower(item.input)

		if item.isProper {
			insertQuery := `
                INSERT INTO server_dictionary (foreign_word, english_translations) 
                VALUES (?, json_array(?))
                ON CONFLICT(foreign_word) DO UPDATE SET 
                    english_translations = json_insert(english_translations, '$[#]', ?)
            `
			_, _ = tx.Exec(insertQuery, input, input, input)
			countInserted++
			continue
		}

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
			generatedForeignWord := GenerateForeignWord(input)
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

	fmt.Printf("           -> DB transaction committed in %v\n", time.Since(dbStart).Round(time.Millisecond))
	fmt.Printf("\n--- Batch Summary ---\n")
	fmt.Printf(" Already in DB : %d\n", countExisting)
	fmt.Printf(" Mapped to Syn : %d\n", countUpdated)
	fmt.Printf(" Newly Created : %d\n", countInserted)
	fmt.Printf(" Errors/Skipped: %d\n", countSkipped)
	fmt.Printf(" Total Elapsed : %v\n", time.Since(startTime).Round(time.Millisecond))
	fmt.Printf("========================================\n\n")
}

// LoadDictionary loads English -> Foreign mappings from server_dictionary into an in-memory map.
func LoadDictionary(db DBQuerier) (map[string]string, error) {
	dict := make(map[string]string)

	query := `
        SELECT LOWER(json_each.value) AS english_word, server_dictionary.foreign_word
        FROM server_dictionary, json_each(server_dictionary.english_translations)
        ORDER BY CASE WHEN LOWER(server_dictionary.foreign_word) = LOWER(json_each.value) THEN 1 ELSE 0 END ASC
    `
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var english, foreign string
		if err := rows.Scan(&english, &foreign); err == nil {
			english = strings.TrimSpace(strings.ToLower(english))
			foreign = strings.TrimSpace(foreign)
			if english != "" && foreign != "" {
				if existing, exists := dict[english]; exists && strings.ToLower(existing) == english && strings.ToLower(foreign) != english {
					continue
				}
				dict[english] = foreign
			}
		}
	}

	return dict, nil
}

// LoadUserDictionary maps English document words to the user's custom user_definition for a given user ID (id).
func LoadUserDictionary(db DBQuerier, userID string) (map[string]string, error) {
	dict := make(map[string]string)

	query := `
        SELECT LOWER(json_each.value) AS english_word, user_dictionary.user_definition
        FROM user_dictionary
        JOIN server_dictionary ON LOWER(user_dictionary.foreign_word) = LOWER(server_dictionary.foreign_word),
             json_each(server_dictionary.english_translations)
        WHERE user_dictionary.id = ?
    `
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var english, userDef string
		if err := rows.Scan(&english, &userDef); err == nil {
			english = strings.TrimSpace(strings.ToLower(english))
			userDef = strings.TrimSpace(userDef)
			if english != "" && userDef != "" {
				dict[english] = userDef
			}
		}
	}

	return dict, nil
}

// TranslateHTML processes raw English HTML using the server dictionary for full conlang rendering.
func TranslateHTML(englishHTML string, dict map[string]string) (string, error) {
	return processHTML(englishHTML, dict, false)
}

// TranslateUserHTML processes raw English HTML using the user's dictionary, substituting missing words with [unknown].
func TranslateUserHTML(englishHTML string, userDict map[string]string) (string, error) {
	return processHTML(englishHTML, userDict, true)
}

func processHTML(englishHTML string, dict map[string]string, fallbackUnknown bool) (string, error) {
	rawHTML := stdhtml.UnescapeString(englishHTML)

	contextNode := &nethtml.Node{
		Type:     nethtml.ElementNode,
		Data:     "body",
		DataAtom: atom.Body,
	}
	nodes, err := nethtml.ParseFragment(strings.NewReader(rawHTML), contextNode)
	if err != nil {
		return "", err
	}

	var traverse func(*nethtml.Node)
	traverse = func(n *nethtml.Node) {
		if n.Type == nethtml.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == nethtml.TextNode {
			n.Data = translateTextString(n.Data, dict, fallbackUnknown)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	var buf bytes.Buffer
	for _, node := range nodes {
		traverse(node)
		if err := nethtml.Render(&buf, node); err != nil {
			return "", err
		}
	}

	return buf.String(), nil
}

// translateTextString translates tokens while preserving formatting, casing, numbers, and fallback handling
func translateTextString(text string, dict map[string]string, fallbackUnknown bool) string {
	var builder strings.Builder
	var wordBuf strings.Builder

	flushWord := func() {
		if wordBuf.Len() == 0 {
			return
		}
		original := wordBuf.String()
		lower := strings.ToLower(original)

		if isNumberOrReference(original) {
			builder.WriteString(original)
		} else if foreign, found := dict[lower]; found {
			if isAllCaps(original) {
				foreign = strings.ToUpper(foreign)
			} else if len(original) > 0 && unicode.IsUpper([]rune(original)[0]) {
				runes := []rune(strings.ToLower(foreign))
				runes[0] = unicode.ToUpper(runes[0])
				foreign = string(runes)
			} else {
				foreign = strings.ToLower(foreign)
			}
			builder.WriteString(foreign)
		} else if fallbackUnknown {
			if isAllCaps(original) {
				builder.WriteString("[UNKNOWN]")
			} else if len(original) > 0 && unicode.IsUpper([]rune(original)[0]) {
				builder.WriteString("[Unknown]")
			} else {
				builder.WriteString("[unknown]")
			}
		} else {
			builder.WriteString(original)
		}
		wordBuf.Reset()
	}

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			wordBuf.WriteRune(r)
		} else {
			flushWord()
			builder.WriteRune(r)
		}
	}
	flushWord()

	return builder.String()
}
