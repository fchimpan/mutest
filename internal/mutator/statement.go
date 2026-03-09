package mutator

import (
	"go/ast"
	"go/token"
)

// StatementMutator removes assignment and expression statements by replacing them with
// empty blocks. This tests whether the statement's side effects are verified by tests.
type StatementMutator struct{}

func NewStatementMutator() *StatementMutator { return &StatementMutator{} }

func (m *StatementMutator) Name() string { return "statement" }

func (m *StatementMutator) Mutate(fset *token.FileSet, filePath string, node ast.Node) []Mutation {
	switch stmt := node.(type) {
	case *ast.AssignStmt:
		// Only remove simple assignments (=), not short declarations (:=)
		if stmt.Tok != token.ASSIGN {
			return nil
		}
		return m.makeMutation(fset, filePath, stmt, stmt.Pos(), stmt.End())
	case *ast.IncDecStmt:
		// i++ or i--
		return m.makeMutation(fset, filePath, stmt, stmt.Pos(), stmt.End())
	default:
		return nil
	}
}

func (m *StatementMutator) makeMutation(fset *token.FileSet, filePath string, node ast.Node, pos, end token.Pos) []Mutation {
	p := fset.Position(pos)
	original := nodeString(fset, node)
	return []Mutation{{
		File:        filePath,
		Line:        p.Line,
		Col:         p.Column,
		Pos:         pos,
		End:         end,
		Original:    truncate(original, 40),
		Mutated:     "<removed>",
		MutatorName: m.Name(),
		Status:      StatusPending,
	}}
}
