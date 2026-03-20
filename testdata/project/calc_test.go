package testproject

import "testing"

func TestMax(t *testing.T) {
	// This test catches > to >= because Max(3,3) would return 3 either way,
	// but Max(5,3) must return 5.
	if Max(5, 3) != 5 {
		t.Error("Max(5,3) should be 5")
	}
	if Max(3, 5) != 5 {
		t.Error("Max(3,5) should be 5")
	}
	// Boundary: equal values
	if Max(3, 3) != 3 {
		t.Error("Max(3,3) should be 3")
	}
}

func TestIsPositive(t *testing.T) {
	if !IsPositive(1) {
		t.Error("1 should be positive")
	}
	if IsPositive(0) {
		t.Error("0 should not be positive")
	}
	if IsPositive(-1) {
		t.Error("-1 should not be positive")
	}
}

// TestClamp intentionally does NOT test the boundary v == lo or v == hi,
// so some mutations should survive.
func TestClamp(t *testing.T) {
	if Clamp(5, 1, 10) != 5 {
		t.Error("Clamp(5,1,10) should be 5")
	}
	if Clamp(-5, 1, 10) != 1 {
		t.Error("Clamp(-5,1,10) should be 1")
	}
	if Clamp(15, 1, 10) != 10 {
		t.Error("Clamp(15,1,10) should be 10")
	}
}
