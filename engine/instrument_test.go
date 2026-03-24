package engine

import (
	"go/token"
	"strings"
	"testing"

	"github.com/fchimpan/mutest/mutator"
)

func TestInstrumentFile_NestedBinaryExpr(t *testing.T) {
	// (a > b) == flag has two mutation targets: > and ==.
	// The == replacement fully contains the > replacement.
	// Only the outermost (==) should survive.
	src := []byte(`package repro

func Foo(a, b int, flag bool) bool {
	return (a > b) == flag
}
`)

	// ast.Inspect visits in pre-order: outer == (nodeID=0), inner > (nodeID=1)
	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0, // outer: (a > b) == flag
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 2,
			Desc:     "== to !=",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   1, // inner: a > b
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

	// The outer == should be instrumented.
	if !strings.Contains(result, "_mutest_eq_2") {
		t.Errorf("expected outer == to be instrumented, got:\n%s", result)
	}

	// The inner > should NOT be instrumented (it's nested inside ==).
	if strings.Contains(result, "_mutest_cmp_1") {
		t.Errorf("expected inner > to be skipped (nested), got:\n%s", result)
	}

	// Should have only one helper (the outer ==).
	if len(helpers) != 1 {
		t.Errorf("expected 1 helper, got %d", len(helpers))
	}
}

func TestInstrumentFile_DoubleNestedNil(t *testing.T) {
	// (x != nil) == (y != nil): two != nested inside ==.
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
			NodeID:   0, // outer ==
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 2,
			Desc:     "== to !=",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   1, // x != nil
			Original: token.NEQ,
			Mutated:  token.EQL,
			MutestID: 1,
			Desc:     "!= to ==",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   2, // y != nil
			Original: token.NEQ,
			Mutated:  token.EQL,
			MutestID: 3,
			Desc:     "!= to ==",
		},
	}

	out, _, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)

	// Only the outer == should be instrumented.
	if !strings.Contains(result, "_mutest_eq_2") && !strings.Contains(result, "_mutest_active == 2") {
		t.Errorf("expected outer == to be instrumented, got:\n%s", result)
	}

	// Neither inner != should be instrumented.
	if strings.Contains(result, "_mutest_active == 1") || strings.Contains(result, "_mutest_active == 3") {
		t.Errorf("expected inner != to be skipped (nested), got:\n%s", result)
	}
}

func TestRemoveNestedPairs(t *testing.T) {
	dummyHelper := helperSpec{ID: 0, Kind: "cmp"}

	tests := []struct {
		name string
		in   []replacement
		want int
	}{
		{
			name: "no overlap",
			in: []replacement{
				{start: 0, end: 10, text: "a"},
				{start: 20, end: 30, text: "b"},
			},
			want: 2,
		},
		{
			name: "inner contained in outer",
			in: []replacement{
				{start: 5, end: 10, text: "inner"},
				{start: 0, end: 20, text: "outer"},
			},
			want: 1,
		},
		{
			name: "two inner contained in one outer",
			in: []replacement{
				{start: 2, end: 5, text: "inner1"},
				{start: 0, end: 20, text: "outer"},
				{start: 10, end: 15, text: "inner2"},
			},
			want: 1,
		},
		{
			name: "single replacement",
			in:   []replacement{{start: 0, end: 10, text: "only"}},
			want: 1,
		},
		{
			name: "empty",
			in:   nil,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpers := make([]helperSpec, len(tt.in))
			for i := range helpers {
				helpers[i] = dummyHelper
			}
			gotR, gotH := removeNestedPairs(tt.in, helpers)
			if len(gotR) != tt.want {
				t.Errorf("got %d replacements, want %d", len(gotR), tt.want)
			}
			if len(gotH) != tt.want {
				t.Errorf("got %d helpers, want %d", len(gotH), tt.want)
			}
		})
	}
}
