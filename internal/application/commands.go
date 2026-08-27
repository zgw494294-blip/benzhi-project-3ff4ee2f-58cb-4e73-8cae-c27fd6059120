package application

import "strata-proof/internal/domain"

type CreateDossierCommand struct {
	TrenchCode           string `json:"trenchCode"`
	ExcavationArea       string `json:"excavationArea"`
	LeadRecorder         string `json:"leadRecorder"`
	ChronologyHypothesis string `json:"chronologyHypothesis"`
	Actor                string `json:"actor"`
	IdempotencyKey       string `json:"idempotencyKey"`
}

type UpdateDossierCommand struct {
	DossierID            string `json:"-"`
	ExpectedVersion      int64  `json:"expectedVersion"`
	ExcavationArea       string `json:"excavationArea"`
	LeadRecorder         string `json:"leadRecorder"`
	ChronologyHypothesis string `json:"chronologyHypothesis"`
	Actor                string `json:"actor"`
	IdempotencyKey       string `json:"idempotencyKey"`
}

type PutUnitCommand struct {
	DossierID       string   `json:"-"`
	UnitID          string   `json:"-"`
	ExpectedVersion int64    `json:"expectedVersion"`
	UnitCode        string   `json:"unitCode"`
	UnitType        string   `json:"unitType"`
	Description     string   `json:"description"`
	TopElevation    float64  `json:"topElevation"`
	BottomElevation float64  `json:"bottomElevation"`
	SoilTraits      string   `json:"soilTraits"`
	PhotoRefs       []string `json:"photoRefs"`
	Actor           string   `json:"actor"`
	IdempotencyKey  string   `json:"idempotencyKey"`
}

type BatchUnitRow struct {
	Row             int      `json:"row"`
	UnitCode        string   `json:"unitCode"`
	UnitType        string   `json:"unitType"`
	Description     string   `json:"description"`
	TopElevation    float64  `json:"topElevation"`
	BottomElevation float64  `json:"bottomElevation"`
	SoilTraits      string   `json:"soilTraits"`
	PhotoRefs       []string `json:"photoRefs"`
}

type BatchPutUnitsCommand struct {
	DossierID       string         `json:"-"`
	ExpectedVersion int64          `json:"expectedVersion"`
	Rows            []BatchUnitRow `json:"rows"`
	Actor           string         `json:"actor"`
	IdempotencyKey  string         `json:"idempotencyKey"`
}

type BatchUnitItem struct {
	Row      int    `json:"row"`
	UnitID   string `json:"unitId"`
	Revision int    `json:"revision"`
}

type BatchPutUnitsResult struct {
	Snapshot domain.Snapshot `json:"snapshot"`
	Items    []BatchUnitItem `json:"items"`
}

type PutRelationCommand struct {
	DossierID       string              `json:"-"`
	RelationID      string              `json:"-"`
	ExpectedVersion int64               `json:"expectedVersion"`
	SourceUnitID    string              `json:"sourceUnitId"`
	TargetUnitID    string              `json:"targetUnitId"`
	RelationType    domain.RelationType `json:"relationType"`
	EvidenceNote    string              `json:"evidenceNote"`
	Actor           string              `json:"actor"`
	IdempotencyKey  string              `json:"idempotencyKey"`
}

type VersionedCommand struct {
	DossierID       string `json:"-"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Actor           string `json:"actor"`
	IdempotencyKey  string `json:"idempotencyKey"`
}

type ResolveFindingCommand struct {
	VersionedCommand
	FindingID      string `json:"-"`
	ResolutionNote string `json:"resolutionNote"`
}

type ResolveRemediationCommand struct {
	VersionedCommand
	RemediationID  string `json:"-"`
	ResolutionNote string `json:"resolutionNote"`
}

type ReviewCommand struct {
	VersionedCommand
	Approved  bool                   `json:"approved"`
	Note      string                 `json:"note"`
	Reviewer  string                 `json:"reviewer"`
	Checklist domain.ReviewChecklist `json:"checklist"`
	Targets   []ReviewTarget         `json:"targets,omitempty"`
}

type ReviewTarget struct {
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
	Reason     string `json:"reason"`
}

type IssueCommand struct {
	VersionedCommand
	IssuedBy string `json:"issuedBy"`
}

type RevokeCredentialCommand struct {
	VersionedCommand
	CredentialID string `json:"-"`
	Reason       string `json:"reason"`
}

type ReissueCredentialCommand struct {
	VersionedCommand
	CredentialID string `json:"-"`
}

type VerificationResult struct {
	Credential              domain.ResearchCredential `json:"credential"`
	Dossier                 domain.TrenchDossier      `json:"dossier"`
	Manifest                *domain.FrozenManifest    `json:"manifest"`
	Valid                   bool                      `json:"valid"`
	Message                 string                    `json:"message"`
	Audit                   []domain.AuditEntry       `json:"audit"`
	ReplacementCredentialID string                    `json:"replacementCredentialId,omitempty"`
}
