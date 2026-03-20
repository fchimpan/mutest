package mutator

import (
	"go/ast"
	"go/token"
)

// equalitySwapTable defines Tier 2 equality mutations.
var equalitySwapTable = map[token.Token]token.Token{
	token.EQL: token.NEQ, // == -> !=
	token.NEQ: token.EQL, // != -> ==
}

// EqualityMutator targets equality comparison operators (Tier 2).
type EqualityMutator struct{}

func (m *EqualityMutator) Name() string { return "comparison-equality" }

func (m *EqualityMutator) Discover(fset *token.FileSet, file *ast.File, filePath, pkg string) []MutationPoint {
	var points []MutationPoint
	nodeID := 0
	ast.Inspect(file, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if mutated, exists := equalitySwapTable[bin.Op]; exists {
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

func (m *EqualityMutator) Apply(file *ast.File, point MutationPoint) {
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
