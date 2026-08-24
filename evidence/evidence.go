// Package evidence implements the "消毒曲线与拭子采集记录册" (disinfection
// curve and swab collection ledger) component. It owns the coverage grid,
// fixed-point readings, ATP version chain, adapter call attempts, and
// retry-pending state, plus the deterministic fixed-point arithmetic used to
// validate and derive readings.
package evidence

// AdapterKind identifies the instrument backing a reading.
type AdapterKind string

const (
	AdapterChlorine  AdapterKind = "chlorine_meter" // 余氯仪
	AdapterATP       AdapterKind = "atp_reader"     // ATP 读数器
	AdapterIncubator AdapterKind = "incubator"      // 培养箱
)

// CallStatus is the lifecycle status of an adapter call.
type CallStatus string

const (
	CallPendingRetry CallStatus = "pending_retry"
	CallSucceeded    CallStatus = "succeeded"
	CallFailed       CallStatus = "failed"
)

// CoverageSample is a single locked-time-point disinfection/physchem reading.
// Chlorine, ph, temperature, and turbidity are fixed-point integers scaled by
// 100; ORP is an integer millivolt.
type CoverageSample struct {
	TaskID          string `json:"task_id"`
	Generation      int    `json:"generation"`
	SampleTime      int64  `json:"sample_time"`
	ChlorineX100    int64  `json:"chlorine_x100"`
	ORPMV           int64  `json:"orp_mv"`
	PHX100          int64  `json:"ph_x100"`
	TemperatureX100 int64  `json:"temperature_x100"`
	TurbidityX100   int64  `json:"turbidity_x100"`
	SourceCallID    string `json:"source_call_id"`
	Valid           bool   `json:"valid"`
}

// EvidenceKind classifies an evidence version.
type EvidenceKind string

const (
	EvidenceATP          EvidenceKind = "atp"
	EvidenceMicrobiology EvidenceKind = "microbiology"
	EvidenceRecheck      EvidenceKind = "recheck"
)

// EvidenceVersion is an immutable evidence version. ATP points are append-only:
// a new reading creates a new version rather than overwriting.
type EvidenceVersion struct {
	EvidenceID     string       `json:"evidence_id"`
	TaskID         string       `json:"task_id"`
	Generation     int          `json:"generation"`
	Kind           EvidenceKind `json:"kind"`
	BlindCode      string       `json:"blind_code"`
	PointCode      string       `json:"point_code"`
	PlateWell      string       `json:"plate_well"`
	VersionNo      int          `json:"version_no"`
	RawJSON        []byte       `json:"raw_json,omitempty"`
	DerivedJSON    []byte       `json:"derived_json,omitempty"`
	Accepted       bool         `json:"accepted"`
	CreatedAtLogic int64        `json:"created_at_logic"`
}

// AdapterCall records one instrument invocation attempt and its retry state.
// Rejected, disconnected, timed-out, or malformed calls are written here as
// pending-retry records without producing accepted evidence or releasing
// occupancy (failure boundary #3).
type AdapterCall struct {
	CallID         string      `json:"call_id"`
	AdapterKind    AdapterKind `json:"adapter_kind"`
	TaskID         string      `json:"task_id"`
	Generation     int         `json:"generation"`
	TargetKey      string      `json:"target_key"`
	AttemptNo      int         `json:"attempt_no"`
	ScriptStep     string      `json:"script_step"`
	Status         CallStatus  `json:"status"`
	ErrorCode      string      `json:"error_code"`
	NextRetryLogic int64       `json:"next_retry_logic"`
	RequestJSON    []byte      `json:"request_json,omitempty"`
	ResponseJSON   []byte      `json:"response_json,omitempty"`
}

// ReviewDecision is one independent-review decision (failure boundary #2 sorts
// reasons by base_lot_id, wash_tank_id, sample_time, blind_code, point_code,
// plate_well).
type ReviewDecision struct {
	ReviewID       string `json:"review_id"`
	TaskID         string `json:"task_id"`
	Generation     int    `json:"generation"`
	ReviewerID     string `json:"reviewer_id"`
	Decision       string `json:"decision"`
	ReasonCode     string `json:"reason_code"`
	CreatedAtLogic int64  `json:"created_at_logic"`
}
