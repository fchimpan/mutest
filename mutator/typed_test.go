package mutator_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/fchimpan/mutest/mutator"
)

func typedFile(t *testing.T, src string) (*token.FileSet, *ast.File, *types.Info) {
	t.Helper()
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "p.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
	cfg := types.Config{Importer: importer.Default()}
	if _, err := cfg.Check("p", fs, []*ast.File{f}, info); err != nil {
		t.Fatal(err)
	}
	return fs, f, info
}

func TestTypedComparisonDistinguishesConstants(t *testing.T) {
	fs, f, info := typedFile(t, `func F(x int) bool {return 1<<100 > 0 && x < 1}`)
	points := (&mutator.ComparisonMutator{}).DiscoverTyped(fs, f, "p.go", "p", info)
	if len(points) != 2 || !points[0].Constant || points[1].Constant {
		t.Fatalf("constant and runtime operands confused: %+v", points)
	}
}

func TestTypedErrorPropagation(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want int
	}{
		{
			name: "direct",
			src:  "func F(err error)error{if err!=nil{return err};return nil}",
		},
		{
			name: "swapped",
			src:  "func F(err error)error{if nil!=err{return err};return nil}",
		},
		{
			name: "constant result",
			src:  "func F(err error)(int,error){if err!=nil{return 1+2,err};return 0,nil}",
		},
		{
			name: "nil result",
			src:  "func F(err error)(*int,error){if err!=nil{return nil,err};return nil,nil}",
		},
		{
			name: "wrapped",
			src: `import f "fmt"
func F(err error)error{if err!=nil{return f.Errorf("wrap: %w",err)};return nil}`,
		},
		{
			name: "non error",
			src:  "func F(err *int)*int{if err!=nil{return err};return nil}",
			want: 1,
		},
		{
			name: "different error",
			src:  "func F(err,other error)error{if err!=nil{return other};return nil}",
			want: 1,
		},
		{
			name: "runtime result",
			src:  "func F(err error,n int)(int,error){if err!=nil{return n+1,err};return 0,nil}",
			want: 1,
		},
		{
			name: "non fmt wrapper",
			src: `type formatter struct{}
func(formatter)Errorf(s string,e error)error{return e}
var fmt formatter
func F(err error)error{if err!=nil{return fmt.Errorf("wrap: %w",err)};return nil}`,
			want: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fs, f, info := typedFile(t, tt.src)
			points := (&mutator.EqualityMutator{SkipErrPropagation: true}).DiscoverTyped(fs, f, "p.go", "p", info)
			if len(points) != tt.want {
				t.Fatalf("got %d points, want %d", len(points), tt.want)
			}
		})
	}
}
