// Package db provides the SQLite connection and schema initializer.
package db

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// columnMigrations are idempotent ADD COLUMN statements applied on top of the
// base schema so existing databases pick up columns added in newer versions.
// SQLite lacks "ADD COLUMN IF NOT EXISTS", so duplicate-column errors are
// tolerated below.
var columnMigrations = []string{
	`ALTER TABLE servers ADD COLUMN port INTEGER NOT NULL DEFAULT 25565`,
	`ALTER TABLE servers ADD COLUMN rcon_port INTEGER NOT NULL DEFAULT 25575`,
	`ALTER TABLE servers ADD COLUMN maintenance_mode BOOLEAN NOT NULL DEFAULT 0`,
	`ALTER TABLE servers ADD COLUMN maintenance_start DATETIME`,
	`ALTER TABLE servers ADD COLUMN maintenance_end DATETIME`,
	`ALTER TABLE servers ADD COLUMN maintenance_reason TEXT`,
	`ALTER TABLE servers ADD COLUMN maintenance_backup BOOLEAN NOT NULL DEFAULT 0`,
	`ALTER TABLE servers ADD COLUMN backup_interval_value INTEGER NOT NULL DEFAULT 6`,
	`ALTER TABLE servers ADD COLUMN backup_interval_unit TEXT NOT NULL DEFAULT 'hour'`,
}

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

// Migrate applies the database schema DDL and any incremental column migrations.
func Migrate(conn *sqlx.DB) error {
	if _, err := conn.Exec(Schema); err != nil {
		return fmt.Errorf("initialize SQLite schema: %w", err)
	}
	for _, stmt := range columnMigrations {
		if _, err := conn.Exec(stmt); err != nil {
			// Ignore "duplicate column name" — the column already exists.
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				continue
			}
			return fmt.Errorf("apply migration %q: %w", stmt, err)
		}
	}
	return nil
}
