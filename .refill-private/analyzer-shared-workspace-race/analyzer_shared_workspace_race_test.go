package analyzer_shared_workspace_race_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"strata-proof/internal/application"
	"strata-proof/internal/domain"
	"strata-proof/internal/evidence"
	"strata-proof/internal/store"
)

type barrierRepository struct {
	store.Repository
	snapshot domain.Snapshot
	entered  chan struct{}
	release  chan struct{}
}

func (r *barrierRepository) Get(context.Context, string) (domain.Snapshot, error) {
	r.entered <- struct{}{}
	<-r.release
	return r.snapshot, nil
}

func (r *barrierRepository) IdempotentResult(context.Context, string) (json.RawMessage, bool, error) {
	return nil, false, nil
}

func (r *barrierRepository) Save(context.Context, domain.Snapshot, int64, string, json.RawMessage) error {
	return nil
}

func TestConcurrentAnalyzerCallsDoNotRaceOnWorkspace(t *testing.T) {
	snapshot := domain.Snapshot{
		Dossier: domain.TrenchDossier{ID: "dossier-race", LeadRecorder: "记录员", Status: domain.StatusDraft, Version: 1},
		Units: []domain.StratigraphicUnit{
			{ID: "unit-a", DossierID: "dossier-race", UnitCode: "A", SoilTraits: "灰土", PhotoRefs: []string{"photo://a"}, Revision: 1},
			{ID: "unit-b", DossierID: "dossier-race", UnitCode: "B", SoilTraits: "黄土", PhotoRefs: []string{"photo://b"}, Revision: 1},
		},
		Relations: []domain.StratigraphicRelation{
			{ID: "relation-ab", DossierID: "dossier-race", SourceUnitID: "unit-a", TargetUnitID: "unit-b", RelationType: domain.RelationOverlies, EvidenceNote: "剖面证据", Revision: 1},
		},
		Audit: []domain.AuditEntry{{DossierID: "dossier-race", Sequence: 1}},
	}
	repository := &barrierRepository{snapshot: snapshot, entered: make(chan struct{}, 4), release: make(chan struct{})}
	service := application.NewService(repository, evidence.NewAnalyzer(), evidence.NewCredentialIssuer("private-race-secret"))
	errorsFound := make(chan error, 4)

	for index := 0; index < 2; index++ {
		index := index
		go func() {
			_, err := service.RunCheck(context.Background(), application.VersionedCommand{
				DossierID:       snapshot.Dossier.ID,
				ExpectedVersion: snapshot.Dossier.Version,
				Actor:           "记录员",
				IdempotencyKey:  fmt.Sprintf("race-check-%02d", index),
			})
			errorsFound <- err
		}()
	}
	for index := 0; index < 2; index++ {
		go func() {
			_, err := service.TraceRelationPath(context.Background(), snapshot.Dossier.ID, "unit-a", "unit-b")
			errorsFound <- err
		}()
	}

	for index := 0; index < 4; index++ {
		<-repository.entered
	}
	close(repository.release)
	for index := 0; index < 4; index++ {
		if err := <-errorsFound; err != nil {
			t.Fatalf("并发调用意外返回错误: %v", err)
		}
	}
}
