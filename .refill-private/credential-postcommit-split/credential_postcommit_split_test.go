package credential_postcommit_split_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"strata-proof/internal/application"
	"strata-proof/internal/domain"
	"strata-proof/internal/evidence"
	"strata-proof/internal/httpui"
	"strata-proof/internal/store"
)

func TestCredentialLedgerFailureRollsBackEntireIssue(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "credential-atomicity.db")
	repository, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	now := time.Date(2026, time.August, 27, 9, 0, 0, 0, time.UTC)
	review := domain.ReviewDecision{ID: "review-private", DossierID: "dossier-private", Reviewer: "复核员", Approved: true, Note: "已通过", DecidedAt: now, Checklist: domain.ReviewChecklist{UnitCompleteness: true, RelationEvidence: true, FindingClosure: true, ManifestPreview: true}}
	snapshot := domain.Snapshot{
		Dossier:        domain.TrenchDossier{ID: "dossier-private", TrenchCode: "PRIVATE-CREDENTIAL-ATOMIC", ExcavationArea: "私有复现区", LeadRecorder: "记录员", ChronologyHypothesis: "测试年代", Status: domain.StatusFrozen, Version: 1, CreatedAt: now, UpdatedAt: now},
		LastCheckRunID: "check-private",
		Units:          []domain.StratigraphicUnit{}, Relations: []domain.StratigraphicRelation{}, Findings: []domain.ConsistencyFinding{}, Credentials: []domain.ResearchCredential{},
		Review: &review,
		Audit:  []domain.AuditEntry{{DossierID: "dossier-private", Sequence: 1, EventType: domain.EventReviewApproved, Actor: "复核员", Summary: "复核冻结", OccurredAt: now}},
	}
	manifest, err := evidence.BuildManifest(snapshot, review, now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Manifest = &manifest
	response, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(context.Background(), snapshot, "private-frozen-create", response); err != nil {
		t.Fatal(err)
	}

	control, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.ExecContext(context.Background(), `CREATE TRIGGER reject_credential_insert BEFORE INSERT ON credentials BEGIN SELECT RAISE(ABORT,'forced credential ledger failure'); END`); err != nil {
		_ = control.Close()
		t.Fatal(err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	issuer := evidence.NewCredentialIssuer("private-atomicity-secret")
	service := application.NewService(repository, evidence.NewAnalyzer(), issuer)
	body := `{"expectedVersion":1,"actor":"负责人","idempotencyKey":"private-issue-failure","issuedBy":"负责人"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/dossiers/dossier-private/credentials", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()
	httpui.NewHandler(service).Routes().ServeHTTP(responseRecorder, request)
	if responseRecorder.Code < 500 {
		t.Fatalf("受控凭据子账本错误未使请求失败: status=%d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}

	probe, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	var durableJSON []byte
	if err := probe.QueryRowContext(context.Background(), `SELECT snapshot_json FROM dossiers WHERE id=?`, snapshot.Dossier.ID).Scan(&durableJSON); err != nil {
		t.Fatal(err)
	}
	var durable domain.Snapshot
	if err := json.Unmarshal(durableJSON, &durable); err != nil {
		t.Fatal(err)
	}
	var credentialRows, idempotencyRows int
	if err := probe.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM credentials WHERE dossier_id=?`, snapshot.Dossier.ID).Scan(&credentialRows); err != nil {
		t.Fatal(err)
	}
	if err := probe.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM idempotency_results WHERE idempotency_key=?`, "private-issue-failure").Scan(&idempotencyRows); err != nil {
		t.Fatal(err)
	}
	if durable.Dossier.Status != domain.StatusFrozen || durable.Dossier.Version != 1 || len(durable.Credentials) != 0 || credentialRows != 0 || idempotencyRows != 0 {
		t.Fatalf("凭据子账本失败后主事务未回滚: status=%s version=%d snapshotCredentials=%d ledgerCredentials=%d idempotency=%d", durable.Dossier.Status, durable.Dossier.Version, len(durable.Credentials), credentialRows, idempotencyRows)
	}
}
