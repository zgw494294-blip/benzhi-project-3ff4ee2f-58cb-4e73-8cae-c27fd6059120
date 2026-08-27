package evidence

import (
	"encoding/json"
	"sort"
	"time"

	"strata-proof/internal/domain"
)

type manifestCanonical struct {
	DossierID        string                   `json:"dossierId"`
	DossierVersion   int64                    `json:"dossierVersion"`
	Units            []revisionRef            `json:"units"`
	Relations        []revisionRef            `json:"relations"`
	ResolvedFindings []string                 `json:"resolvedFindings"`
	ReviewDecisionID string                   `json:"reviewDecisionId"`
	CheckRunID       string                   `json:"checkRunId"`
	ReviewChecklist  domain.ReviewChecklist   `json:"reviewChecklist"`
	RemediationItems []domain.RemediationItem `json:"remediationItems,omitempty"`
}

type revisionRef struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
	Digest   string `json:"digest"`
}

func BuildManifest(snapshot domain.Snapshot, review domain.ReviewDecision, now time.Time) (domain.FrozenManifest, error) {
	canonical := manifestCanonical{DossierID: snapshot.Dossier.ID, DossierVersion: snapshot.Dossier.Version + 1, ReviewDecisionID: review.ID, CheckRunID: snapshot.LastCheckRunID, ReviewChecklist: review.Checklist, RemediationItems: append([]domain.RemediationItem{}, snapshot.RemediationItems...)}
	units := map[string]int{}
	unitDigests := map[string]string{}
	for _, unit := range snapshot.Units {
		units[unit.ID] = unit.Revision
		data, err := json.Marshal(unit)
		if err != nil {
			return domain.FrozenManifest{}, err
		}
		unitDigests[unit.ID] = Digest(data)
		canonical.Units = append(canonical.Units, revisionRef{unit.ID, unit.Revision, unitDigests[unit.ID]})
	}
	relations := map[string]int{}
	relationDigests := map[string]string{}
	for _, relation := range snapshot.Relations {
		relations[relation.ID] = relation.Revision
		data, err := json.Marshal(relation)
		if err != nil {
			return domain.FrozenManifest{}, err
		}
		relationDigests[relation.ID] = Digest(data)
		canonical.Relations = append(canonical.Relations, revisionRef{relation.ID, relation.Revision, relationDigests[relation.ID]})
	}
	for _, finding := range snapshot.Findings {
		if finding.Status == domain.FindingResolved {
			canonical.ResolvedFindings = append(canonical.ResolvedFindings, finding.ID)
		}
	}
	sort.Slice(canonical.Units, func(i, j int) bool { return canonical.Units[i].ID < canonical.Units[j].ID })
	sort.Slice(canonical.Relations, func(i, j int) bool { return canonical.Relations[i].ID < canonical.Relations[j].ID })
	sort.Strings(canonical.ResolvedFindings)
	data, err := json.Marshal(canonical)
	if err != nil {
		return domain.FrozenManifest{}, err
	}
	return domain.FrozenManifest{DossierID: snapshot.Dossier.ID, DossierVersion: canonical.DossierVersion, UnitRevisions: units, UnitDigests: unitDigests, RelationRevisions: relations, RelationDigests: relationDigests, ResolvedFindingIDs: canonical.ResolvedFindings, ReviewDecisionID: review.ID, CheckRunID: canonical.CheckRunID, Digest: Digest(data), FrozenAt: now.UTC(), ReviewChecklist: review.Checklist, RemediationItems: canonical.RemediationItems}, nil
}

func VerifyManifest(manifest domain.FrozenManifest, snapshot domain.Snapshot) bool {
	if manifest.DossierID != snapshot.Dossier.ID {
		return false
	}
	for _, unit := range snapshot.Units {
		if manifest.UnitRevisions[unit.ID] != unit.Revision {
			return false
		}
		data, err := json.Marshal(unit)
		if err != nil || manifest.UnitDigests[unit.ID] != Digest(data) {
			return false
		}
	}
	for _, relation := range snapshot.Relations {
		if manifest.RelationRevisions[relation.ID] != relation.Revision {
			return false
		}
		data, err := json.Marshal(relation)
		if err != nil || manifest.RelationDigests[relation.ID] != Digest(data) {
			return false
		}
	}
	if len(manifest.UnitRevisions) != len(snapshot.Units) || len(manifest.RelationRevisions) != len(snapshot.Relations) {
		return false
	}
	canonical := manifestCanonical{DossierID: manifest.DossierID, DossierVersion: manifest.DossierVersion, ResolvedFindings: append([]string(nil), manifest.ResolvedFindingIDs...), ReviewDecisionID: manifest.ReviewDecisionID, CheckRunID: manifest.CheckRunID, ReviewChecklist: manifest.ReviewChecklist, RemediationItems: append([]domain.RemediationItem{}, manifest.RemediationItems...)}
	for id, revision := range manifest.UnitRevisions {
		canonical.Units = append(canonical.Units, revisionRef{id, revision, manifest.UnitDigests[id]})
	}
	for id, revision := range manifest.RelationRevisions {
		canonical.Relations = append(canonical.Relations, revisionRef{id, revision, manifest.RelationDigests[id]})
	}
	sort.Slice(canonical.Units, func(i, j int) bool { return canonical.Units[i].ID < canonical.Units[j].ID })
	sort.Slice(canonical.Relations, func(i, j int) bool { return canonical.Relations[i].ID < canonical.Relations[j].ID })
	sort.Strings(canonical.ResolvedFindings)
	data, err := json.Marshal(canonical)
	return err == nil && Digest(data) == manifest.Digest
}
