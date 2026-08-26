package store

import (
	"context"
	"testing"

	"leafwash-packaging-release-gate/task"
)

func TestModel_FinalizeReleasesResourcesForTerminalTasks(t *testing.T) {
	tests := []struct {
		name       string
		decision   string
		finalState task.State
	}{
		{name: "ready_to_pack", decision: "ready_to_pack", finalState: task.StateReadyToPack},
		{name: "packed", decision: "packed", finalState: task.StatePacked},
		{name: "sanitary_hold", decision: "sanitary_hold", finalState: task.StateSanitaryHold},
		{name: "cancelled", decision: "cancelled", finalState: task.StateCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc := newTestService(t)

			finishedTaskID := "TASK-finalized-" + tt.name
			driveToPendingReview(t, svc, finishedTaskID)
			approveReview(t, svc, finishedTaskID, "P-2001", "OP-"+tt.name+"-review-1")
			approveReview(t, svc, finishedTaskID, "P-2002", "OP-"+tt.name+"-review-2")

			finalReq := FinalizeRequest{OperationNo: "OP-" + tt.name + "-finalize", Decision: tt.decision}
			finalResp, err := svc.Finalize(ctx, finishedTaskID, finalReq)
			if err != nil {
				t.Fatalf("finalize: %v", err)
			}
			if finalResp.State != tt.finalState || finalResp.FinalResult != tt.decision || finalResp.FinalCredential == "" {
				t.Fatalf("finalize response = %+v, want state %q result %q with credential", finalResp, tt.finalState, tt.decision)
			}

			retryResp, err := svc.Finalize(ctx, finishedTaskID, finalReq)
			if err != nil {
				t.Fatalf("idempotent finalize retry: %v", err)
			}
			if retryResp.State != finalResp.State || retryResp.FinalResult != finalResp.FinalResult || retryResp.FinalCredential != finalResp.FinalCredential {
				t.Fatalf("idempotent retry = %+v, want prior terminal response %+v", retryResp, finalResp)
			}

			if _, err := svc.SubmitReview(ctx, finishedTaskID, ReviewRequest{
				OperationNo: "OP-" + tt.name + "-late-review",
				ReviewerID:  "P-2001",
				Decision:    "approve",
			}); !IsAppError(err, CodeTerminalState) {
				t.Fatalf("late write error = %v, want %s", err, CodeTerminalState)
			}

			reuseSpec := validLockSpec("TASK-reuse-" + tt.name)
			if _, err := svc.LockTask(ctx, LockRequest{LockSpec: reuseSpec}); err != nil {
				t.Fatalf("lock after terminal finalize should reuse released resources: %v", err)
			}

			contendingSpec := validLockSpec("TASK-contending-" + tt.name)
			if _, err := svc.LockTask(ctx, LockRequest{LockSpec: contendingSpec}); !IsAppError(err, CodeOccupied) {
				t.Fatalf("lock while replacement task is open error = %v, want %s", err, CodeOccupied)
			}
		})
	}
}
