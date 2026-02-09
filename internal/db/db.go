package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a *sql.DB connection to SQLite.
type DB struct {
	Conn *sql.DB
	path string
}

// New opens (or creates) a SQLite database at the given path and returns a
// wrapped connection. It creates the parent directory if it doesn't exist and
// enables WAL mode + foreign keys.
func New(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Single connection avoids SQLite locking issues.
	conn.SetMaxOpenConns(1)

	// Enable WAL mode for better concurrent read performance.
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err := conn.Exec("PRAGMA foreign_keys=ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	log.Printf("database opened: %s", dbPath)
	return &DB{Conn: conn, path: dbPath}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.Conn.Close()
}

// Ping verifies the database connection is alive.
func (d *DB) Ping() error {
	return d.Conn.Ping()
}
