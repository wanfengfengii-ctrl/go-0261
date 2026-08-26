package store

import (
	"context"
	"testing"

	"leafwash-packaging-release-gate/task"
)

// TestLockSuccess verifies that locking a complete base-lot snapshot freezes
// every required value and atomically acquires the occupancy.
func TestLockSuccess(t *testing.T) {
	svc := newTestService(t)
	resp := lockValid(t, svc, "TASK-L1")

	if resp.TaskID != "TASK-L1" || resp.Generation != 1 {
		t.Fatalf("unexpected identity: %+v", resp)
	}
	if resp.State != task.StatePendingFeed {
		t.Fatalf("state = %q", resp.State)
	}
	snap := resp.Snapshot
	if snap.BaseLotID != "BL-2026-001" || snap.SealID != "SEAL-001" ||
		snap.CutLineID != "CUT-3" || snap.WashTankID != "TANK-A" {
		t.Fatalf("snapshot missing frozen values: %+v", snap)
	}
	if snap.FormulaRevision != 3 || snap.SummaryHash != "f100-r3" {
		t.Fatalf("snapshot formula not frozen: %+v", snap)
	}
	if len(snap.SampleTimes) != 3 || len(snap.ATPPoints) != 3 || len(snap.PlateWells) != 3 {
		t.Fatalf("snapshot lists incomplete: %+v", snap)
	}
	if len(resp.Leases) != 1+3+3+2 {
		t.Fatalf("expected 9 leases (tank+wells+drains+blinds), got %d", len(resp.Leases))
	}
}

// TestLockSealMismatch verifies that a base-lot/seal mismatch is rejected and
// leaves no occupancy residue.
func TestLockSealMismatch(t *testing.T) {
	svc := newTestService(t)
	spec := validLockSpec("TASK-L2")
	spec.SealID = "SEAL-999" // not allowed for BL-2026-001

	_, err := svc.LockTask(context.Background(), LockRequest{LockSpec: spec})
	if err == nil {
		t.Fatal("expected seal mismatch rejection")
	}
	if !IsAppError(err, CodeSealMismatch) {
		t.Fatalf("expected CodeSealMismatch, got %v", err)
	}
	if _, ok := svc.GetTask(context.Background(), "TASK-L2"); ok {
		t.Fatal("task must not exist after rejected lock")
	}
}

// TestLockStaleRevision verifies that a stale formula revision is rejected with
// no occupancy residue.
func TestLockStaleRevision(t *testing.T) {
	svc := newTestService(t)
	spec := validLockSpec("TASK-L3")
	spec.FormulaRevision = 1 // latest is 3

	_, err := svc.LockTask(context.Background(), LockRequest{LockSpec: spec})
	if err == nil {
		t.Fatal("expected stale revision rejection")
	}
	if !IsAppError(err, CodeStaleRevision) {
		t.Fatalf("expected CodeStaleRevision, got %v", err)
	}
	if _, ok := svc.GetTask(context.Background(), "TASK-L3"); ok {
		t.Fatal("task must not exist after rejected lock")
	}
}
