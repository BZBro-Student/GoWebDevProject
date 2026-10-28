package handler

import (
	"AGoTTHT/internal/translation"
	"AGoTTHT/view/components"
	"AGoTTHT/view/layout"
	"bytes"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/a-h/templ"
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

	// Render your login component or page
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
			htmlOutput = buffer.Bytes()

		case ".html", ".htm":
			htmlOutput, err = io.ReadAll(file)
			if err != nil {
				http.Error(w, "Failed to read file", http.StatusInternalServerError)
				return
			}
		default:
			http.Error(w, "Failed to read file", http.StatusInternalServerError)
			return
		}

		docID := generateUUID()
		now := time.Now().UTC().Format(time.RFC3339)

		insertDocQuery := `
			INSERT INTO document (doc_id, title, content, timestamp) 
			VALUES (?, ?, ?, ?)
		`
		_, err = db.Exec(insertDocQuery, docID, header.Filename, string(htmlOutput), now)
		if err != nil {
			http.Error(w, "Failed to save document to database", http.StatusInternalServerError)
			return
		}
		words, err := translation.ExtractWords(bytes.NewReader(htmlOutput))
		if err != nil {
			http.Error(w, "Failed to extract words", http.StatusInternalServerError)
			return
		}

		// Pass the scoped db parameter
		translation.AddtoDB(words, db)

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

		// Search by exact ID, or matching title/content
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
				// Document not found
				components.ArticleResult(nil).Render(r.Context(), w)
				return
			}
			// Log error and return empty state
			log.Printf("Search database error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Render the found document
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

		// Return empty state to clear out the result box in HTMX
		components.ArticleResult(nil).Render(r.Context(), w)
	}
}

// HandleSearchDocuments searches for matching documents and renders SearchResultsList for the sidebar
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

// HandleLoadDocumentPanes fetches English HTML, translates it, and populates both Desk panes
func HandleLoadDocumentPanes(db *sql.DB) http.HandlerFunc {
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
		queryStr := `SELECT doc_id, title, content FROM document WHERE doc_id = ?`
		err := db.QueryRow(queryStr, docID).Scan(&doc.ID, &doc.Title, &doc.Content)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Document not found", http.StatusNotFound)
				return
			}
			log.Printf("Failed to fetch document: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		dict, err := translation.LoadDictionary(db)
		if err != nil {
			log.Printf("Error loading dictionary: %v", err)
		}

		translatedHTML, err := translation.TranslateHTML(doc.Content, dict)
		if err != nil {
			log.Printf("Error translating HTML: %v", err)
			translatedHTML = doc.Content
		}

		components.DocumentPanesResponse(doc.Content, translatedHTML).Render(r.Context(), w)
	}
}
