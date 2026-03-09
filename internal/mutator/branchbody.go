package mutator

import (
	"go/ast"
	"go/token"
)

// BranchBodyMutator empties the body of if/else branches, testing whether the
// conditional behavior is verified by tests.
type BranchBodyMutator struct{}

func NewBranchBodyMutator() *BranchBodyMutator { return &BranchBodyMutator{} }

func (m *BranchBodyMutator) Name() string { return "branch_body" }

func (m *BranchBodyMutator) Mutate(fset *token.FileSet, filePath string, node ast.Node) []Mutation {
	ifStmt, ok := node.(*ast.IfStmt)
	if !ok {
		return nil
	}

	var mutations []Mutation

	// Mutate if-body: replace with empty block
	if ifStmt.Body != nil && len(ifStmt.Body.List) > 0 {
		pos := fset.Position(ifStmt.Body.Pos())
		mutations = append(mutations, Mutation{
			File:        filePath,
			Line:        pos.Line,
			Col:         pos.Column,
			Pos:         ifStmt.Body.Pos(),
			End:         ifStmt.Body.End(),
			Original:    truncate(nodeString(fset, ifStmt.Body), 40),
			Mutated:     "{}",
			MutatorName: m.Name(),
			Status:      StatusPending,
		})
	}

	// Mutate else-body if it's a block (not else-if)
	if elseBlock, ok := ifStmt.Else.(*ast.BlockStmt); ok && len(elseBlock.List) > 0 {
		pos := fset.Position(elseBlock.Pos())
		mutations = append(mutations, Mutation{
			File:        filePath,
			Line:        pos.Line,
			Col:         pos.Column,
			Pos:         elseBlock.Pos(),
			End:         elseBlock.End(),
			Original:    truncate(nodeString(fset, elseBlock), 40),
			Mutated:     "{}",
			MutatorName: m.Name(),
			Status:      StatusPending,
		})
	}

	return mutations
}
