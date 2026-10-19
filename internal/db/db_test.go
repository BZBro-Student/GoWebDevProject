package db

import (
	"path/filepath"
	"testing"
)

func TestInitDB_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_app.db")

	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("expected no error during InitDB, got: %v", err)
	}
	defer database.Close()

	if err := database.Ping(); err != nil {
		t.Fatalf("failed to ping initialized database: %v", err)
	}

	var fkStatus int
	err = database.QueryRow("PRAGMA foreign_keys;").Scan(&fkStatus)
	if err != nil {
		t.Fatalf("failed to query foreign_keys pragma: %v", err)
	}
	if fkStatus != 1 {
		t.Errorf("expected foreign_keys to be enabled (1), got %d", fkStatus)
	}

	var journalMode string
	err = database.QueryRow("PRAGMA journal_mode;").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode pragma: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode to be 'wal', got %s", journalMode)
	}
}

func TestInitDB_TablesExist(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_tables.db")

	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	expectedTables := []string{
		"user",
		"user_dictionary",
		"server_dictionary",
		"document",
		"global_chat_message",
		"forum",
		"replies",
		"sessions",
	}

	for _, table := range expectedTables {
		t.Run("Table_"+table, func(t *testing.T) {
			var tableName string
			query := "SELECT name FROM sqlite_master WHERE type='table' AND name=?;"
			err := database.QueryRow(query, table).Scan(&tableName)
			if err != nil {
				t.Errorf("expected table '%s' to exist in database schema, scan error: %v", table, err)
			}
		})
	}
}

func TestInitDB_ForeignKeyConstraintsEnforced(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_fk.db")

	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	insertQuery := `INSERT INTO global_chat_message (message_id, sender_id, content, timestamp) VALUES (?, ?, ?, ?)`
	_, err = database.Exec(insertQuery, "msg1", "non_existent_user", "Hello", "2026-10-18T12:00:00Z")
	if err == nil {
		t.Error("expected foreign key constraint violation error, but got nil")
	}
}

func TestInitDB_InvalidPath(t *testing.T) {
	invalidPath := "/invalid_directory_12345/app.db"

	_, err := InitDB(invalidPath)
	if err == nil {
		t.Error("expected error when initializing DB at an invalid path, got nil")
	}
}
