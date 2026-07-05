package diff

import (
	"testing"

	"github.com/fchimpan/mutest/mutator"
)

func TestFilterPoints(t *testing.T) {
	points := []mutator.MutationPoint{
		{File: "/src/a.go", Line: 10, Desc: "a:10"},
		{File: "/src/a.go", Line: 20, Desc: "a:20"},
		{File: "/src/b.go", Line: 5, Desc: "b:5"},
		{File: "/src/b.go", Line: 15, Desc: "b:15"},
		{File: "/src/c.go", Line: 1, Desc: "c:1"},
	}

	tests := []struct {
		name string
		cl   ChangedLines
		want []string // expected Desc values
	}{
		{
			name: "nil ChangedLines returns all points",
			cl:   nil,
			want: []string{"a:10", "a:20", "b:5", "b:15", "c:1"},
		},
		{
			name: "empty ChangedLines returns no points",
			cl:   ChangedLines{},
			want: nil,
		},
		{
			name: "filters to matching file and line",
			cl: ChangedLines{
				"/src/a.go": {10: true},
				"/src/b.go": {15: true},
			},
			want: []string{"a:10", "b:15"},
		},
		{
			name: "no matching files returns empty",
			cl: ChangedLines{
				"/src/x.go": {1: true},
			},
			want: nil,
		},
		{
			name: "nil line set means the whole file changed",
			cl: ChangedLines{
				"/src/a.go": nil,       // untracked file: every line is "changed"
				"/src/b.go": {5: true}, // tracked file: only line 5 changed
			},
			want: []string{"a:10", "a:20", "b:5"},
		},
		{
			name: "nil line set keeps every point in that file",
			cl: ChangedLines{
				"/src/a.go": nil,
			},
			want: []string{"a:10", "a:20"},
		},
		{
			name: "file matches but line does not",
			cl: ChangedLines{
				"/src/a.go": {99: true},
			},
			want: nil,
		},
		{
			name: "multiple points on same line both kept",
			cl: ChangedLines{
				"/src/a.go": {10: true, 20: true},
				"/src/c.go": {1: true},
			},
			want: []string{"a:10", "a:20", "c:1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterPoints(points, tt.cl)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d points, want %d", len(got), len(tt.want))
			}
			for i, g := range got {
				if g.Desc != tt.want[i] {
					t.Errorf("got[%d].Desc = %q, want %q", i, g.Desc, tt.want[i])
				}
			}
		})
	}
}
