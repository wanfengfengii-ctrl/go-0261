// Package task implements the "清洗联检任务聚合" (wash inspection task
// aggregate) component: the task state machine, generation identity, operation
// idempotency, locked snapshot, feed confirmation, stage advancement, and the
// terminal-state barrier.
package task

import "strings"

// State is the inspection task lifecycle state. States advance only in the
// documented order; terminal states reject all ordinary writes.
type State string

const (
	StatePendingLock       State = "pending_lock"       // 待锁定
	StatePendingFeed       State = "pending_feed"       // 待投料确认
	StateTanksOccupied     State = "tanks_occupied"     // 槽位占用中
	StateCurveCollecting   State = "curve_collecting"   // 消毒曲线采集中
	StateATPCovering       State = "atp_covering"       // ATP 覆盖中
	StateMicroVerifying    State = "micro_verifying"    // 微生物核验中
	StatePhysChemRetesting State = "physchem_retesting" // 理化复测中
	StatePendingReview     State = "pending_review"     // 待独立复核
	StateReadyToPack       State = "ready_to_pack"      // 可转封装 (terminal)
	StatePacked            State = "packed"             // 已转封装 (terminal)
	StateSanitaryHold      State = "sanitary_hold"      // 卫生隔离 (terminal)
	StateCancelled         State = "cancelled"          // 已取消 (terminal)
)

// terminalStates is the set of final results. Once reached, no ordinary write,
// reading, recheck, review, or finalize may change the task.
var terminalStates = map[State]bool{
	StateReadyToPack:  true,
	StatePacked:       true,
	StateSanitaryHold: true,
	StateCancelled:    true,
}

// IsTerminal reports whether s is a final state.
func (s State) IsTerminal() bool {
	return terminalStates[s]
}

// progression is the ordered, non-terminal lifecycle. A state may only advance
// to the next entry in this sequence (or to a terminal state produced by the
// arbiter's finalize competition).
var progression = []State{
	StatePendingLock,
	StatePendingFeed,
	StateTanksOccupied,
	StateCurveCollecting,
	StateATPCovering,
	StateMicroVerifying,
	StatePhysChemRetesting,
	StatePendingReview,
}

// CanAdvance reports whether from may transition to next along the documented
// lifecycle. Advancing into a terminal state is governed by the arbiter, not
// this linear progression.
func CanAdvance(from, next State) bool {
	for i, s := range progression {
		if s == from {
			if i+1 < len(progression) && progression[i+1] == next {
				return true
			}
			return false
		}
	}
	return false
}

// IsTerminalResult reports whether the string is one of the four final results.
func IsTerminalResult(s string) bool {
	return terminalStates[State(strings.TrimSpace(s))]
}

// InspectionTask is the aggregate root persisted by the store.
type InspectionTask struct {
	TaskID             string   `json:"task_id"`
	Generation         int      `json:"generation"`
	State              State    `json:"state"`
	LockedSnapshotJSON []byte   `json:"locked_snapshot_json,omitempty"`
	BaseLotID          string   `json:"base_lot_id"`
	SealID             string   `json:"seal_id"`
	PrecoolLot         string   `json:"precool_lot"`
	CutLineID          string   `json:"cut_line_id"`
	WashTankID         string   `json:"wash_tank_id"`
	FormulaID          string   `json:"formula_id,omitempty"`
	FormulaRevision    int      `json:"formula_revision"`
	FeedConfirmers     []string `json:"feed_confirmers,omitempty"`
	Anomalies          []string `json:"anomalies,omitempty"`
	RecheckDone        bool     `json:"recheck_done,omitempty"`
	FinalResult        string   `json:"final_result,omitempty"`
	FinalCredential    string   `json:"final_credential,omitempty"`
	CreatedAtLogic     int64    `json:"created_at_logic"`
	UpdatedAtLogic     int64    `json:"updated_at_logic"`
}

// HasAnomaly reports whether the task carries the given anomaly reason code.
func (t *InspectionTask) HasAnomaly(code string) bool {
	for _, a := range t.Anomalies {
		if a == code {
			return true
		}
	}
	return false
}

// AddAnomaly records an anomaly reason code without duplicating existing ones.
func (t *InspectionTask) AddAnomaly(code string) {
	if !t.HasAnomaly(code) {
		t.Anomalies = append(t.Anomalies, code)
	}
}

// LockSnapshot freezes every value required at lock time (acceptance #1).
type LockSnapshot struct {
	TaskID          string   `json:"task_id"`
	Generation      int      `json:"generation"`
	BaseLotID       string   `json:"base_lot_id"`
	SealID          string   `json:"seal_id"`
	PrecoolLot      string   `json:"precool_lot"`
	CutLineID       string   `json:"cut_line_id"`
	WashTankID      string   `json:"wash_tank_id"`
	FormulaID       string   `json:"formula_id"`
	FormulaRevision int      `json:"formula_revision"`
	SummaryHash     string   `json:"summary_hash"`
	SampleTimes     []int64  `json:"sample_times"`
	BlindCodes      []string `json:"blind_codes"`
	ATPPoints       []string `json:"atp_points"`
	PlateWells      []string `json:"plate_wells"`
	DrainSlots      []string `json:"drain_slots"`
	Reviewers       []string `json:"reviewers"`
}
