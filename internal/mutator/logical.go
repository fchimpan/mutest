package mutator

import (
	"go/ast"
	"go/token"
)

// DefaultLogicalSwaps defines the default replacement rules for logical operators.
var DefaultLogicalSwaps = SwapTable{
	token.LAND: {token.LOR},
	token.LOR:  {token.LAND},
}

// LogicalMutator replaces logical binary operators (&& ↔ ||).
type LogicalMutator struct {
	Swaps SwapTable
}

func NewLogicalMutator() *LogicalMutator {
	return &LogicalMutator{Swaps: DefaultLogicalSwaps}
}

func (m *LogicalMutator) Name() string { return "logical" }

func (m *LogicalMutator) Mutate(fset *token.FileSet, filePath string, node ast.Node) []Mutation {
	return mutateBinaryOp(fset, filePath, m.Name(), m.Swaps, node)
}
