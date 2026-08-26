package store

import (
	"context"
	"encoding/json"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/task"
)

// FeedConfirm records one of two distinct feed confirmations. It validates the
// base lot, seal, and cut line against the locked snapshot, enforces that the
// confirmer is a distinct qualified feed-confirmation operator, and advances the
// task to tanks_occupied once two distinct confirmers have signed off
// (acceptance #3).
func (s *Service) FeedConfirm(ctx context.Context, taskID string, req FeedConfirmRequest) (*FeedConfirmResponse, error) {
	reqHash := hashOf(req)

	var resp *FeedConfirmResponse
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
		if t.State != task.StatePendingFeed {
			return NewAppError(CodeInvalidState, "feed confirmation requires pending_feed state")
		}

		person, ok := s.Catalog().Person(catalog.PersonID(req.PersonID))
		if !ok || !person.HasRole(catalog.RoleFeedConfirm) {
			return NewAppError(CodePersonNotQualified, "person not qualified for feed confirmation")
		}
		for _, c := range t.FeedConfirmers {
			if c == req.PersonID {
				return NewAppError(CodeReviewDuplicate, "person already confirmed feed")
			}
		}
		if req.BaseLotID != t.BaseLotID {
			return NewAppError(CodeSealMismatch, "base lot mismatch")
		}
		if req.SealID != t.SealID {
			return NewAppError(CodeSealMismatch, "seal mismatch")
		}
		if req.CutLineID != t.CutLineID {
			return NewAppError(CodeSealMismatch, "cut line mismatch")
		}

		t.FeedConfirmers = append(t.FeedConfirmers, req.PersonID)
		if len(t.FeedConfirmers) >= 2 {
			t.State = task.StateTanksOccupied
		}
		t.UpdatedAtLogic, err = tx.NextLogicTime()
		if err != nil {
			return err
		}
		if err := tx.PutTask(t); err != nil {
			return err
		}
		if err := audit(tx, t, req.PersonID, "feed_confirmed", "", nil); err != nil {
			return err
		}

		resp = &FeedConfirmResponse{
			TaskID:      t.TaskID,
			Generation:  t.Generation,
			State:       t.State,
			ConfirmedBy: append([]string(nil), t.FeedConfirmers...),
		}
		return recordIdempotent(tx, req.OperationNo, taskID, t.Generation, "feed_confirm", reqHash, resp)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}
