package evidence

import "sort"

// ChlorineSlopeResult is the derived chlorine decay slope between two adjacent
// locked sample times. It is computed from two fixed-point chlorine readings.
type ChlorineSlopeResult struct {
	FromTime     int64 `json:"from_time"`
	ToTime       int64 `json:"to_time"`
	DeltaX100    int64 `json:"delta_x100"`
	SlopePerUnit int64 `json:"slope_per_unit"`
	ExceedsMax   bool  `json:"exceeds_max"`
}

// ChlorineDecaySlope computes the chlorine decay slope between two readings at
// adjacent locked sample times. The slope is the magnitude of the chlorine drop
// divided by the elapsed time, expressed as an integer with half-away-from-zero
// rounding. It detects division by zero (identical times) and overflow.
func ChlorineDecaySlope(fromTime, toTime, fromChlorine, toChlorine int64) (ChlorineSlopeResult, error) {
	if toTime == fromTime {
		return ChlorineSlopeResult{}, ErrDivisionByZero
	}
	delta := fromChlorine - toChlorine // positive when chlorine decays
	if delta < 0 {
		// A rising chlorine reading is treated as a discontinuity (断点).
		delta = -delta
	}
	elapsed := toTime - fromTime
	if elapsed < 0 {
		elapsed = -elapsed
	}
	slope, err := HalfAwayFromZero(delta*100, elapsed)
	if err != nil {
		return ChlorineSlopeResult{}, err
	}
	return ChlorineSlopeResult{
		FromTime:     fromTime,
		ToTime:       toTime,
		DeltaX100:    fromChlorine - toChlorine,
		SlopePerUnit: slope,
	}, nil
}

// CoverageReport summarizes coverage of the locked sample-time grid.
type CoverageReport struct {
	Total    int     `json:"total"`
	Covered  int     `json:"covered"`
	Missing  []int64 `json:"missing"`
	Complete bool    `json:"complete"`
}

// CoverageOver computes how many locked sample times are covered and which are
// missing. It treats a sample as covering a locked time only when valid.
func CoverageOver(lockedTimes []int64, samples []CoverageSample) CoverageReport {
	covered := map[int64]bool{}
	for _, s := range samples {
		if s.Valid {
			covered[s.SampleTime] = true
		}
	}
	rep := CoverageReport{Total: len(lockedTimes)}
	for _, t := range lockedTimes {
		if covered[t] {
			rep.Covered++
		} else {
			rep.Missing = append(rep.Missing, t)
		}
	}
	sort.Slice(rep.Missing, func(i, j int) bool { return rep.Missing[i] < rep.Missing[j] })
	rep.Complete = rep.Covered == rep.Total
	return rep
}

// AdjacentChlorineSlopes computes the chlorine decay slope between every pair
// of adjacent covered sample times in chronological order. It returns the slope
// results plus a flag indicating whether any slope exceeds the maximum.
func AdjacentChlorineSlopes(lockedTimes []int64, samples []CoverageSample, slopeMaxX100 int64) ([]ChlorineSlopeResult, bool, error) {
	byTime := map[int64]CoverageSample{}
	for _, s := range samples {
		if s.Valid {
			byTime[s.SampleTime] = s
		}
	}
	times := append([]int64(nil), lockedTimes...)
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	var out []ChlorineSlopeResult
	exceeds := false
	for i := 1; i < len(times); i++ {
		prev, ok1 := byTime[times[i-1]]
		cur, ok2 := byTime[times[i]]
		if !ok1 || !ok2 {
			continue
		}
		r, err := ChlorineDecaySlope(prev.SampleTime, cur.SampleTime, prev.ChlorineX100, cur.ChlorineX100)
		if err != nil {
			return nil, false, err
		}
		r.ExceedsMax = r.SlopePerUnit > slopeMaxX100
		if r.ExceedsMax {
			exceeds = true
		}
		out = append(out, r)
	}
	return out, exceeds, nil
}

// ValidateCurveReading checks a single curve reading against the rule
// thresholds. It returns the list of stable reason codes for any violation
// (negative, out-of-range). Chlorine must meet the minimum, ORP the minimum,
// pH must be within [min,max], temperature and turbidity must not exceed their
// maxima.
func ValidateCurveReading(r CurveRule, s CoverageSample) (reasons []string) {
	if s.ChlorineX100 < 0 {
		reasons = append(reasons, "NEGATIVE_CHLORINE")
	} else if s.ChlorineX100 < r.ChlorineMinX100 {
		reasons = append(reasons, "CHLORINE_BELOW_MIN")
	}
	if s.ORPMV < 0 {
		reasons = append(reasons, "NEGATIVE_ORP")
	} else if s.ORPMV < r.ORPMinMV {
		reasons = append(reasons, "ORP_BELOW_MIN")
	}
	if s.PHX100 < 0 {
		reasons = append(reasons, "NEGATIVE_PH")
	} else if s.PHX100 < r.PHMinX100 || s.PHX100 > r.PHMaxX100 {
		reasons = append(reasons, "PH_OUT_OF_RANGE")
	}
	if s.TemperatureX100 < 0 {
		reasons = append(reasons, "NEGATIVE_TEMPERATURE")
	} else if s.TemperatureX100 > r.TemperatureMaxX100 {
		reasons = append(reasons, "TEMPERATURE_TOO_HIGH")
	}
	if s.TurbidityX100 < 0 {
		reasons = append(reasons, "NEGATIVE_TURBIDITY")
	} else if s.TurbidityX100 > r.TurbidityMaxX100 {
		reasons = append(reasons, "TURBIDITY_TOO_HIGH")
	}
	return reasons
}

// CurveRule is the fixed-point threshold set used to validate curve readings.
type CurveRule struct {
	ChlorineMinX100      int64
	ChlorineSlopeMaxX100 int64
	ORPMinMV             int64
	PHMinX100            int64
	PHMaxX100            int64
	TemperatureMaxX100   int64
	TurbidityMaxX100     int64
}
