package engine

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fchimpan/mutest/mutator"
)

// Overlay represents the Go overlay JSON structure used by -overlay flag.
type Overlay struct {
	Replace map[string]string `json:"Replace"`
}

// goPackage represents a subset of `go list -json` output.
type goPackage struct {
	Dir        string   `json:"Dir"`
	ImportPath string   `json:"ImportPath"`
	GoFiles    []string `json:"GoFiles"`
}

// Engine scans packages, discovers mutations, and instruments packages for testing.
type Engine struct {
	mutators    []mutator.Mutator
	patterns    []string          // package patterns (e.g. "./...", "./pkg/calc")
	sourceCache map[string][]byte // file path → source bytes
	importPaths map[string]string // file path → import path
}

// New creates an Engine for the given package patterns with the given mutators.
// patterns are Go package patterns like "./...", "./pkg/...".
func New(patterns []string, mutators ...mutator.Mutator) *Engine {
	return &Engine{
		mutators:    mutators,
		patterns:    patterns,
		sourceCache: make(map[string][]byte),
		importPaths: make(map[string]string),
	}
}

// DiscoverAll resolves the package patterns via `go list`, parses all
// non-test .go files, and returns all mutation points found.
func (e *Engine) DiscoverAll() ([]mutator.MutationPoint, error) {
	files, err := e.resolveFiles()
	if err != nil {
		return nil, err
	}

	var points []mutator.MutationPoint
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			continue // skip unparseable files
		}

		e.sourceCache[path] = src
		pkg := file.Name.Name
		importPath := e.importPaths[path]
		si := buildSkipInfo(fset, file)

		for _, m := range e.mutators {
			pts := m.Discover(fset, file, path, pkg)
			for i := range pts {
				pts[i].ImportPath = importPath
				pts[i].MutatorName = m.Name()
				if !si.shouldSkip(pts[i].Line) {
					points = append(points, pts[i])
				}
			}
		}
	}

	return points, nil
}

// resolveFiles uses `go list -json` to resolve package patterns to
// absolute file paths of non-test .go files.
func (e *Engine) resolveFiles() ([]string, error) {
	args := append([]string{"list", "-json"}, e.patterns...)
	cmd := exec.Command("go", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []string
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var pkg goPackage
		if err := dec.Decode(&pkg); err != nil {
			return nil, err
		}
		for _, f := range pkg.GoFiles {
			absPath := filepath.Join(pkg.Dir, f)
			files = append(files, absPath)
			e.importPaths[absPath] = pkg.ImportPath
		}
	}
	return files, nil
}

// --- //mutest:skip directive support ---

type skipInfo struct {
	lines  map[int]bool // specific lines to skip
	ranges []lineRange  // function ranges to skip
}

type lineRange struct {
	start, end int
}

// buildSkipInfo scans the AST's comments for //mutest:skip directives.
// A directive on a function's doc comment skips the entire function body.
// A directive on a block statement (if/for/switch/select) skips the entire block.
// A directive on any other line skips mutations on that specific line.
func buildSkipInfo(fset *token.FileSet, file *ast.File) *skipInfo {
	si := &skipInfo{
		lines: make(map[int]bool),
	}

	// Function-level: check doc comments for mutest:skip
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		for _, c := range fn.Doc.List {
			if strings.Contains(c.Text, "mutest:skip") {
				start := fset.Position(fn.Pos()).Line
				end := fset.Position(fn.End()).Line
				si.ranges = append(si.ranges, lineRange{start, end})
				break
			}
		}
	}

	// Build line → block range map for block-scope skip.
	blockRanges := buildBlockRanges(fset, file)

	// Line-level and block-level: all comments with mutest:skip
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "mutest:skip") {
				line := fset.Position(c.Pos()).Line
				si.lines[line] = true
				// If this line is the start of a block statement, skip the whole block.
				if r, ok := blockRanges[line]; ok {
					si.ranges = append(si.ranges, r)
				}
			}
		}
	}

	return si
}

// buildBlockRanges walks the AST and returns a map from the starting line
// of each block statement (if/for/switch/select) to its full line range.
func buildBlockRanges(fset *token.FileSet, file *ast.File) map[int]lineRange {
	ranges := make(map[int]lineRange)
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		var start, end token.Pos
		switch n := n.(type) {
		case *ast.IfStmt:
			start, end = n.Pos(), n.End()
		case *ast.ForStmt:
			start, end = n.Pos(), n.End()
		case *ast.RangeStmt:
			start, end = n.Pos(), n.End()
		case *ast.SwitchStmt:
			start, end = n.Pos(), n.End()
		case *ast.TypeSwitchStmt:
			start, end = n.Pos(), n.End()
		case *ast.SelectStmt:
			start, end = n.Pos(), n.End()
		default:
			return true
		}
		line := fset.Position(start).Line
		ranges[line] = lineRange{line, fset.Position(end).Line}
		return true
	})
	return ranges
}

func (si *skipInfo) shouldSkip(line int) bool {
	if si.lines[line] {
		return true
	}
	for _, r := range si.ranges {
		if line >= r.start && line <= r.end {
			return true
		}
	}
	return false
}
