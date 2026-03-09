package analysis

import (
	"go/token"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/fchimpan/mutest/internal/mutator"
)

// EquivalenceAnalyzer detects equivalent mutants using SSA analysis.
// A mutation is "equivalent" if it cannot affect observable program behavior.
type EquivalenceAnalyzer struct {
	prog *ssa.Program
	pkgs []*ssa.Package
	fset *token.FileSet
}

// NewEquivalenceAnalyzer builds SSA from loaded packages.
// Requires GlobalDebug mode to emit DebugRef instructions that map
// source expressions to SSA values.
func NewEquivalenceAnalyzer(loadedPkgs []*packages.Package) (*EquivalenceAnalyzer, error) {
	prog, pkgs := ssautil.AllPackages(loadedPkgs, ssa.GlobalDebug|ssa.InstantiateGenerics)
	prog.Build()

	var fset *token.FileSet
	if len(loadedPkgs) > 0 {
		fset = loadedPkgs[0].Fset
	}

	return &EquivalenceAnalyzer{prog: prog, pkgs: pkgs, fset: fset}, nil
}

// IsEquivalent checks if a mutation is equivalent (does not change observable behavior).
// Returns true only when equivalence can be proven; conservative (returns false when uncertain).
func (ea *EquivalenceAnalyzer) IsEquivalent(m mutator.Mutation) bool {
	fn := ea.enclosingFunction(m.Pos)
	if fn == nil {
		return false
	}

	value := ea.valueAtPos(fn, m.Pos)
	if value == nil {
		return false
	}

	if ea.isDeadDefinition(value) {
		return true
	}
	if ea.isUnreachableBlock(value) {
		return true
	}
	if ea.isNonEscaping(value) {
		return true
	}

	return false
}

// enclosingFunction finds the SSA function that contains the given position.
func (ea *EquivalenceAnalyzer) enclosingFunction(pos token.Pos) *ssa.Function {
	for _, pkg := range ea.pkgs {
		if pkg == nil {
			continue
		}
		for _, member := range pkg.Members {
			fn, ok := member.(*ssa.Function)
			if !ok {
				continue
			}
			if containsPos(fn, pos) {
				return fn
			}
			for _, anon := range fn.AnonFuncs {
				if containsPos(anon, pos) {
					return anon
				}
			}
		}
	}
	return nil
}

func containsPos(fn *ssa.Function, pos token.Pos) bool {
	syntax := fn.Syntax()
	if syntax == nil {
		return false
	}
	return syntax.Pos() <= pos && pos <= syntax.End()
}

// valueAtPos finds the SSA value corresponding to a source position using DebugRef instructions.
func (ea *EquivalenceAnalyzer) valueAtPos(fn *ssa.Function, pos token.Pos) ssa.Value {
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			dbg, ok := instr.(*ssa.DebugRef)
			if !ok {
				continue
			}
			if dbg.Expr != nil && dbg.Expr.Pos() <= pos && pos <= dbg.Expr.End() {
				return dbg.X
			}
		}
	}
	return nil
}

// isDeadDefinition checks if the value has no real referrers (only DebugRef).
func (ea *EquivalenceAnalyzer) isDeadDefinition(v ssa.Value) bool {
	refs := v.Referrers()
	if refs == nil {
		return true
	}
	for _, ref := range *refs {
		if _, isDebug := ref.(*ssa.DebugRef); !isDebug {
			return false
		}
	}
	return true
}

// isUnreachableBlock checks if the value's defining instruction is in an unreachable block.
func (ea *EquivalenceAnalyzer) isUnreachableBlock(v ssa.Value) bool {
	instr, ok := v.(ssa.Instruction)
	if !ok {
		return false
	}
	block := instr.Block()
	if block == nil || block.Index == 0 {
		return false
	}
	return len(block.Preds) == 0
}

// isNonEscaping performs forward data-flow traversal from the value.
// Returns true if the value never reaches an "observable" instruction.
func (ea *EquivalenceAnalyzer) isNonEscaping(v ssa.Value) bool {
	visited := make(map[ssa.Value]bool)
	return ea.isNonEscapingDFS(v, visited)
}

func (ea *EquivalenceAnalyzer) isNonEscapingDFS(v ssa.Value, visited map[ssa.Value]bool) bool {
	if visited[v] {
		return true // Already visited — no observable use found on this path
	}
	visited[v] = true

	refs := v.Referrers()
	if refs == nil {
		return true
	}

	for _, ref := range *refs {
		if _, isDebug := ref.(*ssa.DebugRef); isDebug {
			continue
		}
		if isObservable(ref) {
			return false
		}
		// If the instruction produces a value, follow it
		if val, ok := ref.(ssa.Value); ok {
			if !ea.isNonEscapingDFS(val, visited) {
				return false
			}
		}
	}
	return true
}

// isObservable returns true if the instruction has externally visible side effects.
func isObservable(instr ssa.Instruction) bool {
	switch instr.(type) {
	case *ssa.Return:
		return true
	case *ssa.Call:
		return true
	case *ssa.Go:
		return true
	case *ssa.Defer:
		return true
	case *ssa.Send:
		return true
	case *ssa.Panic:
		return true
	case *ssa.Store:
		return true // Conservative: all stores are observable in v1
	case *ssa.MapUpdate:
		return true
	default:
		return false
	}
}
