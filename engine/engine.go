package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// Mutant is a fully prepared mutation ready for testing.
type Mutant struct {
	Point       mutator.MutationPoint
	OverlayPath string // path to the overlay.json temp file
	TempDir     string // directory containing overlay.json and mutated file
}

// goPackage represents a subset of `go list -json` output.
type goPackage struct {
	Dir        string   `json:"Dir"`
	ImportPath string   `json:"ImportPath"`
	GoFiles    []string `json:"GoFiles"`
}

// Engine scans packages, discovers mutations, and prepares overlays.
type Engine struct {
	mutators    []mutator.Mutator
	patterns    []string           // package patterns (e.g. "./...", "./pkg/calc")
	sourceCache map[string][]byte  // file path → source bytes
	importPaths map[string]string  // file path → import path
	mutatorMap  map[string]mutator.Mutator // name → mutator (for fast lookup)
	baseDir     string             // shared temp directory for all mutants
}

// New creates an Engine for the given package patterns with the given mutators.
// patterns are Go package patterns like "./...", "./pkg/...".
func New(patterns []string, mutators ...mutator.Mutator) *Engine {
	mm := make(map[string]mutator.Mutator, len(mutators))
	for _, m := range mutators {
		mm[m.Name()] = m
	}
	return &Engine{
		mutators:    mutators,
		patterns:    patterns,
		sourceCache: make(map[string][]byte),
		importPaths: make(map[string]string),
		mutatorMap:  mm,
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

// Prepare re-parses the source file for the given mutation point,
// applies the mutation, writes a temp file, and generates an overlay.json.
// The mutator is looked up by point.MutatorName from the engine's registered mutators.
// InitTempDir creates a shared temporary directory for all mutants.
// Must be called before Prepare. Call CleanupAll when done.
func (e *Engine) InitTempDir() error {
	dir, err := os.MkdirTemp("", "mutest-*")
	if err != nil {
		return err
	}
	e.baseDir = dir
	return nil
}

// CleanupAll removes the shared temporary directory.
func (e *Engine) CleanupAll() {
	if e.baseDir != "" {
		os.RemoveAll(e.baseDir)
	}
}

// tokenString returns the source representation of a token.
func tokenString(tok token.Token) string {
	switch tok {
	case token.GTR:
		return ">"
	case token.GEQ:
		return ">="
	case token.LSS:
		return "<"
	case token.LEQ:
		return "<="
	case token.EQL:
		return "=="
	case token.NEQ:
		return "!="
	default:
		return tok.String()
	}
}

// lineColToOffset converts 1-based line and column to a byte offset in src.
func lineColToOffset(src []byte, line, col int) int {
	currentLine := 1
	for i, b := range src {
		if currentLine == line {
			return i + col - 1
		}
		if b == '\n' {
			currentLine++
		}
	}
	return -1
}

func (e *Engine) Prepare(point mutator.MutationPoint) (_ *Mutant, retErr error) {
	if e.mutatorMap[point.MutatorName] == nil {
		return nil, fmt.Errorf("unknown mutator: %q", point.MutatorName)
	}

	src := e.sourceCache[point.File]
	if src == nil {
		return nil, fmt.Errorf("source not cached for %s", point.File)
	}

	// Direct byte rewrite: replace the original operator with the mutated one.
	origStr := tokenString(point.Original)
	mutStr := tokenString(point.Mutated)
	offset := lineColToOffset(src, point.Line, point.Column)
	if offset < 0 || offset+len(origStr) > len(src) {
		return nil, fmt.Errorf("invalid offset for %s:%d:%d", point.File, point.Line, point.Column)
	}

	// Verify the original token is at the expected position.
	if string(src[offset:offset+len(origStr)]) != origStr {
		return nil, fmt.Errorf("expected %q at offset %d, got %q", origStr, offset, string(src[offset:offset+len(origStr)]))
	}

	// Build mutated source by splicing in the new operator.
	mutated := make([]byte, 0, len(src)-len(origStr)+len(mutStr))
	mutated = append(mutated, src[:offset]...)
	mutated = append(mutated, mutStr...)
	mutated = append(mutated, src[offset+len(origStr):]...)

	var tempDir string
	if e.baseDir != "" {
		// Use sub-directory under the shared base dir to avoid MkdirTemp overhead.
		subDir := fmt.Sprintf("m%d_%d_%s", point.Line, point.Column, point.MutatorName)
		tempDir = filepath.Join(e.baseDir, subDir)
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return nil, err
		}
	} else {
		var err error
		tempDir, err = os.MkdirTemp("", "mutest-*")
		if err != nil {
			return nil, err
		}
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(tempDir)
		}
	}()

	mutatedPath := filepath.Join(tempDir, "mutated.go")
	if err := os.WriteFile(mutatedPath, mutated, 0644); err != nil {
		return nil, err
	}

	overlay := Overlay{
		Replace: map[string]string{
			point.File: mutatedPath,
		},
	}
	overlayData, err := json.Marshal(overlay)
	if err != nil {
		return nil, err
	}

	overlayPath := filepath.Join(tempDir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlayData, 0644); err != nil {
		return nil, err
	}

	return &Mutant{
		Point:       point,
		OverlayPath: overlayPath,
		TempDir:     tempDir,
	}, nil
}

// Cleanup removes the temp directory for a mutant.
func (e *Engine) Cleanup(m *Mutant) {
	os.RemoveAll(m.TempDir)
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

	// Line-level: all comments with mutest:skip
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "mutest:skip") {
				si.lines[fset.Position(c.Pos()).Line] = true
			}
		}
	}

	return si
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
