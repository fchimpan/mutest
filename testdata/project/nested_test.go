package testproject

import "testing"

func TestMatchesBool(t *testing.T) {
	tests := []struct {
		a, b     int
		expected bool
		want     bool
	}{
		{5, 3, true, true},   // 5 > 3 is true, true == true
		{3, 5, true, false},  // 3 > 5 is false, false == true
		{3, 5, false, true},  // 3 > 5 is false, false == false
		{5, 3, false, false}, // 5 > 3 is true, true == false
		// Boundary: equal values
		{3, 3, true, false},  // 3 > 3 is false, false == true
		{3, 3, false, true},  // 3 > 3 is false, false == false
	}
	for _, tt := range tests {
		got := MatchesBool(tt.a, tt.b, tt.expected)
		if got != tt.want {
			t.Errorf("MatchesBool(%d, %d, %v) = %v, want %v", tt.a, tt.b, tt.expected, got, tt.want)
		}
	}
}

func TestBothNonNil(t *testing.T) {
	a, b := 1, 2
	tests := []struct {
		name string
		x, y *int
		want bool
	}{
		{"both non-nil", &a, &b, true},
		{"both nil", nil, nil, true},
		{"x nil", nil, &a, false},
		{"y nil", &a, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BothNonNil(tt.x, tt.y); got != tt.want {
				t.Errorf("BothNonNil = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNestedTriple(t *testing.T) {
	tests := []struct {
		a, b             int
		flag, expected   bool
		want             bool
	}{
		{5, 3, true, false, true},   // (true == true) != false → true
		{5, 3, true, true, false},   // (true == true) != true  → false
		{3, 5, true, false, false},  // (false == true) != false → false
		{3, 5, false, false, true},  // (false == false) != false → true
		// Boundary
		{3, 3, true, false, false},  // (false == true) != false → false
		{3, 3, false, true, false},  // (false == false) != true → true != true → false
	}
	for _, tt := range tests {
		got := NestedTriple(tt.a, tt.b, tt.flag, tt.expected)
		if got != tt.want {
			t.Errorf("NestedTriple(%d, %d, %v, %v) = %v, want %v",
				tt.a, tt.b, tt.flag, tt.expected, got, tt.want)
		}
	}
}

func TestSideBySide(t *testing.T) {
	tests := []struct {
		a, b, c, d int
		want       bool
	}{
		{5, 3, 1, 2, true},  // (true) == (true)
		{3, 5, 2, 1, true},  // (false) == (false)
		{5, 3, 2, 1, false}, // (true) == (false)
		{3, 5, 1, 2, false}, // (false) == (true)
		// Boundary: equal pairs
		{3, 3, 1, 2, false}, // (false) == (true)
		{5, 3, 2, 2, false}, // (true) == (false)
		{3, 3, 2, 2, true},  // (false) == (false)
	}
	for _, tt := range tests {
		got := SideBySide(tt.a, tt.b, tt.c, tt.d)
		if got != tt.want {
			t.Errorf("SideBySide(%d, %d, %d, %d) = %v, want %v",
				tt.a, tt.b, tt.c, tt.d, got, tt.want)
		}
	}
}

func TestCompareInNotEqual(t *testing.T) {
	tests := []struct {
		a, b     int
		expected bool
		want     bool
	}{
		{3, 5, true, false},  // (3 <= 5) is true, true != true → false
		{3, 5, false, true},  // (3 <= 5) is true, true != false → true
		{5, 3, true, true},   // (5 <= 3) is false, false != true → true
		{5, 3, false, false}, // (5 <= 3) is false, false != false → false
		// Boundary
		{3, 3, true, false},  // (3 <= 3) is true, true != true → false
		{3, 3, false, true},  // (3 <= 3) is true, true != false → true
	}
	for _, tt := range tests {
		got := CompareInNotEqual(tt.a, tt.b, tt.expected)
		if got != tt.want {
			t.Errorf("CompareInNotEqual(%d, %d, %v) = %v, want %v",
				tt.a, tt.b, tt.expected, got, tt.want)
		}
	}
}

func TestNilAndCompare(t *testing.T) {
	v5, v0 := 5, 0
	tests := []struct {
		name      string
		p         *int
		threshold int
		want      bool
	}{
		{"nil pointer", nil, 5, false},
		{"above threshold", &v5, 3, true},
		{"below threshold", &v0, 3, false},
		// Boundary
		{"at threshold", &v5, 5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NilAndCompare(tt.p, tt.threshold); got != tt.want {
				t.Errorf("NilAndCompare = %v, want %v", got, tt.want)
			}
		})
	}
}
