package task

import "testing"

func TestStateTerminal(t *testing.T) {
	terminals := []State{StateReadyToPack, StatePacked, StateSanitaryHold, StateCancelled}
	for _, s := range terminals {
		if !s.IsTerminal() {
			t.Errorf("state %q should be terminal", s)
		}
	}
	nonTerminals := []State{StatePendingLock, StatePendingFeed, StateTanksOccupied, StateCurveCollecting, StateATPCovering, StateMicroVerifying, StatePhysChemRetesting, StatePendingReview}
	for _, s := range nonTerminals {
		if s.IsTerminal() {
			t.Errorf("state %q should not be terminal", s)
		}
	}
}

func TestCanAdvanceLinear(t *testing.T) {
	pairs := [][2]State{
		{StatePendingLock, StatePendingFeed},
		{StatePendingFeed, StateTanksOccupied},
		{StateTanksOccupied, StateCurveCollecting},
		{StateCurveCollecting, StateATPCovering},
		{StateATPCovering, StateMicroVerifying},
		{StateMicroVerifying, StatePhysChemRetesting},
		{StatePhysChemRetesting, StatePendingReview},
	}
	for _, p := range pairs {
		if !CanAdvance(p[0], p[1]) {
			t.Errorf("expected %q -> %q to be allowed", p[0], p[1])
		}
	}
	// Skipping stages or moving backward must be rejected.
	if CanAdvance(StatePendingLock, StateTanksOccupied) {
		t.Error("skipping a stage must be rejected")
	}
	if CanAdvance(StatePendingFeed, StatePendingLock) {
		t.Error("backward transition must be rejected")
	}
}

func TestIsTerminalResult(t *testing.T) {
	for _, s := range []string{"ready_to_pack", "packed", "sanitary_hold", "cancelled"} {
		if !IsTerminalResult(s) {
			t.Errorf("%q should be a terminal result", s)
		}
	}
	if IsTerminalResult("pending_feed") {
		t.Error("pending_feed must not be a terminal result")
	}
}
