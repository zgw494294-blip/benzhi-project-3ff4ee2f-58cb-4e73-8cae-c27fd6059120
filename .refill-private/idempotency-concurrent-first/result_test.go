package idempotencyconcurrentfirst

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"strata-proof/internal/application"
	"strata-proof/internal/evidence"
	"strata-proof/internal/store"
)

type coordinatedRepository struct {
	store.Repository
	mu      sync.Mutex
	arrived int
	release chan struct{}
}

func (r *coordinatedRepository) IdempotentResult(ctx context.Context, key string) (json.RawMessage, bool, error) {
	data, ok, err := r.Repository.IdempotentResult(ctx, key)
	if key != "concurrent-create-001" || ok || err != nil {
		return data, ok, err
	}
	r.mu.Lock()
	r.arrived++
	if r.arrived == 2 {
		close(r.release)
	}
	r.mu.Unlock()
	<-r.release
	return data, ok, err
}

func TestConcurrentFirstUseReturnsOneCommittedResult(t *testing.T) {
	base, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })
	repository := &coordinatedRepository{Repository: base, release: make(chan struct{})}
	service := application.NewService(repository, evidence.NewAnalyzer(), evidence.NewCredentialIssuer("test-secret"))
	command := application.CreateDossierCommand{
		TrenchCode: "IDEM-CONCURRENT", ExcavationArea: "并发区", LeadRecorder: "甲",
		ChronologyHypothesis: "汉代", Actor: "甲", IdempotencyKey: "concurrent-create-001",
	}
	type outcome struct {
		id  string
		err error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			snapshot, callErr := service.CreateDossier(context.Background(), command)
			results <- outcome{id: snapshot.Dossier.ID, err: callErr}
		}()
	}
	close(start)
	first := <-results
	second := <-results
	if first.err != nil || second.err != nil || first.id == "" || first.id != second.id {
		t.Fatalf("TestConcurrentFirstUseReturnsOneCommittedResult: 两个同键请求应共享一次提交，结果为 %#v 和 %#v", first, second)
	}
}
