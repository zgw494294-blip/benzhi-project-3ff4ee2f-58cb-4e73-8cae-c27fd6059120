package idempotencycrossdossier

import (
	"context"
	"testing"

	"strata-proof/internal/application"
	"strata-proof/internal/evidence"
	"strata-proof/internal/store"
)

func TestIdempotencyKeyCannotReturnAnotherDossier(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	service := application.NewService(repository, evidence.NewAnalyzer(), evidence.NewCredentialIssuer("test-secret"))
	ctx := context.Background()

	first, err := service.CreateDossier(ctx, application.CreateDossierCommand{
		TrenchCode: "IDEM-A", ExcavationArea: "一区", LeadRecorder: "甲",
		ChronologyHypothesis: "汉代", Actor: "甲", IdempotencyKey: "shared-key-001",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateDossier(ctx, application.CreateDossierCommand{
		TrenchCode: "IDEM-B", ExcavationArea: "二区", LeadRecorder: "乙",
		ChronologyHypothesis: "唐代", Actor: "乙", IdempotencyKey: "create-key-002",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.PutUnit(ctx, application.PutUnitCommand{
		DossierID: second.Dossier.ID, ExpectedVersion: second.Dossier.Version,
		UnitCode: "U-1", UnitType: "堆积", Description: "测试单位",
		TopElevation: 10, BottomElevation: 9, SoilTraits: "灰土",
		PhotoRefs: []string{"photo://1"}, Actor: "乙", IdempotencyKey: "shared-key-001",
	})
	if err != nil {
		return
	}
	if result.Dossier.ID != second.Dossier.ID {
		t.Fatalf("TestIdempotencyKeyCannotReturnAnotherDossier: 请求案卷 %s，却返回案卷 %s（首次案卷 %s）", second.Dossier.ID, result.Dossier.ID, first.Dossier.ID)
	}
}
