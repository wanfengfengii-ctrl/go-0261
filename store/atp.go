package store

import (
	"context"
	"encoding/json"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/task"
)

// SubmitATPSwab records an append-only ATP relative-light-unit reading for one
// point. Each submission creates a new version rather than overwriting an
// existing reading; an over-limit reading flags an ATP anomaly (acceptance #5).
func (s *Service) SubmitATPSwab(ctx context.Context, taskID string, req ATPSwabRequest) (*ATPSwabResponse, error) {
	reqHash := hashOf(req)

	if req.AdapterFailure != "" {
		return nil, s.adapterRetry(ctx, taskID, req.Generation, evidence.AdapterATP, req.PointCode, req.AdapterFailure, task.StateATPCovering)
	}

	var resp *ATPSwabResponse
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
		if t.State != task.StateATPCovering {
			return NewAppError(CodeInvalidState, "ATP submission requires atp_covering state")
		}

		snapshot, err := t.Snapshot()
		if err != nil {
			return err
		}
		if !snapshot.HasATPPoint(req.PointCode) {
			return NewAppError(CodeInvalidReading, "UNKNOWN_ATP_POINT")
		}
		if req.RLU < 0 {
			return NewAppError(CodeInvalidReading, "NEGATIVE_RLU")
		}

		rule, ok := s.Catalog().WashRule(t.FormulaID, t.FormulaRevision)
		if !ok {
			return NewAppError(CodeNotFound, "formula revision not found")
		}

		versionNo, err := tx.NextEvidenceVersion(taskID, evidence.EvidenceATP, req.PointCode)
		if err != nil {
			return err
		}
		overLimit := req.RLU > rule.ATPMaxRLU

		rawJSON, _ := json.Marshal(map[string]any{"rlu": req.RLU})
		derivedJSON, _ := json.Marshal(map[string]any{"over_limit": overLimit})

		now, err := tx.NextLogicTime()
		if err != nil {
			return err
		}
		v := evidence.EvidenceVersion{
			EvidenceID:     newID("ev"),
			TaskID:         taskID,
			Generation:     t.Generation,
			Kind:           evidence.EvidenceATP,
			PointCode:      req.PointCode,
			VersionNo:      versionNo,
			RawJSON:        rawJSON,
			DerivedJSON:    derivedJSON,
			Accepted:       true,
			CreatedAtLogic: now,
		}
		if err := tx.PutEvidence(v); err != nil {
			return err
		}

		if overLimit {
			t.AddAnomaly(string(arbiter.AnomalyATPOverLimit))
		}

		covered := atpCovered(snapshot, tx, taskID, t.Generation)
		evs, _ := tx.Evidence(taskID, t.Generation)
		if len(evs) >= len(snapshot.ATPPoints) {
			t.State = task.StateMicroVerifying
		}

		t.UpdatedAtLogic, err = tx.NextLogicTime()
		if err != nil {
			return err
		}
		if err := tx.PutTask(t); err != nil {
			return err
		}
		if err := audit(tx, t, "", "atp_swab", "", map[string]any{"point_code": req.PointCode, "version_no": versionNo}); err != nil {
			return err
		}

		resp = &ATPSwabResponse{
			TaskID:     taskID,
			Generation: t.Generation,
			State:      t.State,
			VersionNo:  versionNo,
			OverLimit:  overLimit,
			Covered:    covered,
			Anomalies:  append([]string(nil), t.Anomalies...),
		}
		return recordIdempotent(tx, req.OperationNo, taskID, t.Generation, "atp_swab", reqHash, resp)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// atpCovered returns the sorted list of ATP points that have at least one
// accepted evidence version.
func atpCovered(snapshot task.LockSnapshot, tx Tx, taskID string, generation int) []string {
	evs, _ := tx.Evidence(taskID, generation)
	covered := map[string]bool{}
	for _, e := range evs {
		if e.Kind == evidence.EvidenceATP && e.Accepted {
			covered[e.PointCode] = true
		}
	}
	var out []string
	for _, p := range snapshot.ATPPoints {
		if covered[p] {
			out = append(out, p)
		}
	}
	return out
}
