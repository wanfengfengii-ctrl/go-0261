package catalog

// Memory is a simple in-memory Catalog used by the executable entry point and
// tests. A production implementation loads the catalog from the store.
type Memory struct {
	lots       map[BaseLotID]CatalogBaseLot
	rules      map[string]map[int]WashRuleRevision
	persons    map[PersonID]QualifiedPerson
	atpPoints  map[string]ATPPointTemplate
	plateWells map[string]PlateWellTemplate
}

// NewMemory constructs an empty in-memory catalog.
func NewMemory() *Memory {
	return &Memory{
		lots:       make(map[BaseLotID]CatalogBaseLot),
		rules:      make(map[string]map[int]WashRuleRevision),
		persons:    make(map[PersonID]QualifiedPerson),
		atpPoints:  make(map[string]ATPPointTemplate),
		plateWells: make(map[string]PlateWellTemplate),
	}
}

// AddBaseLot registers a base lot.
func (m *Memory) AddBaseLot(lot CatalogBaseLot) { m.lots[lot.BaseLotID] = lot }

// AddWashRule registers a formula revision.
func (m *Memory) AddWashRule(r WashRuleRevision) {
	if m.rules[r.FormulaID] == nil {
		m.rules[r.FormulaID] = make(map[int]WashRuleRevision)
	}
	m.rules[r.FormulaID][r.Revision] = r
}

// AddPerson registers a qualified person.
func (m *Memory) AddPerson(p QualifiedPerson) { m.persons[p.PersonID] = p }

// AddATPPoint registers an ATP point template.
func (m *Memory) AddATPPoint(p ATPPointTemplate) { m.atpPoints[p.PointCode] = p }

// AddPlateWell registers a plate-well template.
func (m *Memory) AddPlateWell(w PlateWellTemplate) { m.plateWells[w.WellCode] = w }

// BaseLot returns the base lot by id.
func (m *Memory) BaseLot(id BaseLotID) (CatalogBaseLot, bool) {
	lot, ok := m.lots[id]
	return lot, ok
}

// WashRule returns a specific formula revision.
func (m *Memory) WashRule(formulaID string, revision int) (WashRuleRevision, bool) {
	r, ok := m.rules[formulaID][revision]
	return r, ok
}

// LatestWashRule returns the highest revision for a formula.
func (m *Memory) LatestWashRule(formulaID string) (WashRuleRevision, bool) {
	revs := m.rules[formulaID]
	if len(revs) == 0 {
		return WashRuleRevision{}, false
	}
	latest := 0
	for rev := range revs {
		if rev > latest {
			latest = rev
		}
	}
	r := revs[latest]
	return r, true
}

// Person returns the qualified person by id.
func (m *Memory) Person(id PersonID) (QualifiedPerson, bool) {
	p, ok := m.persons[id]
	return p, ok
}

// ATPPoint returns the ATP point template by code.
func (m *Memory) ATPPoint(code string) (ATPPointTemplate, bool) {
	p, ok := m.atpPoints[code]
	return p, ok
}

// PlateWell returns the plate-well template by code.
func (m *Memory) PlateWell(code string) (PlateWellTemplate, bool) {
	w, ok := m.plateWells[code]
	return w, ok
}
