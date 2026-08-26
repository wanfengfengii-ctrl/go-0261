package catalog

import "testing"

func TestCatalogBaseLotSealMatch(t *testing.T) {
	lot := CatalogBaseLot{
		BaseLotID:    "BL-1",
		AllowedSeals: []SealID{"SEAL-001", "SEAL-002"},
		PrecoolLots:  []string{"PC-001"},
	}
	if !lot.AllowsSeal("SEAL-001") {
		t.Error("SEAL-001 should be allowed")
	}
	if lot.AllowsSeal("SEAL-999") {
		t.Error("SEAL-999 must not be allowed")
	}
	if !lot.HasPrecoolLot("PC-001") {
		t.Error("PC-001 should be a valid precool lot")
	}
	if lot.HasPrecoolLot("PC-999") {
		t.Error("PC-999 must not be a valid precool lot")
	}
}

func TestWashRuleRevisionStaleness(t *testing.T) {
	locked := WashRuleRevision{FormulaID: "F-1", Revision: 2}
	current := WashRuleRevision{FormulaID: "F-1", Revision: 3}
	if !locked.IsStaleRelativeTo(current) {
		t.Error("older revision must be stale relative to newer")
	}
	if current.IsStaleRelativeTo(locked) {
		t.Error("newer revision must not be stale relative to older")
	}
	different := WashRuleRevision{FormulaID: "F-2", Revision: 3}
	if !locked.IsStaleRelativeTo(different) {
		t.Error("different formula id must be stale")
	}
}

func TestQualifiedPersonRoles(t *testing.T) {
	p := QualifiedPerson{PersonID: "P-1", Roles: []Role{RoleFeedConfirm, RoleReviewer}}
	if !p.HasRole(RoleReviewer) {
		t.Error("person should hold reviewer role")
	}
	if p.HasRole("unknown") {
		t.Error("person must not hold unknown role")
	}
}
