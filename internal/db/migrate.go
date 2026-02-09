package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate applies all pending SQL migration files in order. Migrations are
// embedded .sql files in the migrations/ directory, named with a numeric
// prefix for ordering (e.g., 001_initial.sql). Each migration runs inside a
// transaction. A migrations tracking table records which have been applied.
func (d *DB) Migrate() error {
	// Ensure the migrations tracking table exists.
	if _, err := d.Conn.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			name     TEXT    NOT NULL UNIQUE,
			applied  DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Read all .sql files from the embedded filesystem.
	files, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migration dir: %w", err)
	}

	// Sort by filename to guarantee order.
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}

		name := f.Name()

		// Check if already applied.
		var count int
		if err := d.Conn.QueryRow(
			"SELECT COUNT(*) FROM _migrations WHERE name = ?", name,
		).Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		// Read migration SQL.
		content, err := fs.ReadFile(migrationFS, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		// Apply in a transaction.
		if err := d.applyMigration(name, string(content)); err != nil {
			return err
		}

		log.Printf("migration applied: %s", name)
	}

	return nil
}

func (d *DB) applyMigration(name, sqlContent string) error {
	tx, err := d.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx for %s: %w", name, err)
	}
	defer tx.Rollback() //nolint: errcheck

	if _, err := tx.Exec(sqlContent); err != nil {
		return fmt.Errorf("exec migration %s: %w", name, err)
	}

	if _, err := tx.Exec(
		"INSERT INTO _migrations (name) VALUES (?)", name,
	); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}

	return tx.Commit()
}

// Applied returns the list of migration names that have been applied, for
// diagnostics. Returns an empty slice if the migrations table doesn't exist.
func (d *DB) Applied() ([]string, error) {
	rows, err := d.Conn.Query("SELECT name FROM _migrations ORDER BY id")
	if err != nil {
		// Table may not exist yet.
		if isNoSuchTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// MigrationCount returns how many migrations have been applied.
func (d *DB) MigrationCount() (int, error) {
	var count int
	err := d.Conn.QueryRow(
		"SELECT COUNT(*) FROM _migrations",
	).Scan(&count)
	if err != nil {
		if isNoSuchTable(err) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

// RowCount returns the number of rows in the given table. Useful for health
// checks and diagnostics. Uses a parameterized approach safe for known table
// names only (caller must not pass user input).
func (d *DB) RowCount(table string) (int, error) {
	var count int
	err := d.Conn.QueryRow(
		fmt.Sprintf("SELECT COUNT(*) FROM %s", sanitizeIdentifier(table)),
	).Scan(&count)
	return count, err
}

func sanitizeIdentifier(s string) string {
	// Only allow alphanumeric and underscore — prevents SQL injection.
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ensure sql.DB is available for tests
var _ *sql.DB
