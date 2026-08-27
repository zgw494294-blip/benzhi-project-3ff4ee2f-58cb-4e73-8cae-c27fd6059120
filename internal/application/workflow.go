package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"strata-proof/internal/domain"
	"strata-proof/internal/evidence"
)

func (s *Service) RunCheck(ctx context.Context, cmd VersionedCommand) (domain.Snapshot, error) {
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventCheckCompleted, "执行地层关系一致性检查", func(snapshot *domain.Snapshot) error {
		if err := s.policy.RequireRecorder(*snapshot, cmd.Actor); err != nil {
			return err
		}
		if err := domain.EnsureMutable(snapshot.Dossier.Status); err != nil {
			return err
		}
		runID := newID("check")
		findings := s.analyzer.Analyze(snapshot.Dossier.ID, runID, snapshot.Units, snapshot.Relations)
		previous := map[string]domain.ConsistencyFinding{}
		for _, finding := range snapshot.Findings {
			previous[finding.ID] = finding
		}
		current := map[string]bool{}
		added, persistent := 0, 0
		for i := range findings {
			finding := &findings[i]
			finding.UnitRevisions, finding.RelationRevisions = findingRevisionRefs(*finding, *snapshot)
			current[finding.ID] = true
			old, ok := previous[finding.ID]
			if !ok {
				finding.ChangeType = "added"
				added++
				continue
			}
			finding.ChangeType = "persistent"
			persistent++
			if finding.Severity == "warning" && old.Status == domain.FindingResolved {
				if sameRevisionRefs(old, *finding) {
					finding.Status, finding.ResolutionNote = domain.FindingResolved, old.ResolutionNote
				} else {
					finding.TriggerRevisions = changedRevisionRefs(old, *finding)
				}
			}
		}
		resolved := []domain.ConsistencyFinding{}
		for id, finding := range previous {
			if !current[id] {
				finding.ChangeType = "resolved"
				resolved = append(resolved, finding)
			}
		}
		sort.Slice(resolved, func(i, j int) bool { return resolved[i].ID < resolved[j].ID })
		snapshot.LastCheckRunID = runID
		snapshot.Findings = findings
		batch := domain.CheckBatch{ID: runID, DossierID: snapshot.Dossier.ID, DossierVersion: snapshot.Dossier.Version + 1, ExecutedAt: s.now().UTC(), AddedCount: added, PersistentCount: persistent, ResolvedCount: len(resolved), Findings: append([]domain.ConsistencyFinding{}, findings...), Resolved: resolved}
		for _, finding := range findings {
			if finding.Severity == "error" {
				batch.ErrorCount++
			} else {
				batch.WarningCount++
			}
		}
		snapshot.CheckBatches = append(snapshot.CheckBatches, batch)
		open := false
		for _, finding := range findings {
			if finding.Status == domain.FindingOpen {
				open = true
			}
		}
		if open {
			snapshot.Dossier.Status = domain.StatusRemediate
		}
		for i := range snapshot.RemediationItems {
			item := &snapshot.RemediationItems[i]
			if item.TargetType == "finding" && item.ClosedAt == nil && !current[item.TargetID] {
				closed := s.now().UTC()
				item.ClosedAt = &closed
			}
			if item.ClosedAt != nil {
				item.VerifiedCheckRunID = runID
			}
		}
		return nil
	})
}

func (s *Service) ResolveRemediation(ctx context.Context, cmd ResolveRemediationCommand) (domain.Snapshot, error) {
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventRemediationExplained, "记录定向整改处理说明", func(snapshot *domain.Snapshot) error {
		if err := s.policy.RequireRecorder(*snapshot, cmd.Actor); err != nil {
			return err
		}
		if err := domain.EnsureMutable(snapshot.Dossier.Status); err != nil {
			return err
		}
		note := strings.TrimSpace(cmd.ResolutionNote)
		if note == "" {
			return domain.NewError(domain.CodeValidation, "定向整改处理说明不能为空")
		}
		for i := range snapshot.RemediationItems {
			item := &snapshot.RemediationItems[i]
			if item.ID != cmd.RemediationID {
				continue
			}
			item.ResolutionNote = note
			closed := s.now().UTC()
			item.ClosedAt = &closed
			item.VerifiedCheckRunID = ""
			return nil
		}
		return domain.NewError(domain.CodeNotFound, "定向整改项不存在")
	})
}

func (s *Service) ResolveFinding(ctx context.Context, cmd ResolveFindingCommand) (domain.Snapshot, error) {
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventFindingResolved, "记录问题整改说明", func(snapshot *domain.Snapshot) error {
		if err := s.policy.RequireRecorder(*snapshot, cmd.Actor); err != nil {
			return err
		}
		if err := domain.EnsureMutable(snapshot.Dossier.Status); err != nil {
			return err
		}
		if strings.TrimSpace(cmd.ResolutionNote) == "" {
			return domain.NewError(domain.CodeValidation, "整改说明不能为空")
		}
		for i := range snapshot.Findings {
			if snapshot.Findings[i].ID == cmd.FindingID {
				if snapshot.Findings[i].Severity == "error" {
					return domain.NewError(domain.CodeState, "严重问题必须修订单位或关系并重新检查，不能仅以说明关闭")
				}
				snapshot.Findings[i].Status = domain.FindingResolved
				snapshot.Findings[i].ResolutionNote = strings.TrimSpace(cmd.ResolutionNote)
				for j := range snapshot.CheckBatches {
					if snapshot.CheckBatches[j].ID == snapshot.LastCheckRunID {
						for k := range snapshot.CheckBatches[j].Findings {
							if snapshot.CheckBatches[j].Findings[k].ID == cmd.FindingID {
								snapshot.CheckBatches[j].Findings[k] = snapshot.Findings[i]
							}
						}
					}
				}
				for j := range snapshot.RemediationItems {
					item := &snapshot.RemediationItems[j]
					if item.TargetType == "finding" && item.TargetID == cmd.FindingID && item.ClosedAt == nil {
						item.ResolutionNote = strings.TrimSpace(cmd.ResolutionNote)
						closed := s.now().UTC()
						item.ClosedAt = &closed
					}
				}
				return nil
			}
		}
		return domain.NewError(domain.CodeNotFound, "问题不存在")
	})
}

func (s *Service) SubmitReview(ctx context.Context, cmd VersionedCommand) (domain.Snapshot, error) {
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventReviewSubmitted, "提交人工复核", func(snapshot *domain.Snapshot) error {
		if err := s.policy.RequireRecorder(*snapshot, cmd.Actor); err != nil {
			return err
		}
		if err := domain.EnsureMutable(snapshot.Dossier.Status); err != nil {
			return err
		}
		if len(snapshot.Units) < 2 || len(snapshot.Relations) < 1 {
			return domain.NewError(domain.CodeState, "至少需要两个单位和一条关系才能提交复核")
		}
		if snapshot.LastCheckRunID == "" {
			return domain.NewError(domain.CodeState, "提交复核前必须执行一致性检查")
		}
		for _, finding := range snapshot.Findings {
			if finding.Status == domain.FindingOpen {
				return domain.NewError(domain.CodeState, "仍有未关闭问题，不能提交复核")
			}
		}
		for _, item := range snapshot.RemediationItems {
			if item.ClosedAt == nil {
				return domain.NewError(domain.CodeState, "定向整改项 %s 尚未闭环", item.ID)
			}
			if item.VerifiedCheckRunID != snapshot.LastCheckRunID {
				return domain.NewError(domain.CodeState, "定向整改项闭环后必须执行更新的一致性检查")
			}
		}
		if err := domain.EnsureTransition(snapshot.Dossier.Status, domain.StatusReview); err != nil {
			return err
		}
		snapshot.Dossier.Status = domain.StatusReview
		return nil
	})
}

func findingRevisionRefs(finding domain.ConsistencyFinding, snapshot domain.Snapshot) (map[string]int, map[string]int) {
	units, relations := map[string]int{}, map[string]int{}
	for _, id := range finding.UnitRefs {
		if unit, ok := domain.UnitByID(snapshot.Units, id); ok {
			units[id] = unit.Revision
		}
	}
	for _, id := range finding.RelationRefs {
		if relation, ok := domain.RelationByID(snapshot.Relations, id); ok {
			relations[id] = relation.Revision
		}
	}
	return units, relations
}

func sameRevisionRefs(a, b domain.ConsistencyFinding) bool {
	if len(a.UnitRevisions) != len(b.UnitRevisions) || len(a.RelationRevisions) != len(b.RelationRevisions) {
		return false
	}
	for id, revision := range a.UnitRevisions {
		if b.UnitRevisions[id] != revision {
			return false
		}
	}
	for id, revision := range a.RelationRevisions {
		if b.RelationRevisions[id] != revision {
			return false
		}
	}
	return true
}

func changedRevisionRefs(a, b domain.ConsistencyFinding) []string {
	out := []string{}
	for id, revision := range b.UnitRevisions {
		if a.UnitRevisions[id] != revision {
			out = append(out, fmt.Sprintf("unit:%s@%d", id, revision))
		}
	}
	for id, revision := range b.RelationRevisions {
		if a.RelationRevisions[id] != revision {
			out = append(out, fmt.Sprintf("relation:%s@%d", id, revision))
		}
	}
	sort.Strings(out)
	return out
}

func (s *Service) Review(ctx context.Context, cmd ReviewCommand) (domain.Snapshot, error) {
	event, summary := domain.EventReviewReturned, "复核退回案卷"
	if cmd.Approved {
		event, summary = domain.EventReviewApproved, "复核通过并冻结证据清单"
	}
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, event, summary, func(snapshot *domain.Snapshot) error {
		if err := s.policy.RequireReviewer(*snapshot, cmd.Actor, cmd.Reviewer); err != nil {
			return err
		}
		if snapshot.Dossier.Status != domain.StatusReview {
			return domain.NewError(domain.CodeState, "只有待复核案卷可以作出复核决定")
		}
		if strings.TrimSpace(cmd.Reviewer) == "" || strings.TrimSpace(cmd.Note) == "" {
			return domain.NewError(domain.CodeValidation, "复核人和复核意见不能为空")
		}
		if cmd.Approved && (!cmd.Checklist.UnitCompleteness || !cmd.Checklist.RelationEvidence || !cmd.Checklist.FindingClosure || !cmd.Checklist.ManifestPreview) {
			return domain.NewError(domain.CodeState, "复核通过前必须逐项确认四类结构化清单")
		}
		items := []domain.RemediationItem{}
		if !cmd.Approved {
			if len(cmd.Targets) == 0 {
				return domain.NewError(domain.CodeValidation, "退回时至少需要一个单位、关系或问题定位对象")
			}
			for _, target := range cmd.Targets {
				item, err := buildRemediationItem(*snapshot, target, s.now())
				if err != nil {
					return err
				}
				items = append(items, item)
			}
		}
		review := domain.ReviewDecision{ID: newID("review"), DossierID: cmd.DossierID, Reviewer: strings.TrimSpace(cmd.Reviewer), Approved: cmd.Approved, Note: strings.TrimSpace(cmd.Note), DecidedAt: s.now().UTC(), Checklist: cmd.Checklist, RemediationItems: append([]domain.RemediationItem{}, snapshot.RemediationItems...)}
		snapshot.Review = &review
		if !cmd.Approved {
			if err := domain.EnsureTransition(snapshot.Dossier.Status, domain.StatusReturned); err != nil {
				return err
			}
			snapshot.Dossier.Status = domain.StatusReturned
			snapshot.RemediationItems = append(snapshot.RemediationItems, items...)
			snapshot.Review.RemediationItems = append([]domain.RemediationItem{}, items...)
			return nil
		}
		if err := domain.EnsureTransition(snapshot.Dossier.Status, domain.StatusFrozen); err != nil {
			return err
		}
		manifest, err := evidence.BuildManifest(*snapshot, review, s.now())
		if err != nil {
			return err
		}
		snapshot.Manifest = &manifest
		snapshot.Dossier.Status = domain.StatusFrozen
		return nil
	})
}

func buildRemediationItem(snapshot domain.Snapshot, target ReviewTarget, now time.Time) (domain.RemediationItem, error) {
	target.TargetType, target.TargetID, target.Reason = strings.TrimSpace(target.TargetType), strings.TrimSpace(target.TargetID), strings.TrimSpace(target.Reason)
	if target.Reason == "" {
		return domain.RemediationItem{}, domain.NewError(domain.CodeValidation, "定向退回原因不能为空")
	}
	item := domain.RemediationItem{ID: newID("remedy"), TargetType: target.TargetType, TargetID: target.TargetID, Reason: target.Reason, CreatedAt: now.UTC()}
	switch target.TargetType {
	case "unit":
		unit, ok := domain.UnitByID(snapshot.Units, target.TargetID)
		if !ok {
			return item, domain.NewError(domain.CodeValidation, "退回定位单位不属于当前案卷")
		}
		item.BaselineRevision = unit.Revision
	case "relation":
		relation, ok := domain.RelationByID(snapshot.Relations, target.TargetID)
		if !ok {
			return item, domain.NewError(domain.CodeValidation, "退回定位关系不属于当前案卷")
		}
		item.BaselineRevision = relation.Revision
	case "finding":
		found := false
		for _, finding := range snapshot.Findings {
			if finding.ID == target.TargetID && finding.CheckRunID == snapshot.LastCheckRunID {
				found = true
				item.CheckRunID = finding.CheckRunID
				break
			}
		}
		if !found {
			return item, domain.NewError(domain.CodeValidation, "退回定位问题不属于最新检查批次")
		}
	default:
		return item, domain.NewError(domain.CodeValidation, "退回定位类型必须是 unit、relation 或 finding")
	}
	return item, nil
}

func (s *Service) IssueCredential(ctx context.Context, cmd IssueCommand) (domain.Snapshot, error) {
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventCredentialIssued, "签发研究使用凭据", func(snapshot *domain.Snapshot) error {
		if snapshot.Dossier.Status != domain.StatusFrozen || snapshot.Manifest == nil || snapshot.Review == nil {
			return domain.NewError(domain.CodeState, "只有复核冻结案卷可以签发凭据")
		}
		if err := s.policy.RequireIssuer(*snapshot, cmd.Actor, cmd.IssuedBy); err != nil {
			return err
		}
		if !evidence.VerifyManifest(*snapshot.Manifest, *snapshot) {
			return domain.NewError(domain.CodeConflict, "冻结清单引用不完整")
		}
		if err := s.repo.ValidateReferences(ctx, *snapshot.Manifest); err != nil {
			return err
		}
		credential := s.issuer.Issue(newID("cred"), *snapshot.Manifest, *snapshot.Review, cmd.IssuedBy, s.now())
		snapshot.Credentials = append(snapshot.Credentials, credential)
		if err := domain.EnsureTransition(snapshot.Dossier.Status, domain.StatusIssued); err != nil {
			return err
		}
		snapshot.Dossier.Status = domain.StatusIssued
		return nil
	})
}

func (s *Service) VerifyCredential(ctx context.Context, id string) (VerificationResult, error) {
	snapshot, credential, err := s.repo.FindCredential(ctx, id)
	if err != nil {
		return VerificationResult{}, err
	}
	result := VerificationResult{Credential: credential, Dossier: snapshot.Dossier, Manifest: snapshot.Manifest, Audit: snapshot.Audit}
	result.ReplacementCredentialID = credential.ReplacementCredentialID
	if snapshot.Manifest == nil {
		result.Message = "案卷缺少冻结清单"
		return result, nil
	}
	if credential.Status == "revoked" {
		result.Message = "凭据已撤销：" + credential.RevocationReason
		return result, nil
	}
	if err := s.repo.ValidateReferences(ctx, *snapshot.Manifest); err != nil {
		result.Message = err.Error()
		return result, nil
	}
	result.Valid, result.Message = s.issuer.Verify(credential, *snapshot.Manifest)
	if result.Valid && !evidence.VerifyManifest(*snapshot.Manifest, snapshot) {
		result.Valid = false
		result.Message = "冻结清单引用的修订不完整"
	}
	return result, nil
}

func (s *Service) RevokeCredential(ctx context.Context, cmd RevokeCredentialCommand) (domain.Snapshot, error) {
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventCredentialRevoked, "撤销研究使用凭据", func(snapshot *domain.Snapshot) error {
		if snapshot.Dossier.Status != domain.StatusIssued {
			return domain.NewError(domain.CodeState, "只有已签发案卷的有效凭据可以撤销")
		}
		reason := strings.TrimSpace(cmd.Reason)
		if reason == "" {
			return domain.NewError(domain.CodeValidation, "撤销原因不能为空")
		}
		for i := range snapshot.Credentials {
			credential := &snapshot.Credentials[i]
			if credential.CredentialID != cmd.CredentialID {
				continue
			}
			if credential.DossierID != cmd.DossierID {
				return domain.NewError(domain.CodeValidation, "凭据不属于当前案卷")
			}
			if credential.Status != "active" {
				return domain.NewError(domain.CodeState, "凭据已经撤销")
			}
			if strings.TrimSpace(cmd.Actor) != credential.IssuedBy {
				return domain.NewError(domain.CodeState, "只有原签发人可以撤销凭据")
			}
			now := s.now().UTC()
			credential.Status, credential.RevokedBy, credential.RevokedAt, credential.RevocationReason = "revoked", strings.TrimSpace(cmd.Actor), &now, reason
			return nil
		}
		return domain.NewError(domain.CodeNotFound, "研究凭据不存在")
	})
}

func (s *Service) ReissueCredential(ctx context.Context, cmd ReissueCredentialCommand) (domain.Snapshot, error) {
	return s.mutate(ctx, cmd.DossierID, cmd.ExpectedVersion, cmd.Actor, cmd.IdempotencyKey, domain.EventCredentialReissued, "关联补发研究使用凭据", func(snapshot *domain.Snapshot) error {
		if snapshot.Dossier.Status != domain.StatusIssued || snapshot.Manifest == nil || snapshot.Review == nil {
			return domain.NewError(domain.CodeState, "只有已签发且冻结摘要完整的案卷可以补发凭据")
		}
		index := -1
		for i := range snapshot.Credentials {
			if snapshot.Credentials[i].CredentialID == cmd.CredentialID {
				index = i
			}
		}
		if index < 0 {
			return domain.NewError(domain.CodeNotFound, "研究凭据不存在")
		}
		old := &snapshot.Credentials[index]
		if old.Status == "active" {
			return domain.NewError(domain.CodeState, "有效凭据不能直接补发，请先撤销")
		}
		if old.Status != "revoked" {
			return domain.NewError(domain.CodeState, "只有已撤销凭据可以补发")
		}
		if strings.TrimSpace(cmd.Actor) != old.IssuedBy {
			return domain.NewError(domain.CodeState, "只有原签发人可以补发凭据")
		}
		if old.ReplacementCredentialID != "" {
			return domain.NewError(domain.CodeState, "该凭据已经关联替代凭据")
		}
		for _, credential := range snapshot.Credentials {
			if credential.Status == "active" {
				return domain.NewError(domain.CodeConflict, "同一补发链已有有效凭据")
			}
		}
		if !evidence.VerifyManifest(*snapshot.Manifest, *snapshot) {
			return domain.NewError(domain.CodeConflict, "冻结清单摘要验真失败")
		}
		if err := s.repo.ValidateReferences(ctx, *snapshot.Manifest); err != nil {
			return err
		}
		credential := s.issuer.Reissue(newID("cred"), *snapshot.Manifest, *snapshot.Review, old.IssuedBy, old.CredentialID, s.now())
		old.ReplacementCredentialID = credential.CredentialID
		snapshot.Credentials = append(snapshot.Credentials, credential)
		return nil
	})
}
