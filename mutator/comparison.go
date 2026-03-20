package mutator

import (
	"go/ast"
	"go/token"
)

// swapTable defines Tier 1 boundary value mutations for comparison operators.
var swapTable = map[token.Token]token.Token{
	token.GTR: token.GEQ, // >  -> >=
	token.GEQ: token.GTR, // >= -> >
	token.LSS: token.LEQ, // <  -> <=
	token.LEQ: token.LSS, // <= -> <
}

// ComparisonMutator targets boundary value comparison operators (Tier 1).
type ComparisonMutator struct{}

func (m *ComparisonMutator) Name() string { return "comparison-boundary" }

func (m *ComparisonMutator) Discover(fset *token.FileSet, file *ast.File, filePath, pkg string) []MutationPoint {
	var points []MutationPoint
	nodeID := 0
	ast.Inspect(file, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if mutated, exists := swapTable[bin.Op]; exists {
			pos := fset.Position(bin.OpPos)
			points = append(points, MutationPoint{
				File:     filePath,
				Package:  pkg,
				Line:     pos.Line,
				Column:   pos.Column,
				Original: bin.Op,
				Mutated:  mutated,
				NodeID:   nodeID,
				Desc:     bin.Op.String() + " to " + mutated.String(),
			})
		}
		nodeID++
		return true
	})
	return points
}

func (m *ComparisonMutator) Apply(file *ast.File, point MutationPoint) {
	done := false
	nodeID := 0
	ast.Inspect(file, func(n ast.Node) bool {
		if done {
			return false
		}
		bin, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if nodeID == point.NodeID {
			bin.Op = point.Mutated
			done = true
		}
		nodeID++
		return true
	})
}
