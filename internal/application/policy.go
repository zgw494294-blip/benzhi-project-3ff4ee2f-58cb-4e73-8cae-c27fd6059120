package application

import (
	"strings"

	"strata-proof/internal/domain"
)

type AuthorizationPolicy struct{}

func (AuthorizationPolicy) RequireRecorder(snapshot domain.Snapshot, actor string) error {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return domain.NewError(domain.CodeValidation, "actor 不能为空")
	}
	if actor != snapshot.Dossier.LeadRecorder {
		return domain.NewError(domain.CodeState, "只有案卷记录负责人 %s 可以修改记录或提交复核", snapshot.Dossier.LeadRecorder)
	}
	return nil
}

func (AuthorizationPolicy) RequireReviewer(snapshot domain.Snapshot, actor, reviewer string) error {
	actor, reviewer = strings.TrimSpace(actor), strings.TrimSpace(reviewer)
	if reviewer == "" {
		return domain.NewError(domain.CodeValidation, "复核员不能为空")
	}
	if actor != reviewer {
		return domain.NewError(domain.CodeState, "actor 必须与复核员一致")
	}
	if reviewer == snapshot.Dossier.LeadRecorder {
		return domain.NewError(domain.CodeState, "复核员不能与记录负责人相同")
	}
	return nil
}

func (AuthorizationPolicy) RequireIssuer(snapshot domain.Snapshot, actor, issuedBy string) error {
	actor, issuedBy = strings.TrimSpace(actor), strings.TrimSpace(issuedBy)
	if issuedBy == "" {
		return domain.NewError(domain.CodeValidation, "签发人不能为空")
	}
	if actor != issuedBy {
		return domain.NewError(domain.CodeState, "actor 必须与签发人一致")
	}
	if snapshot.Review != nil && issuedBy == snapshot.Review.Reviewer {
		return domain.NewError(domain.CodeState, "项目负责人签发人与复核员必须分离")
	}
	return nil
}
