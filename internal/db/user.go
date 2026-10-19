package db

import (
	"database/sql"
	"fmt"
	"time"
)

type User struct {
	ID           string
	Username     string
	IsAdmin      bool
	CreationTime string
}

// FindOrCreateUser looks up a user by GitHub ID string or creates a new record
func FindOrCreateUser(db *sql.DB, githubID int64, username string) (*User, error) {
	var u User
	var isAdminInt int

	idStr := fmt.Sprintf("%d", githubID)

	// 1. Try to find existing user in `user` table
	query := `SELECT id, username, is_admin, creation_time FROM user WHERE id = ?`
	err := db.QueryRow(query, idStr).Scan(&u.ID, &u.Username, &isAdminInt, &u.CreationTime)

	if err == nil {
		u.IsAdmin = (isAdminInt == 1)
		return &u, nil
	}

	if err != sql.ErrNoRows {
		return nil, err
	}

	// 2. User does not exist; insert new record
	creationTimeStr := time.Now().UTC().Format(time.RFC3339)
	insertQuery := `INSERT INTO user (id, username, is_admin, creation_time) VALUES (?, ?, 0, ?)`

	if _, err := db.Exec(insertQuery, idStr, username, creationTimeStr); err != nil {
		return nil, err
	}

	u.ID = idStr
	u.Username = username
	u.IsAdmin = false
	u.CreationTime = creationTimeStr

	return &u, nil
}
