package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"strata-proof/internal/domain"
)

func (s *SQLiteStore) Create(ctx context.Context, snapshot domain.Snapshot, key string, response json.RawMessage) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO dossiers(id,trench_code,status,version,snapshot_json,updated_at) VALUES(?,?,?,?,?,?)`, snapshot.Dossier.ID, snapshot.Dossier.TrenchCode, snapshot.Dossier.Status, snapshot.Dossier.Version, data, snapshot.Dossier.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if isConstraint(err) {
			return domain.NewError(domain.CodeConflict, "探方编号或案卷标识已存在")
		}
		return err
	}
	if err := persistChildren(ctx, tx, snapshot); err != nil {
		return err
	}
	dossierID, requestType, actor := idempotencyScope(snapshot)
	if err := insertIdempotency(ctx, tx, key, dossierID, requestType, actor, response); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) Save(ctx context.Context, snapshot domain.Snapshot, expected int64, key string, response json.RawMessage) error {
	if err := validateAudit(snapshot.Audit); err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE dossiers SET status=?,version=?,snapshot_json=?,updated_at=? WHERE id=? AND version=?`, snapshot.Dossier.Status, snapshot.Dossier.Version, data, snapshot.Dossier.UpdatedAt.UTC().Format(time.RFC3339Nano), snapshot.Dossier.ID, expected)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.NewError(domain.CodeConflict, "案卷版本冲突，请刷新后重试")
	}
	if err := persistChildren(ctx, tx, snapshot); err != nil {
		return err
	}
	dossierID, requestType, actor := idempotencyScope(snapshot)
	if err := insertIdempotency(ctx, tx, key, dossierID, requestType, actor, response); err != nil {
		return err
	}
	return tx.Commit()
}

func persistChildren(ctx context.Context, tx *sql.Tx, snapshot domain.Snapshot) error {
	for _, unit := range snapshot.Units {
		data, _ := json.Marshal(unit)
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO unit_revisions(dossier_id,unit_id,revision,content_json,recorded_at) VALUES(?,?,?,?,?)`, snapshot.Dossier.ID, unit.ID, unit.Revision, data, unit.RecordedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	for _, relation := range snapshot.Relations {
		data, _ := json.Marshal(relation)
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO relation_revisions(dossier_id,relation_id,revision,content_json,recorded_at) VALUES(?,?,?,?,?)`, snapshot.Dossier.ID, relation.ID, relation.Revision, data, relation.RecordedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	for _, finding := range snapshot.Findings {
		data, _ := json.Marshal(finding)
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO finding_snapshots(dossier_id,check_run_id,finding_id,content_json) VALUES(?,?,?,?)`, snapshot.Dossier.ID, finding.CheckRunID, finding.ID, data); err != nil {
			return err
		}
	}
	for index, batch := range snapshot.CheckBatches {
		data, _ := json.Marshal(batch)
		statement := `INSERT OR IGNORE INTO check_batches(dossier_id,check_run_id,content_json,executed_at) VALUES(?,?,?,?)`
		if index == len(snapshot.CheckBatches)-1 {
			statement = `INSERT OR REPLACE INTO check_batches(dossier_id,check_run_id,content_json,executed_at) VALUES(?,?,?,?)`
		}
		if _, err := tx.ExecContext(ctx, statement, snapshot.Dossier.ID, batch.ID, data, batch.ExecutedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if snapshot.Manifest != nil {
		data, _ := json.Marshal(snapshot.Manifest)
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO frozen_manifests(dossier_id,digest,content_json,frozen_at) VALUES(?,?,?,?)`, snapshot.Dossier.ID, snapshot.Manifest.Digest, data, snapshot.Manifest.FrozenAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	for _, credential := range snapshot.Credentials {
		data, _ := json.Marshal(credential)
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO credentials(credential_id,dossier_id,digest,content_json,issued_at) VALUES(?,?,?,?,?)`, credential.CredentialID, snapshot.Dossier.ID, credential.FrozenManifestDigest, data, credential.IssuedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	for _, entry := range snapshot.Audit {
		data, _ := json.Marshal(entry)
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO audit_entries(dossier_id,sequence,content_json,occurred_at) VALUES(?,?,?,?)`, snapshot.Dossier.ID, entry.Sequence, data, entry.OccurredAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return nil
}

// idempotencyScope derives the (dossierID, requestType, actor) triple that
// scopes an idempotency record from the snapshot's own audit trail. The last
// audit entry records the request type and actor that produced this revision,
// which is exactly the scope a legitimate retry must match.
func idempotencyScope(snapshot domain.Snapshot) (string, string, string) {
	if len(snapshot.Audit) == 0 {
		return snapshot.Dossier.ID, "", ""
	}
	last := snapshot.Audit[len(snapshot.Audit)-1]
	return snapshot.Dossier.ID, last.EventType, last.Actor
}

func insertIdempotency(ctx context.Context, tx *sql.Tx, key, dossierID, requestType, actor string, response json.RawMessage) error {
	if key == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO idempotency_results(idempotency_key,dossier_id,request_type,actor,response_json,created_at) VALUES(?,?,?,?,?,?)`, key, dossierID, requestType, actor, []byte(response), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil && isConstraint(err) {
		return domain.NewError(domain.CodeConflict, "idempotencyKey 已被使用")
	}
	return err
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (domain.Snapshot, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT snapshot_json FROM dossiers WHERE id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Snapshot{}, domain.NewError(domain.CodeNotFound, "案卷不存在")
	}
	if err != nil {
		return domain.Snapshot{}, err
	}
	var snapshot domain.Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return snapshot, fmt.Errorf("恢复案卷: %w", err)
	}
	if err := validateAudit(snapshot.Audit); err != nil {
		return snapshot, err
	}
	if err := s.validateNormalizedSnapshot(ctx, snapshot); err != nil {
		return snapshot, fmt.Errorf("校验持久化账本: %w", err)
	}
	return snapshot, nil
}

func (s *SQLiteStore) List(ctx context.Context, limit, offset int) ([]domain.TrenchDossier, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot_json FROM dossiers ORDER BY updated_at DESC,id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dossiers []domain.TrenchDossier
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var snapshot domain.Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return nil, err
		}
		dossiers = append(dossiers, snapshot.Dossier)
	}
	return dossiers, rows.Err()
}

func (s *SQLiteStore) IdempotentResult(ctx context.Context, key string) (json.RawMessage, bool, error) {
	if key == "" {
		return nil, false, nil
	}
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT response_json FROM idempotency_results WHERE idempotency_key=?`, key).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(data), true, nil
}

// IdempotentResultFor looks up a cached idempotent result scoped to the target
// dossier, request type and actor. An empty dossierID (used for dossier
// creation, where the ID is generated during the request) matches any dossier,
// so legitimate create retries still replay the original result while reusing
// the same key for a different actor or a different request type yields no
// match and the subsequent insert fails with a conflict.
func (s *SQLiteStore) IdempotentResultFor(ctx context.Context, key, dossierID, requestType, actor string) (json.RawMessage, bool, error) {
	if key == "" {
		return nil, false, nil
	}
	query := `SELECT response_json FROM idempotency_results WHERE idempotency_key=? AND request_type=? AND actor=?`
	args := []any{key, requestType, actor}
	if dossierID != "" {
		query += ` AND dossier_id=?`
		args = append(args, dossierID)
	}
	var data []byte
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(data), true, nil
}

func (s *SQLiteStore) FindCredential(ctx context.Context, id string) (domain.Snapshot, domain.ResearchCredential, error) {
	var dossierID string
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT dossier_id,content_json FROM credentials WHERE credential_id=?`, id).Scan(&dossierID, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Snapshot{}, domain.ResearchCredential{}, domain.NewError(domain.CodeNotFound, "研究凭据不存在")
	}
	if err != nil {
		return domain.Snapshot{}, domain.ResearchCredential{}, err
	}
	var credential domain.ResearchCredential
	if err := json.Unmarshal(data, &credential); err != nil {
		return domain.Snapshot{}, credential, err
	}
	snapshot, err := s.Get(ctx, dossierID)
	return snapshot, credential, err
}

func validateAudit(entries []domain.AuditEntry) error {
	copyEntries := append([]domain.AuditEntry(nil), entries...)
	sort.Slice(copyEntries, func(i, j int) bool { return copyEntries[i].Sequence < copyEntries[j].Sequence })
	for i, entry := range copyEntries {
		if entry.Sequence != int64(i+1) {
			return fmt.Errorf("案卷审计序号不连续: 期望 %d，实际 %d", i+1, entry.Sequence)
		}
	}
	return nil
}

func isConstraint(err error) bool {
	return err != nil && (contains(err.Error(), "constraint") || contains(err.Error(), "UNIQUE"))
}
func contains(value, target string) bool {
	for i := 0; i+len(target) <= len(value); i++ {
		if value[i:i+len(target)] == target {
			return true
		}
	}
	return false
}
