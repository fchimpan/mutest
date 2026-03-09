package mutator

import (
	"go/ast"
	"go/token"
)

// DefaultConditionalSwaps defines the default replacement rules for comparison operators.
// Currently uses conservative swaps (negate + boundary).
// Extend this table for more thorough testing, e.g.:
//
//	token.LSS: {token.LEQ, token.GTR, token.GEQ, token.EQL, token.NEQ}
var DefaultConditionalSwaps = SwapTable{
	token.EQL: {token.NEQ},
	token.NEQ: {token.EQL},
	token.LSS: {token.LEQ, token.GEQ},
	token.GTR: {token.GEQ, token.LEQ},
	token.LEQ: {token.LSS, token.GTR},
	token.GEQ: {token.GTR, token.LSS},
}

// ConditionalMutator replaces comparison/relational operators.
type ConditionalMutator struct {
	Swaps SwapTable
}

func NewConditionalMutator() *ConditionalMutator {
	return &ConditionalMutator{Swaps: DefaultConditionalSwaps}
}

func (m *ConditionalMutator) Name() string { return "conditional" }

func (m *ConditionalMutator) Mutate(fset *token.FileSet, filePath string, node ast.Node) []Mutation {
	return mutateBinaryOp(fset, filePath, m.Name(), m.Swaps, node)
}
