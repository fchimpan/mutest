package mutator

import (
	"go/ast"
	"go/token"
)

// DefaultArithmeticSwaps defines the default replacement rules for arithmetic operators.
// Extend this table to add new arithmetic mutation rules.
var DefaultArithmeticSwaps = SwapTable{
	token.ADD: {token.SUB},
	token.SUB: {token.ADD},
	token.MUL: {token.QUO},
	token.QUO: {token.MUL},
	token.REM: {token.MUL},
}

// ArithmeticMutator replaces arithmetic binary operators.
type ArithmeticMutator struct {
	Swaps SwapTable
}

func NewArithmeticMutator() *ArithmeticMutator {
	return &ArithmeticMutator{Swaps: DefaultArithmeticSwaps}
}

func (m *ArithmeticMutator) Name() string { return "arithmetic" }

func (m *ArithmeticMutator) Mutate(fset *token.FileSet, filePath string, node ast.Node) []Mutation {
	return mutateBinaryOp(fset, filePath, m.Name(), m.Swaps, node)
}
