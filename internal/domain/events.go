package domain

import (
	"strings"
	"time"
)

const (
	EventDossierCreated       = "dossier.created"
	EventDossierUpdated       = "dossier.updated"
	EventUnitRevised          = "unit.revised"
	EventUnitsBatchCreated    = "units.batch_created"
	EventRelationRevised      = "relation.revised"
	EventCheckCompleted       = "check.completed"
	EventFindingResolved      = "finding.resolved"
	EventRemediationExplained = "remediation.explained"
	EventReviewSubmitted      = "review.submitted"
	EventReviewApproved       = "review.approved"
	EventReviewReturned       = "review.returned"
	EventCredentialIssued     = "credential.issued"
	EventCredentialRevoked    = "credential.revoked"
	EventCredentialReissued   = "credential.reissued"
)

func NextAuditEntry(existing []AuditEntry, dossierID, eventType, actor, summary string, now time.Time) (AuditEntry, error) {
	if strings.TrimSpace(dossierID) == "" {
		return AuditEntry{}, NewError(CodeValidation, "审计事件缺少案卷标识")
	}
	if strings.TrimSpace(eventType) == "" {
		return AuditEntry{}, NewError(CodeValidation, "审计事件类型不能为空")
	}
	if strings.TrimSpace(actor) == "" {
		return AuditEntry{}, NewError(CodeValidation, "审计参与者不能为空")
	}
	if strings.TrimSpace(summary) == "" {
		return AuditEntry{}, NewError(CodeValidation, "审计摘要不能为空")
	}
	next := int64(1)
	if len(existing) > 0 {
		last := existing[len(existing)-1]
		if last.DossierID != dossierID || last.Sequence != int64(len(existing)) {
			return AuditEntry{}, NewError(CodeConflict, "已有审计时间线不连续")
		}
		next = last.Sequence + 1
	}
	return AuditEntry{DossierID: dossierID, Sequence: next, EventType: eventType, Actor: strings.TrimSpace(actor), Summary: strings.TrimSpace(summary), OccurredAt: now.UTC()}, nil
}
