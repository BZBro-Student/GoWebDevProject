package handler_test

import (
	"AGoTTHT/internal/handler"
	"bytes"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	// Create document schema matching handler expectations
	schema := `
	CREATE TABLE IF NOT EXISTS document (
		doc_id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("Failed to create document table: %v", err)
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

	// Seed document to delete
	docID := "doc-to-delete-123"
	_, err := db.Exec(
		"INSERT INTO document (doc_id, title, content, timestamp) VALUES (?, ?, ?, ?)",
		docID, "Delete Me", "<p>Content</p>", "2026-10-27T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("Failed to seed database: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/documents/"+docID, nil)
	// Go 1.22+ path value matching
	req.SetPathValue("id", docID)

	rec := httptest.NewRecorder()

	handler.HandleDelete(db)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Verify document was removed from database
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
	// Do NOT set path value to test missing parameter case
	rec := httptest.NewRecorder()

	handler.HandleDelete(db)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request, got %d", rec.Code)
	}
}

func TestHandleUpload_MarkdownFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create multipart form payload
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

	// Verify database received converted HTML entry
	var title, content string
	err = db.QueryRow("SELECT title, content FROM document LIMIT 1").Scan(&title, &content)
	if err != nil {
		t.Fatalf("Document was not inserted into database: %v", err)
	}

	if title != "sample.md" {
		t.Errorf("Expected title 'sample.md', got '%s'", title)
	}

	// Goldmark converts `# Test Header` to `<h1>Test Header</h1>`
	if !strings.Contains(content, "<h1>Test Header</h1>") {
		t.Errorf("Expected converted HTML content, got '%s'", content)
	}
}
