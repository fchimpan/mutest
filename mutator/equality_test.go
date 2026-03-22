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

func TestEqualityMutator_Discover_SkipsSimpleErrPropagation(t *testing.T) {
	src := `package example

import "fmt"

func g() (int, error) {
	var err error
	// SKIP: simple return err
	if err != nil { return 0, err }
	// SKIP: wrapped error return
	if err != nil { return 0, fmt.Errorf("wrap: %w", err) }
	// SKIP: init form
	if err = doSomething(); err != nil { return 0, err }
	// KEEP: compound condition (Cond is &&, not err != nil)
	var timedOut bool
	if err != nil && !timedOut { return 0, err }
	// KEEP: assignment body, not return
	var rel string
	if err != nil { rel = "fallback" }
	// KEEP: multiple statements
	if err != nil { _ = rel; return 0, err }
	// KEEP: has else branch
	if err != nil { return 0, err } else { _ = rel }
	// KEEP: not "err" identifier
	var myErr error
	if myErr != nil { return 0, myErr }
	return 0, nil
}
func doSomething() error { return nil }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	m := &EqualityMutator{SkipErrPropagation: true}
	points := m.Discover(fset, file, "/fake/test.go", "example")

	// Skipped (3): simple return, wrapped return, init form
	// Kept (5): compound condition (2 ops: != and ==), assignment, multi-stmt, else, myErr
	// compound condition "err != nil && !timedOut" has 1 equality op: err != nil
	// The && is not in equalitySwapTable so it's not counted.
	// Total kept: err!=nil(compound) + err!=nil(assign) + err!=nil(multi) + err!=nil(else) + myErr!=nil = 5
	if len(points) != 5 {
		t.Errorf("expected 5 mutation points (skipping simple err propagation), got %d", len(points))
		for i, p := range points {
			t.Logf("  point[%d]: line %d col %d %s", i, p.Line, p.Column, p.Desc)
		}
	}
}

func TestEqualityMutator_Discover_SkipErrPropagationFlag(t *testing.T) {
	src := `package example

func h() error {
	var err error
	if err != nil { return err }
	if err != nil { return err }
	return nil
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	// SkipErrPropagation=true: skips simple err propagation
	m := &EqualityMutator{SkipErrPropagation: true}
	points := m.Discover(fset, file, "/fake/test.go", "example")
	if len(points) != 0 {
		t.Errorf("SkipErrPropagation=true: expected 0 points, got %d", len(points))
	}

	// SkipErrPropagation=false: includes all
	fset2 := token.NewFileSet()
	file2, err := parser.ParseFile(fset2, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	m2 := &EqualityMutator{SkipErrPropagation: false}
	points2 := m2.Discover(fset2, file2, "/fake/test.go", "example")
	if len(points2) != 2 {
		t.Errorf("SkipErrPropagation=false: expected 2 points, got %d", len(points2))
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
