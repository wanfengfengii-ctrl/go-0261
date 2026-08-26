package store

import (
	"context"
	"sync"
	"testing"
)

// TestLeaseCompetition verifies that two tasks competing for the same wash tank
// result in exactly one successful lock.
func TestLeaseCompetition(t *testing.T) {
	svc := newTestService(t)

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []string{"TASK-A", "TASK-B"} {
		wg.Add(1)
		go func(taskID string) {
			defer wg.Done()
			spec := validLockSpec(taskID)
			_, err := svc.LockTask(context.Background(), LockRequest{LockSpec: spec})
			results <- err
		}(id)
	}
	wg.Wait()
	close(results)

	success := 0
	fail := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			fail++
		}
	}
	if success != 1 || fail != 1 {
		t.Fatalf("expected exactly one success and one failure, got success=%d fail=%d", success, fail)
	}
}

// TestLeaseRollbackOnConflict verifies that a multi-resource acquisition that
// fails partway rolls back entirely, leaving no partial occupancy.
func TestLeaseRollbackOnConflict(t *testing.T) {
	svc := newTestService(t)

	// Occupy TANK-A via the first task.
	lockValid(t, svc, "TASK-HOLD")

	// A second task requests TANK-A plus a fresh tank; the conflict must roll
	// back the fresh tank as well.
	spec := validLockSpec("TASK-CONFLICT")
	spec.WashTankID = "TANK-A"
	_, err := svc.LockTask(context.Background(), LockRequest{LockSpec: spec})
	if err == nil {
		t.Fatal("expected occupancy conflict")
	}
	if !IsAppError(err, CodeOccupied) {
		t.Fatalf("expected CodeOccupied, got %v", err)
	}
	if _, ok := svc.GetTask(context.Background(), "TASK-CONFLICT"); ok {
		t.Fatal("conflicting task must not be persisted")
	}
}

// TestBlindCodeDuplicateReject verifies that duplicate blind codes are rejected
// at lock time.
func TestBlindCodeDuplicateReject(t *testing.T) {
	svc := newTestService(t)
	spec := validLockSpec("TASK-DUP")
	spec.BlindCodes = []string{"BLIND-01", "BLIND-01", "BLIND-02"}

	_, err := svc.LockTask(context.Background(), LockRequest{LockSpec: spec})
	if err == nil {
		t.Fatal("expected duplicate blind code rejection")
	}
	if !IsAppError(err, CodeValidationFailed) {
		t.Fatalf("expected CodeValidationFailed, got %v", err)
	}
}
