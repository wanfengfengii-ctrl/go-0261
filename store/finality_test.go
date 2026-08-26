package store

import (
	"context"
	"sync"
	"testing"

	"leafwash-packaging-release-gate/task"
)

// driveToPendingReview advances a clean task (no anomaly) through feed, curve,
// ATP, and microbiology, landing in pending_review.
func driveToPendingReview(t *testing.T, svc *Service, taskID string) {
	t.Helper()
	driveATP(t, svc, taskID)
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
	if got.State != task.StatePendingReview {
		t.Fatalf("state = %q, want pending_review", got.State)
	}
}

func approveReview(t *testing.T, svc *Service, taskID, reviewer, opNo string) {
	t.Helper()
	if _, err := svc.SubmitReview(context.Background(), taskID, ReviewRequest{
		OperationNo: opNo, ReviewerID: reviewer, Decision: "approve",
	}); err != nil {
		t.Fatalf("review %s: %v", reviewer, err)
	}
}

// TestFinalizeCompetition verifies that after two independent approvals the
// finalize competition produces exactly one terminal conclusion.
func TestFinalizeCompetition(t *testing.T) {
	svc := newTestService(t)
	driveToPendingReview(t, svc, "TASK-F1")
	approveReview(t, svc, "TASK-F1", "P-2001", "OP-r1")
	approveReview(t, svc, "TASK-F1", "P-2002", "OP-r2")

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, op := range []string{"OP-fin-1", "OP-fin-2"} {
		wg.Add(1)
		go func(op string) {
			defer wg.Done()
			_, err := svc.Finalize(context.Background(), "TASK-F1", FinalizeRequest{OperationNo: op, Decision: "ready_to_pack"})
			results <- err
		}(op)
	}
	wg.Wait()
	close(results)

	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !IsAppError(err, CodeFinalizeConflict) {
			t.Fatalf("unexpected finalize error: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly one successful finalize, got %d", success)
	}
	got, _ := svc.GetTask(context.Background(), "TASK-F1")
	if got.FinalResult != "ready_to_pack" || got.FinalCredential == "" {
		t.Fatalf("final result not recorded: %+v", got)
	}
}

// TestReviewerFeedOverlapRejected verifies that a reviewer who also performed
// feed confirmation is rejected.
func TestReviewerFeedOverlapRejected(t *testing.T) {
	svc := newTestService(t)
	driveToPendingReview(t, svc, "TASK-F2") // feed confirmers P-1001, P-1002

	_, err := svc.SubmitReview(context.Background(), "TASK-F2", ReviewRequest{
		OperationNo: "OP-overlap", ReviewerID: "P-1002", Decision: "approve",
	})
	if !IsAppError(err, CodeReviewOverlap) {
		t.Fatalf("expected CodeReviewOverlap, got %v", err)
	}
}

// TestTerminalStateRejectsWrites verifies that after finalization every write
// is rejected and the state is unchanged.
func TestTerminalStateRejectsWrites(t *testing.T) {
	svc := newTestService(t)
	driveToPendingReview(t, svc, "TASK-F3")
	approveReview(t, svc, "TASK-F3", "P-2001", "OP-r1")
	approveReview(t, svc, "TASK-F3", "P-2002", "OP-r2")
	if _, err := svc.Finalize(context.Background(), "TASK-F3", FinalizeRequest{OperationNo: "OP-fin", Decision: "sanitary_hold"}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	if _, err := svc.SubmitReview(context.Background(), "TASK-F3", ReviewRequest{
		OperationNo: "OP-late", ReviewerID: "P-2001", Decision: "approve",
	}); !IsAppError(err, CodeTerminalState) {
		t.Fatalf("expected CodeTerminalState, got %v", err)
	}
	if _, err := svc.Finalize(context.Background(), "TASK-F3", FinalizeRequest{OperationNo: "OP-fin2", Decision: "cancelled"}); !IsAppError(err, CodeFinalizeConflict) {
		t.Fatalf("expected CodeFinalizeConflict, got %v", err)
	}
	got, _ := svc.GetTask(context.Background(), "TASK-F3")
	if got.State != task.StateSanitaryHold {
		t.Fatalf("state changed after terminal rejection: %q", got.State)
	}
}
