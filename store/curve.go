package store

import (
	"context"
	"encoding/json"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/task"
)

// SubmitCurveSample records one locked-time-point disinfection/physchem reading.
// Fixed-point fields are parsed with the deterministic fixed-point rules; wrong
// digit counts, negative values, duplicate times, and missing times are rejected.
// Out-of-range values are recorded and flag a physchem anomaly; a chlorine decay
// slope exceeding the rule maximum flags a chlorine-break anomaly (acceptance #4).
func (s *Service) SubmitCurveSample(ctx context.Context, taskID string, req CurveSampleRequest) (*CurveSampleResponse, error) {
	reqHash := hashOf(req)

	if req.AdapterFailure != "" {
		return nil, s.adapterRetry(ctx, taskID, req.Generation, evidence.AdapterChlorine, taskID, req.AdapterFailure, task.StateTanksOccupied, task.StateCurveCollecting)
	}

	var resp *CurveSampleResponse
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
		if t.State != task.StateTanksOccupied && t.State != task.StateCurveCollecting {
			return NewAppError(CodeInvalidState, "curve collection not started")
		}

		snapshot, err := t.Snapshot()
		if err != nil {
			return err
		}
		rule, ok := s.Catalog().WashRule(t.FormulaID, t.FormulaRevision)
		if !ok {
			return NewAppError(CodeNotFound, "formula revision not found")
		}

		// Parse fixed-point fields with the deterministic rules.
		chlorine, err := evidence.ParseFixed(req.ChlorineX100, 2)
		if err != nil {
			return fixedPointError(err)
		}
		ph, err := evidence.ParseFixed(req.PHX100, 2)
		if err != nil {
			return fixedPointError(err)
		}
		temperature, err := evidence.ParseFixed(req.TemperatureX100, 2)
		if err != nil {
			return fixedPointError(err)
		}
		turbidity, err := evidence.ParseFixed(req.TurbidityX100, 2)
		if err != nil {
			return fixedPointError(err)
		}

		// Negative and duplicate/missing-time rejections.
		for _, v := range []int64{chlorine, ph, temperature, turbidity, req.ORPMV} {
			if v < 0 {
				return NewAppError(CodeInvalidReading, "NEGATIVE_VALUE")
			}
		}
		if !snapshot.HasSampleTime(req.SampleTime) {
			return NewAppError(CodeMissingTime, "sample time not in locked schedule")
		}
		if existing, _ := tx.CoverageSamples(taskID, t.Generation); hasSampleAt(existing, req.SampleTime) {
			return NewAppError(CodeDuplicateTime, "sample time already recorded")
		}

		sample := evidence.CoverageSample{
			TaskID:          taskID,
			Generation:      t.Generation,
			SampleTime:      req.SampleTime,
			ChlorineX100:    chlorine,
			ORPMV:           req.ORPMV,
			PHX100:          ph,
			TemperatureX100: temperature,
			TurbidityX100:   turbidity,
			Valid:           true,
		}

		cr := evidence.CurveRule{
			ChlorineMinX100:      rule.ChlorineMinX100,
			ChlorineSlopeMaxX100: rule.ChlorineSlopeMaxX100,
			ORPMinMV:             rule.ORPMinMV,
			PHMinX100:            rule.PHMinX100,
			PHMaxX100:            rule.PHMaxX100,
			TemperatureMaxX100:   rule.TemperatureMaxX100,
			TurbidityMaxX100:     rule.TurbidityMaxX100,
		}
		outOfRange := evidence.ValidateCurveReading(cr, sample)
		if len(outOfRange) > 0 {
			t.AddAnomaly(string(arbiter.AnomalyPhyschemRange))
		}

		if err := tx.PutCoverageSample(sample); err != nil {
			return err
		}

		// Advance into curve collection on the first accepted sample.
		if t.State == task.StateTanksOccupied {
			t.State = task.StateCurveCollecting
		}

		samples, _ := tx.CoverageSamples(taskID, t.Generation)
		coverage := evidence.CoverageOver(snapshot.SampleTimes, samples)
		if coverage.Complete {
			if _, exceeds, err := evidence.AdjacentChlorineSlopes(snapshot.SampleTimes, samples, rule.ChlorineSlopeMaxX100); err == nil && exceeds {
				t.AddAnomaly(string(arbiter.AnomalyChlorineBreak))
			}
			t.State = task.StateATPCovering
		}

		t.UpdatedAtLogic, err = tx.NextLogicTime()
		if err != nil {
			return err
		}
		if err := tx.PutTask(t); err != nil {
			return err
		}
		if err := audit(tx, t, "", "curve_sample", "", map[string]any{"sample_time": req.SampleTime}); err != nil {
			return err
		}

		resp = &CurveSampleResponse{
			TaskID:     taskID,
			Generation: t.Generation,
			State:      t.State,
			Valid:      true,
			Coverage:   coverage,
			Anomalies:  append([]string(nil), t.Anomalies...),
		}
		return recordIdempotent(tx, req.OperationNo, taskID, t.Generation, "curve_sample", reqHash, resp)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// fixedPointError maps a fixed-point parse/derive error to a stable AppError.
func fixedPointError(err error) *AppError {
	switch err {
	case evidence.ErrTooManyDigits, evidence.ErrInvalidFormat:
		return NewAppError(CodeInvalidReading, "INVALID_FIXED_POINT_FORMAT")
	case evidence.ErrNegativeValue:
		return NewAppError(CodeInvalidReading, "NEGATIVE_VALUE")
	case evidence.ErrDivisionByZero:
		return NewAppError(CodeArithmeticError, "DIVISION_BY_ZERO")
	case evidence.ErrInt64Overflow:
		return NewAppError(CodeArithmeticError, "INT64_OVERFLOW")
	default:
		return NewAppError(CodeInvalidReading, err.Error())
	}
}

func hasSampleAt(samples []evidence.CoverageSample, ts int64) bool {
	for _, s := range samples {
		if s.SampleTime == ts {
			return true
		}
	}
	return false
}
