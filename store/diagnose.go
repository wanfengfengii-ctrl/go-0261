package store

import (
	"context"

	"leafwash-packaging-release-gate/evidence"
)

// ListAdapterCalls returns the adapter attempts for a task in insertion order.
func (s *Service) ListAdapterCalls(ctx context.Context, taskID string) ([]evidence.AdapterCall, error) {
	var out []evidence.AdapterCall
	err := s.p.Tx(ctx, false, func(tx Tx) error {
		t, ok := tx.GetTask(taskID)
		if !ok {
			return NewAppError(CodeNotFound, "task not found")
		}
		var err error
		out, err = tx.AdapterCalls(taskID, t.Generation)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListAudit returns the append-only audit events for a task in logic-time order.
func (s *Service) ListAudit(ctx context.Context, taskID string) ([]AuditEvent, error) {
	var out []AuditEvent
	err := s.p.Tx(ctx, false, func(tx Tx) error {
		var err error
		out, err = tx.Audit(taskID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
