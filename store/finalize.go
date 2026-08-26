package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/task"
)

// finalResultState maps a terminal conclusion to its task state.
var finalResultState = map[arbiter.FinalResult]task.State{
	arbiter.FinalReadyToPack:  task.StateReadyToPack,
	arbiter.FinalPacked:       task.StatePacked,
	arbiter.FinalSanitaryHold: task.StateSanitaryHold,
	arbiter.FinalCancelled:    task.StateCancelled,
}

// Finalize runs the terminal competition and writes the unique conclusion. It
// requires two distinct qualified independent reviewers matching the current
// generation, then evaluates the requested result against any anomaly; exactly
// one finalize succeeds, and a terminal task rejects all further writes
// (acceptance #8).
func (s *Service) Finalize(ctx context.Context, taskID string, req FinalizeRequest) (*FinalizeResponse, error) {
	reqHash := hashOf(req)

	var resp *FinalizeResponse
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
			return NewAppError(CodeFinalizeConflict, "task already finalized")
		}
		if err := checkGeneration(req.Generation, t.Generation); err != nil {
			return err
		}
		if t.State != task.StatePendingReview {
			return NewAppError(CodeInvalidState, "finalize requires pending_review state")
		}

		reviews, err := tx.Reviews(taskID, t.Generation)
		if err != nil {
			return err
		}
		elig := arbiter.FinalizeEligibility{
			Generation:     t.Generation,
			FeedConfirmers: t.FeedConfirmers,
			Reviews:        reviews,
			HasAnomaly:     len(t.Anomalies) > 0,
		}
		if reasons := elig.ReviewEligibilityErrors(); len(reasons) > 0 {
			return NewAppError(CodeValidationFailed, reasons...)
		}

		result, reason := arbiter.EvaluateResult(arbiter.FinalResult(req.Decision), reviews, len(t.Anomalies) > 0)
		if reason != "" {
			return NewAppError(CodeValidationFailed, reason)
		}

		now, err := tx.NextLogicTime()
		if err != nil {
			return err
		}
		state := finalResultState[result]
		credential := finalCredential(taskID, t.Generation, string(result), now)

		t.State = state
		t.FinalResult = string(result)
		t.FinalCredential = credential
		t.UpdatedAtLogic = now
		if err := tx.PutTask(t); err != nil {
			return err
		}
		if err := audit(tx, t, "", "finalized", string(result), map[string]any{"credential": credential}); err != nil {
			return err
		}

		resp = &FinalizeResponse{
			TaskID:          taskID,
			Generation:      t.Generation,
			State:           state,
			FinalResult:     string(result),
			FinalCredential: credential,
		}
		return recordIdempotent(tx, req.OperationNo, taskID, t.Generation, "finalize", reqHash, resp)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// finalCredential derives a stable, unique terminal credential from the task
// identity, generation, result, and logic time.
func finalCredential(taskID string, generation int, result string, logicTime int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%d", taskID, generation, result, logicTime)))
	return "LW-" + hex.EncodeToString(sum[:])[:16]
}
