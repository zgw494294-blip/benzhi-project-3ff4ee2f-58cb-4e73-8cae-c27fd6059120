package snapshot_cache_rollback_leak_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"strata-proof/internal/application"
	"strata-proof/internal/domain"
	"strata-proof/internal/evidence"
	"strata-proof/internal/store"
)

var errForcedSave = errors.New("受控的 Save 提交失败")

type saveFailRepository struct {
	store.Repository
	failSave bool
}

func (r *saveFailRepository) Save(ctx context.Context, snapshot domain.Snapshot, expected int64, key string, response json.RawMessage) error {
	if r.failSave {
		return errForcedSave
	}
	return r.Repository.Save(ctx, snapshot, expected, key, response)
}

func TestFailedSaveCannotLeakThroughSnapshotCache(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "strata-proof.db")
	primaryStore, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = primaryStore.Close() })

	repository := &saveFailRepository{Repository: primaryStore}
	service := application.NewService(repository, evidence.NewAnalyzer(), evidence.NewCredentialIssuer("private-cache-secret"))
	ctx := context.Background()
	snapshot, err := service.CreateDossier(ctx, application.CreateDossierCommand{
		TrenchCode:           "PRIVATE-CACHE-ROLLBACK",
		ExcavationArea:       "私有复现区",
		LeadRecorder:         "记录员",
		ChronologyHypothesis: "测试年代",
		Actor:                "记录员",
		IdempotencyKey:       "private-cache-create",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.PutUnit(ctx, application.PutUnitCommand{
		DossierID:       snapshot.Dossier.ID,
		ExpectedVersion: snapshot.Dossier.Version,
		UnitCode:        "U1",
		UnitType:        "堆积",
		Description:     "已提交描述",
		TopElevation:    10,
		BottomElevation: 9,
		SoilTraits:      "灰土",
		PhotoRefs:       []string{"photo://committed"},
		Actor:           "记录员",
		IdempotencyKey:  "private-cache-unit-create",
	})
	if err != nil {
		t.Fatal(err)
	}

	unit := snapshot.Units[0]
	repository.failSave = true
	_, err = service.PutUnit(ctx, application.PutUnitCommand{
		DossierID:       snapshot.Dossier.ID,
		UnitID:          unit.ID,
		ExpectedVersion: snapshot.Dossier.Version,
		UnitCode:        unit.UnitCode,
		UnitType:        unit.UnitType,
		Description:     "未提交却泄漏的描述",
		TopElevation:    unit.TopElevation,
		BottomElevation: unit.BottomElevation,
		SoilTraits:      unit.SoilTraits,
		PhotoRefs:       unit.PhotoRefs,
		Actor:           "记录员",
		IdempotencyKey:  "private-cache-unit-failed",
	})
	if !errors.Is(err, errForcedSave) {
		t.Fatalf("未进入受控 Save 失败路径: %v", err)
	}

	cached, err := service.GetDossier(ctx, snapshot.Dossier.ID)
	if err != nil {
		t.Fatal(err)
	}
	truthStore, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = truthStore.Close() })
	durable, err := truthStore.Get(ctx, snapshot.Dossier.ID)
	if err != nil {
		t.Fatal(err)
	}

	if cached.Units[0].Description != durable.Units[0].Description || cached.Units[0].Revision != durable.Units[0].Revision {
		t.Fatalf("Save 失败后缓存泄漏未提交状态: cache=%q@%d durable=%q@%d", cached.Units[0].Description, cached.Units[0].Revision, durable.Units[0].Description, durable.Units[0].Revision)
	}
}
