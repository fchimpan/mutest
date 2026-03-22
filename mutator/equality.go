package mutator

import (
	"go/ast"
	"go/token"
)

// equalitySwapTable defines equality mutations.
var equalitySwapTable = map[token.Token]token.Token{
	token.EQL: token.NEQ, // == -> !=
	token.NEQ: token.EQL, // != -> ==
}

// EqualityMutator targets equality comparison operators.
type EqualityMutator struct {
	// SkipErrPropagation skips simple error propagation patterns
	// (e.g., if err != nil { return err }) when true.
	SkipErrPropagation bool
}

func (m *EqualityMutator) Name() string { return "comparison-equality" }

func (m *EqualityMutator) Discover(fset *token.FileSet, file *ast.File, filePath, pkg string) []MutationPoint {
	var errSkip map[*ast.BinaryExpr]bool
	if m.SkipErrPropagation {
		errSkip = buildErrPropagationSet(file)
	}

	var points []MutationPoint
	nodeID := 0
	ast.Inspect(file, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if mutated, exists := equalitySwapTable[bin.Op]; exists {
			if errSkip[bin] {
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

// buildErrPropagationSet pre-walks the AST and returns a set of *ast.BinaryExpr
// pointers that represent simple error propagation patterns (e.g., if err != nil { return err }).
// These are skipped by default because they generate noise in mutation testing.
func buildErrPropagationSet(file *ast.File) map[*ast.BinaryExpr]bool {
	skip := make(map[*ast.BinaryExpr]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if !isSimpleErrPropagation(ifStmt) {
			return true
		}
		if bin, ok := ifStmt.Cond.(*ast.BinaryExpr); ok && isErrNilCheck(bin) {
			skip[bin] = true
		}
		return true
	})
	return skip
}

// isSimpleErrPropagation returns true if the IfStmt is a simple error propagation:
// no else branch, and body contains exactly one ReturnStmt.
//
// Skipped patterns:
//
//	if err != nil { return err }
//	if err != nil { return nil, err }
//	if err != nil { return fmt.Errorf("...: %w", err) }
//
// Kept patterns (complex error handling):
//
//	if err != nil && !timedOut { ... }    // compound condition
//	if err != nil { rel = path }          // assignment, not return
//	if err != nil { a(); b() }            // multiple statements
//	if err != nil { return } else { ... } // has else
func isSimpleErrPropagation(ifStmt *ast.IfStmt) bool {
	if ifStmt.Else != nil {
		return false
	}
	if len(ifStmt.Body.List) != 1 {
		return false
	}
	_, isReturn := ifStmt.Body.List[0].(*ast.ReturnStmt)
	return isReturn
}

// isErrNilCheck returns true if the binary expression compares the identifier
// "err" against nil (e.g., err != nil or err == nil).
func isErrNilCheck(bin *ast.BinaryExpr) bool {
	return (isErrIdent(bin.X) && isNilIdent(bin.Y)) ||
		(isErrIdent(bin.Y) && isNilIdent(bin.X))
}

func isErrIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "err"
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}
