package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"strata-proof/internal/domain"
	"strata-proof/internal/evidence"
	"strata-proof/internal/store"
)

type Clock func() time.Time

type Service struct {
	repo             store.Repository
	analyzer         *evidence.Analyzer
	issuer           *evidence.CredentialIssuer
	policy           AuthorizationPolicy
	now              Clock
	verifyMu         sync.RWMutex
	revoked          map[string]VerificationResult
	localRevocations map[string]struct{}
}

func NewService(repo store.Repository, analyzer *evidence.Analyzer, issuer *evidence.CredentialIssuer) *Service {
	return &Service{
		repo:             repo,
		analyzer:         analyzer,
		issuer:           issuer,
		policy:           AuthorizationPolicy{},
		now:              time.Now,
		revoked:          make(map[string]VerificationResult),
		localRevocations: make(map[string]struct{}),
	}
}

func (s *Service) CreateDossier(ctx context.Context, cmd CreateDossierCommand) (domain.Snapshot, error) {
	if result, ok, err := s.cachedSnapshot(ctx, cmd.IdempotencyKey); ok || err != nil {
		return result, err
	}
	if err := requireCommand(cmd.Actor, cmd.IdempotencyKey); err != nil {
		return domain.Snapshot{}, err
	}
	now := s.now().UTC()
	dossier := domain.TrenchDossier{ID: newID("dos"), TrenchCode: strings.TrimSpace(cmd.TrenchCode), ExcavationArea: strings.TrimSpace(cmd.ExcavationArea), LeadRecorder: strings.TrimSpace(cmd.LeadRecorder), ChronologyHypothesis: strings.TrimSpace(cmd.ChronologyHypothesis), Status: domain.StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := domain.ValidateDossier(dossier); err != nil {
		return domain.Snapshot{}, err
	}
	created, _ := domain.NextAuditEntry(nil, dossier.ID, domain.EventDossierCreated, cmd.Actor, "创建探方案卷", now)
	snapshot := domain.Snapshot{Dossier: dossier, Units: []domain.StratigraphicUnit{}, Relations: []domain.StratigraphicRelation{}, Findings: []domain.ConsistencyFinding{}, Credentials: []domain.ResearchCredential{}, Audit: []domain.AuditEntry{created}}
	response, _ := json.Marshal(snapshot)
	if err := s.repo.Create(ctx, snapshot, cmd.IdempotencyKey, response); err != nil {
		return domain.Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Service) GetDossier(ctx context.Context, id string) (domain.Snapshot, error) {
	return s.repo.Get(ctx, id)
}
func (s *Service) ListDossiers(ctx context.Context, limit, offset int) ([]domain.TrenchDossier, error) {
	return s.repo.List(ctx, limit, offset)
}

func (s *Service) cachedSnapshot(ctx context.Context, key string) (domain.Snapshot, bool, error) {
	data, ok, err := s.repo.IdempotentResult(ctx, key)
	if err != nil || !ok {
		return domain.Snapshot{}, ok, err
	}
	var snapshot domain.Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return snapshot, true, err
	}
	return snapshot, true, nil
}

func (s *Service) mutate(ctx context.Context, id string, expected int64, actor, key, event, summary string, change func(*domain.Snapshot) error) (domain.Snapshot, error) {
	if result, ok, err := s.cachedSnapshot(ctx, key); ok || err != nil {
		return result, err
	}
	if err := requireCommand(actor, key); err != nil {
		return domain.Snapshot{}, err
	}
	snapshot, err := s.repo.Get(ctx, id)
	if err != nil {
		return snapshot, err
	}
	if snapshot.Dossier.Version != expected {
		return snapshot, domain.NewError(domain.CodeConflict, "expectedVersion=%d 与当前版本 %d 不符", expected, snapshot.Dossier.Version)
	}
	if err := change(&snapshot); err != nil {
		return snapshot, err
	}
	now := s.now().UTC()
	snapshot.Dossier.Version++
	snapshot.Dossier.UpdatedAt = now
	audit, err := domain.NextAuditEntry(snapshot.Audit, id, event, actor, summary, now)
	if err != nil {
		return domain.Snapshot{}, err
	}
	snapshot.Audit = append(snapshot.Audit, audit)
	response, _ := json.Marshal(snapshot)
	if err := s.repo.Save(ctx, snapshot, expected, key, response); err != nil {
		return domain.Snapshot{}, err
	}
	return snapshot, nil
}

func requireCommand(actor, key string) error {
	if strings.TrimSpace(actor) == "" {
		return domain.NewError(domain.CodeValidation, "actor 不能为空")
	}
	if len(strings.TrimSpace(key)) < 8 || len(key) > 120 {
		return domain.NewError(domain.CodeValidation, "idempotencyKey 长度必须为 8 到 120")
	}
	return nil
}

func newID(prefix string) string {
	var data [10]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(data[:])
}
