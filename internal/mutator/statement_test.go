package mutator

import (
	"testing"
)

func TestStatementMutator_Name(t *testing.T) {
	m := NewStatementMutator()
	if m.Name() != "statement" {
		t.Errorf("Name() = %q, want %q", m.Name(), "statement")
	}
}

func TestStatementMutator_Mutate(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name:  "simple assignment",
			src:   `package p; func f() { var x int; x = 1; _ = x }`,
			wantN: 2, // "x = 1" and "_ = x" are both ASSIGN
		},
		{
			name:  "increment",
			src:   `package p; func f() { i := 0; i++; _ = i }`,
			wantN: 2, // "i++" (IncDecStmt) and "_ = i" (ASSIGN)
		},
		{
			name:  "decrement",
			src:   `package p; func f() { i := 0; i--; _ = i }`,
			wantN: 2, // "i--" (IncDecStmt) and "_ = i" (ASSIGN)
		},
		{
			name:  "short var decl not matched but blank assign is",
			src:   `package p; func f() { x := 1; _ = x }`,
			wantN: 1, // ":=" excluded, but "_ = x" is ASSIGN
		},
		{
			name:  "multiple assignments",
			src:   `package p; func f() { var a, b int; a = 1; b = 2; _ = a; _ = b }`,
			wantN: 4, // a=1, b=2, _=a, _=b are all ASSIGN
		},
		{
			name:  "no statements",
			src:   `package p; var x = 1`,
			wantN: 0,
		},
		{
			name:  "compound assign (+=) is ADD_ASSIGN, blank assign is ASSIGN",
			src:   `package p; func f() { x := 0; x += 1; _ = x }`,
			wantN: 1, // += is ADD_ASSIGN (excluded), but "_ = x" is ASSIGN
		},
	}

	m := NewStatementMutator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset, file := mustParse(t, tt.src)
			mutations := collectMutations(t, m, fset, file)
			if len(mutations) != tt.wantN {
				t.Fatalf("got %d mutations, want %d", len(mutations), tt.wantN)
			}
			for _, mut := range mutations {
				if mut.Mutated != "<removed>" {
					t.Errorf("Mutated = %q, want %q", mut.Mutated, "<removed>")
				}
			}
		})
	}
}
