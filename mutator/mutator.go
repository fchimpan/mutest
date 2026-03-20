package mutator

import (
	"go/ast"
	"go/token"
)

// MutationPoint describes a single mutation opportunity in source code.
type MutationPoint struct {
	File        string      // absolute path to the original .go file
	Package     string      // package name containing the file
	ImportPath  string      // Go import path of the package (e.g. "github.com/foo/bar/pkg")
	Line        int         // 1-based line number
	Column      int         // 1-based column number
	Original    token.Token // original operator, e.g. token.GTR
	Mutated     token.Token // replacement operator, e.g. token.GEQ
	NodeID      int         // index to identify the BinaryExpr in AST walk order
	Desc        string      // human-readable description, e.g. "> to >="
	MutatorName string      // name of the mutator that discovered this point
}

// Mutator discovers mutation opportunities and applies them to AST nodes.
type Mutator interface {
	// Name returns a human-readable name for this mutator.
	Name() string

	// Discover walks the AST and returns all applicable MutationPoints.
	Discover(fset *token.FileSet, file *ast.File, filePath string, pkg string) []MutationPoint

	// Apply walks the AST and mutates the node identified by point.NodeID.
	// The caller must provide a freshly parsed AST (not shared with other goroutines).
	Apply(file *ast.File, point MutationPoint)
}
