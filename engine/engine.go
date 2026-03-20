package engine

import (
	"bytes"
	"encoding/json"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"

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

		for _, m := range e.mutators {
			pts := m.Discover(fset, file, path, pkg)
			for i := range pts {
				pts[i].ImportPath = importPath
			}
			points = append(points, pts...)
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
func (e *Engine) Prepare(m mutator.Mutator, point mutator.MutationPoint) (_ *Mutant, retErr error) {
	// Use cached source bytes when available to avoid repeated disk reads.
	src := e.sourceCache[point.File]
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, point.File, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	m.Apply(file, point)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "mutest-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(tempDir)
		}
	}()

	mutatedPath := filepath.Join(tempDir, "mutated.go")
	if err := os.WriteFile(mutatedPath, buf.Bytes(), 0644); err != nil {
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
