package credential_reissue_cache_stale_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strata-proof/internal/application"
	"strata-proof/internal/domain"
	"strata-proof/internal/evidence"
	"strata-proof/internal/httpui"
	"strata-proof/internal/store"
)

func TestReissueRefreshesCachedCredentialVerification(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("打开私有测试数据库: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	const (
		dossierID    = "private-reissue-cache-dossier"
		credentialID = "private-revoked-credential"
		issuerName   = "负责人丙"
	)
	now := time.Date(2026, time.August, 27, 10, 0, 0, 0, time.UTC)
	checklist := domain.ReviewChecklist{UnitCompleteness: true, RelationEvidence: true, FindingClosure: true, ManifestPreview: true}
	review := domain.ReviewDecision{ID: "private-review", DossierID: dossierID, Reviewer: "复核员乙", Approved: true, Note: "私有复现通过", DecidedAt: now, Checklist: checklist}
	manifestSource := domain.Snapshot{
		Dossier:        domain.TrenchDossier{ID: dossierID, Version: 7},
		LastCheckRunID: "private-check",
		Units:          []domain.StratigraphicUnit{},
		Relations:      []domain.StratigraphicRelation{},
		Findings:       []domain.ConsistencyFinding{},
	}
	manifest, err := evidence.BuildManifest(manifestSource, review, now)
	if err != nil {
		t.Fatalf("构造冻结清单: %v", err)
	}
	credentialIssuer := evidence.NewCredentialIssuer("private-reissue-secret")
	credential := credentialIssuer.Issue(credentialID, manifest, review, issuerName, now.Add(time.Minute))
	revokedAt := now.Add(2 * time.Minute)
	credential.Status = "revoked"
	credential.RevokedBy = issuerName
	credential.RevokedAt = &revokedAt
	credential.RevocationReason = "私有复现撤销"
	snapshot := domain.Snapshot{
		Dossier: domain.TrenchDossier{
			ID:                   dossierID,
			TrenchCode:           "PRIVATE-REISSUE-CACHE",
			ExcavationArea:       "私有复现区",
			LeadRecorder:         "记录员甲",
			ChronologyHypothesis: "汉代",
			Status:               domain.StatusIssued,
			Version:              8,
			CreatedAt:            now.Add(-time.Hour),
			UpdatedAt:            revokedAt,
		},
		LastCheckRunID: "private-check",
		Units:          []domain.StratigraphicUnit{},
		Relations:      []domain.StratigraphicRelation{},
		Findings:       []domain.ConsistencyFinding{},
		CheckBatches:   []domain.CheckBatch{},
		Review:         &review,
		Manifest:       &manifest,
		Credentials:    []domain.ResearchCredential{credential},
		Audit: []domain.AuditEntry{{
			DossierID:  dossierID,
			Sequence:   1,
			EventType:  domain.EventCredentialRevoked,
			Actor:      issuerName,
			Summary:    "撤销研究使用凭据",
			OccurredAt: revokedAt,
		}},
	}
	if err := repository.Create(context.Background(), snapshot, "", nil); err != nil {
		t.Fatalf("写入私有凭据夹具: %v", err)
	}

	service := application.NewService(repository, evidence.NewAnalyzer(), credentialIssuer)
	routes := httpui.NewHandler(service).Routes()
	var before application.VerificationResult
	executeJSON(t, routes, http.MethodGet, "/api/v1/credentials/"+credentialID, nil, http.StatusOK, &before)
	if before.Valid || before.ReplacementCredentialID != "" || len(before.Audit) != 1 {
		t.Fatalf("补发前验真夹具无效: valid=%v replacement=%q audit=%d", before.Valid, before.ReplacementCredentialID, len(before.Audit))
	}

	payload := map[string]any{"expectedVersion": 8, "actor": issuerName, "idempotencyKey": "private-reissue-001"}
	var reissued domain.Snapshot
	executeJSON(t, routes, http.MethodPost, "/api/v1/dossiers/"+dossierID+"/credentials/"+credentialID+"/reissue", payload, http.StatusCreated, &reissued)
	if len(reissued.Credentials) != 2 {
		t.Fatalf("补发响应未包含两条凭据: count=%d", len(reissued.Credentials))
	}
	replacementID := reissued.Credentials[1].CredentialID
	if reissued.Credentials[0].ReplacementCredentialID != replacementID || len(reissued.Audit) != 2 {
		t.Fatalf("补发未正确提交: replacement=%q audit=%d", reissued.Credentials[0].ReplacementCredentialID, len(reissued.Audit))
	}

	var after application.VerificationResult
	executeJSON(t, routes, http.MethodGet, "/api/v1/credentials/"+credentialID, nil, http.StatusOK, &after)
	if after.ReplacementCredentialID != replacementID || len(after.Audit) != 2 {
		t.Fatalf("已补发凭据仍返回补发前缓存: wantReplacement=%q got=%q wantAudit=2 gotAudit=%d", replacementID, after.ReplacementCredentialID, len(after.Audit))
	}
}

func executeJSON(t *testing.T, handler http.Handler, method, target string, body any, expectedStatus int, result any) {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("编码请求: %v", err)
		}
	}
	request := httptest.NewRequest(method, target, bytes.NewReader(encoded))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != expectedStatus {
		t.Fatalf("%s %s 返回 HTTP %d: %s", method, target, response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), result); err != nil {
		t.Fatalf("解析 %s %s 响应: %v", method, target, err)
	}
}
