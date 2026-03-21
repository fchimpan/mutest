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

func TestAbs(t *testing.T) {
	if Abs(5) != 5 {
		t.Error("Abs(5) should be 5")
	}
	if Abs(-5) != 5 {
		t.Error("Abs(-5) should be 5")
	}
	if Abs(0) != 0 {
		t.Error("Abs(0) should be 0")
	}
}

func TestSign(t *testing.T) {
	if Sign(5) != 1 {
		t.Error("Sign(5) should be 1")
	}
	if Sign(-5) != -1 {
		t.Error("Sign(-5) should be -1")
	}
	if Sign(0) != 0 {
		t.Error("Sign(0) should be 0")
	}
}

func TestEqual(t *testing.T) {
	if !Equal(3, 3) {
		t.Error("Equal(3,3) should be true")
	}
	if Equal(3, 4) {
		t.Error("Equal(3,4) should be false")
	}
}

func TestInRange(t *testing.T) {
	if !InRange(5, 1, 10) {
		t.Error("InRange(5,1,10) should be true")
	}
	if InRange(0, 1, 10) {
		t.Error("InRange(0,1,10) should be false")
	}
	if InRange(11, 1, 10) {
		t.Error("InRange(11,1,10) should be false")
	}
	if !InRange(1, 1, 10) {
		t.Error("InRange(1,1,10) should be true")
	}
	if !InRange(10, 1, 10) {
		t.Error("InRange(10,1,10) should be true")
	}
}

func TestClassify(t *testing.T) {
	if Classify(5) != "positive" {
		t.Error("Classify(5) should be positive")
	}
	if Classify(-5) != "negative" {
		t.Error("Classify(-5) should be negative")
	}
	if Classify(0) != "zero" {
		t.Error("Classify(0) should be zero")
	}
}

func TestMinMax(t *testing.T) {
	min, max := MinMax(3, 5)
	if min != 3 || max != 5 {
		t.Errorf("MinMax(3,5) = (%d,%d), want (3,5)", min, max)
	}
	min, max = MinMax(5, 3)
	if min != 3 || max != 5 {
		t.Errorf("MinMax(5,3) = (%d,%d), want (3,5)", min, max)
	}
	min, max = MinMax(3, 3)
	if min != 3 || max != 3 {
		t.Errorf("MinMax(3,3) = (%d,%d), want (3,3)", min, max)
	}
}
