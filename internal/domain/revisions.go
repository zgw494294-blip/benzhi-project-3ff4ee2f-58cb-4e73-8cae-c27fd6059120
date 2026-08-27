package domain

import "time"

func NextUnitRevision(previous *StratigraphicUnit, proposed StratigraphicUnit, now time.Time) StratigraphicUnit {
	proposed.RecordedAt = now.UTC()
	proposed.Revision = 1
	if previous != nil {
		proposed.ID = previous.ID
		proposed.DossierID = previous.DossierID
		proposed.Revision = previous.Revision + 1
	}
	return proposed
}

func NextRelationRevision(previous *StratigraphicRelation, proposed StratigraphicRelation, now time.Time) StratigraphicRelation {
	proposed.RecordedAt = now.UTC()
	proposed.Revision = 1
	if previous != nil {
		proposed.ID = previous.ID
		proposed.DossierID = previous.DossierID
		proposed.Revision = previous.Revision + 1
	}
	return proposed
}

func UnitByID(units []StratigraphicUnit, id string) (*StratigraphicUnit, bool) {
	for i := range units {
		if units[i].ID == id {
			return &units[i], true
		}
	}
	return nil, false
}

func RelationByID(relations []StratigraphicRelation, id string) (*StratigraphicRelation, bool) {
	for i := range relations {
		if relations[i].ID == id {
			return &relations[i], true
		}
	}
	return nil, false
}
