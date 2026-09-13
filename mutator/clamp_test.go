package mutator_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/fchimpan/mutest/mutator"
)

func TestClampOnlySkipsProvenIntegerAssignments(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want int
	}{
		{
			name: "lower",
			body: "func F(x int) int {if x<1 {x=1};return x}",
		},
		{
			name: "lower inclusive",
			body: "func F(x int) int {if x<=1 {x=1};return x}",
		},
		{
			name: "upper",
			body: "func F(x int) int {if x>1 {x=1};return x}",
		},
		{
			name: "upper inclusive",
			body: "func F(x int) int {if x>=1 {x=1};return x}",
		},
		{
			name: "swapped lower",
			body: "func F(x int) int {if 1>x {x=1};return x}",
		},
		{
			name: "swapped lower inclusive",
			body: "func F(x int) int {if 1>=x {x=1};return x}",
		},
		{
			name: "swapped upper",
			body: "func F(x int) int {if 1<x {x=1};return x}",
		},
		{
			name: "swapped upper inclusive",
			body: "func F(x int) int {if 1<=x {x=1};return x}",
		},
		{
			name: "defined integer",
			body: "type N int;func F(x N) N {if x<1 {x=1};return x}",
		},
		{
			name: "constant expression",
			body: "const C=2;func F(x int) int {if x<C {x=1+1};return x}",
		},
		{
			name: "else",
			body: "func F(x int) int {if x<1 {x=1}else{x=9};return x}",
			want: 1,
		},
		{
			name: "float zero",
			body: "func F(x float64) float64 {if x<0 {x=0};return x}",
			want: 1,
		},
		{
			name: "map",
			body: "func F(m map[string]int) {if m[\"k\"]<0 {m[\"k\"]=0}}",
			want: 1,
		},
		{
			name: "different constant",
			body: "func F(x int) int {if x<0 {x=1};return x}",
			want: 1,
		},
		{
			name: "global",
			body: "var x int;func F(){if x<1 {x=1}}",
			want: 1,
		},
		{
			name: "call",
			body: "func C()int{return 1};func F(x int)int{if x<C(){x=C()};return x}",
			want: 1,
		},
		{
			name: "shadowed variable",
			body: "func F(x int){if x<1{for x:=0;x<2;x++{_ = x}}}",
			want: 2,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fs := token.NewFileSet()
			f, err := parser.ParseFile(fs, "p.go", "package p\n"+tt.body, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
			cfg := types.Config{}
			if _, err := cfg.Check("p", fs, []*ast.File{f}, info); err != nil {
				t.Fatal(err)
			}
			points := (&mutator.ComparisonMutator{}).DiscoverTyped(fs, f, "p.go", "p", info)
			if len(points) != tt.want {
				t.Fatalf("got %d points, want %d", len(points), tt.want)
			}
		})
	}
}
