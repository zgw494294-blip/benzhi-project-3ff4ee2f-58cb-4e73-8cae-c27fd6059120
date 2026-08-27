package domain

import (
	"fmt"
	"strings"
)

func ValidateDossier(d TrenchDossier) error {
	if strings.TrimSpace(d.TrenchCode) == "" {
		return NewError(CodeValidation, "探方编号不能为空")
	}
	if strings.TrimSpace(d.ExcavationArea) == "" {
		return NewError(CodeValidation, "发掘区域不能为空")
	}
	if strings.TrimSpace(d.LeadRecorder) == "" {
		return NewError(CodeValidation, "记录负责人不能为空")
	}
	if strings.TrimSpace(d.ChronologyHypothesis) == "" {
		return NewError(CodeValidation, "年代假设不能为空")
	}
	return nil
}

func ValidateUnit(unit StratigraphicUnit) error {
	if strings.TrimSpace(unit.UnitCode) == "" {
		return NewError(CodeValidation, "地层单位编号不能为空")
	}
	if strings.TrimSpace(unit.UnitType) == "" {
		return NewError(CodeValidation, "地层单位类型不能为空")
	}
	if strings.TrimSpace(unit.Description) == "" {
		return NewError(CodeValidation, "地层单位描述不能为空")
	}
	if unit.TopElevation < unit.BottomElevation {
		return NewError(CodeValidation, "界面顶标高不能低于底标高")
	}
	if strings.TrimSpace(unit.SoilTraits) == "" {
		return NewError(CodeValidation, "土质特征不能为空")
	}
	for _, ref := range unit.PhotoRefs {
		if strings.TrimSpace(ref) == "" {
			return NewError(CodeValidation, "照片引用不能包含空值")
		}
	}
	return nil
}

func ValidateRelation(relation StratigraphicRelation, units []StratigraphicUnit, relations []StratigraphicRelation, replacingID string) error {
	if relation.SourceUnitID == relation.TargetUnitID {
		return NewError(CodeValidation, "地层关系不能自关联")
	}
	known := map[string]bool{}
	for _, unit := range units {
		known[unit.ID] = true
	}
	if !known[relation.SourceUnitID] || !known[relation.TargetUnitID] {
		return NewError(CodeValidation, "地层关系端点不存在")
	}
	switch relation.RelationType {
	case RelationOverlies, RelationCuts, RelationContemporary:
	default:
		return NewError(CodeValidation, "未知关系类型 %q", relation.RelationType)
	}
	for _, existing := range relations {
		if existing.ID == replacingID {
			continue
		}
		sameDirection := existing.SourceUnitID == relation.SourceUnitID && existing.TargetUnitID == relation.TargetUnitID
		reverseContemporary := relation.RelationType == RelationContemporary && existing.RelationType == RelationContemporary && existing.SourceUnitID == relation.TargetUnitID && existing.TargetUnitID == relation.SourceUnitID
		if (sameDirection && existing.RelationType == relation.RelationType) || reverseContemporary {
			return NewError(CodeConflict, "重复关系已经存在")
		}
	}
	return nil
}

func EnsureMutable(status DossierStatus) error {
	if status == StatusFrozen || status == StatusIssued {
		return NewError(CodeFrozen, "案卷已冻结，禁止修改业务内容")
	}
	if status == StatusReview {
		return NewError(CodeState, "案卷正在复核，不能修改业务内容")
	}
	return nil
}

func EnsureTransition(from, to DossierStatus) error {
	allowed := map[DossierStatus]map[DossierStatus]bool{
		StatusDraft:     {StatusRemediate: true, StatusReview: true},
		StatusRemediate: {StatusRemediate: true, StatusReview: true},
		StatusReview:    {StatusReturned: true, StatusFrozen: true},
		StatusReturned:  {StatusRemediate: true, StatusReview: true},
		StatusFrozen:    {StatusIssued: true},
	}
	if !allowed[from][to] {
		return NewError(CodeState, fmt.Sprintf("案卷状态不能从 %s 变为 %s", from, to))
	}
	return nil
}
