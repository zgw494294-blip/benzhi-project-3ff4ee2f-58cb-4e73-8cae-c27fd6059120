package evidence

import (
	"sort"

	"strata-proof/internal/domain"
)

type pathNode struct {
	unit  string
	steps []domain.RelationPathStep
	units []string
}

func TraceRelationPath(snapshot domain.Snapshot, sourceID, targetID string) (domain.RelationPathResult, error) {
	return NewAnalyzer().TraceRelationPath(snapshot, sourceID, targetID)
}

func (a *Analyzer) TraceRelationPath(snapshot domain.Snapshot, sourceID, targetID string) (domain.RelationPathResult, error) {
	known := map[string]bool{}
	for _, unitID := range a.unitIDs(snapshot.Units) {
		known[unitID] = true
	}
	if !known[sourceID] || !known[targetID] {
		return domain.RelationPathResult{}, domain.NewError(domain.CodeValidation, "起点或终点单位不属于当前案卷")
	}
	if sourceID == targetID {
		return domain.RelationPathResult{}, domain.NewError(domain.CodeValidation, "追溯起点和终点不能相同")
	}
	directional := make(map[string][]domain.StratigraphicRelation)
	contemporary := make(map[string][]domain.StratigraphicRelation)
	for _, relation := range snapshot.Relations {
		if relation.RelationType == domain.RelationContemporary {
			contemporary[relation.SourceUnitID] = append(contemporary[relation.SourceUnitID], relation)
			reversed := relation
			reversed.SourceUnitID, reversed.TargetUnitID = relation.TargetUnitID, relation.SourceUnitID
			contemporary[reversed.SourceUnitID] = append(contemporary[reversed.SourceUnitID], reversed)
		} else {
			directional[relation.SourceUnitID] = append(directional[relation.SourceUnitID], relation)
		}
	}
	sortGraph(directional)
	sortGraph(contemporary)
	steps, units, reachable := shortestPath(sourceID, targetID, directional)
	conSteps, conUnits, contemporaryReachable := shortestPath(sourceID, targetID, contemporary)
	reverseSteps, _, reverseReachable := shortestPath(targetID, sourceID, directional)
	result := domain.RelationPathResult{DossierID: snapshot.Dossier.ID, SourceUnitID: sourceID, TargetUnitID: targetID, Path: steps, Units: units, ContemporaryPath: conSteps, ContemporaryUnits: conUnits}
	if !reachable {
		result.Classification = "unreachable"
	} else if len(steps) == 1 {
		result.Classification = "direct"
	} else {
		result.Classification = "transitive_mixed"
	}
	conflicts := map[string]bool{}
	if reachable && reverseReachable {
		result.Conflict = true
		for _, step := range steps {
			conflicts[step.RelationID] = true
		}
		for _, step := range reverseSteps {
			conflicts[step.RelationID] = true
		}
	}
	if reachable && contemporaryReachable {
		result.Conflict = true
		for _, step := range steps {
			conflicts[step.RelationID] = true
		}
		for _, step := range conSteps {
			conflicts[step.RelationID] = true
		}
	}
	if reachable {
		for i := 0; i < len(units); i++ {
			for j := i + 1; j < len(units); j++ {
				linkedSteps, linkedUnits, linked := shortestPath(units[i], units[j], contemporary)
				if !linked {
					continue
				}
				result.Conflict = true
				if len(result.ContemporaryPath) == 0 {
					result.ContemporaryPath, result.ContemporaryUnits = linkedSteps, linkedUnits
				}
				for _, step := range steps[i:j] {
					conflicts[step.RelationID] = true
				}
				for _, step := range linkedSteps {
					conflicts[step.RelationID] = true
				}
			}
		}
	}
	for id := range conflicts {
		result.ConflictRelationIDs = append(result.ConflictRelationIDs, id)
	}
	sort.Strings(result.ConflictRelationIDs)
	return result, nil
}

func sortGraph(graph map[string][]domain.StratigraphicRelation) {
	for id := range graph {
		sort.Slice(graph[id], func(i, j int) bool {
			if graph[id][i].TargetUnitID != graph[id][j].TargetUnitID {
				return graph[id][i].TargetUnitID < graph[id][j].TargetUnitID
			}
			return graph[id][i].ID < graph[id][j].ID
		})
	}
}

func shortestPath(sourceID, targetID string, graph map[string][]domain.StratigraphicRelation) ([]domain.RelationPathStep, []string, bool) {
	queue := []pathNode{{unit: sourceID, units: []string{sourceID}}}
	visited := map[string]bool{sourceID: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, relation := range graph[current.unit] {
			if visited[relation.TargetUnitID] {
				continue
			}
			step := domain.RelationPathStep{SourceUnitID: relation.SourceUnitID, TargetUnitID: relation.TargetUnitID, RelationID: relation.ID, RelationType: relation.RelationType, Revision: relation.Revision, EvidenceNote: relation.EvidenceNote}
			nextSteps := append(append([]domain.RelationPathStep{}, current.steps...), step)
			nextUnits := append(append([]string{}, current.units...), relation.TargetUnitID)
			if relation.TargetUnitID == targetID {
				return nextSteps, nextUnits, true
			}
			visited[relation.TargetUnitID] = true
			queue = append(queue, pathNode{unit: relation.TargetUnitID, steps: nextSteps, units: nextUnits})
		}
	}
	return []domain.RelationPathStep{}, []string{}, false
}
