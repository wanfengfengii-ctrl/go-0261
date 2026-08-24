package store

import (
	"context"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/lease"
	"leafwash-packaging-release-gate/task"
)

// IdempotencyRecord is the persisted result of an operation keyed by
// operation_no. Same operation_no + same content returns the recorded response;
// same operation_no + different content yields IDEMPOTENCY_CONFLICT.
type IdempotencyRecord struct {
	OperationNo      string `json:"operation_no"`
	TaskID           string `json:"task_id"`
	Generation       int    `json:"generation"`
	OperationKind    string `json:"operation_kind"`
	RequestHash      string `json:"request_hash"`
	ResponseCode     string `json:"response_code"`
	ResponseBodyJSON []byte `json:"response_body_json,omitempty"`
}

// AuditEvent is a single append-only audit record written alongside every
// successful and rejected operation.
type AuditEvent struct {
	EventID     int64  `json:"event_id"`
	TaskID      string `json:"task_id"`
	Generation  int    `json:"generation"`
	ActorID     string `json:"actor_id"`
	EventType   string `json:"event_type"`
	ReasonCode  string `json:"reason_code"`
	DetailsJSON []byte `json:"details_json,omitempty"`
	LogicTime   int64  `json:"logic_time"`
}

// Persistence is the low-level transactional storage contract. The service
// layer runs every write operation inside a single transaction via Tx.
type Persistence interface {
	Catalog() catalog.Catalog
	// Tx runs fn within a single transaction. write selects a read-write
	// transaction; a returned error rolls back all writes.
	Tx(ctx context.Context, write bool, fn func(tx Tx) error) error
	Health(ctx context.Context) error
	Close() error
}

// Tx is the transaction-scoped storage handle. Every method here maps to one or
// more rows in the persisted tables and participates in the surrounding
// transaction, so a failed operation leaves no partial occupancy or evidence.
type Tx interface {
	// Tasks
	GetTask(id string) (*task.InspectionTask, bool)
	PutTask(t *task.InspectionTask) error

	// Leases
	AcquireLeases(taskID string, generation int, resources []lease.Resource) error
	ReleaseLeases(taskID string, generation int, resources []lease.Resource) error
	HeldBy(taskID string) ([]lease.LeaseRecord, error)
	LeaseFor(resourceType lease.ResourceType, key string) (lease.LeaseRecord, bool, error)

	// Idempotency
	Idempotency(operationNo string) (IdempotencyRecord, bool, error)
	PutIdempotency(r IdempotencyRecord) error

	// Coverage samples
	PutCoverageSample(s evidence.CoverageSample) error
	CoverageSamples(taskID string, generation int) ([]evidence.CoverageSample, error)

	// Evidence versions
	PutEvidence(v evidence.EvidenceVersion) error
	Evidence(taskID string, generation int) ([]evidence.EvidenceVersion, error)
	NextEvidenceVersion(taskID string, kind evidence.EvidenceKind, pointCode string) (int, error)

	// Adapter calls
	PutAdapterCall(c evidence.AdapterCall) error
	AdapterCalls(taskID string, generation int) ([]evidence.AdapterCall, error)

	// Reviews
	PutReview(r evidence.ReviewDecision) error
	Reviews(taskID string, generation int) ([]evidence.ReviewDecision, error)

	// Audit
	PutAudit(e AuditEvent) error
	Audit(taskID string) ([]AuditEvent, error)

	// Logic clock
	NextLogicTime() (int64, error)
}
