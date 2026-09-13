package mutator

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
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
	points := (&ComparisonMutator{}).DiscoverTyped(fs, f, "p.go", "p", info)
	if len(points) != 2 || !points[0].Constant || points[1].Constant {
		t.Fatalf("constant and runtime operands confused: %+v", points)
	}
}

func TestTypedErrorPropagation(t *testing.T) {
	for _, tt := range []struct {
		name, src string
		want      int
	}{
		{"direct", `func F(err error)error{if err!=nil{return err};return nil}`, 0},
		{"swapped", `func F(err error)error{if nil!=err{return err};return nil}`, 0},
		{"constant result", `func F(err error)(int,error){if err!=nil{return 1+2,err};return 0,nil}`, 0},
		{"nil result", `func F(err error)(*int,error){if err!=nil{return nil,err};return nil,nil}`, 0},
		{"wrapped", `import f "fmt"
func F(err error)error{if err!=nil{return f.Errorf("wrap: %w",err)};return nil}`, 0},
		{"non error", `func F(err *int)*int{if err!=nil{return err};return nil}`, 1},
		{"different error", `func F(err,other error)error{if err!=nil{return other};return nil}`, 1},
		{"runtime result", `func F(err error,n int)(int,error){if err!=nil{return n+1,err};return 0,nil}`, 1},
		{"non fmt wrapper", `type formatter struct{}
func(formatter)Errorf(s string,e error)error{return e}
var fmt formatter
func F(err error)error{if err!=nil{return fmt.Errorf("wrap: %w",err)};return nil}`, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fs, f, info := typedFile(t, tt.src)
			points := (&EqualityMutator{SkipErrPropagation: true}).DiscoverTyped(fs, f, "p.go", "p", info)
			if len(points) != tt.want {
				t.Fatalf("got %d points, want %d", len(points), tt.want)
			}
		})
	}
}
