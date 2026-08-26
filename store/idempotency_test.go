package store

import (
	"context"
	"testing"
)

// TestFeedConfirmIdempotent verifies that retrying the same operation number
// with identical content returns the same result without double-counting.
func TestFeedConfirmIdempotent(t *testing.T) {
	svc := newTestService(t)
	lockValid(t, svc, "TASK-I1")

	req := FeedConfirmRequest{
		OperationNo: "OP-FEED-1", PersonID: "P-1001",
		BaseLotID: "BL-2026-001", SealID: "SEAL-001", CutLineID: "CUT-3",
	}
	first, err := svc.FeedConfirm(context.Background(), "TASK-I1", req)
	if err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	second, err := svc.FeedConfirm(context.Background(), "TASK-I1", req)
	if err != nil {
		t.Fatalf("retry confirm: %v", err)
	}
	if len(second.ConfirmedBy) != len(first.ConfirmedBy) {
		t.Fatalf("retry must not double-count: %v vs %v", first.ConfirmedBy, second.ConfirmedBy)
	}
}

// TestFeedConfirmConflict verifies that reusing an operation number with
// different content yields IDEMPOTENCY_CONFLICT.
func TestFeedConfirmConflict(t *testing.T) {
	svc := newTestService(t)
	lockValid(t, svc, "TASK-I2")

	_, err := svc.FeedConfirm(context.Background(), "TASK-I2", FeedConfirmRequest{
		OperationNo: "OP-FEED-2", PersonID: "P-1001",
		BaseLotID: "BL-2026-001", SealID: "SEAL-001", CutLineID: "CUT-3",
	})
	if err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	_, err = svc.FeedConfirm(context.Background(), "TASK-I2", FeedConfirmRequest{
		OperationNo: "OP-FEED-2", PersonID: "P-1002",
		BaseLotID: "BL-2026-001", SealID: "SEAL-001", CutLineID: "CUT-3",
	})
	if !IsAppError(err, CodeIdempotencyConflict) {
		t.Fatalf("expected IDEMPOTENCY_CONFLICT, got %v", err)
	}
}

// TestOldGenerationRejected verifies that a submission carrying an old
// generation is rejected and does not change state.
func TestOldGenerationRejected(t *testing.T) {
	svc := newTestService(t)
	lockValid(t, svc, "TASK-I3")

	_, err := svc.FeedConfirm(context.Background(), "TASK-I3", FeedConfirmRequest{
		OperationNo: "OP-OLD", Generation: 2, PersonID: "P-1001",
		BaseLotID: "BL-2026-001", SealID: "SEAL-001", CutLineID: "CUT-3",
	})
	if !IsAppError(err, CodeGenerationMismatch) {
		t.Fatalf("expected GENERATION_MISMATCH, got %v", err)
	}
	got, _ := svc.GetTask(context.Background(), "TASK-I3")
	if len(got.FeedConfirmers) != 0 {
		t.Fatalf("old-generation submission must not change state: %v", got.FeedConfirmers)
	}
}
