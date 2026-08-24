package arbiter

import "testing"

func TestFinalResultValid(t *testing.T) {
	for _, r := range []FinalResult{FinalReadyToPack, FinalPacked, FinalSanitaryHold, FinalCancelled} {
		if !r.IsValid() {
			t.Errorf("%q should be a valid final result", r)
		}
	}
	if (FinalResult("pending")).IsValid() {
		t.Error("pending must not be a valid final result")
	}
}

func TestReviewerEligible(t *testing.T) {
	e := ReviewEligibility{Generation: 1, FeedConfirmers: []string{"P-1001"}}
	if !e.ReviewerEligible(1, "P-2001", nil) {
		t.Error("independent reviewer should be eligible")
	}
	if e.ReviewerEligible(1, "P-1001", nil) {
		t.Error("feed confirmer must not be eligible as reviewer")
	}
	if e.ReviewerEligible(2, "P-2001", nil) {
		t.Error("wrong generation must not be eligible")
	}
	if e.ReviewerEligible(1, "P-2001", []string{"P-2001"}) {
		t.Error("already-reviewed person must not review again")
	}
}
