package store

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 3

func (s *SQLiteStore) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS dossiers (id TEXT PRIMARY KEY, trench_code TEXT NOT NULL UNIQUE, status TEXT NOT NULL, version INTEGER NOT NULL, snapshot_json BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS unit_revisions (dossier_id TEXT NOT NULL, unit_id TEXT NOT NULL, revision INTEGER NOT NULL, content_json BLOB NOT NULL, recorded_at TEXT NOT NULL, PRIMARY KEY(unit_id, revision), FOREIGN KEY(dossier_id) REFERENCES dossiers(id))`,
		`CREATE TABLE IF NOT EXISTS relation_revisions (dossier_id TEXT NOT NULL, relation_id TEXT NOT NULL, revision INTEGER NOT NULL, content_json BLOB NOT NULL, recorded_at TEXT NOT NULL, PRIMARY KEY(relation_id, revision), FOREIGN KEY(dossier_id) REFERENCES dossiers(id))`,
		`CREATE TABLE IF NOT EXISTS finding_snapshots (dossier_id TEXT NOT NULL, check_run_id TEXT NOT NULL, finding_id TEXT NOT NULL, content_json BLOB NOT NULL, PRIMARY KEY(dossier_id, check_run_id, finding_id), FOREIGN KEY(dossier_id) REFERENCES dossiers(id))`,
		`CREATE TABLE IF NOT EXISTS check_batches (dossier_id TEXT NOT NULL, check_run_id TEXT NOT NULL, content_json BLOB NOT NULL, executed_at TEXT NOT NULL, PRIMARY KEY(dossier_id, check_run_id), FOREIGN KEY(dossier_id) REFERENCES dossiers(id))`,
		`CREATE TABLE IF NOT EXISTS frozen_manifests (dossier_id TEXT PRIMARY KEY, digest TEXT NOT NULL UNIQUE, content_json BLOB NOT NULL, frozen_at TEXT NOT NULL, FOREIGN KEY(dossier_id) REFERENCES dossiers(id))`,
		`CREATE TABLE IF NOT EXISTS credentials (credential_id TEXT PRIMARY KEY, dossier_id TEXT NOT NULL, digest TEXT NOT NULL, content_json BLOB NOT NULL, issued_at TEXT NOT NULL, FOREIGN KEY(dossier_id) REFERENCES dossiers(id))`,
		`CREATE TABLE IF NOT EXISTS audit_entries (dossier_id TEXT NOT NULL, sequence INTEGER NOT NULL, content_json BLOB NOT NULL, occurred_at TEXT NOT NULL, PRIMARY KEY(dossier_id, sequence), FOREIGN KEY(dossier_id) REFERENCES dossiers(id))`,
		`CREATE TABLE IF NOT EXISTS idempotency_results (idempotency_key TEXT PRIMARY KEY, dossier_id TEXT NOT NULL, request_type TEXT NOT NULL DEFAULT '', actor TEXT NOT NULL DEFAULT '', response_json BLOB NOT NULL, created_at TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("执行迁移: %w", err)
		}
	}
	if err := ensureColumn(ctx, tx, "idempotency_results", "request_type", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, tx, "idempotency_results", "actor", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	var version int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if version < schemaVersion {
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, datetime('now'))`, schemaVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ensureColumn adds a column to an existing table when migrating from a prior
// schema version. New databases already create the column via CREATE TABLE.
func ensureColumn(ctx context.Context, tx *sql.Tx, table, column, definition string) error {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) WHERE name=?`, table, column)
	if err != nil {
		return fmt.Errorf("检查列 %s.%s: %w", table, column, err)
	}
	present := false
	for rows.Next() {
		present = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if present {
		return nil
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, definition)); err != nil {
		return fmt.Errorf("补充列 %s.%s: %w", table, column, err)
	}
	return nil
}
