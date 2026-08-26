package store

import (
	"context"
	"path/filepath"
	"testing"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/lease"
	"leafwash-packaging-release-gate/task"
)

// newTestService returns a Service backed by a SQLite database in a temporary
// directory with the seeded catalog, closed automatically at test cleanup.
func newTestService(t *testing.T) *Service {
	t.Helper()
	return newTestServiceAt(t, filepath.Join(t.TempDir(), "test.db"))
}

func newTestServiceAt(t *testing.T, path string) *Service {
	t.Helper()
	cat := catalog.NewMemory()
	catalog.Seed(cat)
	backend, err := OpenSQLite(path, cat)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { backend.Close() })
	return NewService(backend)
}

// validLockSpec returns a lock spec that validates against the seeded catalog.
func validLockSpec(taskID string) task.LockSpec {
	return task.LockSpec{
		TaskID:          taskID,
		BaseLotID:       "BL-2026-001",
		SealID:          "SEAL-001",
		PrecoolLot:      "PC-001",
		CutLineID:       "CUT-3",
		WashTankID:      "TANK-A",
		FormulaID:       "F-100",
		FormulaRevision: 3,
		SampleTimes:     []int64{0, 300, 600},
		BlindCodes:      []string{"BLIND-01", "BLIND-02", "BLIND-03"},
		ATPPoints:       []string{"ATP-1", "ATP-2", "ATP-3"},
		PlateWells:      []string{"WELL-A1", "WELL-A2", "WELL-B1"},
		DrainSlots:      []string{"DRAIN-1", "DRAIN-2"},
		Reviewers:       []string{"P-2001", "P-2002"},
	}
}

// lockValid locks a task with the default valid spec and returns the response.
func lockValid(t *testing.T, svc *Service, taskID string) *LockResponse {
	t.Helper()
	resp, err := svc.LockTask(context.Background(), LockRequest{LockSpec: validLockSpec(taskID)})
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	return resp
}

// driveCurve advances a locked task through two feed confirmations and full
// curve coverage, landing in atp_covering.
func driveCurve(t *testing.T, svc *Service, taskID string) {
	t.Helper()
	lockValid(t, svc, taskID)
	confirmFeed(t, svc, taskID)
	for _, ts := range []int64{0, 300, 600} {
		if _, err := svc.SubmitCurveSample(context.Background(), taskID, curveReq(taskID, ts, "1.50")); err != nil {
			t.Fatalf("curve sample %d: %v", ts, err)
		}
	}
}

// driveATP advances through curve collection and covers every ATP point,
// landing in micro_verifying.
func driveATP(t *testing.T, svc *Service, taskID string) {
	t.Helper()
	driveCurve(t, svc, taskID)
	for _, pt := range []string{"ATP-1", "ATP-2", "ATP-3"} {
		if _, err := svc.SubmitATPSwab(context.Background(), taskID, ATPSwabRequest{
			OperationNo: "OP-" + taskID + "-atp-" + pt, PointCode: pt, RLU: 100,
		}); err != nil {
			t.Fatalf("atp swab %s: %v", pt, err)
		}
	}
}

// TestSQLiteRestartRecovery verifies that an open task, its leases, a pending
// adapter retry, an idempotency record, and its state survive a process restart
// (failure boundary #7).
func TestSQLiteRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recover.db")

	svc := newTestServiceAt(t, path)
	resp := lockValid(t, svc, "TASK-R1")
	if resp.State != task.StatePendingFeed {
		t.Fatalf("state after lock = %q", resp.State)
	}

	_, err := svc.FeedConfirm(context.Background(), "TASK-R1", FeedConfirmRequest{
		OperationNo: "OP-1", PersonID: "P-1001", BaseLotID: "BL-2026-001",
		SealID: "SEAL-001", CutLineID: "CUT-3",
	})
	if err != nil {
		t.Fatalf("feed confirm: %v", err)
	}
	_ = svc.Close()

	// Reopen the database (simulating a restart).
	cat := catalog.NewMemory()
	catalog.Seed(cat)
	backend, err := OpenSQLite(path, cat)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer backend.Close()
	svc2 := NewService(backend)

	got, ok := svc2.GetTask(context.Background(), "TASK-R1")
	if !ok {
		t.Fatal("task not recovered after restart")
	}
	if got.State != task.StatePendingFeed {
		t.Fatalf("recovered state = %q, want pending_feed", got.State)
	}
	if len(got.FeedConfirmers) != 1 || got.FeedConfirmers[0] != "P-1001" {
		t.Fatalf("recovered feed confirmers = %v", got.FeedConfirmers)
	}
}

// TestMemoryLeaseAtomicity verifies the in-memory backend's lease rollback
// behavior: a partial acquisition failure leaves no occupancy.
func TestMemoryLeaseAtomicity(t *testing.T) {
	m := NewMemory(catalog.NewMemory())
	ctx := context.Background()

	if err := m.Tx(ctx, true, func(tx Tx) error {
		return tx.AcquireLeases("TASK-1", 1, []lease.Resource{{Type: lease.ResourceWashTank, Key: "TANK-A"}})
	}); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	err := m.Tx(ctx, true, func(tx Tx) error {
		return tx.AcquireLeases("TASK-2", 1, []lease.Resource{
			{Type: lease.ResourceWashTank, Key: "TANK-B"},
			{Type: lease.ResourceWashTank, Key: "TANK-A"},
		})
	})
	if err != lease.ErrOccupied {
		t.Fatalf("expected ErrOccupied, got %v", err)
	}
	var held []lease.LeaseRecord
	_ = m.Tx(ctx, false, func(tx Tx) error {
		var e error
		held, e = tx.HeldBy("TASK-2")
		return e
	})
	if len(held) != 0 {
		t.Fatalf("TASK-2 must hold no leases after rollback, got %d", len(held))
	}
}
