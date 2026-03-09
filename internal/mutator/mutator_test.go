package mutator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestMutantStatus_String(t *testing.T) {
	tests := []struct {
		status MutantStatus
		want   string
	}{
		{StatusPending, "pending"},
		{StatusKilled, "killed"},
		{StatusSurvived, "survived"},
		{StatusEquivalent, "equivalent"},
		{StatusNotCovered, "not_covered"},
		{StatusTimeout, "timeout"},
		{StatusBuildError, "build_error"},
		{StatusSkipped, "skipped"},
		{MutantStatus(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("MutantStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestMutateBinaryOp(t *testing.T) {
	t.Run("non-BinaryExpr node returns nil", func(t *testing.T) {
		fset := token.NewFileSet()
		node := &ast.Ident{Name: "x"}
		swaps := SwapTable{token.ADD: {token.SUB}}
		got := mutateBinaryOp(fset, "test.go", "test", swaps, node)
		if got != nil {
			t.Errorf("expected nil for non-BinaryExpr, got %v", got)
		}
	})

	t.Run("operator not in swap table returns nil", func(t *testing.T) {
		src := `package p; var x = 1 + 2`
		fset, file := mustParse(t, src)
		swaps := SwapTable{token.MUL: {token.QUO}} // + not in table

		var found bool
		ast.Inspect(file, func(n ast.Node) bool {
			if bin, ok := n.(*ast.BinaryExpr); ok {
				got := mutateBinaryOp(fset, "test.go", "test", swaps, bin)
				if got != nil {
					t.Errorf("expected nil for operator not in table, got %v", got)
				}
				found = true
			}
			return true
		})
		if !found {
			t.Fatal("BinaryExpr not found in AST")
		}
	})

	t.Run("multiple replacements", func(t *testing.T) {
		src := `package p; var x = 1 + 2`
		fset, file := mustParse(t, src)
		swaps := SwapTable{token.ADD: {token.SUB, token.MUL, token.QUO}}

		var mutations []Mutation
		ast.Inspect(file, func(n ast.Node) bool {
			if bin, ok := n.(*ast.BinaryExpr); ok {
				mutations = mutateBinaryOp(fset, "test.go", "test", swaps, bin)
			}
			return true
		})
		if len(mutations) != 3 {
			t.Fatalf("expected 3 mutations, got %d", len(mutations))
		}
		wantOps := []string{"-", "*", "/"}
		for i, m := range mutations {
			if m.Mutated != wantOps[i] {
				t.Errorf("mutation[%d].Mutated = %q, want %q", i, m.Mutated, wantOps[i])
			}
			if m.Original != "+" {
				t.Errorf("mutation[%d].Original = %q, want %q", i, m.Original, "+")
			}
			if m.Status != StatusPending {
				t.Errorf("mutation[%d].Status = %v, want Pending", i, m.Status)
			}
			if m.File != "test.go" {
				t.Errorf("mutation[%d].File = %q, want %q", i, m.File, "test.go")
			}
		}
	})

	t.Run("empty swap table", func(t *testing.T) {
		src := `package p; var x = 1 + 2`
		fset, file := mustParse(t, src)
		swaps := SwapTable{}

		ast.Inspect(file, func(n ast.Node) bool {
			if bin, ok := n.(*ast.BinaryExpr); ok {
				got := mutateBinaryOp(fset, "test.go", "test", swaps, bin)
				if got != nil {
					t.Errorf("expected nil for empty swap table, got %v", got)
				}
			}
			return true
		})
	})

	t.Run("position is correctly recorded", func(t *testing.T) {
		src := "package p\n\nvar x = 1 + 2\n"
		fset, file := mustParse(t, src)
		swaps := SwapTable{token.ADD: {token.SUB}}

		ast.Inspect(file, func(n ast.Node) bool {
			if bin, ok := n.(*ast.BinaryExpr); ok {
				muts := mutateBinaryOp(fset, "test.go", "test", swaps, bin)
				if len(muts) != 1 {
					t.Fatalf("expected 1 mutation, got %d", len(muts))
				}
				if muts[0].Line != 3 {
					t.Errorf("Line = %d, want 3", muts[0].Line)
				}
			}
			return true
		})
	})
}

func mustParse(t *testing.T, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return fset, file
}
