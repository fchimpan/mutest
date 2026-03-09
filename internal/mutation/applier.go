package mutation

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"

	"github.com/fchimpan/mutest/internal/mutator"
	"golang.org/x/tools/go/ast/astutil"
)

// Applier applies a mutation to source code and produces the mutated source.
type Applier struct{}

// Apply takes original source bytes and a mutation, re-parses the source,
// applies the mutation in the AST, and returns the formatted mutated source.
// It re-parses to avoid sharing AST state between mutations.
func (a *Applier) Apply(originalSource []byte, m mutator.Mutation) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, m.File, originalSource, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing source for mutation: %w", err)
	}

	// Match by line+col since re-parse creates a new FileSet with different token.Pos values.
	// Different node types store the mutation position differently:
	//   - BinaryExpr: OpPos (operator position, not the node start)
	//   - Others: node.Pos()
	applied := false
	astutil.Apply(file, func(cursor *astutil.Cursor) bool {
		if applied {
			return false
		}
		node := cursor.Node()
		if node == nil {
			return true
		}
		if !nodeMatchesMutation(fset, node, m) {
			return true
		}

		if applyMutation(cursor, node, m) {
			applied = true
			return false
		}
		return true
	}, nil)

	if !applied {
		return nil, fmt.Errorf("mutation not applied at %s:%d:%d (%s)", m.File, m.Line, m.Col, m.MutatorName)
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, fmt.Errorf("formatting mutated source: %w", err)
	}
	return buf.Bytes(), nil
}

// nodeMatchesMutation checks if an AST node corresponds to the given mutation
// by comparing line/col positions. Different node types use different reference positions.
func nodeMatchesMutation(fset *token.FileSet, node ast.Node, m mutator.Mutation) bool {
	switch n := node.(type) {
	case *ast.BinaryExpr:
		// Operator mutations store the OpPos position
		pos := fset.Position(n.OpPos)
		return pos.Line == m.Line && pos.Column == m.Col
	case *ast.UnaryExpr:
		pos := fset.Position(n.Pos())
		return pos.Line == m.Line && pos.Column == m.Col
	case *ast.FuncDecl:
		// ReturnValue mutation stores the body's position
		if n.Body != nil {
			pos := fset.Position(n.Body.Pos())
			return pos.Line == m.Line && pos.Column == m.Col
		}
		return false
	default:
		pos := fset.Position(node.Pos())
		return pos.Line == m.Line && pos.Column == m.Col
	}
}

// applyMutation modifies the AST node via the cursor based on the mutation type.
// Returns true if the mutation was successfully applied.
func applyMutation(cursor *astutil.Cursor, node ast.Node, m mutator.Mutation) bool {
	switch m.MutatorName {
	case "arithmetic", "conditional", "logical":
		return applyBinaryOpMutation(node, m)
	case "negate_removal":
		return applyNegateRemoval(cursor, node)
	case "return_value":
		return applyReturnValueMutation(node, m)
	case "statement":
		return applyStatementRemoval(cursor)
	case "branch_body":
		return applyBranchBodyMutation(node)
	case "loop_break":
		return applyLoopBreakMutation(node, m)
	default:
		return false
	}
}

func applyBinaryOpMutation(node ast.Node, m mutator.Mutation) bool {
	binExpr, ok := node.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	newOp := stringToToken(m.Mutated)
	if newOp == token.ILLEGAL {
		return false
	}
	binExpr.Op = newOp
	return true
}

func applyNegateRemoval(cursor *astutil.Cursor, node ast.Node) bool {
	unaryExpr, ok := node.(*ast.UnaryExpr)
	if !ok || unaryExpr.Op != token.NOT {
		return false
	}
	// Replace !expr with expr by replacing the UnaryExpr with its operand
	cursor.Replace(unaryExpr.X)
	return true
}

func applyReturnValueMutation(node ast.Node, m mutator.Mutation) bool {
	funcDecl, ok := node.(*ast.FuncDecl)
	if !ok || funcDecl.Body == nil {
		return false
	}
	// Parse the replacement body
	// Wrap in a dummy function to parse the block statement
	src := fmt.Sprintf("package p\nfunc _() %s", m.Mutated)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return false
	}
	if len(f.Decls) == 0 {
		return false
	}
	newFunc, ok := f.Decls[0].(*ast.FuncDecl)
	if !ok || newFunc.Body == nil {
		return false
	}
	funcDecl.Body.List = newFunc.Body.List
	return true
}

func applyStatementRemoval(cursor *astutil.Cursor) bool {
	// Replace the statement with an empty statement
	cursor.Replace(&ast.EmptyStmt{Implicit: true})
	return true
}

func applyBranchBodyMutation(node ast.Node) bool {
	blockStmt, ok := node.(*ast.BlockStmt)
	if !ok {
		return false
	}
	blockStmt.List = nil
	return true
}

func applyLoopBreakMutation(node ast.Node, m mutator.Mutation) bool {
	branchStmt, ok := node.(*ast.BranchStmt)
	if !ok {
		return false
	}
	newTok := stringToToken(m.Mutated)
	if newTok == token.ILLEGAL {
		return false
	}
	branchStmt.Tok = newTok
	return true
}

func stringToToken(s string) token.Token {
	switch s {
	case "+":
		return token.ADD
	case "-":
		return token.SUB
	case "*":
		return token.MUL
	case "/":
		return token.QUO
	case "%":
		return token.REM
	case "==":
		return token.EQL
	case "!=":
		return token.NEQ
	case "<":
		return token.LSS
	case ">":
		return token.GTR
	case "<=":
		return token.LEQ
	case ">=":
		return token.GEQ
	case "&&":
		return token.LAND
	case "||":
		return token.LOR
	case "break":
		return token.BREAK
	case "continue":
		return token.CONTINUE
	default:
		return token.ILLEGAL
	}
}
