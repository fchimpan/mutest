package mutator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestClampOnlySkipsProvenIntegerAssignments(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       int
	}{
		{"lower", "func F(x int) int {if x<1 {x=1};return x}", 0},
		{"lower inclusive", "func F(x int) int {if x<=1 {x=1};return x}", 0},
		{"upper", "func F(x int) int {if x>1 {x=1};return x}", 0},
		{"upper inclusive", "func F(x int) int {if x>=1 {x=1};return x}", 0},
		{"swapped lower", "func F(x int) int {if 1>x {x=1};return x}", 0},
		{"swapped lower inclusive", "func F(x int) int {if 1>=x {x=1};return x}", 0},
		{"swapped upper", "func F(x int) int {if 1<x {x=1};return x}", 0},
		{"swapped upper inclusive", "func F(x int) int {if 1<=x {x=1};return x}", 0},
		{"defined integer", "type N int;func F(x N) N {if x<1 {x=1};return x}", 0},
		{"constant expression", "const C=2;func F(x int) int {if x<C {x=1+1};return x}", 0},
		{"else", "func F(x int) int {if x<1 {x=1}else{x=9};return x}", 1},
		{"float zero", "func F(x float64) float64 {if x<0 {x=0};return x}", 1},
		{"map", "func F(m map[string]int) {if m[\"k\"]<0 {m[\"k\"]=0}}", 1},
		{"different constant", "func F(x int) int {if x<0 {x=1};return x}", 1},
		{"global", "var x int;func F(){if x<1 {x=1}}", 1},
		{"call", "func C()int{return 1};func F(x int)int{if x<C(){x=C()};return x}", 1},
		{"shadowed variable", "func F(x int){if x<1{for x:=0;x<2;x++{_ = x}}}", 2},
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
			points := (&ComparisonMutator{}).DiscoverTyped(fs, f, "p.go", "p", info)
			if len(points) != tt.want {
				t.Fatalf("got %d points, want %d", len(points), tt.want)
			}
		})
	}
}
