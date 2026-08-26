package evidence

import "testing"

func TestParseFixed(t *testing.T) {
	cases := []struct {
		in    string
		scale int
		want  int64
	}{
		{"1.50", 2, 150},
		{"0.07", 2, 7},
		{"12", 2, 1200},
		{"12.345", 3, 12345},
	}
	for _, c := range cases {
		got, err := ParseFixed(c.in, c.scale)
		if err != nil {
			t.Errorf("ParseFixed(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseFixed(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseFixedRejects(t *testing.T) {
	if _, err := ParseFixed("1.234", 2); err != ErrTooManyDigits {
		t.Errorf("expected ErrTooManyDigits, got %v", err)
	}
	if _, err := ParseFixed("abc", 2); err != ErrInvalidFormat {
		t.Errorf("expected ErrInvalidFormat, got %v", err)
	}
	if _, err := ParseFixed("", 2); err != ErrInvalidFormat {
		t.Errorf("expected ErrInvalidFormat for empty, got %v", err)
	}
}

func TestHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		a, b, want int64
	}{
		{5, 2, 3},   // 2.5 -> 3
		{-5, 2, -3}, // -2.5 -> -3
		{4, 2, 2},
		{0, 5, 0},
	}
	for _, c := range cases {
		got, err := HalfAwayFromZero(c.a, c.b)
		if err != nil {
			t.Errorf("HalfAwayFromZero(%d,%d): %v", c.a, c.b, err)
			continue
		}
		if got != c.want {
			t.Errorf("HalfAwayFromZero(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if _, err := HalfAwayFromZero(1, 0); err != ErrDivisionByZero {
		t.Errorf("expected ErrDivisionByZero, got %v", err)
	}
}

func TestRejectNegative(t *testing.T) {
	if err := RejectNegative(-1); err != ErrNegativeValue {
		t.Errorf("expected ErrNegativeValue, got %v", err)
	}
	if err := RejectNegative(0); err != nil {
		t.Errorf("zero should be allowed, got %v", err)
	}
}
