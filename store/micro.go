package store

import (
	"context"
	"encoding/json"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/task"
)

// SubmitMicrobiology reveals a sampling blind code and records a culture-plate
// reading with its dilution factor and sample volume. The derived colony count
// per 100 mL uses half-away-from-zero rounding and detects division-by-zero and
// int64 overflow; any arithmetic failure writes no derived evidence
// (acceptance #5). A positive colony count flags a suspected-positive anomaly.
func (s *Service) SubmitMicrobiology(ctx context.Context, taskID string, req MicrobiologyRequest) (*MicrobiologyResponse, error) {
	reqHash := hashOf(req)

	if req.AdapterFailure != "" {
		return nil, s.adapterRetry(ctx, taskID, req.Generation, evidence.AdapterIncubator, req.PlateWell, req.AdapterFailure, task.StateMicroVerifying)
	}

	var resp *MicrobiologyResponse
	err := s.p.Tx(ctx, true, func(tx Tx) error {
		body, found, err := lookupIdempotent(tx, req.OperationNo, reqHash)
		if err != nil {
			return err
		}
		if found {
			_ = json.Unmarshal(body, &resp)
			return nil
		}

		t, ok := tx.GetTask(taskID)
		if !ok {
			return NewAppError(CodeNotFound, "task not found")
		}
		if t.State.IsTerminal() {
			return NewAppError(CodeTerminalState, "task is in a terminal state")
		}
		if err := checkGeneration(req.Generation, t.Generation); err != nil {
			return err
		}
		if t.State != task.StateMicroVerifying {
			return NewAppError(CodeInvalidState, "microbiology requires micro_verifying state")
		}

		snapshot, err := t.Snapshot()
		if err != nil {
			return err
		}
		if !snapshot.HasBlindCode(req.BlindCode) {
			return NewAppError(CodeUnknownBlindCode, "blind code not in locked set")
		}
		if !snapshot.HasPlateWell(req.PlateWell) {
			return NewAppError(CodeInvalidReading, "UNKNOWN_PLATE_WELL")
		}

		// Blind codes are one-time mappings: reject a second reveal.
		evs, _ := tx.Evidence(taskID, t.Generation)
		for _, e := range evs {
			if e.Kind == evidence.EvidenceMicrobiology && e.BlindCode == req.BlindCode {
				return NewAppError(CodeBlindCodeDuplicate, "blind code already revealed")
			}
		}

		reading := arbiter.ColonyReading{
			ColonyCFU:      req.ColonyCFU,
			Dilution:       req.Dilution,
			SampleVolumeML: req.SampleVolumeML,
		}
		if reasons := arbiter.ValidateColonyReading(reading); len(reasons) > 0 {
			return NewAppError(CodeInvalidReading, reasons...)
		}

		derived, err := arbiter.DeriveCFUPer100ML(req.ColonyCFU, req.Dilution, req.SampleVolumeML)
		if err != nil {
			return NewAppError(CodeArithmeticError, mapArithReason(err))
		}

		rule, ok := s.Catalog().WashRule(t.FormulaID, t.FormulaRevision)
		if !ok {
			return NewAppError(CodeNotFound, "formula revision not found")
		}
		overLimit := arbiter.ExceedsColonyLimit(derived, rule.ColonyMaxCFUX100ml)

		rawJSON, _ := json.Marshal(map[string]any{
			"colony_cfu": req.ColonyCFU, "dilution": req.Dilution, "sample_volume_ml": req.SampleVolumeML,
		})
		derivedJSON, _ := json.Marshal(map[string]any{"cfu_per_100ml": derived, "over_limit": overLimit})

		now, err := tx.NextLogicTime()
		if err != nil {
			return err
		}
		v := evidence.EvidenceVersion{
			EvidenceID:     newID("ev"),
			TaskID:         taskID,
			Generation:     t.Generation,
			Kind:           evidence.EvidenceMicrobiology,
			BlindCode:      req.BlindCode,
			PlateWell:      req.PlateWell,
			VersionNo:      1,
			RawJSON:        rawJSON,
			DerivedJSON:    derivedJSON,
			Accepted:       true,
			CreatedAtLogic: now,
		}
		if err := tx.PutEvidence(v); err != nil {
			return err
		}

		if overLimit {
			t.AddAnomaly(string(arbiter.AnomalyColonySusp))
		}

		covered := microCovered(snapshot, tx, taskID, t.Generation)
		if len(covered) >= len(snapshot.PlateWells) {
			if len(t.Anomalies) == 0 {
				t.State = task.StatePendingReview
			} else {
				t.State = task.StatePhysChemRetesting
			}
		}

		t.UpdatedAtLogic, err = tx.NextLogicTime()
		if err != nil {
			return err
		}
		if err := tx.PutTask(t); err != nil {
			return err
		}
		if err := audit(tx, t, "", "microbiology_reading", "", map[string]any{"blind_code": req.BlindCode, "plate_well": req.PlateWell}); err != nil {
			return err
		}

		resp = &MicrobiologyResponse{
			TaskID:      taskID,
			Generation:  t.Generation,
			State:       t.State,
			CFUPer100ML: derived,
			OverLimit:   overLimit,
			Covered:     covered,
			Anomalies:   append([]string(nil), t.Anomalies...),
		}
		return recordIdempotent(tx, req.OperationNo, taskID, t.Generation, "microbiology", reqHash, resp)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// microCovered returns the sorted list of plate wells with accepted microbiology
// evidence.
func microCovered(snapshot task.LockSnapshot, tx Tx, taskID string, generation int) []string {
	evs, _ := tx.Evidence(taskID, generation)
	covered := map[string]bool{}
	for _, e := range evs {
		if e.Kind == evidence.EvidenceMicrobiology && e.Accepted {
			covered[e.PlateWell] = true
		}
	}
	var out []string
	for _, w := range snapshot.PlateWells {
		if covered[w] {
			out = append(out, w)
		}
	}
	return out
}

// mapArithReason maps an arithmetic error to a stable reason code.
func mapArithReason(err error) string {
	switch err {
	case evidence.ErrDivisionByZero:
		return "DIVISION_BY_ZERO"
	case evidence.ErrInt64Overflow:
		return "INT64_OVERFLOW"
	case evidence.ErrNegativeValue:
		return "NEGATIVE_VALUE"
	default:
		return "ARITHMETIC_ERROR"
	}
}
