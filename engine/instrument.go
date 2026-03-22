package engine

import (
	"bytes"
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
	"sync"

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
	Kind     string // "cmp" (cmp.Ordered), "eq" (comparable), or "inline" (nil comparisons)
	Original token.Token
	Mutated  token.Token
}

// replacement describes a byte-range replacement in source code.
type replacement struct {
	start int
	end   int
	text  string
}

// InstrumentAll instruments all mutation points grouped by package.
// It assigns MutestIDs directly into the points slice and returns
// instrumented packages ready for a single build each.
func (e *Engine) InstrumentAll(points []mutator.MutationPoint) (map[string]*InstrumentedPackage, error) {
	// Group by import path, keeping original indices for direct ID assignment.
	byPkg := make(map[string][]int) // importPath → indices into points
	for i := range points {
		byPkg[points[i].ImportPath] = append(byPkg[points[i].ImportPath], i)
	}

	result := make(map[string]*InstrumentedPackage, len(byPkg))

	for importPath, indices := range byPkg {
		// Assign MutestIDs (1-based) directly into the original slice.
		pkgPoints := make([]mutator.MutationPoint, len(indices))
		for rank, idx := range indices {
			points[idx].MutestID = rank + 1
			pkgPoints[rank] = points[idx]
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
func (e *Engine) instrumentPackage(importPath string, points []mutator.MutationPoint) (_ *InstrumentedPackage, retErr error) {
	// Group points by file.
	byFile := make(map[string][]mutator.MutationPoint)
	for _, p := range points {
		byFile[p.File] = append(byFile[p.File], p)
	}

	tempDir, err := os.MkdirTemp("", "mutest-inst-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(tempDir)
		}
	}()

	overlayReplace := make(map[string]string)
	var allHelpers []helperSpec
	fileIdx := 0

	for filePath, filePoints := range byFile {
		src := e.sourceCache[filePath]
		if src == nil {
			return nil, fmt.Errorf("source not cached for %s", filePath)
		}

		instrumented, helpers, err := instrumentFile(src, filePath, filePoints)
		if err != nil {
			return nil, fmt.Errorf("instrument %s: %w", filePath, err)
		}

		// Use sequential filenames to avoid filepath.Base collisions.
		outPath := filepath.Join(tempDir, fmt.Sprintf("file%d.go", fileIdx))
		fileIdx++
		if err := os.WriteFile(outPath, instrumented, 0644); err != nil {
			return nil, err
		}
		overlayReplace[filePath] = outPath
		allHelpers = append(allHelpers, helpers...)
	}

	// Write mutest_runtime.go to temp dir and add to overlay.
	pkgName := points[0].Package
	pkgDir := filepath.Dir(points[0].File)
	runtimeSrc := generateRuntime(pkgName, allHelpers)
	runtimeTempPath := filepath.Join(tempDir, "mutest_runtime.go")
	if err := os.WriteFile(runtimeTempPath, runtimeSrc, 0644); err != nil {
		return nil, err
	}
	runtimeVirtualPath := filepath.Join(pkgDir, "mutest_runtime.go")
	overlayReplace[runtimeVirtualPath] = runtimeTempPath

	// Write overlay JSON.
	overlayData, err := json.Marshal(Overlay{Replace: overlayReplace})
	if err != nil {
		return nil, err
	}
	overlayPath := filepath.Join(tempDir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlayData, 0644); err != nil {
		return nil, err
	}

	return &InstrumentedPackage{
		ImportPath:  importPath,
		Mutations:   points,
		TempDir:     tempDir,
		OverlayPath: overlayPath,
	}, nil
}

// nodeKey uniquely identifies a mutation point in the AST walk.
// NodeID alone is insufficient because different mutators maintain
// independent counters during Discover.
type nodeKey struct {
	nodeID int
	op     token.Token // Original operator disambiguates between mutators
}

// instrumentFile replaces mutation target expressions with helper function calls.
func instrumentFile(src []byte, filePath string, points []mutator.MutationPoint) ([]byte, []helperSpec, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	// Build a map keyed on (NodeID, Original) to handle multiple mutators
	// that assign the same NodeID independently.
	pointByKey := make(map[nodeKey]mutator.MutationPoint)
	for _, p := range points {
		pointByKey[nodeKey{p.NodeID, p.Original}] = p
	}

	var repls []replacement
	var helpers []helperSpec
	nodeID := 0

	ast.Inspect(file, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}

		key := nodeKey{nodeID, bin.Op}
		if pt, exists := pointByKey[key]; exists {
			xStart := fset.Position(bin.X.Pos()).Offset
			yEnd := fset.Position(bin.Y.End()).Offset
			lhs := string(src[fset.Position(bin.X.Pos()).Offset:fset.Position(bin.X.End()).Offset])
			rhs := string(src[fset.Position(bin.Y.Pos()).Offset:fset.Position(bin.Y.End()).Offset])

			isNilComparison := isNilIdent(bin.X) || isNilIdent(bin.Y)

			kind := "cmp"
			if pt.Original == token.EQL || pt.Original == token.NEQ {
				kind = "eq"
			}

			if isNilComparison {
				callExpr := fmt.Sprintf("func() bool { _mutest_init(); if _mutest_active == %d { return %s %s %s }; return %s %s %s }()",
					pt.MutestID, lhs, pt.Mutated.String(), rhs, lhs, pt.Original.String(), rhs)
				repls = append(repls, replacement{start: xStart, end: yEnd, text: callExpr})
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

	// Build result in a single forward pass to avoid O(n²) append aliasing.
	// Iterate repls in forward order (ascending start offset).
	var buf bytes.Buffer
	buf.Grow(len(src) * 2)
	pos := 0
	for i := len(repls) - 1; i >= 0; i-- {
		r := repls[i]
		buf.Write(src[pos:r.start])
		buf.WriteString(r.text)
		pos = r.end
	}
	buf.Write(src[pos:])

	return buf.Bytes(), helpers, nil
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
	b.WriteString("\t\"sync\"\n")
	b.WriteString(")\n\n")

	// Use sync.Once to ensure _mutest_active is initialized before first use,
	// regardless of init() execution order across files.
	b.WriteString("var (\n")
	b.WriteString("\t_mutest_active int\n")
	b.WriteString("\t_mutest_once   sync.Once\n")
	b.WriteString(")\n\n")
	b.WriteString("func _mutest_init() {\n")
	b.WriteString("\t_mutest_once.Do(func() {\n")
	b.WriteString("\t\tif s := os.Getenv(\"MUTEST_ID\"); s != \"\" {\n")
	b.WriteString("\t\t\t_mutest_active, _ = strconv.Atoi(s)\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t})\n")
	b.WriteString("}\n")

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
		b.WriteString("\t_mutest_init()\n")
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

// BuildTestBinaries builds test binaries for all packages in parallel.
func (e *Engine) BuildTestBinaries(ctx context.Context, pkgs map[string]*InstrumentedPackage) error {
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	for _, pkg := range pkgs {
		wg.Add(1)
		go func(p *InstrumentedPackage) {
			defer wg.Done()
			if err := e.BuildTestBinary(ctx, p); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(pkg)
	}
	wg.Wait()
	return firstErr
}

// CleanupInstrumented removes temp directories for all instrumented packages.
func CleanupInstrumented(pkgs map[string]*InstrumentedPackage) {
	for _, pkg := range pkgs {
		if pkg.TempDir != "" {
			os.RemoveAll(pkg.TempDir)
		}
	}
}
