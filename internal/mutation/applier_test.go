package mutation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/fchimpan/mutest/internal/mutator"
)

func TestApplier_Apply_ArithmeticMutation(t *testing.T) {
	src := []byte("package p\n\nvar x = 1 + 2\n")
	m := mutator.Mutation{
		File:        "test.go",
		Line:        3,
		Col:         11, // OpPos column for "+"
		Original:    "+",
		Mutated:     "-",
		MutatorName: "arithmetic",
	}
	applier := &Applier{}
	result, err := applier.Apply(src, m)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if !strings.Contains(string(result), "1 - 2") {
		t.Errorf("expected '1 - 2' in result, got:\n%s", string(result))
	}
}

func TestApplier_Apply_ConditionalMutation(t *testing.T) {
	src := []byte("package p\n\nvar x = 1 > 2\n")
	m := mutator.Mutation{
		File:        "test.go",
		Line:        3,
		Col:         11, // OpPos column for ">"
		Original:    ">",
		Mutated:     "<=",
		MutatorName: "conditional",
	}
	applier := &Applier{}
	result, err := applier.Apply(src, m)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if !strings.Contains(string(result), "1 <= 2") {
		t.Errorf("expected '1 <= 2' in result, got:\n%s", string(result))
	}
}

func TestApplier_Apply_LogicalMutation(t *testing.T) {
	src := []byte("package p\n\nvar x = true && false\n")
	m := mutator.Mutation{
		File:        "test.go",
		Line:        3,
		Col:         14, // OpPos column for "&&"
		Original:    "&&",
		Mutated:     "||",
		MutatorName: "logical",
	}
	applier := &Applier{}
	result, err := applier.Apply(src, m)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if !strings.Contains(string(result), "true || false") {
		t.Errorf("expected 'true || false' in result, got:\n%s", string(result))
	}
}

func TestApplier_Apply_NegateMutation(t *testing.T) {
	src := []byte("package p\n\nvar x = !true\n")
	m := mutator.Mutation{
		File:        "test.go",
		Line:        3,
		Col:         9,
		Original:    "!true",
		Mutated:     "true",
		MutatorName: "negate_removal",
	}
	applier := &Applier{}
	result, err := applier.Apply(src, m)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	got := string(result)
	// The result should have `var x = true` without the `!`
	if strings.Contains(got, "!true") {
		t.Errorf("expected '!' to be removed, got:\n%s", got)
	}
}

func TestApplier_Apply_StatementRemoval(t *testing.T) {
	src := []byte("package p\n\nfunc f() { var x int; x = 1; _ = x }\n")
	// Need to find the exact line/col for the assignment `x = 1`
	fset, file := mustParse(t, string(src))
	gen := NewGenerator([]mutator.Mutator{mutator.NewStatementMutator()})
	mutations := gen.Generate(fset, "test.go", file)
	if len(mutations) < 1 {
		t.Fatal("expected at least 1 statement mutation")
	}

	applier := &Applier{}
	result, err := applier.Apply(src, mutations[0])
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	// The assignment should be replaced with an empty statement
	got := string(result)
	if strings.Contains(got, "x = 1") {
		t.Errorf("expected 'x = 1' to be removed, got:\n%s", got)
	}
}

func TestApplier_Apply_BranchBodyMutation(t *testing.T) {
	src := []byte("package p\n\nfunc f() {\n\tif true {\n\t\tprintln(\"hi\")\n\t}\n}\n")
	fset, file := mustParse(t, string(src))
	gen := NewGenerator([]mutator.Mutator{mutator.NewBranchBodyMutator()})
	mutations := gen.Generate(fset, "test.go", file)
	if len(mutations) < 1 {
		t.Fatal("expected at least 1 branch body mutation")
	}

	applier := &Applier{}
	result, err := applier.Apply(src, mutations[0])
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	got := string(result)
	if strings.Contains(got, `println("hi")`) {
		t.Errorf("expected body to be emptied, got:\n%s", got)
	}
}

func TestApplier_Apply_LoopBreakMutation(t *testing.T) {
	src := []byte("package p\n\nfunc f() {\n\tfor {\n\t\tbreak\n\t}\n}\n")
	fset, file := mustParse(t, string(src))
	gen := NewGenerator([]mutator.Mutator{mutator.NewLoopBreakMutator()})
	mutations := gen.Generate(fset, "test.go", file)
	if len(mutations) < 1 {
		t.Fatal("expected at least 1 loop break mutation")
	}

	applier := &Applier{}
	result, err := applier.Apply(src, mutations[0])
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	got := string(result)
	if !strings.Contains(got, "continue") {
		t.Errorf("expected 'break' to become 'continue', got:\n%s", got)
	}
}

func TestApplier_Apply_ReturnValueMutation(t *testing.T) {
	src := []byte("package p\n\nfunc f() int {\n\treturn 42\n}\n")
	fset, file := mustParse(t, string(src))
	gen := NewGenerator([]mutator.Mutator{mutator.NewReturnValueMutator()})
	mutations := gen.Generate(fset, "test.go", file)
	if len(mutations) < 1 {
		t.Fatal("expected at least 1 return value mutation")
	}

	applier := &Applier{}
	// Apply the first mutation (should be "return 0")
	result, err := applier.Apply(src, mutations[0])
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	got := string(result)
	if strings.Contains(got, "return 42") {
		t.Errorf("expected original return to be replaced, got:\n%s", got)
	}
}

func TestApplier_Apply_InvalidSource(t *testing.T) {
	src := []byte("this is not valid go code at all")
	m := mutator.Mutation{
		File:        "test.go",
		Line:        1,
		Col:         1,
		Original:    "+",
		Mutated:     "-",
		MutatorName: "arithmetic",
	}
	applier := &Applier{}
	_, err := applier.Apply(src, m)
	if err == nil {
		t.Error("expected error for invalid source, got nil")
	}
}

func TestApplier_Apply_NoMatchingNode(t *testing.T) {
	src := []byte("package p\n\nvar x = 42\n")
	m := mutator.Mutation{
		File:        "test.go",
		Line:        99,
		Col:         1,
		Original:    "+",
		Mutated:     "-",
		MutatorName: "arithmetic",
	}
	applier := &Applier{}
	_, err := applier.Apply(src, m)
	if err == nil {
		t.Error("expected error for non-matching mutation, got nil")
	}
	if !strings.Contains(err.Error(), "mutation not applied") {
		t.Errorf("error should mention 'mutation not applied', got: %v", err)
	}
}

func TestApplier_Apply_UnknownMutatorName(t *testing.T) {
	src := []byte("package p\n\nvar x = 1 + 2\n")
	m := mutator.Mutation{
		File:        "test.go",
		Line:        3,
		Col:         13,
		Original:    "+",
		Mutated:     "-",
		MutatorName: "nonexistent_mutator",
	}
	applier := &Applier{}
	_, err := applier.Apply(src, m)
	if err == nil {
		t.Error("expected error for unknown mutator, got nil")
	}
}

func TestApplier_Apply_IllegalToken(t *testing.T) {
	src := []byte("package p\n\nvar x = 1 + 2\n")
	m := mutator.Mutation{
		File:        "test.go",
		Line:        3,
		Col:         13,
		Original:    "+",
		Mutated:     "ILLEGAL_TOKEN_STRING",
		MutatorName: "arithmetic",
	}
	applier := &Applier{}
	_, err := applier.Apply(src, m)
	if err == nil {
		t.Error("expected error for illegal token, got nil")
	}
}

func TestStringToToken(t *testing.T) {
	tests := []struct {
		input string
		want  string // token string representation
	}{
		{"+", "+"},
		{"-", "-"},
		{"*", "*"},
		{"/", "/"},
		{"%", "%"},
		{"==", "=="},
		{"!=", "!="},
		{"<", "<"},
		{">", ">"},
		{"<=", "<="},
		{">=", ">="},
		{"&&", "&&"},
		{"||", "||"},
		{"break", "break"},
		{"continue", "continue"},
		{"unknown", "ILLEGAL"},
	}
	for _, tt := range tests {
		got := stringToToken(tt.input)
		if got.String() != tt.want {
			t.Errorf("stringToToken(%q) = %q, want %q", tt.input, got.String(), tt.want)
		}
	}
}

func TestStringToToken_EmptyString(t *testing.T) {
	got := stringToToken("")
	if got.String() != "ILLEGAL" {
		t.Errorf("stringToToken(\"\") = %q, want ILLEGAL", got.String())
	}
}

// TestApplier_RoundTrip tests that generating mutations from a source and
// then applying each one produces valid Go source code.
func TestApplier_RoundTrip(t *testing.T) {
	src := []byte(`package p

func add(a, b int) int {
	if a > 0 && b > 0 {
		return a + b
	}
	return 0
}
`)
	fset, file := mustParse(t, string(src))
	gen := NewGenerator([]mutator.Mutator{
		mutator.NewArithmeticMutator(),
		mutator.NewConditionalMutator(),
		mutator.NewLogicalMutator(),
	})
	mutations := gen.Generate(fset, "test.go", file)
	if len(mutations) == 0 {
		t.Fatal("expected mutations")
	}

	applier := &Applier{}
	for _, m := range mutations {
		result, err := applier.Apply(src, m)
		if err != nil {
			t.Errorf("Apply(%s → %s) error: %v", m.Original, m.Mutated, err)
			continue
		}
		// Verify result is valid Go
		_, err2 := mustParseResult(t, result)
		if err2 != nil {
			t.Errorf("Apply(%s → %s) produced invalid Go: %v\nSource:\n%s", m.Original, m.Mutated, err2, string(result))
		}
	}
}

func mustParseResult(t *testing.T, src []byte) (*ast.File, error) {
	t.Helper()
	fset := token.NewFileSet()
	return parser.ParseFile(fset, "test.go", src, 0)
}
