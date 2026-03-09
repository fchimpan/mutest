package mutator

import (
	"go/ast"
	"go/format"
	"go/token"
	"strings"
)

// NegateMutator removes the ! (NOT) unary operator, replacing !expr with expr.
type NegateMutator struct{}

func NewNegateMutator() *NegateMutator { return &NegateMutator{} }

func (m *NegateMutator) Name() string { return "negate_removal" }

func (m *NegateMutator) Mutate(fset *token.FileSet, filePath string, node ast.Node) []Mutation {
	unaryExpr, ok := node.(*ast.UnaryExpr)
	if !ok || unaryExpr.Op != token.NOT {
		return nil
	}
	pos := fset.Position(unaryExpr.Pos())
	inner := nodeString(fset, unaryExpr.X)
	return []Mutation{{
		File:        filePath,
		Line:        pos.Line,
		Col:         pos.Column,
		Pos:         unaryExpr.Pos(),
		End:         unaryExpr.End(),
		Original:    "!" + inner,
		Mutated:     inner,
		MutatorName: m.Name(),
		Status:      StatusPending,
	}}
}

func nodeString(fset *token.FileSet, node ast.Node) string {
	var b strings.Builder
	if err := format.Node(&b, fset, node); err != nil {
		return "<expr>"
	}
	return b.String()
}
