// Package catalog implements the "叶菜原料与清洗规则目录" (leaf-vegetable
// raw-material and wash-rule catalog) component. It owns the fictional base
// lots, harvest seals, precool lots, formula revisions, detection thresholds,
// personnel qualifications, and ATP/plate-well templates referenced by a
// locked inspection task.
package catalog

// SealID identifies a harvest-basket tamper seal.
type SealID string

// BaseLotID identifies a grower base lot.
type BaseLotID string

// PersonID identifies a qualified QC operator.
type PersonID string

// CatalogBaseLot is the stable catalog record for a grower base lot.
type CatalogBaseLot struct {
	BaseLotID    BaseLotID `json:"base_lot_id"`
	CropName     string    `json:"crop_name"`
	FieldCode    string    `json:"field_code"`
	HarvestDate  string    `json:"harvest_date"`
	AllowedSeals []SealID  `json:"allowed_seals"`
	PrecoolLots  []string  `json:"precool_lots"`
	Active       bool      `json:"active"`
}

// AllowsSeal reports whether the seal is among the lot's allowed seals.
func (l CatalogBaseLot) AllowsSeal(seal SealID) bool {
	for _, s := range l.AllowedSeals {
		if s == seal {
			return true
		}
	}
	return false
}

// HasPrecoolLot reports whether the precool lot belongs to this base lot.
func (l CatalogBaseLot) HasPrecoolLot(precool string) bool {
	for _, p := range l.PrecoolLots {
		if p == precool {
			return true
		}
	}
	return false
}

// WashRuleRevision is a formula revision with fixed-point detection
// thresholds. All chlorine/ph/temperature/turbidity values are integers
// scaled by 100 (e.g. chlorine_min_x100 == 150 means 1.50 mg/L).
type WashRuleRevision struct {
	FormulaID            string `json:"formula_id"`
	Revision             int    `json:"revision"`
	SummaryHash          string `json:"summary_hash"`
	ChlorineMinX100      int64  `json:"chlorine_min_x100"`
	ChlorineSlopeMaxX100 int64  `json:"chlorine_slope_max_x100"`
	ORPMinMV             int64  `json:"orp_min_mv"`
	PHMinX100            int64  `json:"ph_min_x100"`
	PHMaxX100            int64  `json:"ph_max_x100"`
	TemperatureMaxX100   int64  `json:"temperature_max_x100"`
	TurbidityMaxX100     int64  `json:"turbidity_max_x100"`
	ATPMaxRLU            int64  `json:"atp_max_rlu"`
	ColonyMaxCFUX100ml   int64  `json:"colony_max_cfu_x100ml"`
	EffectiveFrom        int64  `json:"effective_from"`
}

// IsStaleRelativeTo reports whether r is superseded by current, meaning a
// task locked against r may no longer proceed once current is in effect.
func (r WashRuleRevision) IsStaleRelativeTo(current WashRuleRevision) bool {
	if r.FormulaID != current.FormulaID {
		return true
	}
	return current.Revision > r.Revision
}

// Role is a personnel role used for feed confirmation and independent review.
type Role string

const (
	RoleFeedConfirm Role = "feed_confirm"
	RoleReviewer    Role = "reviewer"
)

// QualifiedPerson is a QC operator with one or more granted roles.
type QualifiedPerson struct {
	PersonID PersonID `json:"person_id"`
	Roles    []Role   `json:"roles"`
}

// HasRole reports whether the person holds the given role.
func (p QualifiedPerson) HasRole(r Role) bool {
	for _, role := range p.Roles {
		if role == r {
			return true
		}
	}
	return false
}

// ATPPointTemplate is a fixed ATP swab point template referenced at lock time.
type ATPPointTemplate struct {
	PointCode string `json:"point_code"`
}

// PlateWellTemplate is a fixed culture-plate well template referenced at lock time.
type PlateWellTemplate struct {
	WellCode string `json:"well_code"`
}

// Catalog is the read-side catalog contract consumed by the task aggregate and
// the HTTP API. A production implementation is backed by the store; tests may
// provide an in-memory fixture.
type Catalog interface {
	BaseLot(id BaseLotID) (CatalogBaseLot, bool)
	WashRule(formulaID string, revision int) (WashRuleRevision, bool)
	LatestWashRule(formulaID string) (WashRuleRevision, bool)
	Person(id PersonID) (QualifiedPerson, bool)
	ATPPoint(code string) (ATPPointTemplate, bool)
	PlateWell(code string) (PlateWellTemplate, bool)
}

// ValidateLockInput checks every catalog rule that must hold at lock time and
// returns a sorted list of stable reason codes. An empty list means the input
// is valid against the catalog. The rule revision is checked for existence and
// staleness against the latest revision for the same formula.
func ValidateLockInput(c Catalog, lot CatalogBaseLot, seal SealID, precool string,
	formulaID string, revision int, atpPoints, plateWells, reviewers []string,
	reviewerPersons map[PersonID]QualifiedPerson) (reasons []string) {

	if !lot.Active {
		reasons = append(reasons, "INACTIVE_BASE_LOT")
	}
	if !lot.AllowsSeal(seal) {
		reasons = append(reasons, "SEAL_MISMATCH")
	}
	if !lot.HasPrecoolLot(precool) {
		reasons = append(reasons, "PRECOOL_MISMATCH")
	}

	if _, ok := c.WashRule(formulaID, revision); !ok {
		reasons = append(reasons, "UNKNOWN_REVISION")
	} else if latest, ok := c.LatestWashRule(formulaID); ok && revision < latest.Revision {
		reasons = append(reasons, "STALE_REVISION")
	}

	for _, code := range atpPoints {
		if _, ok := c.ATPPoint(code); !ok {
			reasons = append(reasons, "UNKNOWN_ATP_POINT:"+code)
		}
	}
	for _, code := range plateWells {
		if _, ok := c.PlateWell(code); !ok {
			reasons = append(reasons, "UNKNOWN_PLATE_WELL:"+code)
		}
	}

	seen := map[PersonID]bool{}
	for _, id := range reviewers {
		p, ok := reviewerPersons[PersonID(id)]
		if !ok {
			reasons = append(reasons, "UNKNOWN_REVIEWER:"+id)
			continue
		}
		if !p.HasRole(RoleReviewer) {
			reasons = append(reasons, "NOT_REVIEWER:"+id)
		}
		if seen[PersonID(id)] {
			reasons = append(reasons, "DUPLICATE_REVIEWER:"+id)
		}
		seen[PersonID(id)] = true
	}
	return reasons
}
