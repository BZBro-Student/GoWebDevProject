package handler

import (
	"AGoTTHT/internal/translation"
	"AGoTTHT/view/components"
	"AGoTTHT/view/layout"
	"bytes"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
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
	renderHelper(w, r, components.Desk())
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
