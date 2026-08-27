package application

import (
	"context"
	"fmt"
	"strings"

	"strata-proof/internal/domain"
	"strata-proof/internal/evidence"
)

func (s *Service) CheckBatches(ctx context.Context, dossierID, severity, changeType string) ([]domain.CheckBatch, error) {
	snapshot, err := s.repo.Get(ctx, dossierID)
	if err != nil {
		return nil, fmt.Errorf("读取案卷 %s 的检查批次: %w", dossierID, err)
	}
	severity, changeType = strings.TrimSpace(severity), strings.TrimSpace(changeType)
	if severity != "" && severity != "error" && severity != "warning" {
		return nil, domain.NewError(domain.CodeValidation, "severity 必须是 error 或 warning")
	}
	if changeType != "" && changeType != "added" && changeType != "persistent" && changeType != "resolved" {
		return nil, domain.NewError(domain.CodeValidation, "changeType 必须是 added、persistent 或 resolved")
	}
	result := make([]domain.CheckBatch, 0, len(snapshot.CheckBatches))
	for _, batch := range snapshot.CheckBatches {
		copyBatch := batch
		copyBatch.Findings = nil
		copyBatch.Resolved = nil
		for _, finding := range append(append([]domain.ConsistencyFinding{}, batch.Findings...), batch.Resolved...) {
			if severity != "" && finding.Severity != severity {
				continue
			}
			if changeType != "" && finding.ChangeType != changeType {
				continue
			}
			if finding.ChangeType == "resolved" {
				copyBatch.Resolved = append(copyBatch.Resolved, finding)
			} else {
				copyBatch.Findings = append(copyBatch.Findings, finding)
			}
		}
		result = append(result, copyBatch)
	}
	return result, nil
}

func (s *Service) TraceRelationPath(ctx context.Context, dossierID, sourceID, targetID string) (domain.RelationPathResult, error) {
	snapshot, err := s.repo.Get(ctx, dossierID)
	if err != nil {
		return domain.RelationPathResult{}, fmt.Errorf("读取案卷 %s 的关系路径: %w", dossierID, err)
	}
	return evidence.TraceRelationPath(snapshot, sourceID, targetID)
}

func (s *Service) UnitHistory(ctx context.Context, dossierID, unitID string) (domain.RevisionLedger, error) {
	revisions, err := s.repo.UnitHistory(ctx, dossierID, unitID)
	if err != nil {
		return domain.RevisionLedger{}, fmt.Errorf("读取案卷 %s 的单位 %s 修订历史: %w", dossierID, unitID, err)
	}
	return domain.RevisionLedger{DossierID: dossierID, UnitID: unitID, UnitRevisions: revisions, RelationRevisions: []domain.StratigraphicRelation{}}, nil
}

func (s *Service) RelationHistory(ctx context.Context, dossierID, relationID string) (domain.RevisionLedger, error) {
	revisions, err := s.repo.RelationHistory(ctx, dossierID, relationID)
	if err != nil {
		return domain.RevisionLedger{}, fmt.Errorf("读取案卷 %s 的关系 %s 修订历史: %w", dossierID, relationID, err)
	}
	return domain.RevisionLedger{DossierID: dossierID, RelationID: relationID, UnitRevisions: []domain.StratigraphicUnit{}, RelationRevisions: revisions}, nil
}

func (s *Service) AuditPage(ctx context.Context, dossierID string, limit int, before int64) ([]domain.AuditEntry, error) {
	entries, err := s.repo.AuditPage(ctx, dossierID, limit, before)
	if err != nil {
		return nil, fmt.Errorf("读取案卷 %s 的审计分页: %w", dossierID, err)
	}
	return entries, nil
}

func (s *Service) ValidateFrozenReferences(ctx context.Context, snapshot domain.Snapshot) error {
	if snapshot.Manifest == nil {
		return domain.NewError(domain.CodeState, "案卷尚未冻结")
	}
	return s.repo.ValidateReferences(ctx, *snapshot.Manifest)
}
