package arbiter

import (
	"math"

	"leafwash-packaging-release-gate/evidence"
)

// DeriveCFUPer100ML converts a raw culture-plate colony count, dilution factor,
// and sample volume (mL) into a colony count per 100 mL using half-away-from-
// zero rounding. It checks length (sign), division by zero, and int64 overflow
// so that a failed derivation never produces accepted evidence (acceptance #5).
//
// The derivation is: cfu_per_100ml = round_half_away(colony * dilution * 100 / volume).
func DeriveCFUPer100ML(colonyCFU, dilution, sampleVolumeML int64) (int64, error) {
	if colonyCFU < 0 || dilution < 0 || sampleVolumeML < 0 {
		return 0, evidence.ErrNegativeValue
	}
	if sampleVolumeML == 0 {
		return 0, evidence.ErrDivisionByZero
	}
	// colony * dilution, overflow-checked.
	num, err := mulChecked(colonyCFU, dilution)
	if err != nil {
		return 0, err
	}
	// * 100, overflow-checked.
	num, err = mulChecked(num, 100)
	if err != nil {
		return 0, err
	}
	return evidence.HalfAwayFromZero(num, sampleVolumeML)
}

// ColonyReading is the validated microbiology input.
type ColonyReading struct {
	ColonyCFU      int64
	Dilution       int64
	SampleVolumeML int64
}

// ValidateColonyReading checks the reading's shape and sign and returns stable
// reason codes. A positive colony reading flags a suspected-positive anomaly.
func ValidateColonyReading(r ColonyReading) (reasons []string) {
	if r.ColonyCFU < 0 {
		reasons = append(reasons, "NEGATIVE_COLONY")
	}
	if r.Dilution < 0 {
		reasons = append(reasons, "NEGATIVE_DILUTION")
	}
	if r.SampleVolumeML <= 0 {
		reasons = append(reasons, "INVALID_SAMPLE_VOLUME")
	}
	return reasons
}

// ExceedsColonyLimit reports whether the derived CFU/100 mL exceeds the rule's
// colony maximum.
func ExceedsColonyLimit(derived, colonyMaxCFUX100ml int64) bool {
	return derived > colonyMaxCFUX100ml
}

// mulChecked multiplies two int64 values and detects overflow.
func mulChecked(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > 0 {
		if b > 0 {
			if a > math.MaxInt64/b {
				return 0, evidence.ErrInt64Overflow
			}
		} else {
			if b < math.MinInt64/a {
				return 0, evidence.ErrInt64Overflow
			}
		}
	} else {
		if b > 0 {
			if a < math.MinInt64/b {
				return 0, evidence.ErrInt64Overflow
			}
		} else {
			if a != 0 && b < math.MaxInt64/a {
				return 0, evidence.ErrInt64Overflow
			}
		}
	}
	return a * b, nil
}
