package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fchimpan/mutest/mutator"
)

// InstrumentedPackage represents a package with all mutations embedded
// in a single instrumented build.
type InstrumentedPackage struct {
	ImportPath  string
	BinaryPath  string
	Mutations   []mutator.MutationPoint
	TempDir     string
	OverlayPath string
}

// helperSpec describes a generated mutation helper function.
type helperSpec struct {
	ID       int
	Kind     string // "cmp" (cmp.Ordered) or "eq" (comparable)
	Original token.Token
	Mutated  token.Token
}

// replacement describes a byte-range replacement in source code.
type replacement struct {
	start int // byte offset of expression start (bin.X.Pos)
	end   int // byte offset of expression end (bin.Y.End)
	text  string
}

// InstrumentAll instruments all mutation points grouped by package.
// Each package gets one overlay with all mutations embedded, ready for a single build.
func (e *Engine) InstrumentAll(points []mutator.MutationPoint) (map[string]*InstrumentedPackage, error) {
	// Group by import path.
	byPkg := make(map[string][]mutator.MutationPoint)
	for i := range points {
		byPkg[points[i].ImportPath] = append(byPkg[points[i].ImportPath], points[i])
	}

	result := make(map[string]*InstrumentedPackage, len(byPkg))

	for importPath, pkgPoints := range byPkg {
		// Assign MutestIDs (1-based).
		for i := range pkgPoints {
			pkgPoints[i].MutestID = i + 1
		}
		// Update the original points slice with assigned IDs.
		for _, pp := range pkgPoints {
			for i := range points {
				if points[i].File == pp.File && points[i].Line == pp.Line &&
					points[i].Column == pp.Column && points[i].MutatorName == pp.MutatorName {
					points[i].MutestID = pp.MutestID
					break
				}
			}
		}

		pkg, err := e.instrumentPackage(importPath, pkgPoints)
		if err != nil {
			return nil, fmt.Errorf("instrument %s: %w", importPath, err)
		}
		result[importPath] = pkg
	}

	return result, nil
}

// instrumentPackage instruments a single package with all its mutations.
func (e *Engine) instrumentPackage(importPath string, points []mutator.MutationPoint) (*InstrumentedPackage, error) {
	// Group points by file.
	byFile := make(map[string][]mutator.MutationPoint)
	for _, p := range points {
		byFile[p.File] = append(byFile[p.File], p)
	}

	tempDir, err := os.MkdirTemp("", "mutest-inst-*")
	if err != nil {
		return nil, err
	}

	overlayReplace := make(map[string]string)
	var allHelpers []helperSpec

	for filePath, filePoints := range byFile {
		src := e.sourceCache[filePath]
		if src == nil {
			return nil, fmt.Errorf("source not cached for %s", filePath)
		}

		instrumented, helpers, err := instrumentFile(src, filePath, filePoints)
		if err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("instrument %s: %w", filePath, err)
		}

		// Write instrumented source to temp dir.
		outPath := filepath.Join(tempDir, filepath.Base(filePath))
		if err := os.WriteFile(outPath, instrumented, 0644); err != nil {
			os.RemoveAll(tempDir)
			return nil, err
		}
		overlayReplace[filePath] = outPath
		allHelpers = append(allHelpers, helpers...)
	}

	// Write mutest_runtime.go to temp dir and add to overlay.
	// Go's overlay supports adding new files by mapping a virtual path
	// (that doesn't exist on disk) to a real file.
	pkgName := points[0].Package
	pkgDir := filepath.Dir(points[0].File)
	runtimeSrc := generateRuntime(pkgName, allHelpers)
	runtimeTempPath := filepath.Join(tempDir, "mutest_runtime.go")
	if err := os.WriteFile(runtimeTempPath, runtimeSrc, 0644); err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}
	// Map the virtual path in the package directory to our temp file.
	runtimeVirtualPath := filepath.Join(pkgDir, "mutest_runtime.go")
	overlayReplace[runtimeVirtualPath] = runtimeTempPath

	// Write overlay JSON.
	overlayData, err := json.Marshal(Overlay{Replace: overlayReplace})
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}
	overlayPath := filepath.Join(tempDir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlayData, 0644); err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}

	return &InstrumentedPackage{
		ImportPath:  importPath,
		Mutations:   points,
		TempDir:     tempDir,
		OverlayPath: overlayPath,
	}, nil
}

// instrumentFile replaces mutation target expressions with helper function calls.
func instrumentFile(src []byte, filePath string, points []mutator.MutationPoint) ([]byte, []helperSpec, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	// Build a map from NodeID to mutation point for quick lookup.
	pointByNodeID := make(map[int]mutator.MutationPoint)
	for _, p := range points {
		pointByNodeID[p.NodeID] = p
	}

	// Walk AST to find BinaryExpr nodes and build replacements.
	var repls []replacement
	var helpers []helperSpec
	nodeID := 0

	ast.Inspect(file, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}

		if pt, exists := pointByNodeID[nodeID]; exists {
			xStart := fset.Position(bin.X.Pos()).Offset
			yEnd := fset.Position(bin.Y.End()).Offset

			lhs := string(src[fset.Position(bin.X.Pos()).Offset:fset.Position(bin.X.End()).Offset])
			rhs := string(src[fset.Position(bin.Y.Pos()).Offset:fset.Position(bin.Y.End()).Offset])

			// Check if either operand is nil - can't use generics for nil comparisons
			// because function types and other non-comparable types may be involved.
			isNilComparison := isNilIdent(bin.X) || isNilIdent(bin.Y)

			kind := "cmp"
			if pt.Original == token.EQL || pt.Original == token.NEQ {
				kind = "eq"
			}

			if isNilComparison {
				// For nil comparisons, use inline conditional expression.
				// func() bool { if _mutest_active == N { return X mutated_op Y }; return X original_op Y }()
				callExpr := fmt.Sprintf("func() bool { if _mutest_active == %d { return %s %s %s }; return %s %s %s }()",
					pt.MutestID, lhs, pt.Mutated.String(), rhs, lhs, pt.Original.String(), rhs)
				repls = append(repls, replacement{start: xStart, end: yEnd, text: callExpr})
				// No helper needed, but still count it as a mutation with inline kind.
				helpers = append(helpers, helperSpec{ID: pt.MutestID, Kind: "inline", Original: pt.Original, Mutated: pt.Mutated})
			} else {
				funcName := fmt.Sprintf("_mutest_%s_%d", kind, pt.MutestID)
				callExpr := fmt.Sprintf("%s(%s, %s)", funcName, lhs, rhs)
				repls = append(repls, replacement{start: xStart, end: yEnd, text: callExpr})
				helpers = append(helpers, helperSpec{ID: pt.MutestID, Kind: kind, Original: pt.Original, Mutated: pt.Mutated})
			}
		}

		nodeID++
		return true
	})

	// Apply replacements in reverse order to preserve offsets.
	sort.Slice(repls, func(i, j int) bool {
		return repls[i].start > repls[j].start
	})

	result := make([]byte, len(src))
	copy(result, src)
	for _, r := range repls {
		result = append(result[:r.start], append([]byte(r.text), result[r.end:]...)...)
	}

	return result, helpers, nil
}

// isNilIdent returns true if the expression is the identifier "nil".
func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// generateRuntime generates the mutest_runtime.go file content.
func generateRuntime(pkg string, helpers []helperSpec) []byte {
	var b strings.Builder

	b.WriteString("package " + pkg + "\n\n")

	// Determine imports.
	needsCmp := false
	for _, h := range helpers {
		if h.Kind == "cmp" {
			needsCmp = true
			break
		}
	}

	b.WriteString("import (\n")
	if needsCmp {
		b.WriteString("\t\"cmp\"\n")
	}
	b.WriteString("\t\"os\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString(")\n\n")

	// Active mutation variable and init.
	b.WriteString("var _mutest_active int\n\n")
	b.WriteString("func init() {\n")
	b.WriteString("\tif s := os.Getenv(\"MUTEST_ID\"); s != \"\" {\n")
	b.WriteString("\t\t_mutest_active, _ = strconv.Atoi(s)\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")

	// Generate helper functions (skip inline mutations which are expanded in-place).
	for _, h := range helpers {
		if h.Kind == "inline" {
			continue
		}
		b.WriteString("\n")
		if h.Kind == "cmp" {
			fmt.Fprintf(&b, "func _mutest_cmp_%d[T cmp.Ordered](a, b T) bool {\n", h.ID)
		} else {
			fmt.Fprintf(&b, "func _mutest_eq_%d[T comparable](a, b T) bool {\n", h.ID)
		}
		fmt.Fprintf(&b, "\tif _mutest_active == %d {\n", h.ID)
		fmt.Fprintf(&b, "\t\treturn a %s b\n", h.Mutated.String())
		b.WriteString("\t}\n")
		fmt.Fprintf(&b, "\treturn a %s b\n", h.Original.String())
		b.WriteString("}\n")
	}

	return []byte(b.String())
}

// BuildTestBinary compiles the instrumented package into a test binary.
func (e *Engine) BuildTestBinary(ctx context.Context, pkg *InstrumentedPackage) error {
	binPath := filepath.Join(pkg.TempDir, "pkg.test")
	args := []string{"test", "-c", "-overlay=" + pkg.OverlayPath, "-vet=off", "-p=1", "-ldflags=-s -w", "-o", binPath, pkg.ImportPath}
	cmd := exec.CommandContext(ctx, "go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build %s: %w\n%s", pkg.ImportPath, err, out)
	}
	pkg.BinaryPath = binPath
	return nil
}

// CleanupInstrumented removes temp directories for all instrumented packages.
func CleanupInstrumented(pkgs map[string]*InstrumentedPackage) {
	for _, pkg := range pkgs {
		if pkg.TempDir != "" {
			os.RemoveAll(pkg.TempDir)
		}
	}
}
