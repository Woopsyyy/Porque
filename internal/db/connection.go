// Package db provides the SQLite connection and schema initializer.
package db

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// Connect opens a sqlx connection pool to SQLite, configures pragmas, and verifies it with a ping.
func Connect(dbPath string) (*sqlx.DB, error) {
	conn, err := sqlx.Connect("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}

	// SQLite must use a single connection to avoid WAL lock contention.
	// Multiple writers to the same WAL cause SQLITE_BUSY errors and data loss on restart.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	// Enable WAL mode and foreign key constraints for SQLite
	conn.MustExec("PRAGMA foreign_keys = ON;")
	conn.MustExec("PRAGMA journal_mode = WAL;")
	conn.MustExec("PRAGMA synchronous = NORMAL;")

	// Automatically checkpoint the WAL every 100 pages (keeps WAL small and
	// ensures data is flushed into the main .db file on each write batch).
	conn.MustExec("PRAGMA wal_autocheckpoint = 100;")

	// Force a full WAL checkpoint on startup to merge any pending WAL writes
	// from a previous session into the main database file. This ensures that
	// data written in a prior run (e.g. imported servers) is visible after restart.
	if _, err := conn.Exec("PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
		// Non-fatal: log but don't abort startup
		fmt.Printf("WARNING: wal_checkpoint on startup failed: %v\n", err)
	}

	return conn, nil
}

// Migrate applies the database schema DDL.
func Migrate(conn *sqlx.DB) error {
	_, err := conn.Exec(Schema)
	if err != nil {
		return fmt.Errorf("initialize SQLite schema: %w", err)
	}
	return nil
}
