// Package arbiter implements the "微生物复判及终局仲裁器" (microbial recheck
// and finality arbiter) component. It owns colony-read conversion, recheck
// generation isolation, independent-review eligibility, and the single-writer
// final decision among ready_to_pack, packed, sanitary_hold, and cancelled.
package arbiter

// FinalResult is one of the four terminal conclusions. It is written exactly
// once by the finalize competition.
type FinalResult string

const (
	FinalReadyToPack  FinalResult = "ready_to_pack"
	FinalPacked       FinalResult = "packed"
	FinalSanitaryHold FinalResult = "sanitary_hold"
	FinalCancelled    FinalResult = "cancelled"
)

// IsValid reports whether r is one of the four terminal conclusions.
func (r FinalResult) IsValid() bool {
	switch r {
	case FinalReadyToPack, FinalPacked, FinalSanitaryHold, FinalCancelled:
		return true
	}
	return false
}

// ColonyDerivation is the result of converting a culture-plate reading using
// dilution factor and sample volume. The derived colony count per 100 mL uses
// half-away-from-zero rounding to a fixed precision.
type ColonyDerivation struct {
	ColonyCFU      int64 `json:"colony_cfu"`
	Dilution       int64 `json:"dilution"`
	SampleVolumeML int64 `json:"sample_volume_ml"`
	CFUPer100ML    int64 `json:"cfu_per_100ml"`
}

// ReviewEligibility captures the two independent-review constraints: the
// reviewer must not overlap with feed-confirmation personnel, and the two
// reviews must come from different qualified people matching the current task
// generation.
type ReviewEligibility struct {
	Generation     int      `json:"generation"`
	FeedConfirmers []string `json:"feed_confirmers"`
	Reviewers      []string `json:"reviewers"`
}

// ReviewerEligible reports whether reviewerID may issue an independent review
// for the given generation: it must match the generation, must not have
// performed feed confirmation, and must be distinct from any already-recorded
// reviewer.
func (e ReviewEligibility) ReviewerEligible(generation int, reviewerID string, priorReviewers []string) bool {
	if generation != e.Generation {
		return false
	}
	for _, c := range e.FeedConfirmers {
		if c == reviewerID {
			return false
		}
	}
	for _, r := range priorReviewers {
		if r == reviewerID {
			return false
		}
	}
	return true
}
