package unit_history_closed_cursor_test

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

func TestRepeatedUnitHistoryDoesNotReuseClosedCursor(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("打开私有测试数据库: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	const (
		dossierID = "private-history-dossier"
		unitID    = "private-history-unit"
	)
	now := time.Date(2026, time.August, 27, 11, 0, 0, 0, time.UTC)
	unit := domain.StratigraphicUnit{
		ID:              unitID,
		DossierID:       dossierID,
		UnitCode:        "SU-PRIVATE-01",
		UnitType:        "堆积",
		Description:     "私有修订账本",
		TopElevation:    101.2,
		BottomElevation: 100.8,
		SoilTraits:      "灰褐色粉砂土",
		PhotoRefs:       []string{"photo://private-history-01"},
		Revision:        1,
		RecordedAt:      now,
	}
	snapshot := domain.Snapshot{
		Dossier: domain.TrenchDossier{
			ID:                   dossierID,
			TrenchCode:           "PRIVATE-HISTORY",
			ExcavationArea:       "私有复现区",
			LeadRecorder:         "记录员甲",
			ChronologyHypothesis: "汉代",
			Status:               domain.StatusDraft,
			Version:              1,
			CreatedAt:            now,
			UpdatedAt:            now,
		},
		Units:       []domain.StratigraphicUnit{unit},
		Relations:   []domain.StratigraphicRelation{},
		Findings:    []domain.ConsistencyFinding{},
		Credentials: []domain.ResearchCredential{},
		Audit: []domain.AuditEntry{{
			DossierID:  dossierID,
			Sequence:   1,
			EventType:  domain.EventUnitRevised,
			Actor:      "记录员甲",
			Summary:    "登记地层单位",
			OccurredAt: now,
		}},
	}
	if err := repository.Create(context.Background(), snapshot, "", nil); err != nil {
		t.Fatalf("写入私有修订夹具: %v", err)
	}

	service := application.NewService(repository, evidence.NewAnalyzer(), evidence.NewCredentialIssuer("private-history-secret"))
	routes := httpui.NewHandler(service).Routes()
	target := "/api/v1/dossiers/" + dossierID + "/units/" + unitID + "/revisions"
	first := httptest.NewRecorder()
	routes.ServeHTTP(first, httptest.NewRequest(http.MethodGet, target, nil))
	if first.Code != http.StatusOK {
		t.Fatalf("首次读取修订账本返回 HTTP %d: %s", first.Code, first.Body.String())
	}
	var ledger domain.RevisionLedger
	if err := json.Unmarshal(first.Body.Bytes(), &ledger); err != nil {
		t.Fatalf("解析首次修订账本: %v", err)
	}
	if len(ledger.UnitRevisions) != 1 || ledger.UnitRevisions[0].ID != unitID {
		t.Fatalf("首次修订账本夹具无效: %#v", ledger.UnitRevisions)
	}

	second := httptest.NewRecorder()
	routes.ServeHTTP(second, httptest.NewRequest(http.MethodGet, target, nil))
	if second.Code != http.StatusOK {
		t.Fatalf("重复读取复用了已关闭 SQL 游标: HTTP %d body=%s", second.Code, second.Body.String())
	}
	ledger = domain.RevisionLedger{}
	if err := json.Unmarshal(second.Body.Bytes(), &ledger); err != nil {
		t.Fatalf("解析第二次修订账本: %v", err)
	}
	if len(ledger.UnitRevisions) != 1 || ledger.UnitRevisions[0].ID != unitID {
		t.Fatalf("第二次修订账本内容变化: %#v", ledger.UnitRevisions)
	}
}
