package mutator

import (
	"go/token"
	"testing"
)

func TestConditionalMutator_Name(t *testing.T) {
	m := NewConditionalMutator()
	if m.Name() != "conditional" {
		t.Errorf("Name() = %q, want %q", m.Name(), "conditional")
	}
}

func TestConditionalMutator_Mutate(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantN   int
		wantOps []string
	}{
		{
			name:    "equal to not-equal",
			src:     `package p; var x = 1 == 2`,
			wantN:   1,
			wantOps: []string{"!="},
		},
		{
			name:    "not-equal to equal",
			src:     `package p; var x = 1 != 2`,
			wantN:   1,
			wantOps: []string{"=="},
		},
		{
			name:    "less-than produces two mutations",
			src:     `package p; var x = 1 < 2`,
			wantN:   2,
			wantOps: []string{"<=", ">="},
		},
		{
			name:    "greater-than produces two mutations",
			src:     `package p; var x = 1 > 2`,
			wantN:   2,
			wantOps: []string{">=", "<="},
		},
		{
			name:    "less-equal produces two mutations",
			src:     `package p; var x = 1 <= 2`,
			wantN:   2,
			wantOps: []string{"<", ">"},
		},
		{
			name:    "greater-equal produces two mutations",
			src:     `package p; var x = 1 >= 2`,
			wantN:   2,
			wantOps: []string{">", "<"},
		},
		{
			name:  "arithmetic operator not matched",
			src:   `package p; var x = 1 + 2`,
			wantN: 0,
		},
	}

	m := NewConditionalMutator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset, file := mustParse(t, tt.src)
			mutations := collectMutations(t, m, fset, file)
			if len(mutations) != tt.wantN {
				t.Fatalf("got %d mutations, want %d", len(mutations), tt.wantN)
			}
			for i, wantOp := range tt.wantOps {
				if mutations[i].Mutated != wantOp {
					t.Errorf("mutation[%d].Mutated = %q, want %q", i, mutations[i].Mutated, wantOp)
				}
			}
		})
	}
}

func TestConditionalMutator_ExtendedSwapTable(t *testing.T) {
	// Demonstrate extensibility: < → all other comparison operators
	m := &ConditionalMutator{
		Swaps: SwapTable{
			token.LSS: {token.LEQ, token.GTR, token.GEQ, token.EQL, token.NEQ},
		},
	}
	fset, file := mustParse(t, `package p; var x = 1 < 2`)
	mutations := collectMutations(t, m, fset, file)
	if len(mutations) != 5 {
		t.Fatalf("extended swap: got %d mutations, want 5", len(mutations))
	}
	expected := []string{"<=", ">", ">=", "==", "!="}
	for i, e := range expected {
		if mutations[i].Mutated != e {
			t.Errorf("mutation[%d].Mutated = %q, want %q", i, mutations[i].Mutated, e)
		}
	}
}
