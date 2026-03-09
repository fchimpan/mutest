package mutator

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestNegateMutator_Name(t *testing.T) {
	m := NewNegateMutator()
	if m.Name() != "negate_removal" {
		t.Errorf("Name() = %q, want %q", m.Name(), "negate_removal")
	}
}

func TestNegateMutator_Mutate(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantN      int
		wantMutated string
	}{
		{
			name:       "simple negation",
			src:        `package p; var x = !true`,
			wantN:      1,
			wantMutated: "true",
		},
		{
			name:       "negation of identifier",
			src:        `package p; func f(b bool) bool { return !b }`,
			wantN:      1,
			wantMutated: "b",
		},
		{
			name:       "negation of expression",
			src:        `package p; func f(a, b bool) bool { return !(a && b) }`,
			wantN:      1,
			wantMutated: "(a && b)", // format.Node preserves the parentheses
		},
		{
			name:  "double negation produces two mutations",
			src:   `package p; var x = !!true`,
			wantN: 2,
		},
		{
			name:  "unary minus not matched",
			src:   `package p; var x = -1`,
			wantN: 0,
		},
		{
			name:  "unary bitwise-not not matched",
			src:   `package p; var x = ^1`,
			wantN: 0,
		},
		{
			name:  "no unary expr",
			src:   `package p; var x = 42`,
			wantN: 0,
		},
		{
			name:  "non-node",
			src:   `package p`,
			wantN: 0,
		},
	}

	m := NewNegateMutator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset, file := mustParse(t, tt.src)
			mutations := collectMutations(t, m, fset, file)
			if len(mutations) != tt.wantN {
				t.Fatalf("got %d mutations, want %d", len(mutations), tt.wantN)
			}
			if tt.wantMutated != "" && len(mutations) > 0 {
				if mutations[0].Mutated != tt.wantMutated {
					t.Errorf("Mutated = %q, want %q", mutations[0].Mutated, tt.wantMutated)
				}
			}
		})
	}
}

func TestNegateMutator_NonUnaryNode(t *testing.T) {
	m := NewNegateMutator()
	fset := token.NewFileSet()
	node := &ast.Ident{Name: "x"}
	got := m.Mutate(fset, "test.go", node)
	if got != nil {
		t.Errorf("expected nil for non-UnaryExpr, got %v", got)
	}
}
