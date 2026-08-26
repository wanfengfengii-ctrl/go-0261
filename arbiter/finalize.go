package arbiter

import (
	"leafwash-packaging-release-gate/evidence"
)

// Review verdicts recorded by independent reviewers.
const (
	ReviewApprove = "approve" // 合格
	ReviewHold    = "hold"    // 隔离
)

// FinalizeEligibility captures everything the finalize competition validates:
// the generation, the recorded reviews, the feed confirmers, and whether the
// task carries any anomaly that forces a sanitary hold.
type FinalizeEligibility struct {
	Generation     int
	FeedConfirmers []string
	Reviews        []evidence.ReviewDecision
	HasAnomaly     bool
}

// DistinctReviewers returns the unique reviewer ids among the recorded reviews.
func (e FinalizeEligibility) DistinctReviewers() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range e.Reviews {
		if !seen[r.ReviewerID] {
			seen[r.ReviewerID] = true
			out = append(out, r.ReviewerID)
		}
	}
	return out
}

// ReviewEligibilityErrors returns stable reason codes describing why finalize
// cannot proceed yet: missing reviews, overlap with feed confirmers, or a wrong
// generation. An empty list means the reviews are sufficient.
func (e FinalizeEligibility) ReviewEligibilityErrors() (reasons []string) {
	reviewers := e.DistinctReviewers()
	if len(reviewers) < 2 {
		reasons = append(reasons, "INSUFFICIENT_REVIEWS")
	}
	for _, r := range e.Reviews {
		if r.Generation != e.Generation {
			reasons = append(reasons, "REVIEW_GENERATION_MISMATCH")
		}
		for _, c := range e.FeedConfirmers {
			if r.ReviewerID == c {
				reasons = append(reasons, "REVIEW_FEED_OVERLAP")
			}
		}
	}
	_ = reviewers
	return reasons
}

// EvaluateResult determines the single final result for a requested decision,
// given the review verdicts and whether an anomaly is present. It returns the
// resolved FinalResult and any stable reason code for an invalid request.
func EvaluateResult(requested FinalResult, reviews []evidence.ReviewDecision, hasAnomaly bool) (FinalResult, string) {
	if !requested.IsValid() {
		return "", "INVALID_FINAL_RESULT"
	}
	if requested == FinalCancelled {
		return FinalCancelled, ""
	}

	allApprove := true
	for _, r := range reviews {
		if r.Decision != ReviewApprove {
			allApprove = false
		}
	}

	if hasAnomaly {
		if requested == FinalSanitaryHold {
			return FinalSanitaryHold, ""
		}
		return "", "ANOMALY_REQUIRES_HOLD"
	}
	if !allApprove {
		if requested == FinalSanitaryHold {
			return FinalSanitaryHold, ""
		}
		return "", "REVIEW_DENIED"
	}
	if requested == FinalReadyToPack || requested == FinalPacked {
		return requested, ""
	}
	if requested == FinalSanitaryHold {
		return FinalSanitaryHold, ""
	}
	return "", "INVALID_FINAL_RESULT"
}
