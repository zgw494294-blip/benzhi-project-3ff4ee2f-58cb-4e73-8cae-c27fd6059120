package evidence

import (
	"fmt"
	"sort"
	"strings"

	"strata-proof/internal/domain"
)

type Analyzer struct{}

func NewAnalyzer() *Analyzer { return &Analyzer{} }

func (a *Analyzer) Analyze(dossierID, runID string, units []domain.StratigraphicUnit, relations []domain.StratigraphicRelation) []domain.ConsistencyFinding {
	var findings []domain.ConsistencyFinding
	findings = append(findings, missingEvidence(dossierID, runID, units, relations)...)
	findings = append(findings, isolatedUnits(dossierID, runID, units, relations)...)
	findings = append(findings, mutualExclusions(dossierID, runID, relations)...)
	findings = append(findings, cycles(dossierID, runID, units, relations)...)
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
		}
		if findings[i].RuleCode != findings[j].RuleCode {
			return findings[i].RuleCode < findings[j].RuleCode
		}
		return findings[i].ID < findings[j].ID
	})
	return findings
}

func findingID(rule string, units, relations []string) string {
	parts := append([]string{}, units...)
	parts = append(parts, relations...)
	sort.Strings(parts)
	return ShortDigest(rule + ":" + strings.Join(parts, ","))
}

func missingEvidence(dossierID, runID string, units []domain.StratigraphicUnit, relations []domain.StratigraphicRelation) []domain.ConsistencyFinding {
	var out []domain.ConsistencyFinding
	for _, unit := range units {
		missing := []string{}
		if strings.TrimSpace(unit.SoilTraits) == "" {
			missing = append(missing, "土质特征")
		}
		if len(unit.PhotoRefs) == 0 {
			missing = append(missing, "照片引用")
		}
		if len(missing) > 0 {
			refs := []string{unit.ID}
			out = append(out, domain.ConsistencyFinding{ID: findingID("EVIDENCE_UNIT", refs, nil), DossierID: dossierID, CheckRunID: runID, RuleCode: "EVIDENCE_UNIT", Severity: "warning", UnitRefs: refs, RelationRefs: []string{}, Message: fmt.Sprintf("单位 %s 缺少%s", unit.UnitCode, strings.Join(missing, "、")), Status: domain.FindingOpen})
		}
	}
	for _, relation := range relations {
		if strings.TrimSpace(relation.EvidenceNote) == "" {
			refs := []string{relation.ID}
			out = append(out, domain.ConsistencyFinding{ID: findingID("EVIDENCE_RELATION", nil, refs), DossierID: dossierID, CheckRunID: runID, RuleCode: "EVIDENCE_RELATION", Severity: "error", UnitRefs: []string{relation.SourceUnitID, relation.TargetUnitID}, RelationRefs: refs, Message: "关系缺少判定证据说明", Status: domain.FindingOpen})
		}
	}
	return out
}

func isolatedUnits(dossierID, runID string, units []domain.StratigraphicUnit, relations []domain.StratigraphicRelation) []domain.ConsistencyFinding {
	touched := map[string]bool{}
	for _, relation := range relations {
		touched[relation.SourceUnitID] = true
		touched[relation.TargetUnitID] = true
	}
	var out []domain.ConsistencyFinding
	if len(units) < 2 {
		return out
	}
	for _, unit := range units {
		if !touched[unit.ID] {
			refs := []string{unit.ID}
			out = append(out, domain.ConsistencyFinding{ID: findingID("ISOLATED_UNIT", refs, nil), DossierID: dossierID, CheckRunID: runID, RuleCode: "ISOLATED_UNIT", Severity: "warning", UnitRefs: refs, RelationRefs: []string{}, Message: fmt.Sprintf("单位 %s 尚未建立地层关系", unit.UnitCode), Status: domain.FindingOpen})
		}
	}
	return out
}

func mutualExclusions(dossierID, runID string, relations []domain.StratigraphicRelation) []domain.ConsistencyFinding {
	var out []domain.ConsistencyFinding
	for i := 0; i < len(relations); i++ {
		for j := i + 1; j < len(relations); j++ {
			a, b := relations[i], relations[j]
			samePair := (a.SourceUnitID == b.SourceUnitID && a.TargetUnitID == b.TargetUnitID) || (a.SourceUnitID == b.TargetUnitID && a.TargetUnitID == b.SourceUnitID)
			if !samePair {
				continue
			}
			conflict := a.RelationType == domain.RelationContemporary || b.RelationType == domain.RelationContemporary
			opposedDirection := a.SourceUnitID == b.TargetUnitID && a.TargetUnitID == b.SourceUnitID && a.RelationType != domain.RelationContemporary && b.RelationType != domain.RelationContemporary
			if conflict || opposedDirection {
				relationRefs := []string{a.ID, b.ID}
				sort.Strings(relationRefs)
				unitRefs := []string{a.SourceUnitID, a.TargetUnitID}
				sort.Strings(unitRefs)
				out = append(out, domain.ConsistencyFinding{ID: findingID("MUTUAL_EXCLUSION", unitRefs, relationRefs), DossierID: dossierID, CheckRunID: runID, RuleCode: "MUTUAL_EXCLUSION", Severity: "error", UnitRefs: unitRefs, RelationRefs: relationRefs, Message: "同一单位组合存在互斥的年代关系", Status: domain.FindingOpen})
			}
		}
	}
	return out
}

func cycles(dossierID, runID string, units []domain.StratigraphicUnit, relations []domain.StratigraphicRelation) []domain.ConsistencyFinding {
	adj := map[string][]domain.StratigraphicRelation{}
	for _, relation := range relations {
		if relation.RelationType != domain.RelationContemporary {
			adj[relation.SourceUnitID] = append(adj[relation.SourceUnitID], relation)
		}
	}
	for key := range adj {
		sort.Slice(adj[key], func(i, j int) bool { return adj[key][i].ID < adj[key][j].ID })
	}
	color := map[string]int{}
	stackUnits := []string{}
	stackRelations := []string{}
	seen := map[string]bool{}
	var out []domain.ConsistencyFinding
	var visit func(string)
	visit = func(node string) {
		color[node] = 1
		stackUnits = append(stackUnits, node)
		for _, edge := range adj[node] {
			if color[edge.TargetUnitID] == 0 {
				stackRelations = append(stackRelations, edge.ID)
				visit(edge.TargetUnitID)
				stackRelations = stackRelations[:len(stackRelations)-1]
			} else if color[edge.TargetUnitID] == 1 {
				start := 0
				for stackUnits[start] != edge.TargetUnitID {
					start++
				}
				unitRefs := append([]string{}, stackUnits[start:]...)
				relationRefs := append([]string{}, stackRelations[start:]...)
				relationRefs = append(relationRefs, edge.ID)
				sort.Strings(unitRefs)
				sort.Strings(relationRefs)
				id := findingID("DIRECTED_CYCLE", unitRefs, relationRefs)
				if !seen[id] {
					seen[id] = true
					out = append(out, domain.ConsistencyFinding{ID: id, DossierID: dossierID, CheckRunID: runID, RuleCode: "DIRECTED_CYCLE", Severity: "error", UnitRefs: unitRefs, RelationRefs: relationRefs, Message: "方向性地层关系形成环路", Status: domain.FindingOpen})
				}
			}
		}
		stackUnits = stackUnits[:len(stackUnits)-1]
		color[node] = 2
	}
	for _, unit := range units {
		if color[unit.ID] == 0 {
			visit(unit.ID)
		}
	}
	return out
}

func severityRank(value string) int {
	if value == "error" {
		return 0
	}
	return 1
}
