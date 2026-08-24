package arbiter

import (
	"math"
	"testing"

	"leafwash-packaging-release-gate/evidence"
)

func TestDeriveCFUPer100ML(t *testing.T) {
	// 30 colonies * 10 dilution * 100 / 1 mL = 30000 CFU/100 mL.
	got, err := DeriveCFUPer100ML(30, 10, 1)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got != 30000 {
		t.Fatalf("got %d, want 30000", got)
	}
	// 5 colonies * 2 dilution * 100 / 3 mL = 333.33 -> 333 (half away from zero).
	got, err = DeriveCFUPer100ML(5, 2, 3)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got != 333 {
		t.Fatalf("got %d, want 333", got)
	}
}

func TestDeriveCFUDivisionByZero(t *testing.T) {
	if _, err := DeriveCFUPer100ML(5, 2, 0); err != evidence.ErrDivisionByZero {
		t.Fatalf("expected ErrDivisionByZero, got %v", err)
	}
}

func TestDeriveCFUOverflow(t *testing.T) {
	if _, err := DeriveCFUPer100ML(math.MaxInt64, 2, 1); err != evidence.ErrInt64Overflow {
		t.Fatalf("expected ErrInt64Overflow, got %v", err)
	}
}

func TestDeriveCFUNegative(t *testing.T) {
	if _, err := DeriveCFUPer100ML(-1, 2, 1); err != evidence.ErrNegativeValue {
		t.Fatalf("expected ErrNegativeValue, got %v", err)
	}
}
