package mutator

import (
	"go/ast"
	"go/token"
)

// LoopBreakMutator swaps break ↔ continue in loop bodies.
type LoopBreakMutator struct{}

func NewLoopBreakMutator() *LoopBreakMutator { return &LoopBreakMutator{} }

func (m *LoopBreakMutator) Name() string { return "loop_break" }

func (m *LoopBreakMutator) Mutate(fset *token.FileSet, filePath string, node ast.Node) []Mutation {
	branchStmt, ok := node.(*ast.BranchStmt)
	if !ok {
		return nil
	}
	// Only swap unlabeled break/continue
	if branchStmt.Label != nil {
		return nil
	}

	var replacement string
	switch branchStmt.Tok {
	case token.BREAK:
		replacement = "continue"
	case token.CONTINUE:
		replacement = "break"
	default:
		return nil
	}

	pos := fset.Position(branchStmt.Pos())
	return []Mutation{{
		File:        filePath,
		Line:        pos.Line,
		Col:         pos.Column,
		Pos:         branchStmt.Pos(),
		End:         branchStmt.End(),
		Original:    branchStmt.Tok.String(),
		Mutated:     replacement,
		MutatorName: m.Name(),
		Status:      StatusPending,
	}}
}
