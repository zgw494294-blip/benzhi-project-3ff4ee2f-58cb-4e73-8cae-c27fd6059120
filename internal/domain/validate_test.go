package domain

import "testing"

func TestValidateRelationRejectsInvalidEdges(t *testing.T) {
	units := []StratigraphicUnit{{ID: "a"}, {ID: "b"}}
	base := StratigraphicRelation{SourceUnitID: "a", TargetUnitID: "b", RelationType: RelationOverlies}
	if err := ValidateRelation(base, units, nil, ""); err != nil {
		t.Fatalf("合法关系被拒绝: %v", err)
	}
	self := base
	self.TargetUnitID = "a"
	if ErrorCodeOf(ValidateRelation(self, units, nil, "")) != CodeValidation {
		t.Fatal("应拒绝自关联")
	}
	missing := base
	missing.TargetUnitID = "c"
	if ErrorCodeOf(ValidateRelation(missing, units, nil, "")) != CodeValidation {
		t.Fatal("应拒绝缺失端点")
	}
	existing := base
	existing.ID = "r1"
	if ErrorCodeOf(ValidateRelation(base, units, []StratigraphicRelation{existing}, "")) != CodeConflict {
		t.Fatal("应拒绝重复关系")
	}
}

func TestFrozenStatusIsImmutable(t *testing.T) {
	if ErrorCodeOf(EnsureMutable(StatusFrozen)) != CodeFrozen {
		t.Fatal("冻结案卷应禁止修改")
	}
	if err := EnsureTransition(StatusFrozen, StatusIssued); err != nil {
		t.Fatalf("冻结案卷应可签发: %v", err)
	}
	if ErrorCodeOf(EnsureTransition(StatusIssued, StatusDraft)) != CodeState {
		t.Fatal("已签发案卷不能回到草拟")
	}
}
