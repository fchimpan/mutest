package mutator

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestArithmeticMutator_Name(t *testing.T) {
	m := NewArithmeticMutator()
	if m.Name() != "arithmetic" {
		t.Errorf("Name() = %q, want %q", m.Name(), "arithmetic")
	}
}

func TestArithmeticMutator_Mutate(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantN   int
		wantOps []string // expected Mutated values
	}{
		{
			name:    "addition",
			src:     `package p; var x = 1 + 2`,
			wantN:   1,
			wantOps: []string{"-"},
		},
		{
			name:    "subtraction",
			src:     `package p; var x = 1 - 2`,
			wantN:   1,
			wantOps: []string{"+"},
		},
		{
			name:    "multiplication",
			src:     `package p; var x = 1 * 2`,
			wantN:   1,
			wantOps: []string{"/"},
		},
		{
			name:    "division",
			src:     `package p; var x = 1 / 2`,
			wantN:   1,
			wantOps: []string{"*"},
		},
		{
			name:    "modulo",
			src:     `package p; var x = 10 % 3`,
			wantN:   1,
			wantOps: []string{"*"},
		},
		{
			name:  "non-arithmetic operator ignored",
			src:   `package p; var x = 1 == 2`,
			wantN: 0,
		},
		{
			name:  "string concat not matched",
			src:   `package p; var x = "a" + "b"`,
			wantN: 1, // AST-level it's still token.ADD
		},
		{
			name:  "no binary expr",
			src:   `package p; var x = 42`,
			wantN: 0,
		},
		{
			name:  "nested expressions",
			src:   `package p; var x = (1 + 2) * (3 - 4)`,
			wantN: 3, // +→-, *→/, -→+
		},
	}

	m := NewArithmeticMutator()
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

func TestArithmeticMutator_CustomSwapTable(t *testing.T) {
	m := &ArithmeticMutator{
		Swaps: SwapTable{
			token.ADD: {token.SUB, token.MUL, token.QUO, token.REM},
		},
	}
	fset, file := mustParse(t, `package p; var x = 1 + 2`)
	mutations := collectMutations(t, m, fset, file)
	if len(mutations) != 4 {
		t.Fatalf("custom swap: got %d mutations, want 4", len(mutations))
	}
}

func collectMutations(t *testing.T, m Mutator, fset *token.FileSet, file *ast.File) []Mutation {
	t.Helper()
	var all []Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		if n != nil {
			all = append(all, m.Mutate(fset, "test.go", n)...)
		}
		return true
	})
	return all
}
