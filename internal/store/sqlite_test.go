package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strata-proof/internal/domain"
)

func TestSQLiteVersionIdempotencyAndAudit(t *testing.T) {
	repo, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	snapshot := domain.Snapshot{Dossier: domain.TrenchDossier{ID: "d", TrenchCode: "T1", Status: domain.StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now}, Audit: []domain.AuditEntry{{DossierID: "d", Sequence: 1, OccurredAt: now}}}
	data, _ := json.Marshal(snapshot)
	if err := repo.Create(ctx, snapshot, "create-key", data); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo.IdempotentResult(ctx, "create-key"); err != nil || !ok {
		t.Fatalf("幂等结果未保存: %v", err)
	}
	snapshot.Dossier.Version = 2
	snapshot.Audit = append(snapshot.Audit, domain.AuditEntry{DossierID: "d", Sequence: 2, OccurredAt: now})
	data, _ = json.Marshal(snapshot)
	if err := repo.Save(ctx, snapshot, 1, "update-key", data); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, snapshot, 1, "stale-key", data); domain.ErrorCodeOf(err) != domain.CodeConflict {
		t.Fatalf("应拒绝陈旧版本: %v", err)
	}
	loaded, err := repo.Get(ctx, "d")
	if err != nil || loaded.Dossier.Version != 2 || len(loaded.Audit) != 2 {
		t.Fatalf("恢复案卷失败: %#v %v", loaded, err)
	}
}
