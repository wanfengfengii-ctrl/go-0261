package store

import (
	"context"
	"testing"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/task"
)

func TestModel_ATPDuplicateVersionsRequireDistinctLockedPoints(t *testing.T) {
	cases := []struct {
		name          string
		taskID        string
		duplicateRLUs []int64
		wantAnomaly   bool
	}{
		{
			name:          "duplicate ATP-1 readings leave ATP-2 and ATP-3 uncovered",
			taskID:        "TASK-MODEL-ATP-DUP-NORMAL",
			duplicateRLUs: []int64{100, 110, 120},
		},
		{
			name:          "duplicate over-limit ATP-1 keeps anomaly without covering other points",
			taskID:        "TASK-MODEL-ATP-DUP-HIGH",
			duplicateRLUs: []int64{100, 900, 120},
			wantAnomaly:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc := newTestService(t)
			driveCurve(t, svc, tc.taskID)

			for i, rlu := range tc.duplicateRLUs {
				resp, err := svc.SubmitATPSwab(ctx, tc.taskID, ATPSwabRequest{
					OperationNo: "OP-" + tc.taskID + "-atp-1-v" + itoa(uint64(i+1)),
					PointCode:   "ATP-1",
					RLU:         rlu,
				})
				if err != nil {
					t.Fatalf("ATP-1 version %d: %v", i+1, err)
				}
				if resp.VersionNo != i+1 {
					t.Errorf("ATP-1 version number = %d, want %d", resp.VersionNo, i+1)
				}
				if resp.OverLimit != (rlu > 500) {
					t.Errorf("ATP-1 version %d over_limit = %t, want %t", i+1, resp.OverLimit, rlu > 500)
				}
				if resp.State != task.StateATPCovering {
					t.Errorf("state after ATP-1 version %d = %q, want %q until ATP-2 and ATP-3 are accepted", i+1, resp.State, task.StateATPCovering)
				}
				if len(resp.Covered) != 1 || resp.Covered[0] != "ATP-1" {
					t.Errorf("covered after ATP-1 version %d = %v, want [ATP-1]", i+1, resp.Covered)
				}
			}

			got, ok := svc.GetTask(ctx, tc.taskID)
			if !ok {
				t.Fatal("task not found")
			}
			if got.State != task.StateATPCovering {
				t.Errorf("state after duplicate ATP-1 versions = %q, want %q", got.State, task.StateATPCovering)
			}
			if got.HasAnomaly(string(arbiter.AnomalyATPOverLimit)) != tc.wantAnomaly {
				t.Errorf("ATP over-limit anomaly present = %t, want %t", got.HasAnomaly(string(arbiter.AnomalyATPOverLimit)), tc.wantAnomaly)
			}

			audits, err := svc.ListAudit(ctx, tc.taskID)
			if err != nil {
				t.Fatalf("list audit: %v", err)
			}
			atpAuditCount := 0
			for _, event := range audits {
				if event.EventType == "atp_swab" {
					atpAuditCount++
				}
			}
			if atpAuditCount != len(tc.duplicateRLUs) {
				t.Errorf("ATP audit count after duplicate versions = %d, want %d", atpAuditCount, len(tc.duplicateRLUs))
			}

			resp, err := svc.SubmitATPSwab(ctx, tc.taskID, ATPSwabRequest{
				OperationNo: "OP-" + tc.taskID + "-atp-2",
				PointCode:   "ATP-2",
				RLU:         100,
			})
			if err != nil {
				t.Fatalf("ATP-2 after duplicate ATP-1 versions: %v", err)
			}
			if resp.State != task.StateATPCovering {
				t.Errorf("state after ATP-2 = %q, want %q until ATP-3 is accepted", resp.State, task.StateATPCovering)
			}
			if len(resp.Covered) != 2 || resp.Covered[0] != "ATP-1" || resp.Covered[1] != "ATP-2" {
				t.Errorf("covered after ATP-2 = %v, want [ATP-1 ATP-2]", resp.Covered)
			}

			resp, err = svc.SubmitATPSwab(ctx, tc.taskID, ATPSwabRequest{
				OperationNo: "OP-" + tc.taskID + "-atp-3",
				PointCode:   "ATP-3",
				RLU:         100,
			})
			if err != nil {
				t.Fatalf("ATP-3 after ATP-1 and ATP-2 coverage: %v", err)
			}
			if resp.State != task.StateMicroVerifying {
				t.Errorf("state after all distinct ATP points accepted = %q, want %q", resp.State, task.StateMicroVerifying)
			}
			if len(resp.Covered) != 3 || resp.Covered[0] != "ATP-1" || resp.Covered[1] != "ATP-2" || resp.Covered[2] != "ATP-3" {
				t.Errorf("covered after ATP-3 = %v, want [ATP-1 ATP-2 ATP-3]", resp.Covered)
			}
		})
	}
}
