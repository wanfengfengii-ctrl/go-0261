package store

import (
	"context"
	"testing"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/task"
)

// confirmFeed drives two distinct feed confirmations, moving the task into
// tanks_occupied (curve collection start).
func confirmFeed(t *testing.T, svc *Service, taskID string) {
	t.Helper()
	for i, p := range []string{"P-1001", "P-1002"} {
		_, err := svc.FeedConfirm(context.Background(), taskID, FeedConfirmRequest{
			OperationNo: "OP-" + taskID + "-feed-" + string(rune('a'+i)),
			PersonID:    p, BaseLotID: "BL-2026-001", SealID: "SEAL-001", CutLineID: "CUT-3",
		})
		if err != nil {
			t.Fatalf("feed confirm %s: %v", p, err)
		}
	}
}

func curveReq(taskID string, ts int64, chlorine string) CurveSampleRequest {
	return CurveSampleRequest{
		OperationNo:     "OP-" + taskID + "-curve-" + itoa(uint64(ts)),
		SampleTime:      ts,
		ChlorineX100:    chlorine,
		ORPMV:           700,
		PHX100:          "7.00",
		TemperatureX100: "5.00",
		TurbidityX100:   "0.50",
	}
}

// TestCurveFullCoverageAdvance verifies that covering every locked sample time
// advances the task into ATP coverage.
func TestCurveFullCoverageAdvance(t *testing.T) {
	svc := newTestService(t)
	lockValid(t, svc, "TASK-C1")
	confirmFeed(t, svc, "TASK-C1")

	for _, ts := range []int64{0, 300, 600} {
		resp, err := svc.SubmitCurveSample(context.Background(), "TASK-C1", curveReq("TASK-C1", ts, "1.50"))
		if err != nil {
			t.Fatalf("curve sample %d: %v", ts, err)
		}
		if !resp.Valid {
			t.Fatalf("sample %d marked invalid", ts)
		}
	}
	got, _ := svc.GetTask(context.Background(), "TASK-C1")
	if got.State != task.StateATPCovering {
		t.Fatalf("state after full coverage = %q, want atp_covering", got.State)
	}
}

// TestCurveMissingAndDuplicateTimeReject verifies that unknown and duplicate
// sample times are rejected.
func TestCurveMissingAndDuplicateTimeReject(t *testing.T) {
	svc := newTestService(t)
	lockValid(t, svc, "TASK-C2")
	confirmFeed(t, svc, "TASK-C2")

	// Unknown (not locked) sample time.
	_, err := svc.SubmitCurveSample(context.Background(), "TASK-C2", curveReq("TASK-C2", 999, "1.50"))
	if !IsAppError(err, CodeMissingTime) {
		t.Fatalf("expected CodeMissingTime, got %v", err)
	}
	// Valid first submission, then a duplicate.
	if _, err := svc.SubmitCurveSample(context.Background(), "TASK-C2", curveReq("TASK-C2", 0, "1.50")); err != nil {
		t.Fatalf("first sample: %v", err)
	}
	dup := curveReq("TASK-C2", 0, "1.60")
	dup.OperationNo = "OP-dup"
	if _, err := svc.SubmitCurveSample(context.Background(), "TASK-C2", dup); !IsAppError(err, CodeDuplicateTime) {
		t.Fatalf("expected CodeDuplicateTime, got %v", err)
	}
}

// TestChlorineSlopeBoundary verifies that a chlorine decay slope exceeding the
// rule maximum triggers a chlorine-break anomaly.
func TestChlorineSlopeBoundary(t *testing.T) {
	svc := newTestService(t)
	lockValid(t, svc, "TASK-C3")
	confirmFeed(t, svc, "TASK-C3")

	// Chlorine 1.50 -> 1.00 -> 0.50: delta 50 each over 300 units => slope 17 > 12.
	for i, ts := range []int64{0, 300, 600} {
		chlorine := []string{"1.50", "1.00", "0.50"}[i]
		if _, err := svc.SubmitCurveSample(context.Background(), "TASK-C3", curveReq("TASK-C3", ts, chlorine)); err != nil {
			t.Fatalf("curve sample %d: %v", ts, err)
		}
	}
	got, _ := svc.GetTask(context.Background(), "TASK-C3")
	if !got.HasAnomaly(string(arbiter.AnomalyChlorineBreak)) {
		t.Fatalf("expected chlorine-break anomaly, got %v", got.Anomalies)
	}
}
