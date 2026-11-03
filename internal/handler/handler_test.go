package handler_test

import (
	"bytes"
	"database/sql"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"AGoTTHT/internal/handler"

	_ "github.com/mattn/go-sqlite3"
)

func TestHandleLoginPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	handler.HandleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleDeskPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.HandleDesk(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleDictionaryPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dictionary", nil)
	rec := httptest.NewRecorder()

	handler.HandleDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleCommunityPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/community", nil)
	rec := httptest.NewRecorder()

	handler.HandleCommunity(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleTutorialPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tutorial", nil)
	rec := httptest.NewRecorder()

	handler.HandleTutorial(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleAdminPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	handler.HandleAdmin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleHelpPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()

	handler.HandleHelp(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandleSettingsPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	handler.HandleSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	db.SetMaxOpenConns(1)

	schema := `
	CREATE TABLE IF NOT EXISTS user (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		is_admin INTEGER NOT NULL DEFAULT 0,
		creation_time TEXT
	);

	CREATE TABLE IF NOT EXISTS server_dictionary (
		foreign_word TEXT PRIMARY KEY,
		english_translations TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS user_dictionary (
		id TEXT NOT NULL,
		foreign_word TEXT NOT NULL,
		user_definition TEXT NOT NULL,
		update_time TEXT,
		PRIMARY KEY (id, foreign_word),
		FOREIGN KEY (id) REFERENCES user(id),
		FOREIGN KEY (foreign_word) REFERENCES server_dictionary(foreign_word)
	);

	CREATE TABLE IF NOT EXISTS document (
		doc_id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		translated_content TEXT,
		timestamp TEXT
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("Failed to create database schema: %v", err)
	}

	return db
}

func TestHandleSearch_EmptyQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/search?q=", nil)
	rec := httptest.NewRecorder()

	handler.HandleSearch(db)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "No document selected or found.") {
		t.Errorf("Expected empty state response, got: %s", rec.Body.String())
	}
}

func TestHandleSearch_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/search?q=NonExistent", nil)
	rec := httptest.NewRecorder()

	handler.HandleSearch(db)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "No document selected or found.") {
		t.Errorf("Expected empty result message, got: %s", rec.Body.String())
	}
}

func TestHandleDelete_Success(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	docID := "doc-to-delete-123"
	_, err := db.Exec(
		"INSERT INTO document (doc_id, title, content, timestamp) VALUES (?, ?, ?, ?)",
		docID, "Delete Me", "<p>Content</p>", "2026-10-27T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("Failed to seed database: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/documents/"+docID, nil)
	req.SetPathValue("id", docID)

	rec := httptest.NewRecorder()

	handler.HandleDelete(db)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM document WHERE doc_id = ?", docID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected document to be deleted, but found %d rows", count)
	}
}

func TestHandleDelete_MissingID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodDelete, "/documents/", nil)
	rec := httptest.NewRecorder()

	handler.HandleDelete(db)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request, got %d", rec.Code)
	}
}

func TestHandleUpload_MarkdownFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("doc_file", "sample.md")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}

	mdContent := "# Test Header\n\nThis is markdown body content."
	_, _ = fileWriter.Write([]byte(mdContent))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rec := httptest.NewRecorder()

	handler.HandleUpload(db)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var title, content string
	err = db.QueryRow("SELECT title, content FROM document LIMIT 1").Scan(&title, &content)
	if err != nil {
		t.Fatalf("Document was not inserted into database: %v", err)
	}

	if title != "sample.md" {
		t.Errorf("Expected title 'sample.md', got '%s'", title)
	}

	if !strings.Contains(content, "<h1>Test Header</h1>") {
		t.Errorf("Expected converted HTML content, got '%s'", content)
	}
}
