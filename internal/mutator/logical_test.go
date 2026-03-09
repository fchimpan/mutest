package mutator

import (
	"testing"
)

func TestLogicalMutator_Name(t *testing.T) {
	m := NewLogicalMutator()
	if m.Name() != "logical" {
		t.Errorf("Name() = %q, want %q", m.Name(), "logical")
	}
}

func TestLogicalMutator_Mutate(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantN   int
		wantOps []string
	}{
		{
			name:    "and to or",
			src:     `package p; var x = true && false`,
			wantN:   1,
			wantOps: []string{"||"},
		},
		{
			name:    "or to and",
			src:     `package p; var x = true || false`,
			wantN:   1,
			wantOps: []string{"&&"},
		},
		{
			name:  "non-logical operator",
			src:   `package p; var x = 1 + 2`,
			wantN: 0,
		},
		{
			name:  "chained logical operators",
			src:   `package p; var x = true && false || true`,
			wantN: 2,
		},
		{
			name:  "empty source (no binary expr)",
			src:   `package p`,
			wantN: 0,
		},
	}

	m := NewLogicalMutator()
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
