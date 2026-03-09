package mutator

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"strings"
)

// ReturnValueMutator replaces function bodies with type-appropriate default return values.
// Inspired by cargo-mutants: instead of fine-grained operator changes, replace the entire
// function body to test if the function's return value is actually checked.
type ReturnValueMutator struct {
	TypeInfo *types.Info // Set externally after type-checking
}

func NewReturnValueMutator() *ReturnValueMutator {
	return &ReturnValueMutator{}
}

func (m *ReturnValueMutator) Name() string { return "return_value" }

func (m *ReturnValueMutator) Mutate(fset *token.FileSet, filePath string, node ast.Node) []Mutation {
	funcDecl, ok := node.(*ast.FuncDecl)
	if !ok {
		return nil
	}
	// Skip functions without a body or without results
	if funcDecl.Body == nil || funcDecl.Type.Results == nil || len(funcDecl.Type.Results.List) == 0 {
		return nil
	}
	// Skip test functions and main
	name := funcDecl.Name.Name
	if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") || name == "main" || name == "init" {
		return nil
	}

	replacements := m.generateReplacements(funcDecl.Type.Results)
	if len(replacements) == 0 {
		return nil
	}

	pos := fset.Position(funcDecl.Body.Pos())
	bodyStr := nodeString(fset, funcDecl.Body)

	var mutations []Mutation
	for _, repl := range replacements {
		mutations = append(mutations, Mutation{
			File:        filePath,
			Line:        pos.Line,
			Col:         pos.Column,
			Pos:         funcDecl.Body.Pos(),
			End:         funcDecl.Body.End(),
			Original:    truncate(bodyStr, 40),
			Mutated:     repl,
			MutatorName: m.Name(),
			Status:      StatusPending,
		})
	}
	return mutations
}

// generateReplacements produces replacement function bodies based on return types.
func (m *ReturnValueMutator) generateReplacements(results *ast.FieldList) []string {
	var parts []returnPart
	for _, field := range results.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			p := classifyType(field.Type)
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return nil
	}

	// Generate one "zero" replacement and one "non-zero" replacement
	var replacements []string

	zeroVals := make([]string, len(parts))
	nonZeroVals := make([]string, len(parts))
	for i, p := range parts {
		zeroVals[i] = p.zero
		nonZeroVals[i] = p.nonZero
	}

	zeroBody := fmt.Sprintf("{ return %s }", strings.Join(zeroVals, ", "))
	replacements = append(replacements, zeroBody)

	// Only add non-zero if it differs from zero
	nonZeroBody := fmt.Sprintf("{ return %s }", strings.Join(nonZeroVals, ", "))
	if nonZeroBody != zeroBody {
		replacements = append(replacements, nonZeroBody)
	}

	return replacements
}

type returnPart struct {
	zero    string
	nonZero string
}

func classifyType(expr ast.Expr) returnPart {
	switch t := expr.(type) {
	case *ast.Ident:
		return classifyIdent(t.Name)
	case *ast.StarExpr:
		return returnPart{zero: "nil", nonZero: "nil"}
	case *ast.ArrayType:
		typeStr := nodeStringSimple(t)
		return returnPart{zero: "nil", nonZero: typeStr + "{}"}
	case *ast.MapType:
		return returnPart{zero: "nil", nonZero: "nil"}
	case *ast.ChanType:
		return returnPart{zero: "nil", nonZero: "nil"}
	case *ast.FuncType:
		return returnPart{zero: "nil", nonZero: "nil"}
	case *ast.InterfaceType:
		return returnPart{zero: "nil", nonZero: "nil"}
	case *ast.SelectorExpr:
		// e.g., time.Duration, context.Context
		typeStr := nodeStringSimple(t)
		if typeStr == "time.Duration" {
			return returnPart{zero: "0", nonZero: "1"}
		}
		// Default: assume it's an interface or struct; use zero value
		return returnPart{zero: typeStr + "{}", nonZero: typeStr + "{}"}
	default:
		return returnPart{zero: "nil", nonZero: "nil"}
	}
}

func classifyIdent(name string) returnPart {
	switch name {
	case "bool":
		return returnPart{zero: "false", nonZero: "true"}
	case "string":
		return returnPart{zero: `""`, nonZero: `"mutest"`}
	case "int", "int8", "int16", "int32", "int64":
		return returnPart{zero: "0", nonZero: "1"}
	case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte", "rune":
		return returnPart{zero: "0", nonZero: "1"}
	case "float32", "float64":
		return returnPart{zero: "0.0", nonZero: "1.0"}
	case "complex64", "complex128":
		return returnPart{zero: "0", nonZero: "1"}
	case "error":
		return returnPart{zero: "nil", nonZero: `fmt.Errorf("mutest")`}
	default:
		// Named type — attempt zero value with struct literal
		return returnPart{zero: name + "{}", nonZero: name + "{}"}
	}
}

func nodeStringSimple(node ast.Node) string {
	fset := token.NewFileSet()
	var b strings.Builder
	if err := format.Node(&b, fset, node); err != nil {
		return "<type>"
	}
	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
