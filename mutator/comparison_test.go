package mutator

import (
	"go/parser"
	"go/token"
	"testing"
)

const testSrc = `package example

func f(a, b int) bool {
	if a > b {
		return true
	}
	if a < b {
		return false
	}
	if a >= b {
		return true
	}
	if a <= b {
		return false
	}
	return a == b
}
`

func TestComparisonMutator_Name(t *testing.T) {
	m := &ComparisonMutator{}
	if m.Name() != "comparison-boundary" {
		t.Errorf("expected name 'comparison-boundary', got %q", m.Name())
	}
}

func TestComparisonMutator_Discover(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", testSrc, 0)
	if err != nil {
		t.Fatal(err)
	}

	m := &ComparisonMutator{}
	points := m.Discover(fset, file, "/fake/test.go", "example")

	// Should find 4 comparison operators: >, <, >=, <=
	// The == on the last line should NOT be found (Tier 1 only).
	if len(points) != 4 {
		t.Fatalf("expected 4 mutation points, got %d", len(points))
	}

	expected := []struct {
		orig    token.Token
		mutated token.Token
		desc    string
	}{
		{token.GTR, token.GEQ, "> to >="},
		{token.LSS, token.LEQ, "< to <="},
		{token.GEQ, token.GTR, ">= to >"},
		{token.LEQ, token.LSS, "<= to <"},
	}

	for i, e := range expected {
		p := points[i]
		if p.Original != e.orig {
			t.Errorf("point[%d]: expected original %s, got %s", i, e.orig, p.Original)
		}
		if p.Mutated != e.mutated {
			t.Errorf("point[%d]: expected mutated %s, got %s", i, e.mutated, p.Mutated)
		}
		if p.Desc != e.desc {
			t.Errorf("point[%d]: expected desc %q, got %q", i, e.desc, p.Desc)
		}
		if p.File != "/fake/test.go" {
			t.Errorf("point[%d]: expected file /fake/test.go, got %s", i, p.File)
		}
		if p.Package != "example" {
			t.Errorf("point[%d]: expected package example, got %s", i, p.Package)
		}
		if p.Line == 0 || p.Column == 0 {
			t.Errorf("point[%d]: expected non-zero line/column, got %d:%d", i, p.Line, p.Column)
		}
	}
}

func TestComparisonMutator_Discover_NoTargets(t *testing.T) {
	src := `package example
func f(a, b int) bool { return a == b || a != b }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	m := &ComparisonMutator{}
	points := m.Discover(fset, file, "/fake/test.go", "example")

	if len(points) != 0 {
		t.Errorf("expected 0 mutation points for ==, !=, ||; got %d", len(points))
	}
}

func TestComparisonMutator_Apply(t *testing.T) {
	m := &ComparisonMutator{}

	// Discover points on original AST
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", testSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	points := m.Discover(fset, file, "/fake/test.go", "example")
	if len(points) == 0 {
		t.Fatal("no points found")
	}

	// Apply first mutation (> to >=) on a fresh AST and verify via re-discover
	fset2 := token.NewFileSet()
	file2, err := parser.ParseFile(fset2, "test.go", testSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.Apply(file2, points[0])

	mutatedPoints := m.Discover(fset2, file2, "/fake/test.go", "example")
	if len(mutatedPoints) != 4 {
		t.Fatalf("expected 4 points after mutation, got %d", len(mutatedPoints))
	}
	// First operator was >, now should be >=
	if mutatedPoints[0].Original != token.GEQ {
		t.Errorf("after mutation, expected first operator to be >=, got %s", mutatedPoints[0].Original)
	}
}

func TestComparisonMutator_Apply_EachPoint(t *testing.T) {
	m := &ComparisonMutator{}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", testSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	points := m.Discover(fset, file, "/fake/test.go", "example")

	// Apply each mutation independently and verify only that point changes
	for idx, pt := range points {
		fsetN := token.NewFileSet()
		fileN, err := parser.ParseFile(fsetN, "test.go", testSrc, 0)
		if err != nil {
			t.Fatal(err)
		}
		m.Apply(fileN, pt)

		mutatedPoints := m.Discover(fsetN, fileN, "/fake/test.go", "example")
		if len(mutatedPoints) != 4 {
			t.Fatalf("point[%d]: expected 4 points, got %d", idx, len(mutatedPoints))
		}
		// The mutated point should now have the opposite operator
		if mutatedPoints[idx].Original != pt.Mutated {
			t.Errorf("point[%d]: expected mutated operator %s, got %s", idx, pt.Mutated, mutatedPoints[idx].Original)
		}
		// Other points should remain unchanged
		for j, other := range mutatedPoints {
			if j == idx {
				continue
			}
			if other.Original != points[j].Original {
				t.Errorf("point[%d]: mutation of point[%d] changed point[%d] from %s to %s",
					idx, idx, j, points[j].Original, other.Original)
			}
		}
	}
}

func TestComparisonMutator_NodeID_Deterministic(t *testing.T) {
	m := &ComparisonMutator{}

	// Parse twice and verify same NodeIDs
	for run := 0; run < 2; run++ {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "test.go", testSrc, 0)
		if err != nil {
			t.Fatal(err)
		}
		points := m.Discover(fset, file, "/fake/test.go", "example")
		if run == 0 {
			continue
		}

		fset2 := token.NewFileSet()
		file2, err := parser.ParseFile(fset2, "test.go", testSrc, 0)
		if err != nil {
			t.Fatal(err)
		}
		points2 := m.Discover(fset2, file2, "/fake/test.go", "example")

		if len(points) != len(points2) {
			t.Fatalf("different number of points between runs: %d vs %d", len(points), len(points2))
		}
		for i := range points {
			if points[i].NodeID != points2[i].NodeID {
				t.Errorf("point[%d]: NodeID %d vs %d between runs", i, points[i].NodeID, points2[i].NodeID)
			}
		}
	}
}
