package store

import (
	"context"
	"encoding/json"

	"strata-proof/internal/domain"
)

type Repository interface {
	Create(context.Context, domain.Snapshot, string, json.RawMessage) error
	Get(context.Context, string) (domain.Snapshot, error)
	List(context.Context, int, int) ([]domain.TrenchDossier, error)
	Save(context.Context, domain.Snapshot, int64, string, json.RawMessage) error
	IdempotentResult(context.Context, string) (json.RawMessage, bool, error)
	IdempotentResultFor(context.Context, string, string, string, string) (json.RawMessage, bool, error)
	FindCredential(context.Context, string) (domain.Snapshot, domain.ResearchCredential, error)
	UnitHistory(context.Context, string, string) ([]domain.StratigraphicUnit, error)
	RelationHistory(context.Context, string, string) ([]domain.StratigraphicRelation, error)
	AuditPage(context.Context, string, int, int64) ([]domain.AuditEntry, error)
	ValidateReferences(context.Context, domain.FrozenManifest) error
	Close() error
}
