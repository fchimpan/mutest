package mutator

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"
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
	return m.DiscoverTyped(fset, file, filePath, pkg, nil)
}

func (m *EqualityMutator) DiscoverTyped(fset *token.FileSet, file *ast.File, filePath, pkg string, info *types.Info) []MutationPoint {
	var errSkip map[*ast.BinaryExpr]bool
	if m.SkipErrPropagation {
		errSkip = buildErrPropagationSet(file, info)
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
			pos := fset.PositionFor(bin.OpPos, false)
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
func buildErrPropagationSet(file *ast.File, info *types.Info) map[*ast.BinaryExpr]bool {
	skip := make(map[*ast.BinaryExpr]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if !isSimpleErrPropagation(ifStmt, info) {
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
func isSimpleErrPropagation(ifStmt *ast.IfStmt, info *types.Info) bool {
	if ifStmt.Else != nil {
		return false
	}
	if len(ifStmt.Body.List) != 1 {
		return false
	}
	bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ || !isErrNilCheck(bin) {
		return false
	}
	id, _ := bin.X.(*ast.Ident)
	if id.Name == "nil" {
		id, _ = bin.Y.(*ast.Ident)
	}
	if info != nil {
		typ := info.TypeOf(id)
		if typ == nil || !types.Implements(typ, types.Universe.Lookup("error").Type().Underlying().(*types.Interface)) {
			return false
		}
	}
	ret, ok := ifStmt.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	same := func(expr ast.Expr) bool {
		other, ok := ast.Unparen(expr).(*ast.Ident)
		if !ok || other.Name != id.Name {
			return false
		}
		return info == nil || info.ObjectOf(other) == info.ObjectOf(id)
	}
	propagates := false
	for _, expr := range ret.Results {
		if same(expr) {
			propagates = true
			continue
		}
		// Other return values must be constants or nil; calls and computed
		// values can have meaningful behavior beyond error propagation.
		if info != nil {
			if tv := info.Types[expr]; tv.Value != nil || tv.IsNil() {
				continue
			}
		} else if _, ok := expr.(*ast.BasicLit); ok || isNilIdent(expr) {
			continue
		}
		call, ok := expr.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 || !same(call.Args[1]) {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Errorf" {
			return false
		}
		if info != nil {
			obj := info.ObjectOf(sel.Sel)
			if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "fmt" {
				return false
			}
		} else {
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "fmt" {
				return false
			}
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return false
		}
		format, err := strconv.Unquote(lit.Value)
		// Recognize only a single unambiguous wrapping verb. Escaped or
		// indexed verbs and extra arguments remain mutation targets.
		if err != nil || strings.Count(format, "%") != 1 || !strings.Contains(format, "%w") {
			return false
		}
		propagates = true
	}
	return propagates
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
