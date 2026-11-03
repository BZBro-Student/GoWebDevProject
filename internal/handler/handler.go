package handler

import (
	"AGoTTHT/internal/translation"
	"AGoTTHT/view/components"
	"AGoTTHT/view/layout"
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
	"github.com/yuin/goldmark"
)

// renderHelper decides whether to send a partial or full page
func renderHelper(w http.ResponseWriter, r *http.Request, component templ.Component) {
	if r.Header.Get("HX-Request") == "true" {
		component.Render(r.Context(), w)
	} else {
		layout.Base(component).Render(r.Context(), w)
	}
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/login" {
		http.NotFound(w, r)
		return
	}

	renderHelper(w, r, components.Login())
}

// Page handlers
func HandleDesk(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Desk(nil, ""))
}

func HandleDictionary(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/dictionary" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Dictionary())
}

func HandleCommunity(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/community" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Community())
}

func HandleTutorial(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/tutorial" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Tutorial())
}

func HandleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Admin())
}

func HandleHelp(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/help" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Help())
}

func HandleSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings" {
		http.NotFound(w, r)
		return
	}
	renderHelper(w, r, components.Settings())
}

// Health API
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/health" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func HandleUpload(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file, header, err := r.FormFile("doc_file")
		if err != nil {
			http.Error(w, "Failed to upload", http.StatusBadRequest)
			return
		}
		defer file.Close()

		fileExtension := strings.ToLower(filepath.Ext(header.Filename))
		var htmlOutput []byte

		switch fileExtension {
		case ".md", ".markdown":
			mdBytes, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, "Failed to read file", http.StatusInternalServerError)
				return
			}
			var buffer bytes.Buffer
			if err := goldmark.Convert(mdBytes, &buffer); err != nil {
				http.Error(w, "Failed to convert Markdown", http.StatusInternalServerError)
				return
			}
			htmlOutput = []byte(html.UnescapeString(buffer.String()))

		case ".html", ".htm":
			rawBytes, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, "Failed to read file", http.StatusInternalServerError)
				return
			}
			htmlOutput = []byte(html.UnescapeString(string(rawBytes)))

		default:
			http.Error(w, "Unsupported file extension", http.StatusBadRequest)
			return
		}

		// 1. Extract words & process dictionary updates FIRST
		words, err := translation.ExtractWords(bytes.NewReader(htmlOutput))
		if err != nil {
			log.Printf("Failed to extract words: %v", err)
			http.Error(w, "Failed to extract words", http.StatusInternalServerError)
			return
		}
		translation.AddtoDB(words, db)

		// 2. Load dictionary & pre-translate server Conlang pane
		serverDict, err := translation.LoadDictionary(db)
		if err != nil {
			log.Printf("Error loading server dictionary: %v", err)
		}
		serverTranslatedHTML, err := translation.TranslateHTML(string(htmlOutput), serverDict)
		if err != nil {
			log.Printf("Error pre-translating HTML: %v", err)
			serverTranslatedHTML = string(htmlOutput)
		}

		// 3. Save raw content AND pre-translated content into SQLite
		docID := generateUUID()
		now := time.Now().UTC().Format(time.RFC3339)

		insertDocQuery := `
            INSERT INTO document (doc_id, title, content, translated_content, timestamp) 
            VALUES (?, ?, ?, ?, ?)
        `
		_, err = db.Exec(insertDocQuery, docID, header.Filename, string(htmlOutput), serverTranslatedHTML, now)
		if err != nil {
			log.Printf("Failed to save document to database: %v", err)
			http.Error(w, "Failed to save document to database", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<div class="success">File uploaded and processed successfully!</div>`))
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func HandleSearch(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			components.ArticleResult(nil).Render(r.Context(), w)
			return
		}

		queryStr := `
        SELECT doc_id, title, content 
        FROM document 
        WHERE doc_id = ? OR title LIKE ? OR content LIKE ? 
        LIMIT 1
    `
		searchPattern := "%" + query + "%"

		var doc components.Document
		err := db.QueryRow(queryStr, query, searchPattern, searchPattern).Scan(&doc.ID, &doc.Title, &doc.Content)

		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				components.ArticleResult(nil).Render(r.Context(), w)
				return
			}
			log.Printf("Search database error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		components.ArticleResult(&doc).Render(r.Context(), w)
	}
}

func HandleDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := r.PathValue("id")
		if docID == "" {
			http.Error(w, "Missing document ID", http.StatusBadRequest)
			return
		}

		_, err := db.Exec("DELETE FROM document WHERE doc_id = ?", docID)
		if err != nil {
			log.Printf("Delete database error: %v", err)
			http.Error(w, "Failed to delete document", http.StatusInternalServerError)
			return
		}

		components.ArticleResult(nil).Render(r.Context(), w)
	}
}

func HandleSearchDocuments(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query == "" {
			components.SearchResultsList([]components.SearchResultItem{}).Render(r.Context(), w)
			return
		}

		queryStr := `
            SELECT doc_id, title, SUBSTR(content, 1, 80) AS match 
            FROM document 
            WHERE title LIKE ? OR content LIKE ? 
            LIMIT 10
        `
		searchPattern := "%" + query + "%"

		rows, err := db.Query(queryStr, searchPattern, searchPattern)
		if err != nil {
			log.Printf("Search database error: %v", err)
			http.Error(w, "Search failed", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var results []components.SearchResultItem
		for rows.Next() {
			var item components.SearchResultItem
			if err := rows.Scan(&item.ID, &item.Title, &item.Match); err == nil {
				results = append(results, item)
			}
		}

		components.SearchResultsList(results).Render(r.Context(), w)
	}
}

func HandleSearchUserDictionary(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r, sm)

		query := strings.TrimSpace(r.URL.Query().Get("q"))
		pageStr := r.URL.Query().Get("page")
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			page = 1
		}

		limit := 10
		offset := (page - 1) * limit
		searchPattern := "%" + query + "%"

		countQuery := `
            SELECT COUNT(*) 
            FROM user_dictionary 
            WHERE id = ? AND (foreign_word LIKE ? OR user_definition LIKE ?)
        `
		var totalCount int
		err = db.QueryRow(countQuery, userID, searchPattern, searchPattern).Scan(&totalCount)
		if err != nil {
			log.Printf("Error counting user dictionary entries: %v", err)
			totalCount = 0
		}

		selectQuery := `
            SELECT id, foreign_word, user_definition, update_time 
            FROM user_dictionary 
            WHERE id = ? AND (foreign_word LIKE ? OR user_definition LIKE ?)
            ORDER BY update_time DESC 
            LIMIT ? OFFSET ?
        `
		rows, err := db.Query(selectQuery, userID, searchPattern, searchPattern, limit, offset)
		var entries []components.UserDictEntry

		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var item components.UserDictEntry
				var updateTime sql.NullString
				if err := rows.Scan(&item.ID, &item.ForeignWord, &item.UserDefinition, &updateTime); err == nil {
					if updateTime.Valid {
						item.UpdateTime = updateTime.String
					}
					entries = append(entries, item)
				}
			}
		}

		totalPages := (totalCount + limit - 1) / limit
		if totalPages < 1 {
			totalPages = 1
		}

		props := components.DictionaryResultsProps{
			Entries:     entries,
			Query:       query,
			CurrentPage: page,
			TotalPages:  totalPages,
			TotalCount:  totalCount,
		}

		components.DictionaryResults(props).Render(r.Context(), w)
	}
}

// HandleLoadDocumentPanes fetches English HTML, translates it, and populates both Desk panes
func HandleLoadDocumentPanes(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := r.PathValue("id")
		if docID == "" {
			docID = r.URL.Query().Get("id")
		}

		if docID == "" {
			http.Error(w, "Missing document ID", http.StatusBadRequest)
			return
		}

		var doc components.Document
		var cachedServerHTML sql.NullString

		queryStr := `SELECT doc_id, title, content, translated_content FROM document WHERE doc_id = ?`
		err := db.QueryRow(queryStr, docID).Scan(&doc.ID, &doc.Title, &doc.Content, &cachedServerHTML)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Document not found", http.StatusNotFound)
				return
			}
			log.Printf("Failed to fetch document: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// 1. Left Pane: Use cached translation from DB (Fallback to on-the-fly if empty)
		leftPaneHTML := cachedServerHTML.String
		if leftPaneHTML == "" {
			serverDict, _ := translation.LoadDictionary(db)
			leftPaneHTML, _ = translation.TranslateHTML(doc.Content, serverDict)
		}

		// 2. Right Pane: User specific progress translation
		userID := getUserID(r, sm)
		userDict, err := translation.LoadUserDictionary(db, userID)
		if err != nil {
			log.Printf("Error loading user dictionary for user %s: %v", userID, err)
		}

		rightPaneHTML, err := translation.TranslateUserHTML(doc.Content, userDict)
		if err != nil {
			log.Printf("Error translating user progress HTML: %v", err)
			rightPaneHTML = doc.Content
		}

		// 3. Base64 Encode both panes for clean iframe rendering
		leftPaneURI := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(leftPaneHTML))
		rightPaneURI := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(rightPaneHTML))

		components.DocumentPanesResponse(doc.ID, leftPaneURI, rightPaneURI).Render(r.Context(), w)
	}
}

func HandleAddUserDictionary(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		foreignInput := strings.TrimSpace(r.FormValue("foreign_word"))
		userDefinition := strings.TrimSpace(r.FormValue("user_definition"))
		userID := getUserID(r, sm)

		if userID == "" {
			log.Printf("[ERROR] Session user_id is missing or empty")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<div class="text-red-400">❌ Session expired. Please log in again.</div>`))
			return
		}

		if foreignInput == "" || userDefinition == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<div class="text-red-400">⚠️ Please provide both word and definition.</div>`))
			return
		}

		var dbUserID string
		err := db.QueryRow("SELECT id FROM user WHERE id = ?", userID).Scan(&dbUserID)
		if err != nil {
			log.Printf("[ERROR] User ID %q not found in user table: %v", userID, err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<div class="text-red-400">❌ User record not found. Please log in again.</div>`))
			return
		}

		var exactForeignWord string
		err = db.QueryRow("SELECT foreign_word FROM server_dictionary WHERE LOWER(foreign_word) = LOWER(?) LIMIT 1", foreignInput).Scan(&exactForeignWord)
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(fmt.Sprintf(`<div class="text-yellow-400">⚠️ %q does not exist in server dictionary.</div>`, html.EscapeString(foreignInput))))
			return
		} else if err != nil {
			log.Printf("[ERROR] Server dictionary query failed: %v", err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<div class="text-red-400">❌ Database error checking server dictionary.</div>`))
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		query := `
            INSERT INTO user_dictionary (id, foreign_word, user_definition, update_time)
            VALUES (?, ?, ?, ?)
            ON CONFLICT(id, foreign_word) DO UPDATE SET
                user_definition = excluded.user_definition,
                update_time = excluded.update_time
        `
		_, err = db.Exec(query, dbUserID, exactForeignWord, userDefinition, now)
		if err != nil {
			log.Printf("[ERROR] FK Insert into user_dictionary failed (userID=%s, word=%q): %v", dbUserID, exactForeignWord, err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(fmt.Sprintf(`<div class="text-red-400">❌ Error saving entry %q</div>`, html.EscapeString(exactForeignWord))))
			return
		}

		w.Header().Set("HX-Trigger", "refreshDictionary, reloadDocument")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(fmt.Sprintf(
			`<div class="text-green-400">✓ Added: <span class="font-bold text-white">%s</span> → %s</div>`,
			html.EscapeString(exactForeignWord),
			html.EscapeString(userDefinition),
		)))
	}
}

func HandleRemoveUserDictionary(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		foreignWord := strings.TrimSpace(r.FormValue("foreign_word"))
		userID := getUserID(r, sm)

		if foreignWord == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<div class="text-red-400">⚠️ Please enter a foreign word to remove.</div>`))
			return
		}

		query := `DELETE FROM user_dictionary WHERE id = ? AND LOWER(foreign_word) = LOWER(?)`
		res, err := db.Exec(query, userID, foreignWord)
		if err != nil {
			log.Printf("Failed to remove user dictionary entry: %v", err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(fmt.Sprintf(`<div class="text-red-400">❌ Error removing %q</div>`, html.EscapeString(foreignWord))))
			return
		}

		rowsAffected, _ := res.RowsAffected()
		w.Header().Set("HX-Trigger", "refreshDictionary, reloadDocument")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if rowsAffected == 0 {
			w.Write([]byte(fmt.Sprintf(`<div class="text-yellow-400">⚠️ %q not found in dictionary.</div>`, html.EscapeString(foreignWord))))
		} else {
			w.Write([]byte(fmt.Sprintf(`<div class="text-red-400">✓ Removed: <span class="font-bold text-white">%s</span></div>`, html.EscapeString(foreignWord))))
		}
	}
}

func getUserID(r *http.Request, sm *scs.SessionManager) string {
	keys := []string{"user_id", "userID", "id", "github_id"}

	for _, k := range keys {
		val := sm.Get(r.Context(), k)
		if val == nil {
			continue
		}

		switch v := val.(type) {
		case string:
			if v != "" {
				return v
			}
		case int64:
			if v != 0 {
				return fmt.Sprintf("%d", v)
			}
		case int:
			if v != 0 {
				return fmt.Sprintf("%d", v)
			}
		case float64:
			if v != 0 {
				return fmt.Sprintf("%.0f", v)
			}
		}
	}

	return ""
}
