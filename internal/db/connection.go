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

	// Enable WAL mode and foreign key constraints for SQLite
	conn.MustExec("PRAGMA foreign_keys = ON;")
	conn.MustExec("PRAGMA journal_mode = WAL;")
	conn.MustExec("PRAGMA synchronous = NORMAL;")

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
