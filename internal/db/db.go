package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS user (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    is_admin INTEGER DEFAULT 0,
    creation_time TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_dictionary (
    id TEXT,
    user_definition TEXT,
    foreign_word TEXT,
    update_time TEXT,
    PRIMARY KEY (id, foreign_word),
    FOREIGN KEY (id) REFERENCES user(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS server_dictionary (
    foreign_word TEXT PRIMARY KEY,
    english_translations TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS document (
    doc_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT,
    timestamp TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS global_chat_message (
    message_id TEXT PRIMARY KEY,
    sender_id TEXT NOT NULL,
    content TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    FOREIGN KEY (sender_id) REFERENCES user(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS forum (
    thread_id TEXT PRIMARY KEY,
    category_id TEXT NOT NULL,
    author_id TEXT NOT NULL,
    title TEXT NOT NULL,
    locked INTEGER DEFAULT 0,
    created_at TEXT NOT NULL,
    FOREIGN KEY (author_id) REFERENCES user(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS replies (
    reply_id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL,
    title TEXT,
    author_id TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (thread_id) REFERENCES forum(thread_id) ON DELETE CASCADE,
    FOREIGN KEY (author_id) REFERENCES user(id) ON DELETE CASCADE
);

-- Table required for SCS session store
CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    data BLOB NOT NULL,
    expiry REAL NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expiry);
`

func InitDB(dbPath string) (*sql.DB, error) {
	// Enable Foreign Keys, WAL mode (for concurrent web reads/writes), and 5s Busy Timeout
	dsn := fmt.Sprintf("%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", dbPath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	return db, nil
}
