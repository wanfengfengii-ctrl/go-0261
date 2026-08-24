package store

import (
	"context"
	"testing"

	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/task"
)

// TestChlorineDisconnectRetry verifies that a chlorine-meter disconnect produces
// a deterministic pending-retry adapter call without writing evidence or
// advancing state, and that a second attempt increments the attempt number.
func TestChlorineDisconnectRetry(t *testing.T) {
	svc := newTestService(t)
	lockValid(t, svc, "TASK-A1")
	confirmFeed(t, svc, "TASK-A1")

	req := curveReq("TASK-A1", 0, "1.50")
	req.AdapterFailure = "disconnect"
	_, err := svc.SubmitCurveSample(context.Background(), "TASK-A1", req)
	if !IsAppError(err, "ADAPTER_DISCONNECTED") {
		t.Fatalf("expected ADAPTER_DISCONNECTED, got %v", err)
	}

	calls, _ := svc.ListAdapterCalls(context.Background(), "TASK-A1")
	if len(calls) != 1 {
		t.Fatalf("expected 1 adapter call, got %d", len(calls))
	}
	c := calls[0]
	if c.AdapterKind != evidence.AdapterChlorine || c.Status != evidence.CallPendingRetry {
		t.Fatalf("unexpected call: %+v", c)
	}
	if c.AttemptNo != 1 {
		t.Fatalf("attempt_no = %d, want 1", c.AttemptNo)
	}
	if c.NextRetryLogic <= 0 {
		t.Fatalf("next_retry_logic must be positive, got %d", c.NextRetryLogic)
	}

	// Second disconnect increments the attempt number deterministically.
	req2 := curveReq("TASK-A1", 0, "1.50")
	req2.OperationNo = "OP-retry2"
	req2.AdapterFailure = "disconnect"
	if _, err := svc.SubmitCurveSample(context.Background(), "TASK-A1", req2); !IsAppError(err, "ADAPTER_DISCONNECTED") {
		t.Fatalf("expected second disconnect, got %v", err)
	}
	calls, _ = svc.ListAdapterCalls(context.Background(), "TASK-A1")
	if len(calls) != 2 || calls[1].AttemptNo != 2 {
		t.Fatalf("expected second attempt with attempt_no=2, got %+v", calls)
	}

	got, _ := svc.GetTask(context.Background(), "TASK-A1")
	if got.State != task.StateTanksOccupied {
		t.Fatalf("adapter failure must not advance state: %q", got.State)
	}
}

// TestATPFormatErrorRetry verifies an ATP reader format error yields a retry
// call without producing accepted evidence.
func TestATPFormatErrorRetry(t *testing.T) {
	svc := newTestService(t)
	driveCurve(t, svc, "TASK-A2")

	_, err := svc.SubmitATPSwab(context.Background(), "TASK-A2", ATPSwabRequest{
		OperationNo: "OP-atp-fmt", PointCode: "ATP-1", RLU: 100, AdapterFailure: "format_error",
	})
	if !IsAppError(err, "ADAPTER_FORMAT_ERROR") {
		t.Fatalf("expected ADAPTER_FORMAT_ERROR, got %v", err)
	}
	calls, _ := svc.ListAdapterCalls(context.Background(), "TASK-A2")
	if len(calls) != 1 || calls[0].AdapterKind != evidence.AdapterATP {
		t.Fatalf("unexpected adapter calls: %+v", calls)
	}
	if calls[0].Status != evidence.CallPendingRetry {
		t.Fatalf("status = %q, want pending_retry", calls[0].Status)
	}
}

// TestIncubatorTimeoutRetry verifies an incubator timeout yields a retry call.
func TestIncubatorTimeoutRetry(t *testing.T) {
	svc := newTestService(t)
	driveATP(t, svc, "TASK-A3")

	_, err := svc.SubmitMicrobiology(context.Background(), "TASK-A3", MicrobiologyRequest{
		OperationNo: "OP-micro-timeout", BlindCode: "BLIND-01", PlateWell: "WELL-A1",
		ColonyCFU: 0, Dilution: 1, SampleVolumeML: 1, AdapterFailure: "timeout",
	})
	if !IsAppError(err, "ADAPTER_TIMEOUT") {
		t.Fatalf("expected ADAPTER_TIMEOUT, got %v", err)
	}
	calls, _ := svc.ListAdapterCalls(context.Background(), "TASK-A3")
	if len(calls) != 1 || calls[0].AdapterKind != evidence.AdapterIncubator {
		t.Fatalf("unexpected adapter calls: %+v", calls)
	}
	if calls[0].Status != evidence.CallPendingRetry || calls[0].ErrorCode != "ADAPTER_TIMEOUT" {
		t.Fatalf("unexpected call fields: %+v", calls[0])
	}
}
