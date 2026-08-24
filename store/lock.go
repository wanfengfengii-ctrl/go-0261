package store

import (
	"context"
	"sort"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/lease"
	"leafwash-packaging-release-gate/task"
)

// LockTask creates and locks an inspection task. It validates the catalog
// inputs (base lot, seal, precool lot, formula revision freshness, reviewer
// qualifications), builds the frozen snapshot, and atomically acquires the wash
// tank, plate-well, drain-slot, and blind-code occupancy (acceptance #1, #2).
func (s *Service) LockTask(ctx context.Context, req LockRequest) (*LockResponse, error) {
	spec := req.LockSpec

	// Structural invariants (sorted/unique sample times, unique blind codes).
	if reasons := spec.ValidateLockShape(); len(reasons) > 0 {
		return nil, NewAppError(CodeValidationFailed, reasons...)
	}

	cat := s.Catalog()
	lot, ok := cat.BaseLot(catalog.BaseLotID(spec.BaseLotID))
	if !ok {
		return nil, NewAppError(CodeValidationFailed, "UNKNOWN_BASE_LOT")
	}

	rule, ok := cat.WashRule(spec.FormulaID, spec.FormulaRevision)
	if !ok {
		return nil, NewAppError(CodeValidationFailed, "UNKNOWN_REVISION")
	}

	reviewerPersons := map[catalog.PersonID]catalog.QualifiedPerson{}
	for _, id := range spec.Reviewers {
		if p, ok := cat.Person(catalog.PersonID(id)); ok {
			reviewerPersons[catalog.PersonID(id)] = p
		}
	}

	if reasons := catalog.ValidateLockInput(cat, lot, catalog.SealID(spec.SealID),
		spec.PrecoolLot, spec.FormulaID, spec.FormulaRevision, spec.ATPPoints,
		spec.PlateWells, spec.Reviewers, reviewerPersons); len(reasons) > 0 {
		code := CodeValidationFailed
		if containsReason(reasons, "STALE_REVISION") {
			code = CodeStaleRevision
		} else if containsReason(reasons, "SEAL_MISMATCH") {
			code = CodeSealMismatch
		}
		return nil, NewAppError(code, reasons...)
	}

	snapshot := task.BuildSnapshot(spec, rule.SummaryHash)
	snapshotJSON, err := task.SnapshotJSON(snapshot)
	if err != nil {
		return nil, err
	}

	taskID := spec.TaskID
	if taskID == "" {
		taskID = "TASK-" + randID()
	}
	snapshot.TaskID = taskID
	snapshotJSON, _ = task.SnapshotJSON(snapshot)

	resources := lockResources(spec)

	var resp *LockResponse
	err = s.p.Tx(ctx, true, func(tx Tx) error {
		if err := tx.AcquireLeases(taskID, 1, resources); err != nil {
			return NewAppError(CodeOccupied, "one or more resources already occupied")
		}
		now, err := tx.NextLogicTime()
		if err != nil {
			return err
		}
		t := &task.InspectionTask{
			TaskID:             taskID,
			Generation:         1,
			State:              task.StatePendingFeed,
			LockedSnapshotJSON: snapshotJSON,
			BaseLotID:          spec.BaseLotID,
			SealID:             spec.SealID,
			PrecoolLot:         spec.PrecoolLot,
			CutLineID:          spec.CutLineID,
			WashTankID:         spec.WashTankID,
			FormulaID:          spec.FormulaID,
			FormulaRevision:    spec.FormulaRevision,
			CreatedAtLogic:     now,
			UpdatedAtLogic:     now,
		}
		if err := tx.PutTask(t); err != nil {
			return err
		}
		if err := audit(tx, t, "", "task_locked", "", map[string]any{"task_id": taskID}); err != nil {
			return err
		}
		leases, err := tx.HeldBy(taskID)
		if err != nil {
			return err
		}
		resp = &LockResponse{
			TaskID:     taskID,
			Generation: 1,
			State:      task.StatePendingFeed,
			Snapshot:   snapshot,
			Leases:     leases,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// lockResources builds the exclusive-occupancy resource list from a lock spec:
// the wash tank, each plate well, each drain slot, and each blind code.
func lockResources(spec task.LockSpec) []lease.Resource {
	var out []lease.Resource
	out = append(out, lease.Resource{Type: lease.ResourceWashTank, Key: spec.WashTankID})
	for _, w := range spec.PlateWells {
		out = append(out, lease.Resource{Type: lease.ResourcePlateWell, Key: w})
	}
	for _, d := range spec.DrainSlots {
		out = append(out, lease.Resource{Type: lease.ResourceDrainSlot, Key: d})
	}
	for _, b := range spec.BlindCodes {
		out = append(out, lease.Resource{Type: lease.ResourceBlindCode, Key: b})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func containsReason(reasons []string, target string) bool {
	for _, r := range reasons {
		if r == target {
			return true
		}
	}
	return false
}
