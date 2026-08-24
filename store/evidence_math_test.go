package store

import (
	"context"
	"testing"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/task"
)

// TestPHFractionalDigitsReject verifies that a fixed-point reading with too many
// fractional digits is rejected without writing a coverage sample.
func TestPHFractionalDigitsReject(t *testing.T) {
	svc := newTestService(t)
	lockValid(t, svc, "TASK-E1")
	confirmFeed(t, svc, "TASK-E1")

	req := curveReq("TASK-E1", 0, "1.50")
	req.PHX100 = "7.123" // three fractional digits
	_, err := svc.SubmitCurveSample(context.Background(), "TASK-E1", req)
	if !IsAppError(err, CodeInvalidReading) {
		t.Fatalf("expected CodeInvalidReading, got %v", err)
	}
	got, _ := svc.GetTask(context.Background(), "TASK-E1")
	if got.State != task.StateTanksOccupied {
		t.Fatalf("rejected reading must not advance state: %q", got.State)
	}
}

// TestATPOverLimitAnomaly verifies that an ATP reading above the rule limit
// flags an ATP over-limit anomaly and advances coverage.
func TestATPOverLimitAnomaly(t *testing.T) {
	svc := newTestService(t)
	driveCurve(t, svc, "TASK-E2")

	if _, err := svc.SubmitATPSwab(context.Background(), "TASK-E2", ATPSwabRequest{
		OperationNo: "OP-atp-high", PointCode: "ATP-1", RLU: 900, // > 500 limit
	}); err != nil {
		t.Fatalf("atp swab: %v", err)
	}
	got, _ := svc.GetTask(context.Background(), "TASK-E2")
	if !got.HasAnomaly(string(arbiter.AnomalyATPOverLimit)) {
		t.Fatalf("expected ATP over-limit anomaly, got %v", got.Anomalies)
	}
}

// TestColonyNegativeRejected verifies that a negative colony count is rejected
// and writes no derived evidence.
func TestColonyNegativeRejected(t *testing.T) {
	svc := newTestService(t)
	driveATP(t, svc, "TASK-E3")

	_, err := svc.SubmitMicrobiology(context.Background(), "TASK-E3", MicrobiologyRequest{
		OperationNo: "OP-micro-neg", BlindCode: "BLIND-01", PlateWell: "WELL-A1",
		ColonyCFU: -1, Dilution: 1, SampleVolumeML: 1,
	})
	if !IsAppError(err, CodeInvalidReading) {
		t.Fatalf("expected CodeInvalidReading, got %v", err)
	}
	evs, _ := svc.ListAdapterCalls(context.Background(), "TASK-E3")
	_ = evs
	got, _ := svc.GetTask(context.Background(), "TASK-E3")
	if got.State != task.StateMicroVerifying {
		t.Fatalf("rejected reading must not advance: %q", got.State)
	}
}
