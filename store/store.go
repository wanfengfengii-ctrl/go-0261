// Package store is the transactional persistence boundary and service layer of
// the LeafWash backend. It provides the SQLite WAL persistence directory, the
// task aggregate, resource-occupancy registry, evidence version chain, adapter
// call ledger, idempotency results, audit events, and finality credentials, and
// supports deterministic restart recovery (failure boundary #7). The Service
// orchestrates every write operation in a single transaction so that state
// validation, idempotency checks, occupancy updates, evidence writes, and audit
// recording either all succeed or all roll back (failure boundary #1).
package store

import (
	"context"

	"leafwash-packaging-release-gate/arbiter"
	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/evidence"
	"leafwash-packaging-release-gate/lease"
	"leafwash-packaging-release-gate/task"
)

// Store is the public service contract consumed by the HTTP API and tests.
// Every method performs its work atomically within one transaction.
type Store interface {
	// Catalog returns the read-side catalog used at lock time.
	Catalog() catalog.Catalog

	// GetTask returns the current task aggregate by id.
	GetTask(ctx context.Context, id string) (*task.InspectionTask, bool)

	// LockTask creates and locks an inspection task, atomically acquiring the
	// required occupancy for the wash tank, plate wells, drain slots, and blind
	// codes. It returns the frozen snapshot, generation, and initial state.
	LockTask(ctx context.Context, req LockRequest) (*LockResponse, error)

	// FeedConfirm records one of two distinct feed confirmations.
	FeedConfirm(ctx context.Context, taskID string, req FeedConfirmRequest) (*FeedConfirmResponse, error)

	// SubmitCurveSample records one locked-time-point disinfection reading.
	SubmitCurveSample(ctx context.Context, taskID string, req CurveSampleRequest) (*CurveSampleResponse, error)

	// SubmitATPSwab records an append-only ATP point reading version.
	SubmitATPSwab(ctx context.Context, taskID string, req ATPSwabRequest) (*ATPSwabResponse, error)

	// SubmitMicrobiology reveals a blind code and records a plate-well reading.
	SubmitMicrobiology(ctx context.Context, taskID string, req MicrobiologyRequest) (*MicrobiologyResponse, error)

	// SubmitRecheck records the single current-generation recheck evidence.
	SubmitRecheck(ctx context.Context, taskID string, req RecheckRequest) (*RecheckResponse, error)

	// SubmitReview records one independent review decision.
	SubmitReview(ctx context.Context, taskID string, req ReviewRequest) (*ReviewResponse, error)

	// Finalize runs the terminal competition and writes the unique conclusion.
	Finalize(ctx context.Context, taskID string, req FinalizeRequest) (*FinalizeResponse, error)

	// ListAdapterCalls returns the adapter attempts (including pending retries)
	// for a task, used by the failure-script diagnostic interface.
	ListAdapterCalls(ctx context.Context, taskID string) ([]evidence.AdapterCall, error)

	// ListAudit returns the append-only audit events for a task.
	ListAudit(ctx context.Context, taskID string) ([]AuditEvent, error)

	// Health reports liveness for the health endpoint.
	Health(ctx context.Context) error

	// Close releases any underlying resources.
	Close() error
}

// LockRequest is the input to LockTask. It embeds the full lock spec.
type LockRequest struct {
	task.LockSpec
}

// LockResponse is the result of a successful lock.
type LockResponse struct {
	TaskID     string              `json:"task_id"`
	Generation int                 `json:"generation"`
	State      task.State          `json:"state"`
	Snapshot   task.LockSnapshot   `json:"snapshot"`
	Leases     []lease.LeaseRecord `json:"leases"`
}

// FeedConfirmRequest is one of two feed confirmations.
type FeedConfirmRequest struct {
	OperationNo string `json:"operation_no"`
	Generation  int    `json:"generation,omitempty"`
	PersonID    string `json:"person_id"`
	BaseLotID   string `json:"base_lot_id"`
	SealID      string `json:"seal_id"`
	CutLineID   string `json:"cut_line_id"`
}

// FeedConfirmResponse reports the task state after a confirmation.
type FeedConfirmResponse struct {
	TaskID      string     `json:"task_id"`
	Generation  int        `json:"generation"`
	State       task.State `json:"state"`
	ConfirmedBy []string   `json:"confirmed_by"`
}

// CurveSampleRequest is a disinfection/physchem reading at one locked sample
// time. Fixed-point fields are decimal strings parsed by the fixed-point rules.
type CurveSampleRequest struct {
	OperationNo     string `json:"operation_no"`
	Generation      int    `json:"generation,omitempty"`
	SampleTime      int64  `json:"sample_time"`
	ChlorineX100    string `json:"chlorine_x100"`
	ORPMV           int64  `json:"orp_mv"`
	PHX100          string `json:"ph_x100"`
	TemperatureX100 string `json:"temperature_x100"`
	TurbidityX100   string `json:"turbidity_x100"`
	AdapterFailure  string `json:"adapter_failure,omitempty"`
}

// CurveSampleResponse reports the coverage and any detected anomaly.
type CurveSampleResponse struct {
	TaskID     string                  `json:"task_id"`
	Generation int                     `json:"generation"`
	State      task.State              `json:"state"`
	Valid      bool                    `json:"valid"`
	Coverage   evidence.CoverageReport `json:"coverage"`
	Anomalies  []string                `json:"anomalies,omitempty"`
}

// ATPSwabRequest is an ATP relative-light-unit reading for one point.
type ATPSwabRequest struct {
	OperationNo    string `json:"operation_no"`
	Generation     int    `json:"generation,omitempty"`
	PointCode      string `json:"point_code"`
	RLU            int64  `json:"rlu"`
	AdapterFailure string `json:"adapter_failure,omitempty"`
}

// ATPSwabResponse reports ATP coverage and any over-limit anomaly.
type ATPSwabResponse struct {
	TaskID     string     `json:"task_id"`
	Generation int        `json:"generation"`
	State      task.State `json:"state"`
	VersionNo  int        `json:"version_no"`
	OverLimit  bool       `json:"over_limit"`
	Covered    []string   `json:"covered"`
	Anomalies  []string   `json:"anomalies,omitempty"`
}

// MicrobiologyRequest reveals a blind code and records a culture-plate reading.
type MicrobiologyRequest struct {
	OperationNo    string `json:"operation_no"`
	Generation     int    `json:"generation,omitempty"`
	BlindCode      string `json:"blind_code"`
	PlateWell      string `json:"plate_well"`
	ColonyCFU      int64  `json:"colony_cfu"`
	Dilution       int64  `json:"dilution"`
	SampleVolumeML int64  `json:"sample_volume_ml"`
	AdapterFailure string `json:"adapter_failure,omitempty"`
}

// MicrobiologyResponse reports the derived colony count per 100 mL.
type MicrobiologyResponse struct {
	TaskID      string     `json:"task_id"`
	Generation  int        `json:"generation"`
	State       task.State `json:"state"`
	CFUPer100ML int64      `json:"cfu_per_100ml"`
	OverLimit   bool       `json:"over_limit"`
	Covered     []string   `json:"covered"`
	Anomalies   []string   `json:"anomalies,omitempty"`
}

// RecheckRequest records the single current-generation recheck evidence.
type RecheckRequest struct {
	OperationNo string                 `json:"operation_no"`
	Generation  int                    `json:"generation,omitempty"`
	ReviewerID  string                 `json:"reviewer_id"`
	Targets     arbiter.RecheckTargets `json:"targets"`
}

// RecheckResponse reports the task state after the recheck.
type RecheckResponse struct {
	TaskID     string     `json:"task_id"`
	Generation int        `json:"generation"`
	State      task.State `json:"state"`
}

// ReviewRequest is one independent review decision.
type ReviewRequest struct {
	OperationNo string `json:"operation_no"`
	Generation  int    `json:"generation,omitempty"`
	ReviewerID  string `json:"reviewer_id"`
	Decision    string `json:"decision"`
	ReasonCode  string `json:"reason_code,omitempty"`
}

// ReviewResponse reports the recorded reviewers.
type ReviewResponse struct {
	TaskID     string     `json:"task_id"`
	Generation int        `json:"generation"`
	State      task.State `json:"state"`
	Reviewers  []string   `json:"reviewers"`
}

// FinalizeRequest requests one of the four terminal conclusions.
type FinalizeRequest struct {
	OperationNo string `json:"operation_no"`
	Generation  int    `json:"generation,omitempty"`
	Decision    string `json:"decision"`
}

// FinalizeResponse reports the unique terminal conclusion and credential.
type FinalizeResponse struct {
	TaskID          string     `json:"task_id"`
	Generation      int        `json:"generation"`
	State           task.State `json:"state"`
	FinalResult     string     `json:"final_result"`
	FinalCredential string     `json:"final_credential"`
}
