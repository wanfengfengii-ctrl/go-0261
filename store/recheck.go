package store

import (
	"context"
	"encoding/json"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/task"
)

// SubmitRecheck records the single current-generation recheck evidence. It
// enforces that only one recheck exists per generation, that the rechecker is a
// qualified independent reviewer, and that the submitted targets cover every
// affected sample time, blind code, point code, and plate well (acceptance #7).
func (s *Service) SubmitRecheck(ctx context.Context, taskID string, req RecheckRequest) (*RecheckResponse, error) {
	reqHash := hashOf(req)

	var resp *RecheckResponse
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
		if t.RecheckDone {
			return NewAppError(CodeRecheckAlreadyExists, "a recheck already exists for this generation")
		}
		if t.State != task.StatePhysChemRetesting {
			return NewAppError(CodeInvalidState, "recheck requires physchem_retesting state")
		}

		person, ok := s.Catalog().Person(catalog.PersonID(req.ReviewerID))
		if !ok || !person.HasRole(catalog.RoleReviewer) {
			return NewAppError(CodePersonNotQualified, "rechecker not a qualified reviewer")
		}
		for _, c := range t.FeedConfirmers {
			if c == req.ReviewerID {
				return NewAppError(CodeReviewOverlap, "rechecker overlaps with feed confirmer")
			}
		}

		required, err := s.deriveRequiredTargets(tx, t)
		if err != nil {
			return err
		}
		if !req.Targets.CoversRequired(required) {
			return NewAppError(CodeValidationFailed, "RECHECK_TARGETS_INCOMPLETE")
		}

		now, err := tx.NextLogicTime()
		if err != nil {
			return err
		}
		rawJSON, _ := json.Marshal(req.Targets)
		v := evidence.EvidenceVersion{
			EvidenceID:     newID("ev"),
			TaskID:         taskID,
			Generation:     t.Generation,
			Kind:           evidence.EvidenceRecheck,
			VersionNo:      1,
			RawJSON:        rawJSON,
			Accepted:       true,
			CreatedAtLogic: now,
		}
		if err := tx.PutEvidence(v); err != nil {
			return err
		}

		t.RecheckDone = true
		t.State = task.StatePendingReview
		t.UpdatedAtLogic, err = tx.NextLogicTime()
		if err != nil {
			return err
		}
		if err := tx.PutTask(t); err != nil {
			return err
		}
		if err := audit(tx, t, req.ReviewerID, "recheck", "", map[string]any{"targets": req.Targets}); err != nil {
			return err
		}

		resp = &RecheckResponse{
			TaskID:     taskID,
			Generation: t.Generation,
			State:      t.State,
		}
		return recordIdempotent(tx, req.OperationNo, taskID, t.Generation, "recheck", reqHash, resp)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// deriveRequiredTargets computes the affected coordinates that a recheck must
// cover, derived deterministically from the stored evidence and samples:
// over-limit ATP points, over-limit microbiology wells/blind codes, out-of-range
// curve sample times, and sample times involved in an exceeding chlorine slope.
func (s *Service) deriveRequiredTargets(tx Tx, t *task.InspectionTask) (arbiter.RecheckTargets, error) {
	var out arbiter.RecheckTargets

	snapshot, err := t.Snapshot()
	if err != nil {
		return out, err
	}
	rule, ok := s.Catalog().WashRule(t.FormulaID, t.FormulaRevision)
	if !ok {
		return out, NewAppError(CodeNotFound, "formula revision not found")
	}

	evs, _ := tx.Evidence(t.TaskID, t.Generation)
	for _, e := range evs {
		switch e.Kind {
		case evidence.EvidenceATP:
			var d struct {
				OverLimit bool `json:"over_limit"`
			}
			_ = json.Unmarshal(e.DerivedJSON, &d)
			if d.OverLimit {
				out.PointCodes = appendUnique(out.PointCodes, e.PointCode)
			}
		case evidence.EvidenceMicrobiology:
			var d struct {
				OverLimit bool `json:"over_limit"`
			}
			_ = json.Unmarshal(e.DerivedJSON, &d)
			if d.OverLimit {
				out.BlindCodes = appendUnique(out.BlindCodes, e.BlindCode)
				out.PlateWells = appendUnique(out.PlateWells, e.PlateWell)
			}
		}
	}

	samples, _ := tx.CoverageSamples(t.TaskID, t.Generation)
	if slopes, _, err := evidence.AdjacentChlorineSlopes(snapshot.SampleTimes, samples, rule.ChlorineSlopeMaxX100); err == nil {
		for _, sl := range slopes {
			if sl.ExceedsMax {
				out.SampleTimes = appendUniqueInt64(out.SampleTimes, sl.FromTime, sl.ToTime)
			}
		}
	}
	return out, nil
}

func appendUnique(xs []string, v string) []string {
	if v == "" {
		return xs
	}
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}

func appendUniqueInt64(xs []int64, vs ...int64) []int64 {
	for _, v := range vs {
		found := false
		for _, x := range xs {
			if x == v {
				found = true
				break
			}
		}
		if !found {
			xs = append(xs, v)
		}
	}
	return xs
}
