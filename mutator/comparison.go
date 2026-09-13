package mutator

import (
	"go/ast"
	"go/token"
	"go/types"
)

// swapTable defines boundary value mutations for comparison operators.
var swapTable = map[token.Token]token.Token{
	token.GTR: token.GEQ, // >  -> >=
	token.GEQ: token.GTR, // >= -> >
	token.LSS: token.LEQ, // <  -> <=
	token.LEQ: token.LSS, // <= -> <
}

// ComparisonMutator targets boundary value comparison operators.
type ComparisonMutator struct{}

func (m *ComparisonMutator) Name() string { return "comparison-boundary" }

func (m *ComparisonMutator) Discover(fset *token.FileSet, file *ast.File, filePath, pkg string) []MutationPoint {
	return m.DiscoverTyped(fset, file, filePath, pkg, nil)
}

func (m *ComparisonMutator) DiscoverTyped(fset *token.FileSet, file *ast.File, filePath, pkg string, info *types.Info) []MutationPoint {
	skip := clampComparisons(file, info)
	var points []MutationPoint
	nodeID := 0
	ast.Inspect(file, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if mutated, exists := swapTable[bin.Op]; exists {
			if skip[bin] {
				nodeID++
				return true
			}
			pos := fset.PositionFor(bin.OpPos, false)
			points = append(points, MutationPoint{
				File:     filePath,
				Package:  pkg,
				Line:     pos.Line,
				Column:   pos.Column,
				Original: bin.Op,
				Mutated:  mutated,
				NodeID:   nodeID,
				Constant: info != nil && info.Types[bin].Value != nil,
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
