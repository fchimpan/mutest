package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/version"
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
	Dir        string    `json:"Dir"`
	ImportPath string    `json:"ImportPath"`
	GoFiles    []string  `json:"GoFiles"`
	Module     *goModule `json:"Module"`
}

// goModule is the subset of `go list -json`'s Module object needed to
// preflight-check the target module's go directive (see checkGoVersion).
type goModule struct {
	Path      string `json:"Path"`
	GoVersion string `json:"GoVersion"`
}

// minTargetGoVersion is the lowest `go` directive mutest supports in target
// modules: the generated cmp.Ordered helpers require generics (Go 1.18+),
// and 1.20 is mutest's published floor.
const minTargetGoVersion = "go1.20"

// checkGoVersion fails fast if mod's go directive is older than mutest's
// generated helpers require. Without this check, an old go directive
// produces a confusing compiler error deep inside generated code instead
// of a clear diagnostic.
//
// A nil mod, or one with an empty GoVersion (e.g. GOPATH mode, where `go
// list -json` reports no Module at all), is skipped: there is nothing to
// compare.
func checkGoVersion(mod *goModule) error {
	if mod == nil || mod.GoVersion == "" {
		return nil
	}
	found := "go" + mod.GoVersion
	if version.Compare(found, minTargetGoVersion) < 0 {
		return fmt.Errorf("mutest requires the target module's go directive to be >= 1.20 (found go %s in module %s)", mod.GoVersion, mod.Path)
	}
	return nil
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
		si := buildSkipInfo(fset, file, src)

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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("go list: %s", bytes.TrimSpace(exitErr.Stderr))
		}
		return nil, err
	}

	var files []string
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var pkg goPackage
		if err := dec.Decode(&pkg); err != nil {
			return nil, err
		}
		if err := checkGoVersion(pkg.Module); err != nil {
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

// buildSkipInfo scans the AST's comments for //mutest:skip directives, and
// also unconditionally excludes every `const` declaration (see
// buildConstRanges). A directive on a function's doc comment skips the
// entire function body. A directive on a block statement (if/for/switch/select)
// skips the entire block. A directive at the end of a line of code skips
// mutations on that specific line. A directive alone on its own line (a
// natural but previously silent misuse) skips the line that follows instead
// — and, if that following line begins a block statement, the entire block —
// since a standalone comment has no code of its own to attach to. src is the
// same source bytes the file was parsed from; it is needed to tell a
// standalone comment apart from one at the end of a line of code.
func buildSkipInfo(fset *token.FileSet, file *ast.File, src []byte) *skipInfo {
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
			if !strings.Contains(c.Text, "mutest:skip") {
				continue
			}
			line := fset.Position(c.Pos()).Line
			si.lines[line] = true

			target := line
			if isStandaloneComment(fset, c.Pos(), src) {
				// Nothing precedes the comment on its own line, so the
				// directive cannot mean "skip this line" (there is no code
				// on it); apply it to the line that follows instead.
				target = line + 1
				si.lines[target] = true
			}
			// If the target line is the start of a block statement, skip the whole block.
			if r, ok := blockRanges[target]; ok {
				si.ranges = append(si.ranges, r)
			}
		}
	}

	// const declarations: always excluded, regardless of any directive (see
	// buildConstRanges for why this is safe).
	si.ranges = append(si.ranges, buildConstRanges(fset, file)...)

	return si
}

// isStandaloneComment reports whether the comment at pos sits alone on its
// source line — i.e. everything before it on that line is whitespace — as
// opposed to being appended to the end of a line of code. src must be the
// same source bytes pos was parsed from (buildSkipInfo's contract), so the
// line-start and comment offsets derived from pos's *token.File are
// guaranteed valid byte indices into src: no additional bounds checking is
// needed beyond the nil guard for a pos outside fset.
func isStandaloneComment(fset *token.FileSet, pos token.Pos, src []byte) bool {
	tf := fset.File(pos)
	if tf == nil {
		return false
	}
	lineStart := tf.Offset(tf.LineStart(tf.Line(pos)))
	commentStart := tf.Offset(pos)
	return len(bytes.TrimSpace(src[lineStart:commentStart])) == 0
}

// buildConstRanges returns the line range of every `const` declaration
// (*ast.GenDecl with Tok == token.CONST) in file, including package-level
// declarations, block-form `const ( ... )` groups, and const declarations
// local to a function body (ast.Inspect reaches the *ast.GenDecl nested
// inside a function's *ast.DeclStmt just as it does top-level decls).
//
// This is always safe: a const expression can never contain a runtime
// comparison, so nothing that legitimately needs mutating is excluded —
// this can only suppress mutation points, never miss a real one. Without
// it, instrumenting a comparison inside a const expression (e.g.
// `const IsBig = MaxSize > 10`) replaces it with a helper function call,
// which is not a constant expression and fails to build the whole package.
//
// Known limitation: if a const declaration shares a line with a runtime
// comparison (e.g. `if x > 0 { const c = 1 < 2 }`), the runtime comparison
// on that shared line is also suppressed. This is an accepted, rare loss of
// a single mutation point.
func buildConstRanges(fset *token.FileSet, file *ast.File) []lineRange {
	var ranges []lineRange
	ast.Inspect(file, func(n ast.Node) bool {
		decl, ok := n.(*ast.GenDecl)
		if !ok || decl.Tok != token.CONST {
			return true
		}
		start := fset.Position(decl.Pos()).Line
		end := fset.Position(decl.End()).Line
		ranges = append(ranges, lineRange{start, end})
		return true
	})
	return ranges
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
