package mutator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

const equalityTestSrc = `package example

func f(a, b int) bool {
	if a == b {
		return true
	}
	if a != b {
		return false
	}
	if a > b {
		return true
	}
	return false
}
`

func mustParseEqualityTestSrc(t *testing.T) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", equalityTestSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	return fset, file
}

func TestEqualityMutator_Name(t *testing.T) {
	m := &EqualityMutator{}
	if m.Name() != "comparison-equality" {
		t.Errorf("expected name 'comparison-equality', got %q", m.Name())
	}
}

func TestEqualityMutator_Discover(t *testing.T) {
	fset, file := mustParseEqualityTestSrc(t)

	m := &EqualityMutator{}
	points := m.Discover(fset, file, "/fake/test.go", "example")

	if len(points) != 2 {
		t.Fatalf("expected 2 mutation points, got %d", len(points))
	}

	expected := []struct {
		orig    token.Token
		mutated token.Token
		desc    string
	}{
		{token.EQL, token.NEQ, "== to !="},
		{token.NEQ, token.EQL, "!= to =="},
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
	}
}

func TestEqualityMutator_Discover_NoTargets(t *testing.T) {
	src := `package example
func f(a, b int) bool { return a > b && a < b }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	m := &EqualityMutator{}
	points := m.Discover(fset, file, "/fake/test.go", "example")

	if len(points) != 0 {
		t.Errorf("expected 0 mutation points for >, <, &&; got %d", len(points))
	}
}

func TestEqualityMutator_Apply(t *testing.T) {
	m := &EqualityMutator{}

	fset, file := mustParseEqualityTestSrc(t)
	points := m.Discover(fset, file, "/fake/test.go", "example")
	if len(points) == 0 {
		t.Fatal("no points found")
	}

	// Apply first mutation (== to !=) on a fresh AST
	fset2, file2 := mustParseEqualityTestSrc(t)
	m.Apply(file2, points[0])

	mutatedPoints := m.Discover(fset2, file2, "/fake/test.go", "example")
	if len(mutatedPoints) != 2 {
		t.Fatalf("expected 2 points after mutation, got %d", len(mutatedPoints))
	}
	if mutatedPoints[0].Original != token.NEQ {
		t.Errorf("after mutation, expected first operator to be !=, got %s", mutatedPoints[0].Original)
	}
}

func TestEqualityMutator_Apply_EachPoint(t *testing.T) {
	m := &EqualityMutator{}

	fset, file := mustParseEqualityTestSrc(t)
	points := m.Discover(fset, file, "/fake/test.go", "example")

	for idx, pt := range points {
		fsetN, fileN := mustParseEqualityTestSrc(t)
		m.Apply(fileN, pt)

		mutatedPoints := m.Discover(fsetN, fileN, "/fake/test.go", "example")
		if len(mutatedPoints) != 2 {
			t.Fatalf("point[%d]: expected 2 points, got %d", idx, len(mutatedPoints))
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
