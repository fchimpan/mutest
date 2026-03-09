package mutation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/fchimpan/mutest/internal/mutator"
)

func mustParse(t *testing.T, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return fset, file
}

func TestNewGenerator(t *testing.T) {
	g := NewGenerator(nil)
	if g == nil {
		t.Fatal("NewGenerator(nil) returned nil")
	}
	if g.mutators != nil {
		t.Errorf("expected nil mutators, got %v", g.mutators)
	}
}

func TestGenerator_Generate_NoMutators(t *testing.T) {
	g := NewGenerator(nil)
	fset, file := mustParse(t, `package p; var x = 1 + 2`)
	mutations := g.Generate(fset, "test.go", file)
	if len(mutations) != 0 {
		t.Errorf("expected 0 mutations with no mutators, got %d", len(mutations))
	}
}

func TestGenerator_Generate_EmptyMutators(t *testing.T) {
	g := NewGenerator([]mutator.Mutator{})
	fset, file := mustParse(t, `package p; var x = 1 + 2`)
	mutations := g.Generate(fset, "test.go", file)
	if len(mutations) != 0 {
		t.Errorf("expected 0 mutations with empty mutators, got %d", len(mutations))
	}
}

func TestGenerator_Generate_Arithmetic(t *testing.T) {
	g := NewGenerator([]mutator.Mutator{mutator.NewArithmeticMutator()})
	fset, file := mustParse(t, `package p; var x = 1 + 2`)
	mutations := g.Generate(fset, "test.go", file)
	if len(mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(mutations))
	}
	m := mutations[0]
	if m.Original != "+" {
		t.Errorf("Original = %q, want %q", m.Original, "+")
	}
	if m.Mutated != "-" {
		t.Errorf("Mutated = %q, want %q", m.Mutated, "-")
	}
	if m.MutatorName != "arithmetic" {
		t.Errorf("MutatorName = %q, want %q", m.MutatorName, "arithmetic")
	}
}

func TestGenerator_Generate_IDFormat(t *testing.T) {
	g := NewGenerator([]mutator.Mutator{mutator.NewArithmeticMutator()})
	fset, file := mustParse(t, `package p; var x = 1 + 2`)
	mutations := g.Generate(fset, "test.go", file)
	if len(mutations) < 1 {
		t.Fatal("expected at least 1 mutation")
	}
	id := mutations[0].ID
	// ID format: "base_filename:line:col:mutator_name"
	if !strings.HasPrefix(id, "test.go:") {
		t.Errorf("ID should start with 'test.go:', got %q", id)
	}
	if !strings.HasSuffix(id, ":arithmetic") {
		t.Errorf("ID should end with ':arithmetic', got %q", id)
	}
	parts := strings.Split(id, ":")
	if len(parts) != 4 {
		t.Errorf("ID should have 4 parts separated by ':', got %d parts: %q", len(parts), id)
	}
}

func TestGenerator_Generate_MultipleMutators(t *testing.T) {
	g := NewGenerator([]mutator.Mutator{
		mutator.NewArithmeticMutator(),
		mutator.NewConditionalMutator(),
	})
	src := `package p; func f(a, b int) bool { return a + b > 0 }`
	fset, file := mustParse(t, src)
	mutations := g.Generate(fset, "test.go", file)

	hasArithmetic := false
	hasConditional := false
	for _, m := range mutations {
		if m.MutatorName == "arithmetic" {
			hasArithmetic = true
		}
		if m.MutatorName == "conditional" {
			hasConditional = true
		}
	}
	if !hasArithmetic {
		t.Error("expected arithmetic mutations")
	}
	if !hasConditional {
		t.Error("expected conditional mutations")
	}
}

func TestGenerator_Generate_NoMutableNodes(t *testing.T) {
	g := NewGenerator([]mutator.Mutator{mutator.NewArithmeticMutator()})
	fset, file := mustParse(t, `package p; var x = 42`)
	mutations := g.Generate(fset, "test.go", file)
	if len(mutations) != 0 {
		t.Errorf("expected 0 mutations for simple assignment, got %d", len(mutations))
	}
}

func TestGenerator_Generate_MultipleExpressions(t *testing.T) {
	g := NewGenerator([]mutator.Mutator{mutator.NewArithmeticMutator()})
	src := `package p; func f() int { return 1 + 2 + 3 }`
	fset, file := mustParse(t, src)
	mutations := g.Generate(fset, "test.go", file)
	// Two + operators → 2 mutations
	if len(mutations) != 2 {
		t.Fatalf("expected 2 mutations, got %d", len(mutations))
	}
	for _, m := range mutations {
		if m.Original != "+" || m.Mutated != "-" {
			t.Errorf("unexpected mutation: %q → %q", m.Original, m.Mutated)
		}
	}
}

func TestGenerator_Generate_AllIDs_NonEmpty(t *testing.T) {
	g := NewGenerator(mutator.DefaultMutators())
	src := `package p
func f(a, b int) bool {
	if a + b > 0 && a - b < 10 {
		return !false
	}
	return true
}
`
	fset, file := mustParse(t, src)
	mutations := g.Generate(fset, "test.go", file)
	if len(mutations) == 0 {
		t.Fatal("expected mutations but got 0")
	}
	for _, m := range mutations {
		if m.ID == "" {
			t.Error("mutation ID is empty")
		}
		// ID format: "filename:line:col:mutator_name"
		parts := strings.Split(m.ID, ":")
		if len(parts) != 4 {
			t.Errorf("ID %q should have 4 colon-separated parts", m.ID)
		}
	}
	// Note: IDs may not be unique when the same mutator generates multiple
	// mutations at the same position (e.g., return_value generates zero and
	// non-zero replacements at the same FuncDecl.Body.Pos()).
}

func TestGenerator_Generate_FilePathPropagated(t *testing.T) {
	g := NewGenerator([]mutator.Mutator{mutator.NewArithmeticMutator()})
	fset, file := mustParse(t, `package p; var x = 1 + 2`)
	customPath := "/some/custom/path/foo.go"
	mutations := g.Generate(fset, customPath, file)
	if len(mutations) < 1 {
		t.Fatal("expected at least 1 mutation")
	}
	if mutations[0].File != customPath {
		t.Errorf("File = %q, want %q", mutations[0].File, customPath)
	}
	// ID should use base filename
	if !strings.HasPrefix(mutations[0].ID, "foo.go:") {
		t.Errorf("ID should use base filename, got %q", mutations[0].ID)
	}
}

func TestGenerator_Generate_EmptySource(t *testing.T) {
	g := NewGenerator(mutator.DefaultMutators())
	fset, file := mustParse(t, `package p`)
	mutations := g.Generate(fset, "test.go", file)
	if len(mutations) != 0 {
		t.Errorf("expected 0 mutations for empty source, got %d", len(mutations))
	}
}
