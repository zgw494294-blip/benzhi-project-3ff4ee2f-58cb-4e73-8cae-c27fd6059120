package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"strata-proof/internal/domain"
)

func (s *SQLiteStore) validateNormalizedSnapshot(ctx context.Context, snapshot domain.Snapshot) error {
	unitRows, err := s.db.QueryContext(ctx, `SELECT u.unit_id,u.revision,u.content_json FROM unit_revisions u JOIN (SELECT unit_id,MAX(revision) revision FROM unit_revisions WHERE dossier_id=? GROUP BY unit_id) latest ON latest.unit_id=u.unit_id AND latest.revision=u.revision WHERE u.dossier_id=? ORDER BY u.unit_id`, snapshot.Dossier.ID, snapshot.Dossier.ID)
	if err != nil {
		return err
	}
	storedUnits := map[string]domain.StratigraphicUnit{}
	for unitRows.Next() {
		var id string
		var revision int
		var data []byte
		if err := unitRows.Scan(&id, &revision, &data); err != nil {
			unitRows.Close()
			return err
		}
		var unit domain.StratigraphicUnit
		if err := json.Unmarshal(data, &unit); err != nil {
			unitRows.Close()
			return fmt.Errorf("单位修订账本损坏: %w", err)
		}
		if unit.ID != id || unit.Revision != revision {
			unitRows.Close()
			return fmt.Errorf("单位修订账本索引不一致")
		}
		storedUnits[id] = unit
	}
	if err := unitRows.Close(); err != nil {
		return err
	}
	if len(storedUnits) != len(snapshot.Units) {
		return fmt.Errorf("案卷当前单位与修订账本数量不一致")
	}
	for _, unit := range snapshot.Units {
		stored, ok := storedUnits[unit.ID]
		if !ok || !sameJSON(stored, unit) {
			return fmt.Errorf("案卷当前单位 %s 与修订账本不一致", unit.ID)
		}
	}

	relationRows, err := s.db.QueryContext(ctx, `SELECT r.relation_id,r.revision,r.content_json FROM relation_revisions r JOIN (SELECT relation_id,MAX(revision) revision FROM relation_revisions WHERE dossier_id=? GROUP BY relation_id) latest ON latest.relation_id=r.relation_id AND latest.revision=r.revision WHERE r.dossier_id=? ORDER BY r.relation_id`, snapshot.Dossier.ID, snapshot.Dossier.ID)
	if err != nil {
		return err
	}
	storedRelations := map[string]domain.StratigraphicRelation{}
	for relationRows.Next() {
		var id string
		var revision int
		var data []byte
		if err := relationRows.Scan(&id, &revision, &data); err != nil {
			relationRows.Close()
			return err
		}
		var relation domain.StratigraphicRelation
		if err := json.Unmarshal(data, &relation); err != nil {
			relationRows.Close()
			return fmt.Errorf("关系修订账本损坏: %w", err)
		}
		if relation.ID != id || relation.Revision != revision {
			relationRows.Close()
			return fmt.Errorf("关系修订账本索引不一致")
		}
		storedRelations[id] = relation
	}
	if err := relationRows.Close(); err != nil {
		return err
	}
	if len(storedRelations) != len(snapshot.Relations) {
		return fmt.Errorf("案卷当前关系与修订账本数量不一致")
	}
	for _, relation := range snapshot.Relations {
		stored, ok := storedRelations[relation.ID]
		if !ok || !sameJSON(stored, relation) {
			return fmt.Errorf("案卷当前关系 %s 与修订账本不一致", relation.ID)
		}
	}

	if err := s.validateFindingSnapshot(ctx, snapshot); err != nil {
		return err
	}
	if err := s.validateManifestRow(ctx, snapshot); err != nil {
		return err
	}
	if err := s.validateCredentialRows(ctx, snapshot); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) validateFindingSnapshot(ctx context.Context, snapshot domain.Snapshot) error {
	if snapshot.LastCheckRunID == "" {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT content_json FROM finding_snapshots WHERE dossier_id=? AND check_run_id=? ORDER BY finding_id`, snapshot.Dossier.ID, snapshot.LastCheckRunID)
	if err != nil {
		return err
	}
	defer rows.Close()
	stored := map[string]domain.ConsistencyFinding{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return err
		}
		var finding domain.ConsistencyFinding
		if err := json.Unmarshal(data, &finding); err != nil {
			return err
		}
		stored[finding.ID] = finding
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(stored) != len(snapshot.Findings) {
		return fmt.Errorf("当前问题清单与问题快照数量不一致")
	}
	for _, finding := range snapshot.Findings {
		value, ok := stored[finding.ID]
		if !ok || !sameJSON(value, finding) {
			return fmt.Errorf("问题 %s 与问题快照不一致", finding.ID)
		}
	}
	return nil
}

func (s *SQLiteStore) validateManifestRow(ctx context.Context, snapshot domain.Snapshot) error {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT content_json FROM frozen_manifests WHERE dossier_id=?`, snapshot.Dossier.ID).Scan(&data)
	if snapshot.Manifest == nil {
		if err == sql.ErrNoRows {
			return nil
		}
		if err == nil {
			return fmt.Errorf("未冻结案卷存在冻结清单")
		}
		return err
	}
	if err == sql.ErrNoRows {
		return fmt.Errorf("冻结案卷缺少冻结清单")
	}
	if err != nil {
		return err
	}
	var stored domain.FrozenManifest
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	if !sameJSON(stored, *snapshot.Manifest) {
		return fmt.Errorf("冻结清单与案卷快照不一致")
	}
	return nil
}

func (s *SQLiteStore) validateCredentialRows(ctx context.Context, snapshot domain.Snapshot) error {
	rows, err := s.db.QueryContext(ctx, `SELECT content_json FROM credentials WHERE dossier_id=? ORDER BY credential_id`, snapshot.Dossier.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	stored := map[string]domain.ResearchCredential{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return err
		}
		var credential domain.ResearchCredential
		if err := json.Unmarshal(data, &credential); err != nil {
			return err
		}
		stored[credential.CredentialID] = credential
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(stored) != len(snapshot.Credentials) {
		return fmt.Errorf("凭据账本与案卷快照数量不一致")
	}
	for _, credential := range snapshot.Credentials {
		value, ok := stored[credential.CredentialID]
		if !ok || !sameJSON(value, credential) {
			return fmt.Errorf("凭据 %s 与凭据账本不一致", credential.CredentialID)
		}
	}
	return nil
}

func sameJSON(left, right any) bool {
	a, err := json.Marshal(left)
	if err != nil {
		return false
	}
	b, err := json.Marshal(right)
	return err == nil && bytes.Equal(a, b)
}
