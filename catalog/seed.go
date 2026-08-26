package catalog

// Seed populates a Memory catalog with the fictional base lots, wash-rule
// revisions, qualified personnel, and ATP/plate-well templates that the
// single-node demo and its tests exercise. It is the single source of demo
// fixture data and is reachable from the executable entry point.
func Seed(c *Memory) {
	c.AddBaseLot(CatalogBaseLot{
		BaseLotID:    "BL-2026-001",
		CropName:     "生菜",
		FieldCode:    "FIELD-7",
		HarvestDate:  "2026-08-24",
		AllowedSeals: []SealID{"SEAL-001", "SEAL-002"},
		PrecoolLots:  []string{"PC-001", "PC-002"},
		Active:       true,
	})
	c.AddBaseLot(CatalogBaseLot{
		BaseLotID:    "BL-2026-002",
		CropName:     "菠菜",
		FieldCode:    "FIELD-3",
		HarvestDate:  "2026-08-24",
		AllowedSeals: []SealID{"SEAL-011"},
		PrecoolLots:  []string{"PC-011"},
		Active:       true,
	})

	c.AddWashRule(WashRuleRevision{
		FormulaID:            "F-100",
		Revision:             1,
		SummaryHash:          "f100-r1",
		ChlorineMinX100:      100,
		ChlorineSlopeMaxX100: 8,
		ORPMinMV:             600,
		PHMinX100:            600,
		PHMaxX100:            800,
		TemperatureMaxX100:   900,
		TurbidityMaxX100:     120,
		ATPMaxRLU:            500,
		ColonyMaxCFUX100ml:   1000,
		EffectiveFrom:        1,
	})
	c.AddWashRule(WashRuleRevision{
		FormulaID:            "F-100",
		Revision:             3,
		SummaryHash:          "f100-r3",
		ChlorineMinX100:      150,
		ChlorineSlopeMaxX100: 12,
		ORPMinMV:             650,
		PHMinX100:            620,
		PHMaxX100:            750,
		TemperatureMaxX100:   800,
		TurbidityMaxX100:     100,
		ATPMaxRLU:            500,
		ColonyMaxCFUX100ml:   1000,
		EffectiveFrom:        1,
	})

	c.AddPerson(QualifiedPerson{PersonID: "P-1001", Roles: []Role{RoleFeedConfirm}})
	c.AddPerson(QualifiedPerson{PersonID: "P-1002", Roles: []Role{RoleFeedConfirm, RoleReviewer}})
	c.AddPerson(QualifiedPerson{PersonID: "P-2001", Roles: []Role{RoleReviewer}})
	c.AddPerson(QualifiedPerson{PersonID: "P-2002", Roles: []Role{RoleReviewer}})

	c.AddATPPoint(ATPPointTemplate{PointCode: "ATP-1"})
	c.AddATPPoint(ATPPointTemplate{PointCode: "ATP-2"})
	c.AddATPPoint(ATPPointTemplate{PointCode: "ATP-3"})

	c.AddPlateWell(PlateWellTemplate{WellCode: "WELL-A1"})
	c.AddPlateWell(PlateWellTemplate{WellCode: "WELL-A2"})
	c.AddPlateWell(PlateWellTemplate{WellCode: "WELL-B1"})
}
