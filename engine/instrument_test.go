package engine

import (
	"go/token"
	"strings"
	"testing"

	"github.com/fchimpan/mutest/mutator"
)

func TestInstrumentFile_NestedBinaryExpr(t *testing.T) {
	// (a > b) == flag: both > and == should be instrumented.
	// The inner > call is embedded in the outer == call's LHS.
	src := []byte(`package repro

func Foo(a, b int, flag bool) bool {
	return (a > b) == flag
}
`)

	// ast.Inspect pre-order: outer == (nodeID=0), inner > (nodeID=1)
	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 2,
			Desc:     "== to !=",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   1,
			Original: token.GTR,
			Mutated:  token.GEQ,
			MutestID: 1,
			Desc:     "> to >=",
		},
	}

	out, helpers, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)

	// Both mutations should be instrumented.
	if !strings.Contains(result, "_mutest_eq_2") {
		t.Errorf("expected outer == to be instrumented, got:\n%s", result)
	}
	if !strings.Contains(result, "_mutest_cmp_1") {
		t.Errorf("expected inner > to be instrumented (nested in outer), got:\n%s", result)
	}

	// The inner call should appear inside the outer call's LHS argument.
	if !strings.Contains(result, "_mutest_eq_2(_mutest_cmp_1(a, b), flag)") &&
		!strings.Contains(result, "_mutest_eq_2((_mutest_cmp_1(a, b)), flag)") {
		t.Errorf("expected inner > call nested in outer == call, got:\n%s", result)
	}

	if len(helpers) != 2 {
		t.Errorf("expected 2 helpers, got %d", len(helpers))
	}
}

func TestInstrumentFile_DoubleNestedNil(t *testing.T) {
	// (x != nil) == (y != nil): two != nested inside ==.
	// All three mutations should be instrumented.
	src := []byte(`package repro

func Bar(x, y *int) bool {
	return (x != nil) == (y != nil)
}
`)

	// ast.Inspect pre-order: outer == (0), left != (1), right != (2)
	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 2,
			Desc:     "== to !=",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   1,
			Original: token.NEQ,
			Mutated:  token.EQL,
			MutestID: 1,
			Desc:     "!= to ==",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   2,
			Original: token.NEQ,
			Mutated:  token.EQL,
			MutestID: 3,
			Desc:     "!= to ==",
		},
	}

	out, helpers, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)

	// All three should be present.
	// Inner != are nil comparisons → inline funcs with _mutest_active == N.
	if !strings.Contains(result, "_mutest_active == 1") {
		t.Errorf("expected left != (ID=1) to be instrumented, got:\n%s", result)
	}
	if !strings.Contains(result, "_mutest_active == 3") {
		t.Errorf("expected right != (ID=3) to be instrumented, got:\n%s", result)
	}
	// Outer == is a non-nil comparison → helper func _mutest_eq_2.
	if !strings.Contains(result, "_mutest_eq_2") {
		t.Errorf("expected outer == (ID=2) to be instrumented as _mutest_eq_2, got:\n%s", result)
	}

	if len(helpers) != 3 {
		t.Errorf("expected 3 helpers, got %d", len(helpers))
	}
}

func TestInstrumentFile_TripleNested(t *testing.T) {
	// ((a > b) == flag) != expected: three levels of nesting.
	src := []byte(`package repro

func Baz(a, b int, flag, expected bool) bool {
	return ((a > b) == flag) != expected
}
`)

	// ast.Inspect pre-order: != (0), == (1), > (2)
	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.NEQ,
			Mutated:  token.EQL,
			MutestID: 3,
			Desc:     "!= to ==",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   1,
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 2,
			Desc:     "== to !=",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   2,
			Original: token.GTR,
			Mutated:  token.GEQ,
			MutestID: 1,
			Desc:     "> to >=",
		},
	}

	out, helpers, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)

	// All three mutations should be present.
	if !strings.Contains(result, "_mutest_cmp_1") {
		t.Errorf("expected innermost > to be instrumented, got:\n%s", result)
	}
	if !strings.Contains(result, "_mutest_eq_2") {
		t.Errorf("expected middle == to be instrumented, got:\n%s", result)
	}
	if !strings.Contains(result, "_mutest_eq_3") {
		t.Errorf("expected outermost != to be instrumented, got:\n%s", result)
	}

	// Verify nesting: cmp_1 inside eq_2 inside eq_3
	if !strings.Contains(result, "_mutest_eq_2(_mutest_cmp_1(a, b), flag)") &&
		!strings.Contains(result, "_mutest_eq_2((_mutest_cmp_1(a, b)), flag)") {
		t.Errorf("expected cmp_1 nested inside eq_2, got:\n%s", result)
	}

	if len(helpers) != 3 {
		t.Errorf("expected 3 helpers, got %d", len(helpers))
	}
}

func TestInstrumentFile_NoNesting(t *testing.T) {
	// a > b without nesting should still work as before.
	src := []byte(`package repro

func Simple(a, b int) bool {
	return a > b
}
`)

	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.GTR,
			Mutated:  token.GEQ,
			MutestID: 1,
			Desc:     "> to >=",
		},
	}

	out, helpers, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, "_mutest_cmp_1(a, b)") {
		t.Errorf("expected simple replacement, got:\n%s", result)
	}
	if len(helpers) != 1 {
		t.Errorf("expected 1 helper, got %d", len(helpers))
	}
}
