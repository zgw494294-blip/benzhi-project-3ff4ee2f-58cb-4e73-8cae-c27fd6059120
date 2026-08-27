package application

import (
	"context"
	"testing"

	"strata-proof/internal/domain"
)

func createForExtensions(t *testing.T, code string) (*Service, context.Context, domain.Snapshot) {
	t.Helper()
	s := testService(t)
	ctx := context.Background()
	snapshot, err := s.CreateDossier(ctx, CreateDossierCommand{TrenchCode: code, ExcavationArea: "测试区", LeadRecorder: "记录员", ChronologyHypothesis: "汉代", Actor: "记录员", IdempotencyKey: "create-" + code})
	if err != nil {
		t.Fatal(err)
	}
	return s, ctx, snapshot
}

func batchRows(codes ...string) []BatchUnitRow {
	rows := make([]BatchUnitRow, 0, len(codes))
	for i, code := range codes {
		rows = append(rows, BatchUnitRow{Row: i + 1, UnitCode: " " + code + " ", UnitType: " 堆积 ", Description: " 单位描述 ", TopElevation: 10 - float64(i), BottomElevation: 9 - float64(i), SoilTraits: " 灰土 ", PhotoRefs: []string{" photo://1 ", "photo://1"}})
	}
	return rows
}

func TestBatchUnitsAtomicValidationAndIdempotency(t *testing.T) {
	s, ctx, snapshot := createForExtensions(t, "EXT-BATCH")
	cmd := BatchPutUnitsCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Rows: batchRows("A", "B", "C"), Actor: "记录员", IdempotencyKey: "batch-units-001"}
	result, err := s.PutUnitsBatch(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Dossier.Version != 2 || len(result.Snapshot.Units) != 3 || len(result.Items) != 3 || len(result.Snapshot.Audit) != 2 {
		t.Fatalf("批量登记结果错误: %#v", result)
	}
	for _, unit := range result.Snapshot.Units {
		if unit.Revision != 1 || len(unit.PhotoRefs) != 1 {
			t.Fatalf("单位未规范化或非首版: %#v", unit)
		}
	}
	retry, err := s.PutUnitsBatch(ctx, cmd)
	if err != nil || retry.Snapshot.Dossier.Version != 2 || len(retry.Snapshot.Audit) != 2 {
		t.Fatalf("批量幂等重试失败: %#v %v", retry, err)
	}
	invalid := batchRows("D", "B", "E")
	invalid[2].TopElevation, invalid[2].BottomElevation = 1, 2
	_, err = s.PutUnitsBatch(ctx, BatchPutUnitsCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: 2, Rows: invalid, Actor: "记录员", IdempotencyKey: "batch-units-002"})
	if domain.ErrorCodeOf(err) != domain.CodeValidation {
		t.Fatalf("应返回逐行校验错误: %v", err)
	}
	details, ok := domain.ErrorDetails(err).([]domain.RowValidationError)
	if !ok || len(details) != 2 || details[0].Row != 2 || details[1].Row != 3 {
		t.Fatalf("错误行位置不明确: %#v", domain.ErrorDetails(err))
	}
	loaded, _ := s.GetDossier(ctx, snapshot.Dossier.ID)
	if loaded.Dossier.Version != 2 || len(loaded.Units) != 3 || len(loaded.Audit) != 2 {
		t.Fatalf("无效批次发生了部分落账: %#v", loaded)
	}
}

func TestRelationPathReadOnlyAndConflict(t *testing.T) {
	s, ctx, snapshot := createForExtensions(t, "EXT-PATH")
	batch, err := s.PutUnitsBatch(ctx, BatchPutUnitsCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: 1, Rows: batchRows("A", "B", "C"), Actor: "记录员", IdempotencyKey: "path-batch-001"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot = batch.Snapshot
	a, b, c := snapshot.Units[0].ID, snapshot.Units[1].ID, snapshot.Units[2].ID
	for i, rel := range []PutRelationCommand{{SourceUnitID: a, TargetUnitID: b, RelationType: domain.RelationOverlies, EvidenceNote: "证据甲"}, {SourceUnitID: b, TargetUnitID: c, RelationType: domain.RelationCuts, EvidenceNote: "证据乙"}} {
		rel.DossierID, rel.ExpectedVersion, rel.Actor, rel.IdempotencyKey = snapshot.Dossier.ID, snapshot.Dossier.Version, "记录员", "path-rel-00"+string(rune('1'+i))
		snapshot, err = s.PutRelation(ctx, rel)
		if err != nil {
			t.Fatal(err)
		}
	}
	version, audits := snapshot.Dossier.Version, len(snapshot.Audit)
	path, err := s.TraceRelationPath(ctx, snapshot.Dossier.ID, a, c)
	if err != nil || path.Classification != "transitive_mixed" || len(path.Path) != 2 {
		t.Fatalf("传递路径错误: %#v %v", path, err)
	}
	reverse, _ := s.TraceRelationPath(ctx, snapshot.Dossier.ID, c, a)
	if reverse.Classification != "unreachable" {
		t.Fatalf("反向应不可达: %#v", reverse)
	}
	loaded, _ := s.GetDossier(ctx, snapshot.Dossier.ID)
	if loaded.Dossier.Version != version || len(loaded.Audit) != audits {
		t.Fatal("路径查询改变了案卷")
	}
	snapshot, err = s.PutRelation(ctx, PutRelationCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, SourceUnitID: a, TargetUnitID: c, RelationType: domain.RelationContemporary, EvidenceNote: "同期证据", Actor: "记录员", IdempotencyKey: "path-rel-contemporary"})
	if err != nil {
		t.Fatal(err)
	}
	path, _ = s.TraceRelationPath(ctx, snapshot.Dossier.ID, a, c)
	if !path.Conflict || len(path.ConflictRelationIDs) != 3 {
		t.Fatalf("同期方向冲突未标记: %#v", path)
	}
	other, err := s.CreateDossier(ctx, CreateDossierCommand{TrenchCode: "EXT-PATH-OTHER", ExcavationArea: "测试区", LeadRecorder: "记录员", ChronologyHypothesis: "汉代", Actor: "记录员", IdempotencyKey: "create-path-other"})
	if err != nil {
		t.Fatal(err)
	}
	otherBatch, err := s.PutUnitsBatch(ctx, BatchPutUnitsCommand{DossierID: other.Dossier.ID, ExpectedVersion: 1, Rows: batchRows("X"), Actor: "记录员", IdempotencyKey: "path-other-batch"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.TraceRelationPath(ctx, snapshot.Dossier.ID, a, otherBatch.Snapshot.Units[0].ID)
	if domain.ErrorCodeOf(err) != domain.CodeValidation {
		t.Fatalf("跨案卷端点应拒绝: %v", err)
	}
}

func TestCheckBatchInheritanceAndRevisionReopen(t *testing.T) {
	s, ctx, snapshot := createForExtensions(t, "EXT-CHECK")
	batch, _ := s.PutUnitsBatch(ctx, BatchPutUnitsCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: 1, Rows: batchRows("A", "B", "C"), Actor: "记录员", IdempotencyKey: "check-batch-001"})
	snapshot = batch.Snapshot
	relation := PutRelationCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, SourceUnitID: snapshot.Units[0].ID, TargetUnitID: snapshot.Units[1].ID, RelationType: domain.RelationOverlies, EvidenceNote: "证据", Actor: "记录员", IdempotencyKey: "check-rel-001"}
	snapshot, _ = s.PutRelation(ctx, relation)
	snapshot, _ = s.RunCheck(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "check-run-001"})
	if len(snapshot.Findings) != 1 || snapshot.Findings[0].Severity != "warning" {
		t.Fatalf("应只有孤立单位提醒: %#v", snapshot.Findings)
	}
	findingID := snapshot.Findings[0].ID
	snapshot, _ = s.ResolveFinding(ctx, ResolveFindingCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "check-resolve-001"}, FindingID: findingID, ResolutionNote: "暂保持孤立"})
	current := snapshot.Relations[0]
	snapshot, _ = s.PutRelation(ctx, PutRelationCommand{DossierID: snapshot.Dossier.ID, RelationID: current.ID, ExpectedVersion: snapshot.Dossier.Version, SourceUnitID: current.SourceUnitID, TargetUnitID: current.TargetUnitID, RelationType: current.RelationType, EvidenceNote: "补充证据", Actor: "记录员", IdempotencyKey: "check-rel-002"})
	snapshot, _ = s.RunCheck(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "check-run-002"})
	if snapshot.Findings[0].Status != domain.FindingResolved || snapshot.Findings[0].ResolutionNote != "暂保持孤立" || snapshot.Findings[0].ChangeType != "persistent" {
		t.Fatalf("未继承未受影响提醒: %#v", snapshot.Findings[0])
	}
	cUnit := snapshot.Units[2]
	snapshot, _ = s.PutUnit(ctx, PutUnitCommand{DossierID: snapshot.Dossier.ID, UnitID: cUnit.ID, ExpectedVersion: snapshot.Dossier.Version, UnitCode: cUnit.UnitCode, UnitType: cUnit.UnitType, Description: "修订描述", TopElevation: cUnit.TopElevation, BottomElevation: cUnit.BottomElevation, SoilTraits: cUnit.SoilTraits, PhotoRefs: cUnit.PhotoRefs, Actor: "记录员", IdempotencyKey: "check-unit-revise"})
	snapshot, _ = s.RunCheck(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "check-run-003"})
	if snapshot.Findings[0].Status != domain.FindingOpen || len(snapshot.Findings[0].TriggerRevisions) == 0 || len(snapshot.CheckBatches) != 3 {
		t.Fatalf("引用修订变化后未重开: %#v", snapshot.Findings[0])
	}
	if snapshot.CheckBatches[0].Findings[0].ResolutionNote != "暂保持孤立" {
		t.Fatal("旧检查批次的整改说明丢失")
	}
}

func TestStructuredReviewTargetedClosureAndCredentialReissue(t *testing.T) {
	s, ctx, snapshot := createForExtensions(t, "EXT-REVIEW")
	batch, err := s.PutUnitsBatch(ctx, BatchPutUnitsCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: 1, Rows: batchRows("A", "B", "C"), Actor: "记录员", IdempotencyKey: "review-batch-001"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot = batch.Snapshot
	snapshot, err = s.PutRelation(ctx, PutRelationCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, SourceUnitID: snapshot.Units[0].ID, TargetUnitID: snapshot.Units[1].ID, RelationType: domain.RelationOverlies, EvidenceNote: "关系证据", Actor: "记录员", IdempotencyKey: "review-rel-001"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ = s.RunCheck(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "review-check-001"})
	findingID := snapshot.Findings[0].ID
	snapshot, _ = s.ResolveFinding(ctx, ResolveFindingCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "review-resolve-001"}, FindingID: findingID, ResolutionNote: "保留孤立作为采样单元"})
	snapshot, err = s.SubmitReview(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "review-submit-001"})
	if err != nil {
		t.Fatal(err)
	}
	before := snapshot.Dossier.Version
	_, err = s.Review(ctx, ReviewCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: before, Actor: "复核员", IdempotencyKey: "review-missing-checklist"}, Approved: true, Reviewer: "复核员", Note: "通过"})
	if domain.ErrorCodeOf(err) != domain.CodeState {
		t.Fatalf("遗漏清单确认应拒绝: %v", err)
	}
	loaded, _ := s.GetDossier(ctx, snapshot.Dossier.ID)
	if loaded.Dossier.Version != before {
		t.Fatal("失败复核改变了版本")
	}
	snapshot, err = s.Review(ctx, ReviewCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: before, Actor: "复核员", IdempotencyKey: "review-return-001"}, Approved: false, Reviewer: "复核员", Note: "定向整改", Targets: []ReviewTarget{{TargetType: "relation", TargetID: snapshot.Relations[0].ID, Reason: "补充剖面证据"}, {TargetType: "finding", TargetID: findingID, Reason: "补充孤立说明"}}})
	if err != nil || snapshot.Dossier.Status != domain.StatusReturned || len(snapshot.RemediationItems) != 2 {
		t.Fatalf("定向退回失败: %#v %v", snapshot, err)
	}
	relation := snapshot.Relations[0]
	snapshot, err = s.PutRelation(ctx, PutRelationCommand{DossierID: snapshot.Dossier.ID, RelationID: relation.ID, ExpectedVersion: snapshot.Dossier.Version, SourceUnitID: relation.SourceUnitID, TargetUnitID: relation.TargetUnitID, RelationType: relation.RelationType, EvidenceNote: "补充剖面照片证据", Actor: "记录员", IdempotencyKey: "review-rel-002"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.ResolveFinding(ctx, ResolveFindingCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "review-resolve-002"}, FindingID: findingID, ResolutionNote: "补充采样目的说明"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SubmitReview(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "review-submit-too-early"})
	if domain.ErrorCodeOf(err) != domain.CodeState {
		t.Fatalf("整改后未复查应拒绝送审: %v", err)
	}
	snapshot, _ = s.RunCheck(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "review-check-002"})
	snapshot, err = s.SubmitReview(ctx, VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "记录员", IdempotencyKey: "review-submit-002"})
	if err != nil {
		t.Fatal(err)
	}
	checklist := domain.ReviewChecklist{UnitCompleteness: true, RelationEvidence: true, FindingClosure: true, ManifestPreview: true}
	snapshot, err = s.Review(ctx, ReviewCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "复核员", IdempotencyKey: "review-approve-001"}, Approved: true, Reviewer: "复核员", Note: "整改完成", Checklist: checklist})
	if err != nil || snapshot.Manifest == nil || !snapshot.Manifest.ReviewChecklist.RelationEvidence || len(snapshot.Manifest.RemediationItems) != 2 {
		t.Fatalf("冻结摘要未纳入清单和闭环: %#v %v", snapshot.Manifest, err)
	}
	snapshot, err = s.IssueCredential(ctx, IssueCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "负责人", IdempotencyKey: "review-issue-001"}, IssuedBy: "负责人"})
	if err != nil {
		t.Fatal(err)
	}
	credentialID, version, audits := snapshot.Credentials[0].CredentialID, snapshot.Dossier.Version, len(snapshot.Audit)
	_, err = s.RevokeCredential(ctx, RevokeCredentialCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: version, Actor: "他人", IdempotencyKey: "review-revoke-wrong"}, CredentialID: credentialID, Reason: "无权撤销"})
	if domain.ErrorCodeOf(err) != domain.CodeState {
		t.Fatalf("非签发人撤销应拒绝: %v", err)
	}
	snapshot, err = s.RevokeCredential(ctx, RevokeCredentialCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: version, Actor: "负责人", IdempotencyKey: "review-revoke-001"}, CredentialID: credentialID, Reason: "研究用途终止"})
	if err != nil || snapshot.Dossier.Status != domain.StatusIssued || snapshot.Credentials[0].Status != "revoked" || len(snapshot.Audit) != audits+1 {
		t.Fatalf("撤销状态错误: %#v %v", snapshot, err)
	}
	retry, err := s.RevokeCredential(ctx, RevokeCredentialCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: version, Actor: "负责人", IdempotencyKey: "review-revoke-001"}, CredentialID: credentialID, Reason: "研究用途终止"})
	if err != nil || retry.Dossier.Version != snapshot.Dossier.Version || len(retry.Audit) != len(snapshot.Audit) {
		t.Fatalf("撤销幂等失败: %#v %v", retry, err)
	}
	oldResult, _ := s.VerifyCredential(ctx, credentialID)
	if oldResult.Valid || oldResult.Message != "凭据已撤销：研究用途终止" {
		t.Fatalf("旧凭据验真错误: %#v", oldResult)
	}
	snapshot, err = s.ReissueCredential(ctx, ReissueCredentialCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "负责人", IdempotencyKey: "review-reissue-001"}, CredentialID: credentialID})
	if err != nil || len(snapshot.Credentials) != 2 || snapshot.Credentials[0].ReplacementCredentialID != snapshot.Credentials[1].CredentialID {
		t.Fatalf("关联补发失败: %#v %v", snapshot.Credentials, err)
	}
	newID := snapshot.Credentials[1].CredentialID
	oldResult, _ = s.VerifyCredential(ctx, credentialID)
	newResult, _ := s.VerifyCredential(ctx, newID)
	if oldResult.ReplacementCredentialID != newID || !newResult.Valid || len(newResult.Audit) != len(snapshot.Audit) {
		t.Fatalf("新旧凭据验真链错误: %#v %#v", oldResult, newResult)
	}
	_, err = s.ReissueCredential(ctx, ReissueCredentialCommand{VersionedCommand: VersionedCommand{DossierID: snapshot.Dossier.ID, ExpectedVersion: snapshot.Dossier.Version, Actor: "负责人", IdempotencyKey: "review-reissue-active"}, CredentialID: newID})
	if domain.ErrorCodeOf(err) != domain.CodeState {
		t.Fatalf("有效凭据不能补发: %v", err)
	}
}
