package store

import (
	"context"
	"testing"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/task"
)

// driveToPhyschemRetesting advances a task through an ATP over-limit anomaly and
// full microbiology coverage, landing in physchem_retesting.
func driveToPhyschemRetesting(t *testing.T, svc *Service, taskID string) {
	t.Helper()
	driveCurve(t, svc, taskID)
	// ATP-1 exceeds the 500 RLU limit, creating an anomaly.
	for _, pt := range []string{"ATP-1", "ATP-2", "ATP-3"} {
		rlu := int64(100)
		if pt == "ATP-1" {
			rlu = 900
		}
		if _, err := svc.SubmitATPSwab(context.Background(), taskID, ATPSwabRequest{
			OperationNo: "OP-" + taskID + "-atp-" + pt, PointCode: pt, RLU: rlu,
		}); err != nil {
			t.Fatalf("atp %s: %v", pt, err)
		}
	}
	for i, well := range []string{"WELL-A1", "WELL-A2", "WELL-B1"} {
		blind := []string{"BLIND-01", "BLIND-02", "BLIND-03"}[i]
		if _, err := svc.SubmitMicrobiology(context.Background(), taskID, MicrobiologyRequest{
			OperationNo: "OP-" + taskID + "-micro-" + well, BlindCode: blind, PlateWell: well,
			ColonyCFU: 0, Dilution: 1, SampleVolumeML: 1,
		}); err != nil {
			t.Fatalf("micro %s: %v", well, err)
		}
	}
	got, _ := svc.GetTask(context.Background(), taskID)
	if got.State != task.StatePhysChemRetesting {
		t.Fatalf("state = %q, want physchem_retesting", got.State)
	}
}

// TestRecheckSinglePerGeneration verifies that only one recheck may be created
// for the current generation.
func TestRecheckSinglePerGeneration(t *testing.T) {
	svc := newTestService(t)
	driveToPhyschemRetesting(t, svc, "TASK-R1")

	req := RecheckRequest{
		OperationNo: "OP-recheck", ReviewerID: "P-2001",
		Targets: arbiter.RecheckTargets{PointCodes: []string{"ATP-1"}},
	}
	resp, err := svc.SubmitRecheck(context.Background(), "TASK-R1", req)
	if err != nil {
		t.Fatalf("recheck: %v", err)
	}
	if resp.State != task.StatePendingReview {
		t.Fatalf("state = %q, want pending_review", resp.State)
	}

	if _, err := svc.SubmitRecheck(context.Background(), "TASK-R1", RecheckRequest{
		OperationNo: "OP-recheck-2", ReviewerID: "P-2002",
		Targets: arbiter.RecheckTargets{PointCodes: []string{"ATP-1"}},
	}); !IsAppError(err, CodeRecheckAlreadyExists) {
		t.Fatalf("expected CodeRecheckAlreadyExists, got %v", err)
	}
}

// TestRecheckTargetsMustCover verifies that a recheck must cover every affected
// coordinate (here the over-limit ATP point).
func TestRecheckTargetsMustCover(t *testing.T) {
	svc := newTestService(t)
	driveToPhyschemRetesting(t, svc, "TASK-R2")

	_, err := svc.SubmitRecheck(context.Background(), "TASK-R2", RecheckRequest{
		OperationNo: "OP-recheck-empty", ReviewerID: "P-2001",
		Targets: arbiter.RecheckTargets{},
	})
	if !IsAppError(err, CodeValidationFailed) {
		t.Fatalf("expected CodeValidationFailed, got %v", err)
	}

	if _, err := svc.SubmitRecheck(context.Background(), "TASK-R2", RecheckRequest{
		OperationNo: "OP-recheck-ok", ReviewerID: "P-2001",
		Targets: arbiter.RecheckTargets{PointCodes: []string{"ATP-1"}},
	}); err != nil {
		t.Fatalf("covering recheck: %v", err)
	}
}

// TestLateReadingCannotOverwrite verifies that a reading submitted after the
// task advanced past its stage is rejected and cannot overwrite evidence.
func TestLateReadingCannotOverwrite(t *testing.T) {
	svc := newTestService(t)
	driveATP(t, svc, "TASK-R3") // now in micro_verifying

	// A late curve sample must be rejected.
	late := curveReq("TASK-R3", 0, "1.10")
	late.OperationNo = "OP-late-curve"
	if _, err := svc.SubmitCurveSample(context.Background(), "TASK-R3", late); !IsAppError(err, CodeInvalidState) {
		t.Fatalf("expected CodeInvalidState for late curve sample, got %v", err)
	}
	// A late ATP swab must also be rejected.
	if _, err := svc.SubmitATPSwab(context.Background(), "TASK-R3", ATPSwabRequest{
		OperationNo: "OP-late-atp", PointCode: "ATP-1", RLU: 50,
	}); !IsAppError(err, CodeInvalidState) {
		t.Fatalf("expected CodeInvalidState for late ATP swab, got %v", err)
	}
}
