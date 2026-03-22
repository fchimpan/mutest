package mutator

import (
	"go/ast"
	"go/token"
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
	var points []MutationPoint
	nodeID := 0
	ast.Inspect(file, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if mutated, exists := swapTable[bin.Op]; exists {
			if isNonNegativeComparison(bin) {
				nodeID++
				return true
			}
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

// isNonNegativeComparison returns true if the expression compares a
// non-negative builtin (len, cap) against the literal 0. Mutating such
// comparisons always produces a false positive because len/cap never
// return negative values, making the mutation semantically equivalent.
//
// Skipped patterns (all mutations are no-ops):
//
//	len(x) > 0  → len(x) >= 0  (>= 0 is always true, not a useful boundary)
//	len(x) >= 0 → len(x) > 0   (narrows from always-true, but not a boundary bug)
//	len(x) < 0  → len(x) <= 0  (< 0 is always false)
//	len(x) <= 0 → len(x) < 0   (always-false vs empty-check, not a boundary)
func isNonNegativeComparison(bin *ast.BinaryExpr) bool {
	return (isNonNegativeCall(bin.X) && isIntLit(bin.Y, "0")) ||
		(isNonNegativeCall(bin.Y) && isIntLit(bin.X, "0"))
}

func isNonNegativeCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "len" || ident.Name == "cap"
}

func isIntLit(expr ast.Expr, value string) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value == value
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
