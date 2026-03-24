package testproject

// Nested binary comparison expressions — regression tests for #23.
// These patterns caused a panic in instrumentFile when replacement
// ranges overlapped.

// MatchesBool checks whether (a > b) produces the expected bool.
// Pattern: comparison nested in equality.
func MatchesBool(a, b int, expected bool) bool {
	return (a > b) == expected
}

// BothNonNil returns true when both pointers are non-nil or both are nil.
// Pattern: two nil-checks nested in equality.
func BothNonNil(x, y *int) bool {
	return (x != nil) == (y != nil)
}

// NestedTriple nests three levels: comparison inside equality inside inequality.
func NestedTriple(a, b int, flag, expected bool) bool {
	return ((a > b) == flag) != expected
}

// SideBySide nests two independent comparisons inside an equality.
func SideBySide(a, b, c, d int) bool {
	return (a > b) == (c < d)
}

// CompareInNotEqual nests a <= comparison inside !=.
func CompareInNotEqual(a, b int, expected bool) bool {
	return (a <= b) != expected
}

// NilAndCompare nests a nil-check and a comparison in an equality.
func NilAndCompare(p *int, threshold int) bool {
	if p == nil {
		return false
	}
	return (*p > threshold) == true
}
