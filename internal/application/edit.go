package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"strata-proof/internal/domain"
)

func (s *Service) PutUnitsBatch(ctx context.Context, cmd BatchPutUnitsCommand) (BatchPutUnitsResult, error) {
	if data, ok, err := s.repo.IdempotentResult(ctx, cmd.IdempotencyKey); ok || err != nil {
		var result BatchPutUnitsResult
		if err == nil {
			err = json.Unmarshal(data, &result)
		}
		return result, err
	}
	if err := requireCommand(cmd.Actor, cmd.IdempotencyKey); err != nil {
		return BatchPutUnitsResult{}, err
	}
	if len(cmd.Rows) == 0 || len(cmd.Rows) > 100 {
		return BatchPutUnitsResult{}, domain.NewError(domain.CodeValidation, "批量单位行数必须为 1 到 100")
	}
	snapshot, err := s.repo.Get(ctx, cmd.DossierID)
	if err != nil {
		if retry, ok, retryErr := s.cachedBatchResult(ctx, cmd.IdempotencyKey); ok || retryErr != nil {
			return retry, retryErr
		}
		return BatchPutUnitsResult{}, err
	}
	if snapshot.Dossier.Version != cmd.ExpectedVersion {
		if retry, ok, retryErr := s.cachedBatchResult(ctx, cmd.IdempotencyKey); ok || retryErr != nil {
			return retry, retryErr
		}
		return BatchPutUnitsResult{}, domain.NewError(domain.CodeConflict, "expectedVersion=%d 与当前版本 %d 不符", cmd.ExpectedVersion, snapshot.Dossier.Version)
	}
	if err := s.policy.RequireRecorder(snapshot, cmd.Actor); err != nil {
		if retry, ok, retryErr := s.cachedBatchResult(ctx, cmd.IdempotencyKey); ok || retryErr != nil {
			return retry, retryErr
		}
		return BatchPutUnitsResult{}, err
	}
	inputs := make([]domain.BatchUnitInput, 0, len(cmd.Rows))
	for index, row := range cmd.Rows {
		if row.Row <= 0 {
			row.Row = index + 1
		}
		unit := domain.StratigraphicUnit{ID: newID("unit"), DossierID: cmd.DossierID, UnitCode: strings.TrimSpace(row.UnitCode), UnitType: strings.TrimSpace(row.UnitType), Description: strings.TrimSpace(row.Description), TopElevation: row.TopElevation, BottomElevation: row.BottomElevation, SoilTraits: strings.TrimSpace(row.SoilTraits), PhotoRefs: cleanRefs(row.PhotoRefs)}
		inputs = append(inputs, domain.BatchUnitInput{Row: row.Row, Unit: unit})
	}
	if details := domain.ValidateUnitBatch(snapshot.Dossier.Status, snapshot.Units, inputs); len(details) > 0 {
		if retry, ok, retryErr := s.cachedBatchResult(ctx, cmd.IdempotencyKey); ok || retryErr != nil {
			return retry, retryErr
		}
		if details[0].Field == "status" {
			return BatchPutUnitsResult{}, domain.NewDetailedError(domain.CodeState, "当前案卷状态不允许批量登记单位", details)
		}
		return BatchPutUnitsResult{}, domain.NewDetailedError(domain.CodeValidation, "批量单位校验失败", details)
	}
	now := s.now().UTC()
	result := BatchPutUnitsResult{Items: make([]BatchUnitItem, 0, len(inputs))}
	for _, input := range inputs {
		unit := domain.NextUnitRevision(nil, input.Unit, now)
		snapshot.Units = append(snapshot.Units, unit)
		result.Items = append(result.Items, BatchUnitItem{Row: input.Row, UnitID: unit.ID, Revision: unit.Revision})
	}
	markRemediation(&snapshot)
	snapshot.Dossier.Version++
	snapshot.Dossier.UpdatedAt = now
	audit, err := domain.NextAuditEntry(snapshot.Audit, cmd.DossierID, domain.EventUnitsBatchCreated, cmd.Actor, fmt.Sprintf("批量登记 %d 个地层单位", len(inputs)), now)
	if err != nil {
		if retry, ok, retryErr := s.cachedBatchResult(ctx, cmd.IdempotencyKey); ok || retryErr != nil {
			return retry, retryErr
		}
		return BatchPutUnitsResult{}, err
	}
	snapshot.Audit = append(snapshot.Audit, audit)
	result.Snapshot = snapshot
	response, _ := json.Marshal(result)
	if err := s.repo.Save(ctx, snapshot, cmd.ExpectedVersion, cmd.IdempotencyKey, response); err != nil {
		if retry, ok, retryErr := s.cachedBatchResult(ctx, cmd.IdempotencyKey); ok || retryErr != nil {
			return retry, retryErr
		}
		return BatchPutUnitsResult{}, err
	}
	return result, nil
}

func (s *Service) cachedBatchResult(ctx context.Context, key string) (BatchPutUnitsResult, bool, error) {
	data, ok, err := s.repo.IdempotentResult(ctx, key)
	if err != nil || !ok {
		return BatchPutUnitsResult{}, ok, err
	}
	var result BatchPutUnitsResult
	if err := json.Unmarshal(data, &result); err != nil {
		return result, true, err
	}
	return result, true, nil
}

func (s *Service) UpdateDossier(ctx context.Context, cmd UpdateDossierCommand) (domain.Snapshot, error) {
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventDossierUpdated, "修订探方基本信息", func(snapshot *domain.Snapshot) error {
		if err := s.policy.RequireRecorder(*snapshot, cmd.Actor); err != nil {
			return err
		}
		if err := domain.EnsureMutable(snapshot.Dossier.Status); err != nil {
			return err
		}
		snapshot.Dossier.ExcavationArea = strings.TrimSpace(cmd.ExcavationArea)
		snapshot.Dossier.LeadRecorder = strings.TrimSpace(cmd.LeadRecorder)
		snapshot.Dossier.ChronologyHypothesis = strings.TrimSpace(cmd.ChronologyHypothesis)
		return domain.ValidateDossier(snapshot.Dossier)
	})
}

func (s *Service) PutUnit(ctx context.Context, cmd PutUnitCommand) (domain.Snapshot, error) {
	operation := "登记地层单位"
	if cmd.UnitID != "" {
		operation = "修订地层单位"
	}
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventUnitRevised, operation, func(snapshot *domain.Snapshot) error {
		if err := s.policy.RequireRecorder(*snapshot, cmd.Actor); err != nil {
			return err
		}
		if err := domain.EnsureMutable(snapshot.Dossier.Status); err != nil {
			return err
		}
		proposed := domain.StratigraphicUnit{ID: cmd.UnitID, DossierID: cmd.DossierID, UnitCode: strings.TrimSpace(cmd.UnitCode), UnitType: strings.TrimSpace(cmd.UnitType), Description: strings.TrimSpace(cmd.Description), TopElevation: cmd.TopElevation, BottomElevation: cmd.BottomElevation, SoilTraits: strings.TrimSpace(cmd.SoilTraits), PhotoRefs: cleanRefs(cmd.PhotoRefs)}
		var previous *domain.StratigraphicUnit
		if cmd.UnitID == "" {
			proposed.ID = newID("unit")
		} else {
			var ok bool
			previous, ok = domain.UnitByID(snapshot.Units, cmd.UnitID)
			if !ok {
				return domain.NewError(domain.CodeNotFound, "地层单位不存在")
			}
		}
		for _, unit := range snapshot.Units {
			if unit.UnitCode == proposed.UnitCode && unit.ID != proposed.ID {
				return domain.NewError(domain.CodeConflict, "单位编号 %s 已存在", proposed.UnitCode)
			}
		}
		proposed = domain.NextUnitRevision(previous, proposed, s.now())
		if err := domain.ValidateUnit(proposed); err != nil {
			return err
		}
		if previous == nil {
			snapshot.Units = append(snapshot.Units, proposed)
		} else {
			for i := range snapshot.Units {
				if snapshot.Units[i].ID == proposed.ID {
					snapshot.Units[i] = proposed
				}
			}
		}
		markRemediation(snapshot)
		trackRemediationRevision(snapshot, "unit", proposed.ID, proposed.Revision, s.now())
		return nil
	})
}

func (s *Service) PutRelation(ctx context.Context, cmd PutRelationCommand) (domain.Snapshot, error) {
	operation := "建立地层关系"
	if cmd.RelationID != "" {
		operation = "替换地层关系"
	}
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventRelationRevised, operation, func(snapshot *domain.Snapshot) error {
		if err := s.policy.RequireRecorder(*snapshot, cmd.Actor); err != nil {
			return err
		}
		if err := domain.EnsureMutable(snapshot.Dossier.Status); err != nil {
			return err
		}
		proposed := domain.StratigraphicRelation{ID: cmd.RelationID, DossierID: cmd.DossierID, SourceUnitID: cmd.SourceUnitID, TargetUnitID: cmd.TargetUnitID, RelationType: cmd.RelationType, EvidenceNote: strings.TrimSpace(cmd.EvidenceNote)}
		var previous *domain.StratigraphicRelation
		if cmd.RelationID == "" {
			proposed.ID = newID("rel")
		} else {
			var ok bool
			previous, ok = domain.RelationByID(snapshot.Relations, cmd.RelationID)
			if !ok {
				return domain.NewError(domain.CodeNotFound, "地层关系不存在")
			}
		}
		if err := domain.ValidateRelation(proposed, snapshot.Units, snapshot.Relations, cmd.RelationID); err != nil {
			return err
		}
		proposed = domain.NextRelationRevision(previous, proposed, s.now())
		if previous == nil {
			snapshot.Relations = append(snapshot.Relations, proposed)
		} else {
			for i := range snapshot.Relations {
				if snapshot.Relations[i].ID == proposed.ID {
					snapshot.Relations[i] = proposed
				}
			}
		}
		markRemediation(snapshot)
		trackRemediationRevision(snapshot, "relation", proposed.ID, proposed.Revision, s.now())
		return nil
	})
}

func trackRemediationRevision(snapshot *domain.Snapshot, targetType, targetID string, revision int, now time.Time) {
	for i := range snapshot.RemediationItems {
		item := &snapshot.RemediationItems[i]
		if item.TargetType == targetType && item.TargetID == targetID && item.ClosedAt == nil && revision > item.BaselineRevision {
			item.ActualRevision = revision
			closed := now.UTC()
			item.ClosedAt = &closed
		}
	}
}

func cleanRefs(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func markRemediation(snapshot *domain.Snapshot) {
	snapshot.LastCheckRunID = ""
	if snapshot.Dossier.Status == domain.StatusReturned {
		snapshot.Dossier.Status = domain.StatusRemediate
	}
	for i := range snapshot.Findings {
		if snapshot.Findings[i].Status == domain.FindingOpen {
			snapshot.Findings[i].ResolutionNote = fmt.Sprintf("内容已变更，等待重新检查")
		}
	}
}
