package mutator

import (
	"go/ast"
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

func mustParseTestSrc(t *testing.T) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", testSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	return fset, file
}

func TestComparisonMutator_Name(t *testing.T) {
	m := &ComparisonMutator{}
	if m.Name() != "comparison-boundary" {
		t.Errorf("expected name 'comparison-boundary', got %q", m.Name())
	}
}

func TestComparisonMutator_Discover(t *testing.T) {
	fset, file := mustParseTestSrc(t)

	m := &ComparisonMutator{}
	points := m.Discover(fset, file, "/fake/test.go", "example")

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

func TestComparisonMutator_Discover_SkipsLenCapZero(t *testing.T) {
	src := `package example

func f(s []int) {
	if len(s) > 0 {}
	if len(s) >= 0 {}
	if len(s) < 0 {}
	if len(s) <= 0 {}
	if cap(s) > 0 {}
	if 0 < len(s) {}
	if len(s) > 1 {}
	if len(s) > len(s) {}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	m := &ComparisonMutator{}
	points := m.Discover(fset, file, "/fake/test.go", "example")

	// Skipped (6): len(s)>0, len(s)>=0, len(s)<0, len(s)<=0, cap(s)>0, 0<len(s)
	// Kept (2): len(s)>1 (non-zero literal), len(s)>len(s) (no literal 0)
	if len(points) != 2 {
		t.Errorf("expected 2 mutation points, got %d", len(points))
		for i, p := range points {
			t.Logf("  point[%d]: line %d col %d %s", i, p.Line, p.Column, p.Desc)
		}
	}
}

func TestComparisonMutator_Apply(t *testing.T) {
	m := &ComparisonMutator{}

	fset, file := mustParseTestSrc(t)
	points := m.Discover(fset, file, "/fake/test.go", "example")
	if len(points) == 0 {
		t.Fatal("no points found")
	}

	// Apply first mutation (> to >=) on a fresh AST and verify via re-discover
	fset2, file2 := mustParseTestSrc(t)
	m.Apply(file2, points[0])

	mutatedPoints := m.Discover(fset2, file2, "/fake/test.go", "example")
	if len(mutatedPoints) != 4 {
		t.Fatalf("expected 4 points after mutation, got %d", len(mutatedPoints))
	}
	if mutatedPoints[0].Original != token.GEQ {
		t.Errorf("after mutation, expected first operator to be >=, got %s", mutatedPoints[0].Original)
	}
}

func TestComparisonMutator_Apply_EachPoint(t *testing.T) {
	m := &ComparisonMutator{}

	fset, file := mustParseTestSrc(t)
	points := m.Discover(fset, file, "/fake/test.go", "example")

	for idx, pt := range points {
		fsetN, fileN := mustParseTestSrc(t)
		m.Apply(fileN, pt)

		mutatedPoints := m.Discover(fsetN, fileN, "/fake/test.go", "example")
		if len(mutatedPoints) != 4 {
			t.Fatalf("point[%d]: expected 4 points, got %d", idx, len(mutatedPoints))
		}
		if mutatedPoints[idx].Original != pt.Mutated {
			t.Errorf("point[%d]: expected mutated operator %s, got %s", idx, pt.Mutated, mutatedPoints[idx].Original)
		}
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

	// Parse twice independently and verify NodeIDs match
	fset1, file1 := mustParseTestSrc(t)
	points1 := m.Discover(fset1, file1, "/fake/test.go", "example")

	fset2, file2 := mustParseTestSrc(t)
	points2 := m.Discover(fset2, file2, "/fake/test.go", "example")

	if len(points1) != len(points2) {
		t.Fatalf("different number of points between runs: %d vs %d", len(points1), len(points2))
	}
	for i := range points1 {
		if points1[i].NodeID != points2[i].NodeID {
			t.Errorf("point[%d]: NodeID %d vs %d between runs", i, points1[i].NodeID, points2[i].NodeID)
		}
	}
}
