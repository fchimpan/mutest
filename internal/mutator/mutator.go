// Package mutator defines the core types and interfaces for mutation testing.
package mutator

import (
	"go/ast"
	"go/token"
)

// MutantStatus represents the outcome of a mutation test.
type MutantStatus int

const (
	StatusPending    MutantStatus = iota
	StatusKilled                  // Test failed → mutation detected
	StatusSurvived                // Test passed → mutation not detected
	StatusEquivalent              // SSA analysis determined no behavioral change
	StatusNotCovered              // No test covers this line
	StatusTimeout                 // Test execution timed out
	StatusBuildError              // Mutant caused a build failure
	StatusSkipped                 // Skipped (e.g., dry-run)
)

func (s MutantStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusKilled:
		return "killed"
	case StatusSurvived:
		return "survived"
	case StatusEquivalent:
		return "equivalent"
	case StatusNotCovered:
		return "not_covered"
	case StatusTimeout:
		return "timeout"
	case StatusBuildError:
		return "build_error"
	case StatusSkipped:
		return "skipped"
	default:
		return "unknown"
	}
}

// Mutation represents a single concrete mutation to apply.
type Mutation struct {
	ID          string       `json:"id"`
	File        string       `json:"file"`
	Line        int          `json:"line"`
	Col         int          `json:"col"`
	Original    string       `json:"original"`
	Mutated     string       `json:"mutated"`
	MutatorName string       `json:"mutator"`
	Status      MutantStatus `json:"status"`
	Pos         token.Pos    `json:"-"` // AST position
	End         token.Pos    `json:"-"` // AST end position

	// Set by the applier after rendering; not serialized.
	MutatedSource []byte `json:"-"`
}

// Mutator generates mutations for AST nodes.
// Each Mutator handles one category of mutation (arithmetic, conditional, etc.).
type Mutator interface {
	// Name returns a human-readable identifier for this mutator (e.g., "arithmetic").
	Name() string

	// Mutate inspects a single AST node and returns zero or more mutations.
	// Implementations must not modify the AST node.
	Mutate(fset *token.FileSet, filePath string, node ast.Node) []Mutation
}

// SwapTable maps a token to the list of tokens it can be replaced with.
// This is the primary extension point for operator-based mutators.
// Users can modify or extend swap tables to add new replacement rules.
type SwapTable map[token.Token][]token.Token

// mutateBinaryOp is a shared helper for operator-based mutators.
// It checks whether node is a *ast.BinaryExpr with an operator in the swap table,
// and returns mutations for each replacement.
func mutateBinaryOp(fset *token.FileSet, filePath string, mutatorName string, swaps SwapTable, node ast.Node) []Mutation {
	binExpr, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	replacements, exists := swaps[binExpr.Op]
	if !exists {
		return nil
	}
	pos := fset.Position(binExpr.OpPos)
	mutations := make([]Mutation, 0, len(replacements))
	for _, repl := range replacements {
		mutations = append(mutations, Mutation{
			File:        filePath,
			Line:        pos.Line,
			Col:         pos.Column,
			Pos:         binExpr.OpPos,
			End:         binExpr.OpPos + token.Pos(len(binExpr.Op.String())),
			Original:    binExpr.Op.String(),
			Mutated:     repl.String(),
			MutatorName: mutatorName,
			Status:      StatusPending,
		})
	}
	return mutations
}
