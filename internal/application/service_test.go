package application

import (
	"context"
	"testing"

	"strata-proof/internal/domain"
	"strata-proof/internal/evidence"
	"strata-proof/internal/store"
)

func testService(t *testing.T) *Service {
	t.Helper()
	repo, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return NewService(repo, evidence.NewAnalyzer(), evidence.NewCredentialIssuer("test-secret"))
}

func TestCompleteWorkflowIdempotencyAndFreeze(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	snapshot, err := s.CreateDossier(ctx, CreateDossierCommand{TrenchCode: "T1", ExcavationArea: "北区", LeadRecorder: "甲", ChronologyHypothesis: "汉代", Actor: "甲", IdempotencyKey: "create-0001"})
	if err != nil {
		t.Fatal(err)
	}
	first := PutUnitCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, UnitCode: "U1", UnitType: "堆积", Description: "上层", TopElevation: 10, BottomElevation: 9, SoilTraits: "灰土", PhotoRefs: []string{"p1"}, Actor: "甲", IdempotencyKey: "unit-00001"}
	snapshot, err = s.PutUnit(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	id1 := snapshot.Units[0].ID
	retry, err := s.PutUnit(ctx, first)
	if err != nil || retry.Dossier.Version != snapshot.Dossier.Version || len(retry.Units) != 1 {
		t.Fatalf("幂等重试失败: %v", err)
	}
	second := first
	second.ExpectedVersion = snapshot.Dossier.Version
	second.UnitCode = "U2"
	second.Description = "下层"
	second.IdempotencyKey = "unit-00002"
	snapshot, err = s.PutUnit(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	id2 := snapshot.Units[1].ID
	snapshot, err = s.PutRelation(ctx, PutRelationCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, SourceUnitID: id1, TargetUnitID: id2, RelationType: domain.RelationOverlies, EvidenceNote: "剖面关系明确", Actor: "甲", IdempotencyKey: "relation-001"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.RunCheck(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "甲", IdempotencyKey: "check-00001"})
	if err != nil || len(snapshot.Findings) != 0 {
		t.Fatalf("检查失败: %v %#v", err, snapshot.Findings)
	}
	snapshot, err = s.SubmitReview(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "甲", IdempotencyKey: "submit-001"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.Review(ctx, ReviewCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "乙", IdempotencyKey: "review-001"}, Approved: true, Reviewer: "乙", Note: "通过", Checklist: domain.ReviewChecklist{UnitCompleteness: true, RelationEvidence: true, FindingClosure: true, ManifestPreview: true}})
	if err != nil || snapshot.Manifest == nil {
		t.Fatalf("冻结失败: %v", err)
	}
	_, err = s.PutUnit(ctx, PutUnitCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, UnitID: id1, UnitCode: "U1", Actor: "甲", IdempotencyKey: "frozen-edit"})
	if domain.ErrorCodeOf(err) != domain.CodeFrozen {
		t.Fatalf("冻结后应拒绝修改: %v", err)
	}
	snapshot, err = s.IssueCredential(ctx, IssueCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "丙", IdempotencyKey: "issue-0001"}, IssuedBy: "丙"})
	if err != nil {
		t.Fatal(err)
	}
	verified, err := s.VerifyCredential(ctx, snapshot.Credentials[0].CredentialID)
	if err != nil || !verified.Valid || len(verified.Audit) != 8 {
		t.Fatalf("验真失败: %#v %v", verified, err)
	}
}

func TestSeriousFindingCannotBeClosedByNote(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	snapshot, err := s.CreateDossier(ctx, CreateDossierCommand{TrenchCode: "T2", ExcavationArea: "区", LeadRecorder: "甲", ChronologyHypothesis: "汉", Actor: "甲", IdempotencyKey: "create-0002"})
	if err != nil {
		t.Fatal(err)
	}
	unit := PutUnitCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, UnitCode: "A", UnitType: "堆积", Description: "甲层", TopElevation: 2, BottomElevation: 1, SoilTraits: "灰土", PhotoRefs: []string{"p"}, Actor: "甲", IdempotencyKey: "cycle-unit-a"}
	snapshot, err = s.PutUnit(ctx, unit)
	if err != nil {
		t.Fatal(err)
	}
	first := snapshot.Units[0].ID
	unit.ExpectedVersion, unit.UnitCode, unit.Description, unit.IdempotencyKey = snapshot.Dossier.Version, "B", "乙层", "cycle-unit-b"
	snapshot, err = s.PutUnit(ctx, unit)
	if err != nil {
		t.Fatal(err)
	}
	second := snapshot.Units[1].ID
	relation := PutRelationCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, SourceUnitID: first, TargetUnitID: second, RelationType: domain.RelationOverlies, EvidenceNote: "证据一", Actor: "甲", IdempotencyKey: "cycle-rel-a"}
	snapshot, err = s.PutRelation(ctx, relation)
	if err != nil {
		t.Fatal(err)
	}
	relation.ExpectedVersion, relation.SourceUnitID, relation.TargetUnitID, relation.IdempotencyKey = snapshot.Dossier.Version, second, first, "cycle-rel-b"
	snapshot, err = s.PutRelation(ctx, relation)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.RunCheck(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "甲", IdempotencyKey: "cycle-check"})
	if err != nil {
		t.Fatal(err)
	}
	var serious string
	for _, finding := range snapshot.Findings {
		if finding.Severity == "error" {
			serious = finding.ID
			break
		}
	}
	if serious == "" {
		t.Fatal("应生成严重问题")
	}
	_, err = s.ResolveFinding(ctx, ResolveFindingCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "甲", IdempotencyKey: "resolve-001"}, FindingID: serious, ResolutionNote: "忽略"})
	if domain.ErrorCodeOf(err) != domain.CodeState {
		t.Fatalf("严重问题不能仅以说明关闭: %v", err)
	}
}
