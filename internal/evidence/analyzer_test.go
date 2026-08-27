package evidence

import (
	"testing"

	"strata-proof/internal/domain"
)

func TestAnalyzerDetectsCycleConflictIsolationAndEvidence(t *testing.T) {
	units := []domain.StratigraphicUnit{{ID: "a", UnitCode: "A", SoilTraits: "土", PhotoRefs: []string{"p"}}, {ID: "b", UnitCode: "B", SoilTraits: "土", PhotoRefs: []string{"p"}}, {ID: "c", UnitCode: "C", SoilTraits: "土"}}
	relations := []domain.StratigraphicRelation{{ID: "r1", SourceUnitID: "a", TargetUnitID: "b", RelationType: domain.RelationOverlies, EvidenceNote: "剖面"}, {ID: "r2", SourceUnitID: "b", TargetUnitID: "a", RelationType: domain.RelationCuts, EvidenceNote: "标高"}, {ID: "r3", SourceUnitID: "a", TargetUnitID: "b", RelationType: domain.RelationContemporary}}
	findings := NewAnalyzer().Analyze("d", "run", units, relations)
	rules := map[string]bool{}
	for _, finding := range findings {
		rules[finding.RuleCode] = true
	}
	for _, rule := range []string{"DIRECTED_CYCLE", "MUTUAL_EXCLUSION", "ISOLATED_UNIT", "EVIDENCE_UNIT", "EVIDENCE_RELATION"} {
		if !rules[rule] {
			t.Errorf("缺少规则结果 %s", rule)
		}
	}
	for i := 1; i < len(findings); i++ {
		if severityRank(findings[i-1].Severity) > severityRank(findings[i].Severity) {
			t.Fatal("问题未按严重级别稳定排序")
		}
	}
}

func TestManifestAndCredentialDetectTampering(t *testing.T) {
	snapshot := domain.Snapshot{Dossier: domain.TrenchDossier{ID: "d", Version: 7}, LastCheckRunID: "check", Units: []domain.StratigraphicUnit{{ID: "u", Revision: 2}}, Relations: []domain.StratigraphicRelation{{ID: "r", Revision: 3}}}
	review := domain.ReviewDecision{ID: "review", DossierID: "d", Approved: true}
	manifest, err := BuildManifest(snapshot, review, snapshot.Dossier.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	issuer := NewCredentialIssuer("secret")
	credential := issuer.Issue("cred", manifest, review, "负责人", snapshot.Dossier.CreatedAt)
	if valid, _ := issuer.Verify(credential, manifest); !valid {
		t.Fatal("合法凭据未通过")
	}
	credential.IssuedBy = "篡改者"
	if valid, _ := issuer.Verify(credential, manifest); valid {
		t.Fatal("篡改凭据不应通过")
	}
}
