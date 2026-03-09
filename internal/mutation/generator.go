// Package mutation handles generation and application of mutations to Go source code.
package mutation

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"

	"github.com/fchimpan/mutest/internal/mutator"
)

// Generator walks AST files and collects mutations from registered mutators.
type Generator struct {
	mutators []mutator.Mutator
}

func NewGenerator(mutators []mutator.Mutator) *Generator {
	return &Generator{mutators: mutators}
}

// Generate walks all AST nodes in the file and collects mutations from all mutators.
func (g *Generator) Generate(fset *token.FileSet, filePath string, file *ast.File) []mutator.Mutation {
	var mutations []mutator.Mutation
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			return false
		}
		for _, m := range g.mutators {
			muts := m.Mutate(fset, filePath, node)
			mutations = append(mutations, muts...)
		}
		return true
	})
	// Assign unique IDs
	for i := range mutations {
		mutations[i].ID = fmt.Sprintf("%s:%d:%d:%s",
			filepath.Base(mutations[i].File),
			mutations[i].Line,
			mutations[i].Col,
			mutations[i].MutatorName,
		)
	}
	return mutations
}
