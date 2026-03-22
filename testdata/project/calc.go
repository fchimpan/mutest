package testproject

// Max returns the larger of a or b.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// IsPositive returns true if n is strictly positive.
func IsPositive(n int) bool {
	return n > 0
}

// Clamp restricts v to the range [lo, hi].
func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Abs returns the absolute value of n.
func Abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Sign returns -1, 0, or 1 depending on the sign of n.
func Sign(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}

// Equal returns true if a and b are equal.
func Equal(a, b int) bool {
	return a == b
}

// InRange returns true if v is in [lo, hi].
func InRange(v, lo, hi int) bool {
	return v >= lo && v <= hi
}

// Classify returns "negative", "zero", or "positive".
func Classify(n int) string {
	if n == 0 {
		return "zero"
	}
	if n > 0 {
		return "positive"
	}
	return "negative"
}

// MinMax returns the minimum and maximum of a and b.
func MinMax(a, b int) (int, int) {
	if a <= b {
		return a, b
	}
	return b, a
}
