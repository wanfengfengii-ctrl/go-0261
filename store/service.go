package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/task"
)

// Service is the orchestration layer implementing Store on top of a
// Persistence backend. It owns the business flows (lock, feed confirmation,
// curve collection, ATP coverage, microbiology, recheck, review, finalize) and
// the failure boundaries, delegating raw row access to the Persistence and pure
// rules to the domain packages.
type Service struct {
	p Persistence
}

// NewService constructs a Service over the given persistence backend.
func NewService(p Persistence) *Service { return &Service{p: p} }

// Catalog returns the read-side catalog used at lock time.
func (s *Service) Catalog() catalog.Catalog { return s.p.Catalog() }

// Health reports backend liveness.
func (s *Service) Health(ctx context.Context) error { return s.p.Health(ctx) }

// Close releases the underlying backend.
func (s *Service) Close() error { return s.p.Close() }

// GetTask returns the task aggregate by id within a read transaction.
func (s *Service) GetTask(ctx context.Context, id string) (*task.InspectionTask, bool) {
	var (
		out *task.InspectionTask
		ok  bool
	)
	_ = s.p.Tx(ctx, false, func(tx Tx) error {
		out, ok = tx.GetTask(id)
		return nil
	})
	return out, ok
}

// hashOf computes a deterministic content hash of a request value.
func hashOf(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// lookupIdempotent returns a prior response for operationNo. It returns
// (body, found, conflictError): found=true means the caller should replay the
// recorded body; a non-nil error means IDEMPOTENCY_CONFLICT.
func lookupIdempotent(tx Tx, opNo, requestHash string) ([]byte, bool, error) {
	if opNo == "" {
		return nil, false, nil
	}
	rec, ok, err := tx.Idempotency(opNo)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	if rec.RequestHash != requestHash {
		return nil, false, NewAppError(CodeIdempotencyConflict, "operation_no reused with different content")
	}
	return rec.ResponseBodyJSON, true, nil
}

// recordIdempotent persists the response for a successful idempotent operation.
func recordIdempotent(tx Tx, opNo, taskID string, generation int, kind string, requestHash string, resp any) error {
	if opNo == "" {
		return nil
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return tx.PutIdempotency(IdempotencyRecord{
		OperationNo:      opNo,
		TaskID:           taskID,
		Generation:       generation,
		OperationKind:    kind,
		RequestHash:      requestHash,
		ResponseCode:     "OK",
		ResponseBodyJSON: body,
	})
}

// checkGeneration validates an optional request generation against the task's
// current generation, rejecting late (old-generation) submissions.
func checkGeneration(reqGen, taskGen int) error {
	if reqGen != 0 && reqGen != taskGen {
		return NewAppError(CodeGenerationMismatch, "stale generation")
	}
	return nil
}

// audit records an audit event for an operation outcome.
func audit(tx Tx, t *task.InspectionTask, actorID, eventType, reasonCode string, details any) error {
	var detailsJSON []byte
	if details != nil {
		detailsJSON, _ = json.Marshal(details)
	}
	logicTime, err := tx.NextLogicTime()
	if err != nil {
		return err
	}
	return tx.PutAudit(AuditEvent{
		TaskID:      t.TaskID,
		Generation:  t.Generation,
		ActorID:     actorID,
		EventType:   eventType,
		ReasonCode:  reasonCode,
		DetailsJSON: detailsJSON,
		LogicTime:   logicTime,
	})
}

// adapterFailureCode maps a simulated instrument failure to a stable error code.
func adapterFailureCode(failure string) string {
	switch failure {
	case "reject":
		return "ADAPTER_REJECTED"
	case "disconnect":
		return "ADAPTER_DISCONNECTED"
	case "timeout":
		return "ADAPTER_TIMEOUT"
	case "format_error":
		return "ADAPTER_FORMAT_ERROR"
	default:
		return "ADAPTER_ERROR"
	}
}

// scriptStepFor maps an adapter kind to its failure-script step label.
func scriptStepFor(kind evidence.AdapterKind) string {
	switch kind {
	case evidence.AdapterChlorine:
		return "read_chlorine_curve"
	case evidence.AdapterATP:
		return "read_atp_swab"
	case evidence.AdapterIncubator:
		return "read_incubator_plate"
	default:
		return "read"
	}
}

// recordAdapterFailure writes a pending-retry AdapterCall and does not produce
// accepted evidence or release occupancy (failure boundary #3). The attempt
// number and next retry logic time are deterministic.
func recordAdapterFailure(tx Tx, t *task.InspectionTask, kind evidence.AdapterKind, target, failure string) error {
	calls, err := tx.AdapterCalls(t.TaskID, t.Generation)
	if err != nil {
		return err
	}
	attempt := 1
	for _, c := range calls {
		if c.AdapterKind == kind && c.TargetKey == target {
			attempt++
		}
	}
	logicTime, err := tx.NextLogicTime()
	if err != nil {
		return err
	}
	return tx.PutAdapterCall(evidence.AdapterCall{
		CallID:         newID("call"),
		AdapterKind:    kind,
		TaskID:         t.TaskID,
		Generation:     t.Generation,
		TargetKey:      target,
		AttemptNo:      attempt,
		ScriptStep:     scriptStepFor(kind),
		Status:         evidence.CallPendingRetry,
		ErrorCode:      adapterFailureCode(failure),
		NextRetryLogic: logicTime + int64(attempt),
	})
}

func stateAllowed(s task.State, allowed ...task.State) bool {
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}

// adapterRetry records a pending-retry adapter call in its own committed
// transaction (so the retry record is durable even though the reading is not
// accepted), then returns the stable adapter error. This keeps the retry record
// out of the main operation transaction, which rolls back on rejection.
func (s *Service) adapterRetry(ctx context.Context, taskID string, generation int, kind evidence.AdapterKind, target, failure string, allowed ...task.State) error {
	err := s.p.Tx(ctx, true, func(tx Tx) error {
		t, ok := tx.GetTask(taskID)
		if !ok {
			return NewAppError(CodeNotFound, "task not found")
		}
		if t.State.IsTerminal() {
			return NewAppError(CodeTerminalState, "task is in a terminal state")
		}
		if err := checkGeneration(generation, t.Generation); err != nil {
			return err
		}
		if !stateAllowed(t.State, allowed...) {
			return NewAppError(CodeInvalidState, "operation not allowed in current state")
		}
		return recordAdapterFailure(tx, t, kind, target, failure)
	})
	if err != nil {
		return err
	}
	return NewAppError(adapterFailureCode(failure), "instrument failure recorded for retry")
}
