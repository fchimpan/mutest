package mutator

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestLoopBreakMutator_Name(t *testing.T) {
	m := NewLoopBreakMutator()
	if m.Name() != "loop_break" {
		t.Errorf("Name() = %q, want %q", m.Name(), "loop_break")
	}
}

func TestLoopBreakMutator_Mutate(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantN      int
		wantMutated string
	}{
		{
			name:       "break to continue",
			src:        `package p; func f() { for { break } }`,
			wantN:      1,
			wantMutated: "continue",
		},
		{
			name:       "continue to break",
			src:        `package p; func f() { for i := 0; i < 10; i++ { continue } }`,
			wantN:      1,
			wantMutated: "break",
		},
		{
			name:  "labeled break not matched",
			src:   `package p; func f() { outer: for { break outer } }`,
			wantN: 0,
		},
		{
			name:  "labeled continue not matched",
			src:   `package p; func f() { outer: for { continue outer } }`,
			wantN: 0,
		},
		{
			name:  "goto not matched",
			src:   `package p; func f() { label: goto label }`,
			wantN: 0,
		},
		{
			name:  "fallthrough not matched",
			src:   `package p; func f(x int) { switch x { case 1: fallthrough; default: } }`,
			wantN: 0,
		},
		{
			name:  "no loop statements",
			src:   `package p; func f() { println("hi") }`,
			wantN: 0,
		},
		{
			name:  "both break and continue in same loop",
			src:   `package p; func f() { for i := 0; i < 10; i++ { if i == 5 { break }; continue } }`,
			wantN: 2,
		},
	}

	m := NewLoopBreakMutator()
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

func TestLoopBreakMutator_DirectNodeCall(t *testing.T) {
	m := NewLoopBreakMutator()
	fset := token.NewFileSet()

	// Non-BranchStmt
	got := m.Mutate(fset, "test.go", &ast.Ident{Name: "x"})
	if got != nil {
		t.Errorf("expected nil for non-BranchStmt, got %v", got)
	}
}
