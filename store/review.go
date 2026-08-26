package store

import (
	"context"
	"encoding/json"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/task"
)

// SubmitReview records one independent review decision. The reviewer must be a
// qualified reviewer distinct from every feed confirmer and from any previously
// recorded reviewer, matching the current task generation (domain rule #8).
func (s *Service) SubmitReview(ctx context.Context, taskID string, req ReviewRequest) (*ReviewResponse, error) {
	reqHash := hashOf(req)

	var resp *ReviewResponse
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
		if t.State != task.StatePendingReview {
			return NewAppError(CodeInvalidState, "review requires pending_review state")
		}
		if req.Decision != arbiter.ReviewApprove && req.Decision != arbiter.ReviewHold {
			return NewAppError(CodeValidationFailed, "INVALID_REVIEW_DECISION")
		}

		person, ok := s.Catalog().Person(catalog.PersonID(req.ReviewerID))
		if !ok || !person.HasRole(catalog.RoleReviewer) {
			return NewAppError(CodePersonNotQualified, "reviewer not qualified")
		}
		for _, c := range t.FeedConfirmers {
			if c == req.ReviewerID {
				return NewAppError(CodeReviewOverlap, "reviewer overlaps with feed confirmer")
			}
		}
		reviews, _ := tx.Reviews(taskID, t.Generation)
		for _, r := range reviews {
			if r.ReviewerID == req.ReviewerID {
				return NewAppError(CodeReviewDuplicate, "reviewer already recorded a decision")
			}
		}

		now, err := tx.NextLogicTime()
		if err != nil {
			return err
		}
		review := evidence.ReviewDecision{
			ReviewID:       newID("rev"),
			TaskID:         taskID,
			Generation:     t.Generation,
			ReviewerID:     req.ReviewerID,
			Decision:       req.Decision,
			ReasonCode:     req.ReasonCode,
			CreatedAtLogic: now,
		}
		if err := tx.PutReview(review); err != nil {
			return err
		}

		reviews = append(reviews, review)
		if err := audit(tx, t, req.ReviewerID, "review", req.Decision, nil); err != nil {
			return err
		}

		resp = &ReviewResponse{
			TaskID:     taskID,
			Generation: t.Generation,
			State:      t.State,
			Reviewers:  reviewerIDs(reviews),
		}
		return recordIdempotent(tx, req.OperationNo, taskID, t.Generation, "review", reqHash, resp)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func reviewerIDs(reviews []evidence.ReviewDecision) []string {
	var out []string
	for _, r := range reviews {
		out = append(out, r.ReviewerID)
	}
	return out
}
