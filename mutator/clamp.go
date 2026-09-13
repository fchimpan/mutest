package mutator

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

// Only local integer variables and equal constants are proven safe here.
// Floats (signed zero), map writes, else branches and calls are observable.
func clampComparisons(file *ast.File, info *types.Info) map[*ast.BinaryExpr]bool {
	skip := make(map[*ast.BinaryExpr]bool)
	if info == nil {
		return skip
	}
	ast.Inspect(file, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok || stmt.Else != nil || len(stmt.Body.List) != 1 {
			return true
		}
		bin, ok := ast.Unparen(stmt.Cond).(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if _, ok := swapTable[bin.Op]; !ok {
			return true
		}
		assign, ok := stmt.Body.List[0].(*ast.AssignStmt)
		if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		dest, ok := ast.Unparen(assign.Lhs[0]).(*ast.Ident)
		if !ok {
			return true
		}
		obj, ok := info.ObjectOf(dest).(*types.Var)
		if !ok || obj.Pkg() == nil || obj.Parent() == obj.Pkg().Scope() {
			return true
		}
		typ, ok := obj.Type().Underlying().(*types.Basic)
		if !ok || typ.Info()&types.IsInteger == 0 {
			return true
		}
		for _, pair := range [][2]ast.Expr{{bin.X, bin.Y}, {bin.Y, bin.X}} {
			id, ok := ast.Unparen(pair[0]).(*ast.Ident)
			if !ok || info.ObjectOf(id) != obj {
				continue
			}
			a, b := info.Types[pair[1]].Value, info.Types[assign.Rhs[0]].Value
			if a != nil && b != nil && constant.Compare(a, token.EQL, b) {
				skip[bin] = true
			}
		}
		return true
	})
	return skip
}
