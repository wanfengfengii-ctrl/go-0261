package task

import (
	"encoding/json"
	"sort"
)

// LockSpec is the full lock-time input set submitted to create a task. It
// freezes the base lot, seal, precool lot, cut line, wash tank, formula
// revision, sample times, blind codes, ATP points, plate wells, drain slots,
// and reviewer candidates (acceptance #1).
type LockSpec struct {
	TaskID          string   `json:"task_id,omitempty"`
	BaseLotID       string   `json:"base_lot_id"`
	SealID          string   `json:"seal_id"`
	PrecoolLot      string   `json:"precool_lot"`
	CutLineID       string   `json:"cut_line_id"`
	WashTankID      string   `json:"wash_tank_id"`
	FormulaID       string   `json:"formula_id"`
	FormulaRevision int      `json:"formula_revision"`
	SampleTimes     []int64  `json:"sample_times"`
	BlindCodes      []string `json:"blind_codes"`
	ATPPoints       []string `json:"atp_points"`
	PlateWells      []string `json:"plate_wells"`
	DrainSlots      []string `json:"drain_slots"`
	Reviewers       []string `json:"reviewers"`
}

// ValidateLockShape checks the structural invariants of the lock input that do
// not require the catalog: sample times must be sorted and unique, and the
// blind codes must be unique. It returns stable reason codes for violations.
func (s LockSpec) ValidateLockShape() (reasons []string) {
	if s.BaseLotID == "" {
		reasons = append(reasons, "MISSING_BASE_LOT")
	}
	if s.SealID == "" {
		reasons = append(reasons, "MISSING_SEAL")
	}
	if s.CutLineID == "" {
		reasons = append(reasons, "MISSING_CUT_LINE")
	}
	if s.WashTankID == "" {
		reasons = append(reasons, "MISSING_WASH_TANK")
	}
	if s.FormulaID == "" {
		reasons = append(reasons, "MISSING_FORMULA")
	}
	if len(s.SampleTimes) == 0 {
		reasons = append(reasons, "MISSING_SAMPLE_TIMES")
	}
	if len(s.Reviewers) < 2 {
		reasons = append(reasons, "INSUFFICIENT_REVIEWERS")
	}

	times := append([]int64(nil), s.SampleTimes...)
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	for i := 1; i < len(times); i++ {
		if times[i] == times[i-1] {
			reasons = append(reasons, "DUPLICATE_SAMPLE_TIME")
		}
	}
	for i, t := range times {
		if t != s.SampleTimes[i] {
			reasons = append(reasons, "UNSORTED_SAMPLE_TIMES")
			break
		}
	}

	seen := map[string]bool{}
	for _, b := range s.BlindCodes {
		if b == "" {
			reasons = append(reasons, "EMPTY_BLIND_CODE")
			continue
		}
		if seen[b] {
			reasons = append(reasons, "DUPLICATE_BLIND_CODE")
		}
		seen[b] = true
	}
	return reasons
}

// SortedSampleTimes returns a sorted copy of the locked sample times.
func (s LockSpec) SortedSampleTimes() []int64 {
	times := append([]int64(nil), s.SampleTimes...)
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	return times
}

// BuildSnapshot freezes the lock input into a snapshot using the selected rule
// revision's summary hash. The snapshot is the immutable reference every later
// stage validates against.
func BuildSnapshot(spec LockSpec, summaryHash string) LockSnapshot {
	snap := LockSnapshot{
		TaskID:          spec.TaskID,
		Generation:      1,
		BaseLotID:       spec.BaseLotID,
		SealID:          spec.SealID,
		PrecoolLot:      spec.PrecoolLot,
		CutLineID:       spec.CutLineID,
		WashTankID:      spec.WashTankID,
		FormulaID:       spec.FormulaID,
		FormulaRevision: spec.FormulaRevision,
		SummaryHash:     summaryHash,
		SampleTimes:     spec.SortedSampleTimes(),
		BlindCodes:      append([]string(nil), spec.BlindCodes...),
		ATPPoints:       append([]string(nil), spec.ATPPoints...),
		PlateWells:      append([]string(nil), spec.PlateWells...),
		DrainSlots:      append([]string(nil), spec.DrainSlots...),
		Reviewers:       append([]string(nil), spec.Reviewers...),
	}
	return snap
}

// SnapshotFromJSON decodes a persisted locked snapshot.
func SnapshotFromJSON(b []byte) (LockSnapshot, error) {
	var s LockSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return LockSnapshot{}, err
	}
	return s, nil
}

// SnapshotJSON encodes a locked snapshot for persistence.
func SnapshotJSON(s LockSnapshot) ([]byte, error) {
	return json.Marshal(s)
}

// Snapshot returns the task's decoded locked snapshot.
func (t *InspectionTask) Snapshot() (LockSnapshot, error) {
	return SnapshotFromJSON(t.LockedSnapshotJSON)
}

// HasSampleTime reports whether the given sample time is one of the locked
// sample times.
func (s LockSnapshot) HasSampleTime(ts int64) bool {
	for _, v := range s.SampleTimes {
		if v == ts {
			return true
		}
	}
	return false
}

// HasBlindCode reports whether the blind code is locked on this snapshot.
func (s LockSnapshot) HasBlindCode(code string) bool {
	for _, b := range s.BlindCodes {
		if b == code {
			return true
		}
	}
	return false
}

// HasATPPoint reports whether the ATP point is locked on this snapshot.
func (s LockSnapshot) HasATPPoint(code string) bool {
	for _, p := range s.ATPPoints {
		if p == code {
			return true
		}
	}
	return false
}

// HasPlateWell reports whether the plate well is locked on this snapshot.
func (s LockSnapshot) HasPlateWell(code string) bool {
	for _, w := range s.PlateWells {
		if w == code {
			return true
		}
	}
	return false
}
