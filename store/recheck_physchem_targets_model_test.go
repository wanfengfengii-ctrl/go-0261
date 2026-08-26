package store

import (
	"context"
	"path/filepath"
	"testing"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/task"
)

func TestModel_RecheckTargetsRequirePhyschemSampleTimes(t *testing.T) {
	tests := []struct {
		name        string
		targets     arbiter.RecheckTargets
		wantErrCode string
	}{
		{
			name:        "empty targets do not cover the out of range pH sample",
			targets:     arbiter.RecheckTargets{},
			wantErrCode: CodeValidationFailed,
		},
		{
			name:        "different sample time does not cover the out of range pH sample",
			targets:     arbiter.RecheckTargets{SampleTimes: []int64{0}},
			wantErrCode: CodeValidationFailed,
		},
		{
			name: "affected sample time is accepted with extra targets",
			targets: arbiter.RecheckTargets{
				SampleTimes: []int64{300, 600},
				BlindCodes:  []string{"BLIND-02"},
				PointCodes:  []string{"ATP-2"},
				PlateWells:  []string{"WELL-A2"},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dbPath := filepath.Join(t.TempDir(), "physchem-targets.db")

			cat := catalog.NewMemory()
			catalog.Seed(cat)
			backend, err := OpenSQLite(dbPath, cat)
			if err != nil {
				t.Fatalf("open sqlite: %v", err)
			}
			svc := NewService(backend)

			taskID := "TASK-MODEL-PHYSCHEM"
			lockValid(t, svc, taskID)
			confirmFeed(t, svc, taskID)

			for _, ts := range []int64{0, 300, 600} {
				req := curveReq(taskID, ts, "1.50")
				if ts == 300 {
					req.PHX100 = "8.60"
				}
				if _, err := svc.SubmitCurveSample(ctx, taskID, req); err != nil {
					t.Fatalf("curve sample %d: %v", ts, err)
				}
			}
			for _, pt := range []string{"ATP-1", "ATP-2", "ATP-3"} {
				if _, err := svc.SubmitATPSwab(ctx, taskID, ATPSwabRequest{
					OperationNo: "OP-" + taskID + "-atp-" + pt,
					PointCode:   pt,
					RLU:         100,
				}); err != nil {
					t.Fatalf("atp swab %s: %v", pt, err)
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
					t.Fatalf("microbiology %s: %v", well, err)
				}
			}

			got, ok := svc.GetTask(ctx, taskID)
			if !ok {
				t.Fatal("task not found")
			}
			if got.State != task.StatePhysChemRetesting {
				t.Fatalf("state = %q, want physchem_retesting", got.State)
			}
			if !got.HasAnomaly(string(arbiter.AnomalyPhyschemRange)) {
				t.Fatalf("expected physchem anomaly, got %v", got.Anomalies)
			}
			if got.HasAnomaly(string(arbiter.AnomalyChlorineBreak)) ||
				got.HasAnomaly(string(arbiter.AnomalyATPOverLimit)) ||
				got.HasAnomaly(string(arbiter.AnomalyColonySusp)) {
				t.Fatalf("expected only physchem anomaly, got %v", got.Anomalies)
			}

			if err := svc.Close(); err != nil {
				t.Fatalf("close before recheck: %v", err)
			}
			cat = catalog.NewMemory()
			catalog.Seed(cat)
			backend, err = OpenSQLite(dbPath, cat)
			if err != nil {
				t.Fatalf("reopen sqlite: %v", err)
			}
			svc = NewService(backend)
			t.Cleanup(func() { _ = svc.Close() })

			resp, err := svc.SubmitRecheck(ctx, taskID, RecheckRequest{
				OperationNo: "OP-" + taskID + "-recheck",
				ReviewerID:  "P-2001",
				Targets:     tc.targets,
			})
			if tc.wantErrCode != "" {
				if !IsAppError(err, tc.wantErrCode) {
					t.Fatalf("expected %s, got response=%v err=%v", tc.wantErrCode, resp, err)
				}
				got, _ := svc.GetTask(ctx, taskID)
				if got.State != task.StatePhysChemRetesting {
					t.Fatalf("state after rejected recheck = %q, want physchem_retesting", got.State)
				}
				return
			}
			if err != nil {
				t.Fatalf("recheck: %v", err)
			}
			if resp.State != task.StatePendingReview {
				t.Fatalf("state after recheck = %q, want pending_review", resp.State)
			}
		})
	}
}
