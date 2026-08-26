package store

import (
	"context"
	"testing"

	"leafwash-packaging-release-gate/task"
)

func TestModel_ReviewSnapshotAuthorization(t *testing.T) {
	ctx := context.Background()

	advanceToPendingReview := func(t *testing.T, svc *Service, taskID string, lockedReviewers []string) {
		t.Helper()
		spec := validLockSpec(taskID)
		spec.Reviewers = append([]string(nil), lockedReviewers...)
		if _, err := svc.LockTask(ctx, LockRequest{LockSpec: spec}); err != nil {
			t.Fatalf("lock: %v", err)
		}
		confirmFeed(t, svc, taskID)
		for _, ts := range []int64{0, 300, 600} {
			if _, err := svc.SubmitCurveSample(ctx, taskID, curveReq(taskID, ts, "1.50")); err != nil {
				t.Fatalf("curve sample %d: %v", ts, err)
			}
		}
		for _, point := range []string{"ATP-1", "ATP-2", "ATP-3"} {
			if _, err := svc.SubmitATPSwab(ctx, taskID, ATPSwabRequest{
				OperationNo: "OP-" + taskID + "-atp-" + point,
				PointCode:   point,
				RLU:         100,
			}); err != nil {
				t.Fatalf("atp swab %s: %v", point, err)
			}
		}
		for i, well := range []string{"WELL-A1", "WELL-A2", "WELL-B1"} {
			blind := []string{"BLIND-01", "BLIND-02", "BLIND-03"}[i]
			if _, err := svc.SubmitMicrobiology(ctx, taskID, MicrobiologyRequest{
				OperationNo:    "OP-" + taskID + "-micro-" + well,
				BlindCode:      blind,
				PlateWell:      well,
				ColonyCFU:      0,
				Dilution:       1,
				SampleVolumeML: 1,
			}); err != nil {
				t.Fatalf("micro %s: %v", well, err)
			}
		}
		got, ok := svc.GetTask(ctx, taskID)
		if !ok {
			t.Fatal("task missing")
		}
		if got.State != task.StatePendingReview {
			t.Fatalf("state = %q, want pending_review", got.State)
		}
	}

	tests := []struct {
		name              string
		taskID            string
		lockedReviewers   []string
		rejectedReviewer  string
		acceptedReviewers []string
		wantFinalize      bool
	}{
		{
			name:              "rejects reviewer outside locked candidates and keeps finalize blocked",
			taskID:            "TASK-SNAP-REJECT",
			lockedReviewers:   []string{"P-1002", "P-2001"},
			rejectedReviewer:  "P-2002",
			acceptedReviewers: []string{"P-2001"},
			wantFinalize:      false,
		},
		{
			name:              "allows two locked independent candidates to finalize",
			taskID:            "TASK-SNAP-ALLOW",
			lockedReviewers:   []string{"P-2001", "P-2002"},
			acceptedReviewers: []string{"P-2001", "P-2002"},
			wantFinalize:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			advanceToPendingReview(t, svc, tc.taskID, tc.lockedReviewers)

			if tc.rejectedReviewer != "" {
				_, err := svc.SubmitReview(ctx, tc.taskID, ReviewRequest{
					OperationNo: "OP-" + tc.taskID + "-review-rejected",
					ReviewerID:  tc.rejectedReviewer,
					Decision:    "approve",
				})
				if err == nil {
					t.Fatalf("reviewer %s outside locked reviewers %v was accepted", tc.rejectedReviewer, tc.lockedReviewers)
				}
			}

			for _, reviewer := range tc.acceptedReviewers {
				resp, err := svc.SubmitReview(ctx, tc.taskID, ReviewRequest{
					OperationNo: "OP-" + tc.taskID + "-review-" + reviewer,
					ReviewerID:  reviewer,
					Decision:    "approve",
				})
				if err != nil {
					t.Fatalf("review %s: %v", reviewer, err)
				}
				if tc.rejectedReviewer != "" && (len(resp.Reviewers) != 1 || resp.Reviewers[0] != reviewer) {
					t.Fatalf("rejected reviewer was recorded: reviewers = %v", resp.Reviewers)
				}
			}

			resp, err := svc.Finalize(ctx, tc.taskID, FinalizeRequest{
				OperationNo: "OP-" + tc.taskID + "-finalize",
				Decision:    "ready_to_pack",
			})
			if tc.wantFinalize {
				if err != nil {
					t.Fatalf("finalize: %v", err)
				}
				if resp.State != task.StateReadyToPack || resp.FinalResult != "ready_to_pack" || resp.FinalCredential == "" {
					t.Fatalf("finalize response = %+v", resp)
				}
				return
			}
			if err == nil {
				t.Fatal("finalize succeeded after rejected reviewer should not count")
			}
			got, ok := svc.GetTask(ctx, tc.taskID)
			if !ok {
				t.Fatal("task missing after finalize attempt")
			}
			if got.State != task.StatePendingReview || got.FinalResult != "" || got.FinalCredential != "" {
				t.Fatalf("task changed after blocked finalize: %+v", got)
			}
		})
	}
}
