package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"strata-proof/internal/domain"
)

func (s *SQLiteStore) UnitHistory(ctx context.Context, dossierID, unitID string) ([]domain.StratigraphicUnit, error) {
	if err := s.ensureDossier(ctx, dossierID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT content_json FROM unit_revisions WHERE dossier_id=? AND unit_id=? ORDER BY revision ASC`, dossierID, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.StratigraphicUnit{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var revision domain.StratigraphicUnit
		if err := json.Unmarshal(data, &revision); err != nil {
			return nil, fmt.Errorf("恢复单位修订: %w", err)
		}
		result = append(result, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, domain.NewError(domain.CodeNotFound, "地层单位不存在")
	}
	for i, revision := range result {
		if revision.Revision != i+1 {
			return nil, fmt.Errorf("单位 %s 修订序号不连续", unitID)
		}
	}
	return result, nil
}

func (s *SQLiteStore) RelationHistory(ctx context.Context, dossierID, relationID string) ([]domain.StratigraphicRelation, error) {
	if err := s.ensureDossier(ctx, dossierID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT content_json FROM relation_revisions WHERE dossier_id=? AND relation_id=? ORDER BY revision ASC`, dossierID, relationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.StratigraphicRelation{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var revision domain.StratigraphicRelation
		if err := json.Unmarshal(data, &revision); err != nil {
			return nil, fmt.Errorf("恢复关系修订: %w", err)
		}
		result = append(result, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, domain.NewError(domain.CodeNotFound, "地层关系不存在")
	}
	for i, revision := range result {
		if revision.Revision != i+1 {
			return nil, fmt.Errorf("关系 %s 修订序号不连续", relationID)
		}
	}
	return result, nil
}

func (s *SQLiteStore) AuditPage(ctx context.Context, dossierID string, limit int, before int64) ([]domain.AuditEntry, error) {
	if err := s.ensureDossier(ctx, dossierID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if before <= 0 {
		before = int64(^uint64(0) >> 1)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT content_json FROM audit_entries WHERE dossier_id=? AND sequence<? ORDER BY sequence DESC LIMIT ?`, dossierID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []domain.AuditEntry{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var entry domain.AuditEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("恢复审计记录: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *SQLiteStore) ValidateReferences(ctx context.Context, manifest domain.FrozenManifest) error {
	if err := s.ensureDossier(ctx, manifest.DossierID); err != nil {
		return err
	}
	for id, revision := range manifest.UnitRevisions {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM unit_revisions WHERE dossier_id=? AND unit_id=? AND revision=?`, manifest.DossierID, id, revision).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return domain.NewError(domain.CodeConflict, "冻结清单引用的单位修订 %s/%d 不完整", id, revision)
		}
	}
	for id, revision := range manifest.RelationRevisions {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM relation_revisions WHERE dossier_id=? AND relation_id=? AND revision=?`, manifest.DossierID, id, revision).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return domain.NewError(domain.CodeConflict, "冻结清单引用的关系修订 %s/%d 不完整", id, revision)
		}
	}
	var count, minimum, maximum int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MIN(sequence),0),COALESCE(MAX(sequence),0) FROM audit_entries WHERE dossier_id=?`, manifest.DossierID).Scan(&count, &minimum, &maximum); err != nil {
		return err
	}
	if count == 0 || minimum != 1 || maximum != count {
		return domain.NewError(domain.CodeConflict, "案卷审计序号不连续")
	}
	return nil
}

func (s *SQLiteStore) ensureDossier(ctx context.Context, id string) error {
	var found string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM dossiers WHERE id=?`, id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewError(domain.CodeNotFound, "案卷不存在")
	}
	return err
}
