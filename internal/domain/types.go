package domain

import "time"

type DossierStatus string

const (
	StatusDraft     DossierStatus = "draft"
	StatusRemediate DossierStatus = "remediation"
	StatusReview    DossierStatus = "pending_review"
	StatusReturned  DossierStatus = "returned"
	StatusFrozen    DossierStatus = "frozen"
	StatusIssued    DossierStatus = "issued"
)

type RelationType string

const (
	RelationOverlies     RelationType = "overlies"
	RelationCuts         RelationType = "cuts"
	RelationContemporary RelationType = "contemporary"
)

type TrenchDossier struct {
	ID                   string        `json:"id"`
	TrenchCode           string        `json:"trenchCode"`
	ExcavationArea       string        `json:"excavationArea"`
	LeadRecorder         string        `json:"leadRecorder"`
	ChronologyHypothesis string        `json:"chronologyHypothesis"`
	Status               DossierStatus `json:"status"`
	Version              int64         `json:"version"`
	CreatedAt            time.Time     `json:"createdAt"`
	UpdatedAt            time.Time     `json:"updatedAt"`
}

type StratigraphicUnit struct {
	ID              string    `json:"id"`
	DossierID       string    `json:"dossierId"`
	UnitCode        string    `json:"unitCode"`
	UnitType        string    `json:"unitType"`
	Description     string    `json:"description"`
	TopElevation    float64   `json:"topElevation"`
	BottomElevation float64   `json:"bottomElevation"`
	SoilTraits      string    `json:"soilTraits"`
	PhotoRefs       []string  `json:"photoRefs"`
	Revision        int       `json:"revision"`
	RecordedAt      time.Time `json:"recordedAt"`
}

type StratigraphicRelation struct {
	ID           string       `json:"id"`
	DossierID    string       `json:"dossierId"`
	SourceUnitID string       `json:"sourceUnitId"`
	TargetUnitID string       `json:"targetUnitId"`
	RelationType RelationType `json:"relationType"`
	EvidenceNote string       `json:"evidenceNote"`
	Revision     int          `json:"revision"`
	RecordedAt   time.Time    `json:"recordedAt"`
}

type RelationPathStep struct {
	SourceUnitID string       `json:"sourceUnitId"`
	TargetUnitID string       `json:"targetUnitId"`
	RelationID   string       `json:"relationId"`
	RelationType RelationType `json:"relationType"`
	Revision     int          `json:"revision"`
	EvidenceNote string       `json:"evidenceNote"`
}

type RelationPathResult struct {
	DossierID           string             `json:"dossierId"`
	SourceUnitID        string             `json:"sourceUnitId"`
	TargetUnitID        string             `json:"targetUnitId"`
	Classification      string             `json:"classification"`
	Units               []string           `json:"units"`
	Path                []RelationPathStep `json:"path"`
	ContemporaryUnits   []string           `json:"contemporaryUnits"`
	ContemporaryPath    []RelationPathStep `json:"contemporaryPath"`
	Conflict            bool               `json:"conflict"`
	ConflictRelationIDs []string           `json:"conflictRelationIds"`
}

type FindingStatus string

const (
	FindingOpen     FindingStatus = "open"
	FindingResolved FindingStatus = "resolved"
)

type ConsistencyFinding struct {
	ID                string         `json:"id"`
	DossierID         string         `json:"dossierId"`
	CheckRunID        string         `json:"checkRunId"`
	RuleCode          string         `json:"ruleCode"`
	Severity          string         `json:"severity"`
	UnitRefs          []string       `json:"unitRefs"`
	RelationRefs      []string       `json:"relationRefs"`
	Message           string         `json:"message"`
	ResolutionNote    string         `json:"resolutionNote,omitempty"`
	Status            FindingStatus  `json:"status"`
	ChangeType        string         `json:"changeType,omitempty"`
	UnitRevisions     map[string]int `json:"unitRevisions,omitempty"`
	RelationRevisions map[string]int `json:"relationRevisions,omitempty"`
	TriggerRevisions  []string       `json:"triggerRevisions,omitempty"`
}

type CheckBatch struct {
	ID              string               `json:"id"`
	DossierID       string               `json:"dossierId"`
	DossierVersion  int64                `json:"dossierVersion"`
	ExecutedAt      time.Time            `json:"executedAt"`
	ErrorCount      int                  `json:"errorCount"`
	WarningCount    int                  `json:"warningCount"`
	AddedCount      int                  `json:"addedCount"`
	PersistentCount int                  `json:"persistentCount"`
	ResolvedCount   int                  `json:"resolvedCount"`
	Findings        []ConsistencyFinding `json:"findings"`
	Resolved        []ConsistencyFinding `json:"resolved"`
}

type ReviewChecklist struct {
	UnitCompleteness bool `json:"unitCompleteness"`
	RelationEvidence bool `json:"relationEvidence"`
	FindingClosure   bool `json:"findingClosure"`
	ManifestPreview  bool `json:"manifestPreview"`
}

type RemediationItem struct {
	ID                 string     `json:"id"`
	TargetType         string     `json:"targetType"`
	TargetID           string     `json:"targetId"`
	CheckRunID         string     `json:"checkRunId,omitempty"`
	Reason             string     `json:"reason"`
	BaselineRevision   int        `json:"baselineRevision,omitempty"`
	ActualRevision     int        `json:"actualRevision,omitempty"`
	ResolutionNote     string     `json:"resolutionNote,omitempty"`
	VerifiedCheckRunID string     `json:"verifiedCheckRunId,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	ClosedAt           *time.Time `json:"closedAt,omitempty"`
}

type ReviewDecision struct {
	ID               string            `json:"id"`
	DossierID        string            `json:"dossierId"`
	Reviewer         string            `json:"reviewer"`
	Approved         bool              `json:"approved"`
	Note             string            `json:"note"`
	DecidedAt        time.Time         `json:"decidedAt"`
	Checklist        ReviewChecklist   `json:"checklist"`
	RemediationItems []RemediationItem `json:"remediationItems,omitempty"`
}

type FrozenManifest struct {
	DossierID          string            `json:"dossierId"`
	DossierVersion     int64             `json:"dossierVersion"`
	UnitRevisions      map[string]int    `json:"unitRevisions"`
	UnitDigests        map[string]string `json:"unitDigests"`
	RelationRevisions  map[string]int    `json:"relationRevisions"`
	RelationDigests    map[string]string `json:"relationDigests"`
	ResolvedFindingIDs []string          `json:"resolvedFindingIds"`
	ReviewDecisionID   string            `json:"reviewDecisionId"`
	CheckRunID         string            `json:"checkRunId"`
	Digest             string            `json:"digest"`
	FrozenAt           time.Time         `json:"frozenAt"`
	ReviewChecklist    ReviewChecklist   `json:"reviewChecklist"`
	RemediationItems   []RemediationItem `json:"remediationItems,omitempty"`
}

type RevisionLedger struct {
	DossierID         string                  `json:"dossierId"`
	UnitID            string                  `json:"unitId,omitempty"`
	RelationID        string                  `json:"relationId,omitempty"`
	UnitRevisions     []StratigraphicUnit     `json:"unitRevisions"`
	RelationRevisions []StratigraphicRelation `json:"relationRevisions"`
}

type ResearchCredential struct {
	CredentialID            string     `json:"credentialId"`
	DossierID               string     `json:"dossierId"`
	FrozenManifestDigest    string     `json:"frozenManifestDigest"`
	ReviewDecisionID        string     `json:"reviewDecisionId"`
	IssuedBy                string     `json:"issuedBy"`
	IssuedAt                time.Time  `json:"issuedAt"`
	Status                  string     `json:"status"`
	VerificationCode        string     `json:"verificationCode"`
	RevokedBy               string     `json:"revokedBy,omitempty"`
	RevokedAt               *time.Time `json:"revokedAt,omitempty"`
	RevocationReason        string     `json:"revocationReason,omitempty"`
	ReplacesCredentialID    string     `json:"replacesCredentialId,omitempty"`
	ReplacementCredentialID string     `json:"replacementCredentialId,omitempty"`
}

type AuditEntry struct {
	DossierID  string    `json:"dossierId"`
	Sequence   int64     `json:"sequence"`
	EventType  string    `json:"eventType"`
	Actor      string    `json:"actor"`
	Summary    string    `json:"summary"`
	OccurredAt time.Time `json:"occurredAt"`
}

type Snapshot struct {
	Dossier          TrenchDossier           `json:"dossier"`
	LastCheckRunID   string                  `json:"lastCheckRunId,omitempty"`
	Units            []StratigraphicUnit     `json:"units"`
	Relations        []StratigraphicRelation `json:"relations"`
	Findings         []ConsistencyFinding    `json:"findings"`
	CheckBatches     []CheckBatch            `json:"checkBatches"`
	RemediationItems []RemediationItem       `json:"remediationItems"`
	Review           *ReviewDecision         `json:"review,omitempty"`
	Manifest         *FrozenManifest         `json:"manifest,omitempty"`
	Credentials      []ResearchCredential    `json:"credentials"`
	Audit            []AuditEntry            `json:"audit"`
}
