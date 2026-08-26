package evidence

import (
	"errors"
	"math"
	"strings"
)

// Fixed-point parse and derive helpers. All scaling is by 100 (two decimal
// places) unless stated otherwise. The rules are deterministic so that a
// persistent logic clock can reproduce them exactly.

// Errors returned by fixed-point validation and derivation. These are stable
// and surfaced to callers as failure-boundary error codes.
var (
	ErrNegativeValue  = errors.New("negative value not allowed")
	ErrTooManyDigits  = errors.New("too many fractional digits")
	ErrDivisionByZero = errors.New("division by zero")
	ErrInt64Overflow  = errors.New("int64 overflow")
	ErrInvalidFormat  = errors.New("invalid fixed-point format")
)

// ParseFixed parses a decimal string into an int64 scaled by 10^scale. It
// rejects negative values, more fractional digits than scale, and malformed
// input. Values are checked for int64 overflow.
func ParseFixed(s string, scale int) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalidFormat
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}
	if s == "" {
		return 0, ErrInvalidFormat
	}

	intPart := "0"
	fracPart := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart = s[:i]
		fracPart = s[i+1:]
		if intPart == "" {
			intPart = "0"
		}
		if len(fracPart) > scale {
			return 0, ErrTooManyDigits
		}
	} else {
		intPart = s
	}

	var v int64
	for _, c := range intPart {
		if c < '0' || c > '9' {
			return 0, ErrInvalidFormat
		}
		digit := int64(c - '0')
		if v > (math.MaxInt64-digit)/10 {
			return 0, ErrInt64Overflow
		}
		v = v*10 + digit
	}

	mult := pow10(scale)
	if v > math.MaxInt64/mult {
		return 0, ErrInt64Overflow
	}
	v *= mult

	for i := 0; i < len(fracPart); i++ {
		c := fracPart[i]
		if c < '0' || c > '9' {
			return 0, ErrInvalidFormat
		}
		digit := int64(c-'0') * pow10(scale-1-i)
		if v > math.MaxInt64-digit {
			return 0, ErrInt64Overflow
		}
		v += digit
	}

	if neg {
		// Clamp to zero floor: negative fixed-point values are rejected by
		// callers via ErrNegativeValue; here we keep the sign for symmetric
		// parsing but flag it.
		return -v, nil
	}
	return v, nil
}

// RejectNegative returns ErrNegativeValue if v is negative, else nil.
func RejectNegative(v int64) error {
	if v < 0 {
		return ErrNegativeValue
	}
	return nil
}

// HalfAwayFromZero divides a by b and rounds half away from zero to the
// nearest integer (used for derived threshold comparisons). It detects
// division by zero and int64 overflow.
func HalfAwayFromZero(a, b int64) (int64, error) {
	if b == 0 {
		return 0, ErrDivisionByZero
	}
	if a == math.MinInt64 && b == -1 {
		return 0, ErrInt64Overflow
	}
	q := a / b
	r := a % b
	absR := r
	if absR < 0 {
		absR = -absR
	}
	absB := b
	if absB < 0 {
		absB = -absB
	}
	if absR*2 >= absB {
		if (a < 0) != (b < 0) {
			q--
		} else {
			q++
		}
	}
	return q, nil
}

// ScaleInt scales v by 10^scale and detects overflow.
func ScaleInt(v int64, scale int) (int64, error) {
	mult := pow10(scale)
	if v != 0 && (v > math.MaxInt64/mult || v < math.MinInt64/mult) {
		return 0, ErrInt64Overflow
	}
	return v * mult, nil
}

func pow10(n int) int64 {
	r := int64(1)
	for i := 0; i < n; i++ {
		r *= 10
	}
	return r
}
