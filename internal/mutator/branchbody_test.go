package mutator

import (
	"testing"
)

func TestBranchBodyMutator_Name(t *testing.T) {
	m := NewBranchBodyMutator()
	if m.Name() != "branch_body" {
		t.Errorf("Name() = %q, want %q", m.Name(), "branch_body")
	}
}

func TestBranchBodyMutator_Mutate(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name:  "if with body",
			src:   `package p; func f() { if true { println("hi") } }`,
			wantN: 1,
		},
		{
			name:  "if-else both bodies",
			src:   `package p; func f() { if true { println("a") } else { println("b") } }`,
			wantN: 2,
		},
		{
			name:  "if-elseif (else is IfStmt, not BlockStmt)",
			src:   `package p; func f() { if true { println("a") } else if false { println("b") } }`,
			wantN: 2, // if body + nested if body
		},
		{
			name:  "empty if body not mutated",
			src:   `package p; func f() { if true {} }`,
			wantN: 0,
		},
		{
			name:  "empty else body not mutated",
			src:   `package p; func f() { if true { println("a") } else {} }`,
			wantN: 1, // only the if body
		},
		{
			name:  "not an if statement",
			src:   `package p; func f() { for i := 0; i < 10; i++ { println(i) } }`,
			wantN: 0,
		},
		{
			name:  "nested if statements",
			src:   `package p; func f() { if true { if false { println("inner") } } }`,
			wantN: 2, // outer if body + inner if body
		},
	}

	m := NewBranchBodyMutator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset, file := mustParse(t, tt.src)
			mutations := collectMutations(t, m, fset, file)
			if len(mutations) != tt.wantN {
				for i, mut := range mutations {
					t.Logf("  mutation[%d]: %s -> %s (line %d)", i, mut.Original, mut.Mutated, mut.Line)
				}
				t.Fatalf("got %d mutations, want %d", len(mutations), tt.wantN)
			}
			for _, mut := range mutations {
				if mut.Mutated != "{}" {
					t.Errorf("Mutated = %q, want %q", mut.Mutated, "{}")
				}
			}
		})
	}
}
