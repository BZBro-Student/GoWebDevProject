package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFindOrCreateUser_CreateNewUser(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_user_create.db")

	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	githubID := int64(123456)
	username := "octocat"

	user, err := FindOrCreateUser(database, githubID, username)
	if err != nil {
		t.Fatalf("expected no error creating user, got: %v", err)
	}

	if user.ID != "123456" {
		t.Errorf("expected ID '123456', got '%s'", user.ID)
	}
	if user.Username != username {
		t.Errorf("expected Username '%s', got '%s'", username, user.Username)
	}
	if user.IsAdmin != false {
		t.Errorf("expected IsAdmin false for new user, got true")
	}
	if _, err := time.Parse(time.RFC3339, user.CreationTime); err != nil {
		t.Errorf("expected valid RFC3339 CreationTime, got '%s'", user.CreationTime)
	}
}

func TestFindOrCreateUser_FindExistingUser(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_user_find.db")

	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	githubID := int64(789012)
	username := "existing_dev"

	// 1. Initial creation
	u1, err := FindOrCreateUser(database, githubID, username)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// 2. Subsequent call should find and return the existing record
	u2, err := FindOrCreateUser(database, githubID, username)
	if err != nil {
		t.Fatalf("failed to find existing user: %v", err)
	}

	if u1.ID != u2.ID || u1.Username != u2.Username || u1.CreationTime != u2.CreationTime {
		t.Errorf("expected identical user structs across calls, got original: %+v, retrieved: %+v", u1, u2)
	}
}

func TestFindOrCreateUser_FindExistingAdminUser(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_user_admin.db")

	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	// Seed an admin user directly into the database (is_admin = 1)
	githubID := int64(999999)
	idStr := "999999"
	username := "admin_user"
	creationTime := time.Now().UTC().Format(time.RFC3339)

	_, err = database.Exec(
		`INSERT INTO user (id, username, is_admin, creation_time) VALUES (?, ?, 1, ?)`,
		idStr, username, creationTime,
	)
	if err != nil {
		t.Fatalf("failed to seed admin user: %v", err)
	}

	// Verify that FindOrCreateUser maps SQLite integer 1 to Go boolean true
	user, err := FindOrCreateUser(database, githubID, username)
	if err != nil {
		t.Fatalf("expected no error retrieving admin user, got: %v", err)
	}

	if !user.IsAdmin {
		t.Errorf("expected IsAdmin to be true for admin user, got false")
	}
}

func TestFindOrCreateUser_DatabaseError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_closed_db.db")

	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}

	// Force a database query failure by closing the handle early
	database.Close()

	_, err = FindOrCreateUser(database, 12345, "error_test")
	if err == nil {
		t.Error("expected error when querying closed database, got nil")
	}
}
