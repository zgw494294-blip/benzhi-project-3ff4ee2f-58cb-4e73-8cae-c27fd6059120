package audit_cursor_cache_scope_test

import (
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

func TestAuditPaginationCacheSeparatesCursorPages(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("打开私有测试数据库: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	const dossierID = "private-audit-cache-dossier"
	now := time.Date(2026, time.August, 27, 9, 0, 0, 0, time.UTC)
	audit := make([]domain.AuditEntry, 0, 5)
	for sequence := int64(1); sequence <= 5; sequence++ {
		audit = append(audit, domain.AuditEntry{
			DossierID:  dossierID,
			Sequence:   sequence,
			EventType:  domain.EventDossierUpdated,
			Actor:      "private-auditor",
			Summary:    "确定性审计记录",
			OccurredAt: now.Add(time.Duration(sequence) * time.Minute),
		})
	}
	snapshot := domain.Snapshot{
		Dossier: domain.TrenchDossier{
			ID:                   dossierID,
			TrenchCode:           "PRIVATE-AUDIT-CACHE",
			ExcavationArea:       "私有复现区",
			LeadRecorder:         "私有记录员",
			ChronologyHypothesis: "私有年代假设",
			Status:               domain.StatusDraft,
			Version:              5,
			CreatedAt:            now,
			UpdatedAt:            now.Add(5 * time.Minute),
		},
		Units:       []domain.StratigraphicUnit{},
		Relations:   []domain.StratigraphicRelation{},
		Findings:    []domain.ConsistencyFinding{},
		Credentials: []domain.ResearchCredential{},
		Audit:       audit,
	}
	if err := repository.Create(context.Background(), snapshot, "", nil); err != nil {
		t.Fatalf("写入私有审计夹具: %v", err)
	}

	service := application.NewService(repository, evidence.NewAnalyzer(), evidence.NewCredentialIssuer("private-audit-secret"))
	routes := httpui.NewHandler(service).Routes()
	first := requestAuditPage(t, routes, "/api/v1/dossiers/"+dossierID+"/audit?limit=2")
	if len(first) != 2 || first[0].Sequence != 5 || first[1].Sequence != 4 {
		t.Fatalf("第一页夹具无效: sequences=%v", auditSequences(first))
	}

	second := requestAuditPage(t, routes, "/api/v1/dossiers/"+dossierID+"/audit?limit=2&before=4")
	if len(second) != 2 || second[0].Sequence != 3 || second[1].Sequence != 2 {
		t.Fatalf("审计游标第二页复用了第一页: first=%v second=%v", auditSequences(first), auditSequences(second))
	}
}

func requestAuditPage(t *testing.T, handler http.Handler, target string) []domain.AuditEntry {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("读取审计页返回 HTTP %d: %s", response.Code, response.Body.String())
	}
	var entries []domain.AuditEntry
	if err := json.Unmarshal(response.Body.Bytes(), &entries); err != nil {
		t.Fatalf("解析审计页响应: %v", err)
	}
	return entries
}

func auditSequences(entries []domain.AuditEntry) []int64 {
	sequences := make([]int64, len(entries))
	for index, entry := range entries {
		sequences[index] = entry.Sequence
	}
	return sequences
}
