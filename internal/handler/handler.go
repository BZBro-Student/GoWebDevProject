package handler

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"AGoTTHT/internal/censor"
	"AGoTTHT/internal/translation"
	"AGoTTHT/view/components"
	"AGoTTHT/view/layout"

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

func HandleSettings(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/settings" {
			http.NotFound(w, r)
			return
		}

		var username string
		if userID := getUserID(r, sm); userID != "" {
			_ = db.QueryRow("SELECT username FROM user WHERE id = ?", userID).Scan(&username)
		}

		renderHelper(w, r, components.Settings(username))
	}
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

		// 1. Left Pane: Conlang translation
		leftPaneHTML := cachedServerHTML.String
		if leftPaneHTML == "" {
			serverDict, _ := translation.LoadDictionary(db)
			leftPaneHTML, _ = translation.TranslateHTML(doc.Content, serverDict)
		}

		// 2. Right Pane: User dictionary progress translation
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

		components.DocumentPanesResponse(doc.ID, leftPaneHTML, rightPaneHTML).Render(r.Context(), w)
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

// Check database directly for user's admin privilege flag
func checkIsAdmin(db *sql.DB, r *http.Request, sm *scs.SessionManager) bool {
	userID := getUserID(r, sm)
	if userID == "" || db == nil {
		return false
	}

	var isAdmin int
	err := db.QueryRow("SELECT is_admin FROM user WHERE id = ?", userID).Scan(&isAdmin)
	if err == nil {
		return isAdmin == 1
	}

	// Session fallback check
	if sm != nil {
		if v, ok := sm.Get(r.Context(), "is_admin").(bool); ok {
			return v
		}
		if v, ok := sm.Get(r.Context(), "is_admin").(int); ok {
			return v == 1
		}
		if v, ok := sm.Get(r.Context(), "role").(string); ok && v == "admin" {
			return true
		}
	}

	return false
}

const postsPerPage = 5

// HandleCommunity renders the initial full community page with both forums
func HandleCommunity(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/community" {
			http.NotFound(w, r)
			return
		}

		generalProps := fetchForumProps(db, "general", "General Announcements & News", 1)
		qnaProps := fetchForumProps(db, "qna", "Conlang Q&A & Support", 1)

		renderHelper(w, r, components.Community(generalProps, qnaProps))
	}
}

// HandleGetForumSection renders an HTMX outerHTML partial for single-forum pagination
func HandleGetForumSection(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		category := r.URL.Query().Get("category")
		if category == "" {
			category = "general"
		}

		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		if err != nil || page < 1 {
			page = 1
		}

		title := "General Announcements & News"
		if category == "qna" {
			title = "Conlang Q&A & Support"
		}

		props := fetchForumProps(db, category, title, page)
		components.ForumSection(props).Render(r.Context(), w)
	}
}

func fetchForumProps(db *sql.DB, category string, title string, page int) components.ForumProps {
	offset := (page - 1) * postsPerPage

	// Count total threads in this category
	var totalPosts int
	_ = db.QueryRow("SELECT COUNT(*) FROM forum WHERE category_id = ?", category).Scan(&totalPosts)

	totalPages := (totalPosts + postsPerPage - 1) / postsPerPage
	if totalPages < 1 {
		totalPages = 1
	}

	// Fetch paginated threads matching schema
	query := `
        SELECT f.thread_id, f.title, COALESCE(u.username, 'Anonymous'), f.created_at,
               (SELECT COUNT(*) FROM replies r WHERE r.thread_id = f.thread_id) AS reply_count
        FROM forum f
        LEFT JOIN user u ON f.author_id = u.id
        WHERE f.category_id = ?
        ORDER BY f.created_at DESC
        LIMIT ? OFFSET ?
    `
	rows, err := db.Query(query, category, postsPerPage, offset)
	var posts []components.ForumPost

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p components.ForumPost
			if err := rows.Scan(&p.ID, &p.Title, &p.Author, &p.CreatedAt, &p.Replies); err == nil {
				posts = append(posts, p)
			}
		}
	}

	return components.ForumProps{
		Category:    category,
		Title:       title,
		Posts:       posts,
		CurrentPage: page,
		TotalPages:  totalPages,
	}
}

func HandleNewPostModal(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	if category == "" {
		category = "general"
	}

	components.NewPostModal(category).Render(r.Context(), w)
}

// HandleCreatePost saves a new forum post with auto-censoring applied to title & content
func HandleCreatePost(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		category := strings.TrimSpace(r.FormValue("category"))
		title := strings.TrimSpace(r.FormValue("title"))
		content := strings.TrimSpace(r.FormValue("content"))
		userID := getUserID(r, sm)

		if category == "" || title == "" {
			http.Error(w, "Title and category are required", http.StatusBadRequest)
			return
		}

		// 1. Sanitize title and content against bad word filter
		title = censor.Sanitize(title)
		content = censor.Sanitize(content)

		threadID := generateUUID()
		now := time.Now().UTC().Format(time.RFC3339)

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Database transaction error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 2. Insert sanitized thread header into `forum`
		queryThread := `
            INSERT INTO forum (thread_id, category_id, author_id, title, locked, created_at)
            VALUES (?, ?, ?, ?, 0, ?)
        `
		_, err = tx.Exec(queryThread, threadID, category, userID, title, now)
		if err != nil {
			log.Printf("Failed to insert forum thread: %v", err)
			http.Error(w, "Database error creating thread", http.StatusInternalServerError)
			return
		}

		// 3. Insert sanitized initial post body into `replies`
		if content != "" {
			replyID := generateUUID()
			queryReply := `
                INSERT INTO replies (reply_id, thread_id, title, author_id, text, created_at)
                VALUES (?, ?, ?, ?, ?, ?)
            `
			_, err = tx.Exec(queryReply, replyID, threadID, title, userID, content, now)
			if err != nil {
				log.Printf("Failed to insert initial reply content: %v", err)
				http.Error(w, "Database error creating thread content", http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(); err != nil {
			log.Printf("Failed to commit transaction: %v", err)
			http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
			return
		}

		// Re-fetch updated page 1 for target category section
		sectionTitle := "General Announcements & News"
		if category == "qna" {
			sectionTitle = "Conlang Q&A & Support"
		}

		props := fetchForumProps(db, category, sectionTitle, 1)
		components.ForumSection(props).Render(r.Context(), w)
	}
}

// HandleGetThreadDetails fetches thread metadata and reply messages for the modal
func HandleGetThreadDetails(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		threadID := r.PathValue("id")
		if threadID == "" {
			http.Error(w, "Missing thread ID", http.StatusBadRequest)
			return
		}

		props, err := fetchThreadDetails(db, threadID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Thread not found", http.StatusNotFound)
				return
			}
			log.Printf("Error fetching thread details: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		props.IsAdmin = checkIsAdmin(db, r, sm)
		components.ThreadModal(props).Render(r.Context(), w)
	}
}

// HandleToggleLockThread toggles the locked state (0 <-> 1) of a thread
func HandleToggleLockThread(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkIsAdmin(db, r, sm) {
			http.Error(w, "Unauthorized: Admin privileges required", http.StatusForbidden)
			return
		}

		threadID := r.PathValue("id")
		if threadID == "" {
			http.Error(w, "Missing thread ID", http.StatusBadRequest)
			return
		}

		query := `UPDATE forum SET locked = CASE WHEN locked = 1 THEN 0 ELSE 1 END WHERE thread_id = ?`
		_, err := db.Exec(query, threadID)
		if err != nil {
			log.Printf("Failed to toggle thread lock status: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		props, err := fetchThreadDetails(db, threadID)
		if err != nil {
			http.Error(w, "Database error loading thread", http.StatusInternalServerError)
			return
		}

		props.IsAdmin = true
		components.ThreadModal(props).Render(r.Context(), w)
	}
}

// HandleDeleteThread deletes the thread and all associated replies, then triggers a page reload
func HandleDeleteThread(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkIsAdmin(db, r, sm) {
			http.Error(w, "Unauthorized: Admin privileges required", http.StatusForbidden)
			return
		}

		threadID := r.PathValue("id")
		if threadID == "" {
			http.Error(w, "Missing thread ID", http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Database transaction error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		_, err = tx.Exec("DELETE FROM replies WHERE thread_id = ?", threadID)
		if err != nil {
			log.Printf("Failed to delete replies for thread %s: %v", threadID, err)
			http.Error(w, "Database error deleting replies", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec("DELETE FROM forum WHERE thread_id = ?", threadID)
		if err != nil {
			log.Printf("Failed to delete thread %s: %v", threadID, err)
			http.Error(w, "Database error deleting thread", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
			return
		}

		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
	}
}

// HandleCreateReply inserts a sanitized reply into SQLite and re-renders the ThreadModal
func HandleCreateReply(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		threadID := r.PathValue("id")
		if threadID == "" {
			http.Error(w, "Missing thread ID", http.StatusBadRequest)
			return
		}

		// Check if thread is locked (using COALESCE to fallback NULL to 0)
		var threadTitle string
		var isLocked int
		err := db.QueryRow("SELECT title, COALESCE(locked, 0) FROM forum WHERE thread_id = ?", threadID).Scan(&threadTitle, &isLocked)
		if err != nil {
			http.Error(w, "Thread not found", http.StatusNotFound)
			return
		}

		if isLocked == 1 {
			http.Error(w, "This thread is locked. Replies are disabled.", http.StatusForbidden)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		replyText := strings.TrimSpace(r.FormValue("text"))
		userID := getUserID(r, sm)

		if replyText == "" {
			http.Error(w, "Reply text cannot be empty", http.StatusBadRequest)
			return
		}

		// Sanitize reply text against bad word filter
		replyText = censor.Sanitize(replyText)

		replyID := generateUUID()
		now := time.Now().UTC().Format(time.RFC3339)

		query := `
            INSERT INTO replies (reply_id, thread_id, title, author_id, text, created_at)
            VALUES (?, ?, ?, ?, ?, ?)
        `
		_, err = db.Exec(query, replyID, threadID, threadTitle, userID, replyText, now)
		if err != nil {
			log.Printf("Failed to insert reply: %v", err)
			http.Error(w, "Database error inserting reply", http.StatusInternalServerError)
			return
		}

		props, err := fetchThreadDetails(db, threadID)
		if err != nil {
			http.Error(w, "Database error loading updated thread", http.StatusInternalServerError)
			return
		}

		props.IsAdmin = checkIsAdmin(db, r, sm)
		components.ThreadModal(props).Render(r.Context(), w)
	}
}

func fetchThreadDetails(db *sql.DB, threadID string) (components.ThreadDetailsProps, error) {
	var props components.ThreadDetailsProps
	var lockedInt int

	// Query thread header info with NULL safety
	queryThread := `
        SELECT f.thread_id, f.category_id, f.title, COALESCE(u.username, 'Anonymous'), COALESCE(f.locked, 0), f.created_at
        FROM forum f
        LEFT JOIN user u ON f.author_id = u.id
        WHERE f.thread_id = ?
    `
	err := db.QueryRow(queryThread, threadID).Scan(&props.ThreadID, &props.Category, &props.Title, &props.Author, &lockedInt, &props.CreatedAt)
	if err != nil {
		return props, err
	}

	props.IsLocked = (lockedInt == 1)

	// Query replies
	queryReplies := `
        SELECT r.reply_id, COALESCE(u.username, 'Anonymous'), r.text, r.created_at
        FROM replies r
        LEFT JOIN user u ON r.author_id = u.id
        WHERE r.thread_id = ?
        ORDER BY r.created_at ASC
    `
	rows, err := db.Query(queryReplies, threadID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var r components.ReplyItem
			if err := rows.Scan(&r.ID, &r.Author, &r.Text, &r.CreatedAt); err == nil {
				props.Replies = append(props.Replies, r)
			}
		}
	}
	return props, nil
}

const (
	maxChatLength   = 100 // Character limit
	chatCooldownSec = 2   // 10-second reply cooldown per user
)

func HandleGetChatMessages(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		_, _ = db.Exec("DELETE FROM global_chat_message WHERE timestamp < ?", cutoff)

		messages, err := fetchActiveChatMessages(db)
		if err != nil {
			log.Printf("Error fetching chat messages: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		userID := getUserID(r, sm)
		isAdmin := checkIsAdmin(db, r, sm)
		components.ChatMessages(messages, userID, isAdmin).Render(r.Context(), w)
	}
}

// HandlePostChatMessage processes new chat submissions with auto-censoring, character limits, and a 10s cooldown
func HandlePostChatMessage(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r, sm)
		if userID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			return
		}

		msgText := strings.TrimSpace(r.FormValue("message"))
		if msgText == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// 1. Censor profanity using censor package
		msgText = censor.Sanitize(msgText)

		// 2. Enforce character limit
		if len(msgText) > maxChatLength {
			msgText = msgText[:maxChatLength]
		}

		// 3. Enforce 10-second per-user cooldown
		var lastPostStr string
		err := db.QueryRow(`
			SELECT timestamp FROM global_chat_message 
			WHERE sender_id = ? 
			ORDER BY timestamp DESC LIMIT 1
		`, userID).Scan(&lastPostStr)

		if err == nil {
			lastPostTime, parseErr := time.Parse(time.RFC3339, lastPostStr)
			if parseErr == nil && time.Since(lastPostTime) < time.Duration(chatCooldownSec)*time.Second {
				messages, _ := fetchActiveChatMessages(db)
				isAdmin := checkIsAdmin(db, r, sm)
				components.ChatMessages(messages, userID, isAdmin).Render(r.Context(), w)
				return
			}
		}

		msgID := generateUUID()
		now := time.Now().UTC().Format(time.RFC3339)

		query := `INSERT INTO global_chat_message (message_id, sender_id, content, timestamp) VALUES (?, ?, ?, ?)`
		_, err = db.Exec(query, msgID, userID, msgText, now)
		if err != nil {
			log.Printf("Failed to insert chat message: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		messages, _ := fetchActiveChatMessages(db)
		isAdmin := checkIsAdmin(db, r, sm)
		components.ChatMessages(messages, userID, isAdmin).Render(r.Context(), w)
	}
}

// HandleDeleteChatMessage allows users to delete their own messages, and admins to delete any
func HandleDeleteChatMessage(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r, sm)
		if userID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		isAdmin := checkIsAdmin(db, r, sm)
		msgID := r.PathValue("id")

		if msgID != "" {
			var err error
			if isAdmin {
				_, err = db.Exec("DELETE FROM global_chat_message WHERE message_id = ?", msgID)
			} else {
				_, err = db.Exec("DELETE FROM global_chat_message WHERE message_id = ? AND sender_id = ?", msgID, userID)
			}
			if err != nil {
				log.Printf("Failed to delete chat message %s: %v", msgID, err)
			}
		}

		messages, _ := fetchActiveChatMessages(db)
		components.ChatMessages(messages, userID, isAdmin).Render(r.Context(), w)
	}
}

func fetchActiveChatMessages(db *sql.DB) ([]components.ChatMessage, error) {
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	query := `
		SELECT m.message_id, m.sender_id, COALESCE(u.username, 'User'), m.content, m.timestamp
		FROM global_chat_message m
		LEFT JOIN user u ON m.sender_id = u.id
		WHERE m.timestamp >= ?
		ORDER BY m.timestamp ASC
		LIMIT 50
	`
	rows, err := db.Query(query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []components.ChatMessage
	for rows.Next() {
		var msg components.ChatMessage
		var rawTime string
		if err := rows.Scan(&msg.ID, &msg.UserID, &msg.Username, &msg.Message, &rawTime); err == nil {
			if parsedTime, parseErr := time.Parse(time.RFC3339, rawTime); parseErr == nil {
				msg.CreatedAt = parsedTime.Format("15:04")
			} else {
				msg.CreatedAt = rawTime
			}
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

// HandleClearDictionary wipes all saved dictionary entries for the current user
func HandleClearDictionary(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r, sm)
		if userID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		_, err := db.Exec("DELETE FROM user_dictionary WHERE id = ?", userID)
		if err != nil {
			log.Printf("Failed to clear dictionary for user %s: %v", userID, err)
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<div class="bg-red-950/50 border border-red-700 text-red-200 px-4 py-3 rounded text-xs">Failed to clear dictionary data. Please try again.</div>`)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<div class="bg-emerald-950/50 border border-emerald-700 text-emerald-200 px-4 py-3 rounded text-xs">Your personal dictionary has been cleared.</div>`)
	}
}

// HandleDeleteAccount permanently deletes the user's data, anonymizes forum history, and destroys the session
// Make sure "io" and "net/url" are imported at the top of the file

func HandleDeleteAccount(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r, sm)
		if userID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Go's r.ParseForm() skips the request body on DELETE requests, so parse it explicitly
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Invalid form submission", http.StatusBadRequest)
			return
		}

		values, err := url.ParseQuery(string(bodyBytes))
		if err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		confirmVal := strings.TrimSpace(values.Get("confirm_delete"))
		if confirmVal == "" {
			confirmVal = strings.TrimSpace(r.URL.Query().Get("confirm_delete"))
		}

		if confirmVal != "DELETE" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<div class="bg-red-950/50 border border-red-700 text-red-200 px-4 py-3 rounded text-xs">Confirmation failed. Please type <strong>DELETE</strong> in all caps to delete your account.</div>`)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Database transaction error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 1. Delete user's dictionary entries
		if _, err := tx.Exec("DELETE FROM user_dictionary WHERE id = ?", userID); err != nil {
			log.Printf("Account deletion - failed removing dictionary for %s: %v", userID, err)
			http.Error(w, "Error deleting dictionary data", http.StatusInternalServerError)
			return
		}

		// 2. Delete user's active global chat messages
		if _, err := tx.Exec("DELETE FROM global_chat_message WHERE sender_id = ?", userID); err != nil {
			log.Printf("Account deletion - failed removing chat messages for %s: %v", userID, err)
			http.Error(w, "Error deleting chat messages", http.StatusInternalServerError)
			return
		}

		// 3. Anonymize forum threads & replies so COALESCE(u.username, 'Anonymous') displays cleanly
		if _, err := tx.Exec("UPDATE forum SET author_id = NULL WHERE author_id = ?", userID); err != nil {
			log.Printf("Account deletion - failed anonymizing threads for %s: %v", userID, err)
			http.Error(w, "Error updating community threads", http.StatusInternalServerError)
			return
		}

		if _, err := tx.Exec("UPDATE replies SET author_id = NULL WHERE author_id = ?", userID); err != nil {
			log.Printf("Account deletion - failed anonymizing replies for %s: %v", userID, err)
			http.Error(w, "Error updating community replies", http.StatusInternalServerError)
			return
		}

		// 4. Delete the user record
		if _, err := tx.Exec("DELETE FROM user WHERE id = ?", userID); err != nil {
			log.Printf("Account deletion - failed deleting user %s: %v", userID, err)
			http.Error(w, "Error deleting user account", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Failed to finalize account deletion", http.StatusInternalServerError)
			return
		}

		// 5. Destroy SCS session and redirect home
		_ = sm.Destroy(r.Context())
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
	}
}
