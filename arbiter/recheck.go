package arbiter

import (
	"leafwash-packaging-release-gate/evidence"
)

// RecheckTargets is the set of affected coordinates a recheck evidence must
// cover: the affected sample times, blind codes, ATP point codes, and plate
// wells (acceptance #7).
type RecheckTargets struct {
	SampleTimes []int64  `json:"sample_times"`
	BlindCodes  []string `json:"blind_codes"`
	PointCodes  []string `json:"point_codes"`
	PlateWells  []string `json:"plate_wells"`
}

// CoversRequired reports whether the submitted recheck targets cover every
// required coordinate. Each required element must appear in the submitted set;
// extra submitted coordinates are tolerated but not required.
func (t RecheckTargets) CoversRequired(required RecheckTargets) bool {
	for _, ts := range required.SampleTimes {
		if !containsInt64(t.SampleTimes, ts) {
			return false
		}
	}
	for _, b := range required.BlindCodes {
		if !containsStr(t.BlindCodes, b) {
			return false
		}
	}
	for _, p := range required.PointCodes {
		if !containsStr(t.PointCodes, p) {
			return false
		}
	}
	for _, w := range required.PlateWells {
		if !containsStr(t.PlateWells, w) {
			return false
		}
	}
	return true
}

// IsEmpty reports whether the recheck targets carry no coordinates.
func (t RecheckTargets) IsEmpty() bool {
	return len(t.SampleTimes) == 0 && len(t.BlindCodes) == 0 &&
		len(t.PointCodes) == 0 && len(t.PlateWells) == 0
}

// AnomalyReason maps a detected anomaly to the stable reason code used by the
// recheck flow. It identifies chlorine discontinuity, ATP over-limit, colony
// suspected-positive, or wash-water physchem out-of-range.
type AnomalyReason string

const (
	AnomalyChlorineBreak AnomalyReason = "CHLORINE_BREAK"
	AnomalyATPOverLimit  AnomalyReason = "ATP_OVER_LIMIT"
	AnomalyColonySusp    AnomalyReason = "COLONY_SUSPECTED"
	AnomalyPhyschemRange AnomalyReason = "PHYSCHEM_OUT_OF_RANGE"
)

// IsAnomaly reports whether the string is a recognized anomaly reason.
func IsAnomaly(s string) bool {
	switch AnomalyReason(s) {
	case AnomalyChlorineBreak, AnomalyATPOverLimit, AnomalyColonySusp, AnomalyPhyschemRange:
		return true
	}
	return false
}

// EvidenceForKind filters evidence versions by kind.
func EvidenceForKind(evs []evidence.EvidenceVersion, kind evidence.EvidenceKind) []evidence.EvidenceVersion {
	var out []evidence.EvidenceVersion
	for _, e := range evs {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func containsInt64(xs []int64, v int64) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func containsStr(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
